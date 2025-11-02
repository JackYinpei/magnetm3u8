package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"worker/client"
	"worker/config"
	"worker/database"
	"worker/downloader"
	"worker/models"
	"worker/transcoder"
	"worker/webrtc"
	
	webrtcLib "github.com/pion/webrtc/v3"
)

var (
	gatewayURL = flag.String("gateway", "ws://localhost:8080/ws/nodes", "Gateway WebSocket URL")
	nodeID     = flag.String("id", "", "Worker node ID (auto-generated if empty)")
	nodeName   = flag.String("name", "", "Worker node name")
	configFile = flag.String("config", "config/worker.json", "Configuration file path")
)

// WorkerNode 工作节点主结构
type WorkerNode struct {
	config     *config.Config
	client     *client.GatewayClient
	downloader *downloader.Manager
	transcoder *transcoder.Manager
	webrtc     *webrtc.Manager

	iceConfigMu     sync.RWMutex
	iceTurnServers  []webrtcLib.ICEServer
	iceConfigExpiry time.Time
}

func main() {
	flag.Parse()

	// 加载配置
	cfg, err := config.Load(*configFile)
	if err != nil {
		log.Printf("Failed to load config, using defaults: %v", err)
		cfg = config.Default()
	}

	// 命令行参数覆盖配置
	if *gatewayURL != "ws://localhost:8080/ws/nodes" {
		cfg.Gateway.URL = *gatewayURL
	}
	if *nodeID != "" {
		cfg.Node.ID = *nodeID
	}
	if *nodeName != "" {
		cfg.Node.Name = *nodeName
	}

	// 创建工作节点
	worker, err := NewWorkerNode(cfg)
	if err != nil {
		log.Fatalf("Failed to create worker node: %v", err)
	}

	log.Printf("Worker Node starting: ID=%s, Name=%s", cfg.Node.ID, cfg.Node.Name)
	log.Printf("Gateway URL: %s", cfg.Gateway.URL)

	// 启动工作节点
	if err := worker.Start(); err != nil {
		log.Fatalf("Failed to start worker node: %v", err)
	}

	// 等待中断信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down worker node...")
	worker.Stop()
}

// NewWorkerNode 创建新的工作节点
func NewWorkerNode(cfg *config.Config) (*WorkerNode, error) {
	// 确保存储路径存在
	if err := cfg.GetStoragePaths(); err != nil {
		return nil, fmt.Errorf("failed to create storage paths: %v", err)
	}

	// 初始化数据库
	if err := database.Initialize("data/config"); err != nil {
		return nil, fmt.Errorf("failed to initialize database: %v", err)
	}

	// 创建各个组件
	gatewayClient := client.New(cfg.Gateway.URL, cfg.Node.ID)
	downloaderMgr := downloader.New(cfg.Storage.DownloadPath, cfg.Node.ID)
	transcoderMgr := transcoder.New(cfg.Storage.DownloadPath, cfg.Storage.M3U8Path)
	webrtcMgr := webrtc.New()

	worker := &WorkerNode{
		config:     cfg,
		client:     gatewayClient,
		downloader: downloaderMgr,
		transcoder: transcoderMgr,
		webrtc:     webrtcMgr,
	}

	// 设置消息处理器
	gatewayClient.SetMessageHandler(worker.handleGatewayMessage)
	
	// 设置下载状态处理器，用于自动转码
	downloaderMgr.SetExternalStatusHandler(worker.handleDownloadStatusChange)
	
	// 设置WebRTC ICE候选者处理器
	webrtcMgr.SetICECandidateHandler(worker.handleWebRTCICECandidate)

	return worker, nil
}

// Start 启动工作节点
func (w *WorkerNode) Start() error {
	// 启动各个组件
	if err := w.downloader.Start(); err != nil {
		return err
	}

	if err := w.transcoder.Start(); err != nil {
		return err
	}

	if err := w.webrtc.Start(); err != nil {
		return err
	}

	// 连接到网关
	nodeInfo := client.NodeInfo{
		ID:           w.config.Node.ID,
		Name:         w.config.Node.Name,
		Address:      w.config.Node.Address,
		Status:       "online",
		Capabilities: []string{"torrent", "transcode", "webrtc"},
		Resources: map[string]int{
			"max_downloads": w.config.Limits.MaxDownloads,
			"max_transcodes": w.config.Limits.MaxTranscodes,
			"disk_space_gb": w.config.Limits.DiskSpaceGB,
		},
		Metadata: map[string]string{
			"version": "1.0.0",
			"arch":    "amd64",
		},
	}

	if err := w.client.Connect(nodeInfo); err != nil {
		return err
	}

	// 启动心跳
	go w.startHeartbeat()

	log.Printf("Worker node started successfully")
	return nil
}

// Stop 停止工作节点
func (w *WorkerNode) Stop() {
	w.client.Disconnect()
	w.webrtc.Stop()
	w.transcoder.Stop()
	w.downloader.Stop()
	
	// 关闭数据库连接
	if err := database.Close(); err != nil {
		log.Printf("Failed to close database: %v", err)
	}
}

// startHeartbeat 启动心跳
func (w *WorkerNode) startHeartbeat() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		w.client.SendHeartbeat()
	}
}

// handleGatewayMessage 处理来自网关的消息
func (w *WorkerNode) handleGatewayMessage(msgType string, payload map[string]interface{}) {
	switch msgType {
	case "registration_confirmed":
		log.Printf("Registration confirmed by gateway")
	case "task_submit":
		w.handleTaskSubmit(payload)
	case "get_tasks":
		w.handleGetTasks(payload)
	case "get_task_detail":
		w.handleGetTaskDetail(payload)
	case "webrtc_offer":
		w.handleWebRTCOffer(payload)
	case "ice_candidate":
		w.handleICECandidate(payload)
	default:
		log.Printf("Unknown message type: %s", msgType)
	}
}

// handleTaskSubmit 处理任务提交
func (w *WorkerNode) handleTaskSubmit(payload map[string]interface{}) {
	magnetURL, ok := payload["magnet_url"].(string)
	if !ok {
		log.Printf("Invalid magnet URL in task submit")
		return
	}

	log.Printf("Received task: %s", magnetURL)

	// 开始下载
	taskID, err := w.downloader.StartDownload(magnetURL)
	if err != nil {
		log.Printf("Failed to start download: %v", err)
		return
	}

	// 发送任务状态更新
	w.client.SendTaskStatus(taskID, "downloading", 0, nil)
}

// handleGetTasks 处理获取任务列表请求
func (w *WorkerNode) handleGetTasks(payload map[string]interface{}) {
	tasks := w.downloader.GetAllTasks()
	
	// 转换为适合传输的格式
	taskList := make([]map[string]interface{}, 0, len(tasks))
	for _, task := range tasks {
		// 获取文件列表
		files, _ := task.GetTorrentFiles()
		fileNames := make([]string, len(files))
		for i, file := range files {
			fileNames[i] = file.FileName
		}

		// 获取字幕文件列表
		srts, _ := task.GetSrts()

		taskData := map[string]interface{}{
			"id":           task.TaskID,
			"magnet_url":   task.MagnetURL,
			"status":       task.Status,
			"progress":     task.Progress,
			"speed":        task.Speed,
			"size":         task.Size,
			"downloaded":   task.Downloaded,
			"files":        fileNames,
			"torrent_name": task.TorrentName,
			"m3u8_path":    task.M3U8FilePath,
			"srts":         srts,
			"created_at":   task.CreatedAt,
			"updated_at":   task.UpdatedAt,
			"worker_id":    w.config.Node.ID,
		}
		taskList = append(taskList, taskData)
	}
	
	// 构建响应，包含request_id（如果提供）
	response := map[string]interface{}{
		"tasks": taskList,
	}
	
	// 如果请求中包含request_id，则在响应中包含它
	if requestID, ok := payload["request_id"]; ok {
		response["request_id"] = requestID
	}
	
	// 发送任务列表响应
	w.client.SendMessage("tasks_response", response)
}

// handleGetTaskDetail 处理获取任务详情请求
func (w *WorkerNode) handleGetTaskDetail(payload map[string]interface{}) {
	taskID, ok := payload["task_id"].(string)
	if !ok {
		log.Printf("Invalid task ID in get task detail request")
		return
	}
	
	task, exists := w.downloader.GetTask(taskID)
	if !exists {
		// 发送任务不存在响应
		w.client.SendMessage("task_detail_response", map[string]interface{}{
			"task_id": taskID,
			"found":   false,
		})
		return
	}
	
	// 获取文件列表
	files, _ := task.GetTorrentFiles()
	fileDetails := make([]map[string]interface{}, len(files))
	for i, file := range files {
		fileDetails[i] = map[string]interface{}{
			"file_name":   file.FileName,
			"file_size":   file.FileSize,
			"file_path":   file.FilePath,
			"is_selected": file.IsSelected,
		}
	}

	// 获取字幕文件列表
	srts, _ := task.GetSrts()

	// 获取元数据
	metadata, _ := task.GetMetadata()
	
	// 发送任务详情响应
	taskData := map[string]interface{}{
		"id":           task.TaskID,
		"magnet_url":   task.MagnetURL,
		"status":       task.Status,
		"progress":     task.Progress,
		"speed":        task.Speed,
		"size":         task.Size,
		"downloaded":   task.Downloaded,
		"files":        fileDetails,
		"torrent_name": task.TorrentName,
		"m3u8_path":    task.M3U8FilePath,
		"srts":         srts,
		"created_at":   task.CreatedAt,
		"updated_at":   task.UpdatedAt,
		"worker_id":    w.config.Node.ID,
		"metadata":     metadata,
	}
	
	w.client.SendMessage("task_detail_response", map[string]interface{}{
		"task_id": taskID,
		"found":   true,
		"task":    taskData,
	})
}

// handleWebRTCOffer 处理WebRTC Offer
func (w *WorkerNode) handleWebRTCOffer(payload map[string]interface{}) {
	sessionID, _ := payload["session_id"].(string)
	clientID, _ := payload["client_id"].(string)
	sdp, _ := payload["sdp"].(string)

	log.Printf("Received WebRTC offer for session %s from client %s", sessionID, clientID)

	// 更新WebRTC配置，确保包含最新的TURN/STUN信息
	config := w.ensureWebRTCConfiguration()
	w.webrtc.UpdateConfiguration(config)

	// 处理Offer并生成Answer
	answer, err := w.webrtc.HandleOffer(sessionID, sdp)
	if err != nil {
		log.Printf("Failed to handle WebRTC offer: %v", err)
		return
	}

	// 发送Answer到网关
	w.client.SendWebRTCAnswer(sessionID, answer)
}

// handleICECandidate 处理ICE候选者
func (w *WorkerNode) handleICECandidate(payload map[string]interface{}) {
	sessionID, _ := payload["session_id"].(string)
	candidate, _ := payload["candidate"].(string)

	log.Printf("Received ICE candidate for session %s", sessionID)

	if err := w.webrtc.AddICECandidate(sessionID, candidate); err != nil {
		log.Printf("Failed to add ICE candidate: %v", err)
	}
}

// handleDownloadStatusChange 处理下载状态变化，自动启动转码
func (w *WorkerNode) handleDownloadStatusChange(task *models.Task) {
	// 当任务状态为 completed 时，自动启动转码
	if task.Status == "completed" {
		log.Printf("Download completed for task %s, starting transcoding", task.TaskID)
		
		// 获取种子文件列表，寻找视频文件
		files, err := task.GetTorrentFiles()
		if err != nil {
			log.Printf("Failed to get torrent files for task %s: %v", task.TaskID, err)
			return
		}
		
		// 查找第一个视频文件进行转码
		var videoFile string
		videoExtensions := []string{".mp4", ".mkv", ".avi", ".mov", ".wmv", ".flv", ".webm", ".m4v"}
		
		for _, file := range files {
			for _, ext := range videoExtensions {
				if strings.HasSuffix(strings.ToLower(file.FileName), ext) {
					// 构建完整的文件路径
					videoFile = filepath.Join(w.config.Storage.DownloadPath, file.FilePath)
					break
				}
			}
			if videoFile != "" {
				break
			}
		}
		
		if videoFile != "" {
			// 启动转码
			go w.startTranscodingForTask(task, videoFile)
		} else {
			log.Printf("No video file found in task %s", task.TaskID)
			// 将任务状态设置为ready（没有需要转码的内容）
			w.updateTaskStatusInDB(task.TaskID, "ready")
		}
	}
}

// startTranscodingForTask 为指定任务启动转码
func (w *WorkerNode) startTranscodingForTask(task *models.Task, videoFile string) {
	// 更新任务状态为转码中
	w.updateTaskStatusInDB(task.TaskID, "transcoding")
	
	// 启动转码
	transcodeID, err := w.transcoder.StartTranscode(videoFile)
	if err != nil {
		log.Printf("Failed to start transcoding for task %s: %v", task.TaskID, err)
		w.updateTaskStatusInDB(task.TaskID, "error")
		return
	}
	
	log.Printf("Started transcoding for task %s with transcode ID %s", task.TaskID, transcodeID)
	
	// 监控转码进度
	go w.monitorTranscodingProgress(task.TaskID, transcodeID)
}

// monitorTranscodingProgress 监控转码进度
func (w *WorkerNode) monitorTranscodingProgress(taskID, transcodeID string) {
	statusChan := w.transcoder.GetStatusChannel()
	
	// 监控转码状态变化
	for transcodeTask := range statusChan {
		if transcodeTask.ID == transcodeID {
			log.Printf("Transcode progress for task %s: status=%s, progress=%d%%", 
				taskID, transcodeTask.Status, transcodeTask.Progress)
			
			if transcodeTask.Status == transcoder.TranscodeStatusCompleted {
				// 转码完成，保存结果到数据库
				err := w.saveTranscodingResults(taskID, transcodeTask)
				if err != nil {
					log.Printf("Failed to save transcoding results for task %s: %v", taskID, err)
					w.updateTaskStatusInDB(taskID, "error")
				} else {
					log.Printf("Transcoding completed and saved for task %s", taskID)
					w.updateTaskStatusInDB(taskID, "ready")
				}
				return
			} else if transcodeTask.Status == transcoder.TranscodeStatusError {
				log.Printf("Transcoding failed for task %s: %s", taskID, 
					transcodeTask.Metadata["error"])
				w.updateTaskStatusInDB(taskID, "error")
				return
			}
		}
	}
}

// saveTranscodingResults 保存转码结果到数据库
func (w *WorkerNode) saveTranscodingResults(taskID string, transcodeTask *transcoder.TranscodeTask) error {
	// 获取任务仓库
	taskRepo := database.NewTaskRepository()
	
	// 获取任务
	task, err := taskRepo.GetByTaskID(taskID)
	if err != nil {
		return fmt.Errorf("failed to get task: %v", err)
	}
	
	// 更新任务信息
	task.M3U8FilePath = transcodeTask.M3U8Path
	
	// 保存字幕文件列表
	if len(transcodeTask.Subtitles) > 0 {
		err = task.SetSrts(transcodeTask.Subtitles)
		if err != nil {
			log.Printf("Failed to set subtitle files: %v", err)
		}
	}
	
	// 读取并保存分片文件列表
	segments, err := w.readSegmentsFromM3U8(transcodeTask.M3U8Path)
	if err != nil {
		log.Printf("Failed to read segments from M3U8: %v", err)
	} else {
		err = task.SetSegments(segments)
		if err != nil {
			log.Printf("Failed to set segments: %v", err)
		}
	}
	
	// 保存转码输出路径到元数据中
	metadata, _ := task.GetMetadata()
	metadata["output_path"] = transcodeTask.OutputPath
	metadata["segment_count"] = len(segments)
	task.SetMetadata(metadata)
	
	// 更新数据库
	return taskRepo.Update(task)
}

// readSegmentsFromM3U8 从M3U8文件中读取分片列表
func (w *WorkerNode) readSegmentsFromM3U8(m3u8Path string) ([]string, error) {
	content, err := os.ReadFile(m3u8Path)
	if err != nil {
		return nil, fmt.Errorf("failed to read M3U8 file: %v", err)
	}
	
	var segments []string
	lines := strings.Split(string(content), "\n")
	
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// M3U8文件中的分片文件以.ts结尾且不以#开头
		if line != "" && !strings.HasPrefix(line, "#") && strings.HasSuffix(line, ".ts") {
			// 构建完整路径
			segmentPath := filepath.Join(filepath.Dir(m3u8Path), line)
			segments = append(segments, segmentPath)
		}
	}
	
	return segments, nil
}

// handleWebRTCICECandidate 处理来自WebRTC的ICE候选者
func (w *WorkerNode) handleWebRTCICECandidate(sessionID string, candidate *webrtcLib.ICECandidate) {
	log.Printf("Sending ICE candidate for session %s: %s", sessionID, candidate.String())
	
	// 序列化ICE候选者
	candidateJSON := candidate.ToJSON()
	candidateStr := candidateJSON.Candidate
	
	// 发送ICE候选者到Gateway
	w.client.SendICECandidate(sessionID, candidateStr)
}

// updateTaskStatusInDB 更新数据库中的任务状态
func (w *WorkerNode) updateTaskStatusInDB(taskID string, status string) {
	taskRepo := database.NewTaskRepository()
	err := taskRepo.UpdateStatus(taskID, status)
	if err != nil {
		log.Printf("Failed to update task status in database: %v", err)
	}
}
