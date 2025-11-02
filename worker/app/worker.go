package app

import (
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
	"worker/domain"
	"worker/downloader"
	"worker/models"
	"worker/transcoder"
	"worker/webrtc"

	webrtcLib "github.com/pion/webrtc/v3"
)

// TaskRepositoryFactory exposes the ability to obtain a task repository instance.
type TaskRepositoryFactory func() database.TaskRepository

// Dependencies groups the pluggable subsystems required by the worker.
type Dependencies struct {
	Gateway           client.Gateway
	Downloader        downloader.Service
	Transcoder        transcoder.Service
	WebRTC            webrtc.Service
	TaskRepoFactory   TaskRepositoryFactory
	HeartbeatInterval time.Duration
	Clock             func() time.Time
}

// Worker orchestrates the worker node lifecycle via injected dependencies.
type Worker struct {
	config          *config.Config
	gateway         client.Gateway
	downloader      downloader.Service
	transcoder      transcoder.Service
	webrtc          webrtc.Service
	taskRepoFactory TaskRepositoryFactory
	heartbeatEvery  time.Duration
	now             func() time.Time

	iceConfigMu     sync.RWMutex
	iceTurnServers  []webrtcLib.ICEServer
	iceConfigExpiry time.Time
}

// New constructs a Worker with the supplied configuration and dependencies.
func New(cfg *config.Config, deps Dependencies) (*Worker, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config must not be nil")
	}
	if deps.Gateway == nil {
		return nil, fmt.Errorf("gateway dependency is required")
	}
	if deps.Downloader == nil {
		return nil, fmt.Errorf("downloader dependency is required")
	}
	if deps.Transcoder == nil {
		return nil, fmt.Errorf("transcoder dependency is required")
	}
	if deps.WebRTC == nil {
		return nil, fmt.Errorf("webrtc dependency is required")
	}

	factory := deps.TaskRepoFactory
	if factory == nil {
		factory = database.NewTaskRepository
	}

	heartbeat := deps.HeartbeatInterval
	if heartbeat == 0 {
		heartbeat = cfg.Gateway.HeartbeatPeriod
		if heartbeat == 0 {
			heartbeat = 30 * time.Second
		}
	}

	nowFn := deps.Clock
	if nowFn == nil {
		nowFn = time.Now
	}

	worker := &Worker{
		config:          cfg,
		gateway:         deps.Gateway,
		downloader:      deps.Downloader,
		transcoder:      deps.Transcoder,
		webrtc:          deps.WebRTC,
		taskRepoFactory: factory,
		heartbeatEvery:  heartbeat,
		now:             nowFn,
	}

	worker.gateway.SetMessageHandler(worker.handleGatewayMessage)
	worker.downloader.SetExternalStatusHandler(worker.handleDownloadStatusChange)
	worker.webrtc.SetICECandidateHandler(worker.handleWebRTCICECandidate)

	return worker, nil
}

// Start boots up all subsystems and connects to the gateway.
func (w *Worker) Start() error {
	if err := w.downloader.Start(); err != nil {
		return err
	}

	if err := w.transcoder.Start(); err != nil {
		return err
	}

	if err := w.webrtc.Start(); err != nil {
		return err
	}

	nodeInfo := domain.NodeInfo{
		ID:           w.config.Node.ID,
		Name:         w.config.Node.Name,
		Address:      w.config.Node.Address,
		Status:       domain.WorkerStatusOnline,
		Capabilities: []string{"torrent", "transcode", "webrtc"},
		Resources: map[string]int{
			"max_downloads":  w.config.Limits.MaxDownloads,
			"max_transcodes": w.config.Limits.MaxTranscodes,
			"disk_space_gb":  w.config.Limits.DiskSpaceGB,
		},
		Metadata: map[string]string{
			"version": "1.0.0",
			"arch":    "amd64",
		},
	}

	if err := w.gateway.Connect(nodeInfo); err != nil {
		return err
	}

	go w.startHeartbeat()
	return nil
}

// Stop gracefully stops subsystems and disconnects from the gateway.
func (w *Worker) Stop() {
	w.gateway.Disconnect()
	w.webrtc.Stop()
	w.transcoder.Stop()
	w.downloader.Stop()

	if err := database.Close(); err != nil {
		log.Printf("Failed to close database: %v", err)
	}
}

// Run provides a convenience wrapper that starts the worker and blocks until
// an interrupt or terminate signal is received.
func (w *Worker) Run() error {
	if err := w.Start(); err != nil {
		return err
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	w.Stop()
	return nil
}

func (w *Worker) startHeartbeat() {
	ticker := time.NewTicker(w.heartbeatEvery)
	defer ticker.Stop()

	for range ticker.C {
		if err := w.gateway.SendHeartbeat(); err != nil {
			log.Printf("Failed to send heartbeat: %v", err)
		}
	}
}

func (w *Worker) handleGatewayMessage(msgType domain.MessageType, payload map[string]interface{}) {
	switch msgType {
	case domain.MessageTypeRegistrationConfirmed:
		log.Printf("Registration confirmed by gateway")
	case domain.MessageTypeTaskSubmit:
		w.handleTaskSubmit(payload)
	case domain.MessageTypeGetTasks:
		w.handleGetTasks(payload)
	case domain.MessageTypeGetTaskDetail:
		w.handleGetTaskDetail(payload)
	case domain.MessageTypeWebRTCOffer:
		w.handleWebRTCOffer(payload)
	case domain.MessageTypeICECandidate:
		w.handleICECandidate(payload)
	default:
		log.Printf("Unknown message type: %s", msgType)
	}
}

func (w *Worker) handleTaskSubmit(payload map[string]interface{}) {
	magnetURL, ok := payload["magnet_url"].(string)
	if !ok {
		log.Printf("Invalid magnet URL in task submit")
		return
	}

	log.Printf("Received task: %s", magnetURL)

	taskID, err := w.downloader.StartDownload(magnetURL)
	if err != nil {
		log.Printf("Failed to start download: %v", err)
		return
	}

	if err := w.gateway.SendTaskStatus(taskID, domain.TaskStatusDownloading, 0, nil); err != nil {
		log.Printf("Failed to notify gateway about task status: %v", err)
	}
}

func (w *Worker) handleGetTasks(payload map[string]interface{}) {
	tasks := w.downloader.GetAllTasks()

	taskList := make([]map[string]interface{}, 0, len(tasks))
	for _, task := range tasks {
		files, _ := task.GetTorrentFiles()
		fileNames := make([]string, len(files))
		for i, file := range files {
			fileNames[i] = file.FileName
		}

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

	response := map[string]interface{}{
		"tasks": taskList,
	}

	if requestID, ok := payload["request_id"]; ok {
		response["request_id"] = requestID
	}

	if err := w.gateway.SendMessage(domain.MessageTypeTasksResponse, response); err != nil {
		log.Printf("Failed to send tasks response: %v", err)
	}
}

func (w *Worker) handleGetTaskDetail(payload map[string]interface{}) {
	taskID, ok := payload["task_id"].(string)
	if !ok {
		log.Printf("Invalid task ID in get task detail request")
		return
	}

	task, exists := w.downloader.GetTask(taskID)
	if !exists {
		_ = w.gateway.SendMessage(domain.MessageTypeTaskDetailResponse, map[string]interface{}{
			"task_id": taskID,
			"found":   false,
		})
		return
	}

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

	srts, _ := task.GetSrts()
	metadata, _ := task.GetMetadata()

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

	_ = w.gateway.SendMessage(domain.MessageTypeTaskDetailResponse, map[string]interface{}{
		"task_id": taskID,
		"found":   true,
		"task":    taskData,
	})
}

func (w *Worker) handleWebRTCOffer(payload map[string]interface{}) {
	sessionID, _ := payload["session_id"].(string)
	clientID, _ := payload["client_id"].(string)
	sdp, _ := payload["sdp"].(string)

	log.Printf("Received WebRTC offer for session %s from client %s", sessionID, clientID)

	config := w.ensureWebRTCConfiguration()
	w.webrtc.UpdateConfiguration(config)

	answer, err := w.webrtc.HandleOffer(sessionID, sdp)
	if err != nil {
		log.Printf("Failed to handle WebRTC offer: %v", err)
		return
	}

	if err := w.gateway.SendWebRTCAnswer(sessionID, answer); err != nil {
		log.Printf("Failed to send WebRTC answer: %v", err)
	}
}

func (w *Worker) handleICECandidate(payload map[string]interface{}) {
	sessionID, _ := payload["session_id"].(string)
	candidate, _ := payload["candidate"].(string)

	log.Printf("Received ICE candidate for session %s", sessionID)

	if err := w.webrtc.AddICECandidate(sessionID, candidate); err != nil {
		log.Printf("Failed to add ICE candidate: %v", err)
	}
}

func (w *Worker) handleDownloadStatusChange(task *models.Task) {
	if task.Status == domain.TaskStatusCompleted {
		log.Printf("Download completed for task %s, starting transcoding", task.TaskID)

		files, err := task.GetTorrentFiles()
		if err != nil {
			log.Printf("Failed to get torrent files for task %s: %v", task.TaskID, err)
			return
		}

		var videoFile string
		videoExtensions := []string{".mp4", ".mkv", ".avi", ".mov", ".wmv", ".flv", ".webm", ".m4v"}

		for _, file := range files {
			for _, ext := range videoExtensions {
				if strings.HasSuffix(strings.ToLower(file.FileName), ext) {
					videoFile = filepath.Join(w.config.Storage.DownloadPath, file.FilePath)
					break
				}
			}
			if videoFile != "" {
				break
			}
		}

		if videoFile != "" {
			go w.startTranscodingForTask(task, videoFile)
		} else {
			log.Printf("No video file found in task %s", task.TaskID)
			w.updateTaskStatusInDB(task.TaskID, domain.TaskStatusReady)
		}
	}
}

func (w *Worker) startTranscodingForTask(task *models.Task, videoFile string) {
	w.updateTaskStatusInDB(task.TaskID, domain.TaskStatusTranscoding)

	transcodeID, err := w.transcoder.StartTranscode(videoFile)
	if err != nil {
		log.Printf("Failed to start transcoding for task %s: %v", task.TaskID, err)
		w.updateTaskStatusInDB(task.TaskID, domain.TaskStatusError)
		return
	}

	log.Printf("Started transcoding for task %s with transcode ID %s", task.TaskID, transcodeID)

	go w.monitorTranscodingProgress(task.TaskID, transcodeID)
}

func (w *Worker) monitorTranscodingProgress(taskID, transcodeID string) {
	statusChan := w.transcoder.GetStatusChannel()

	for transcodeTask := range statusChan {
		if transcodeTask.ID != transcodeID {
			continue
		}

		log.Printf("Transcode progress for task %s: status=%s, progress=%d%%",
			taskID, transcodeTask.Status, transcodeTask.Progress)

		switch transcodeTask.Status {
		case domain.TranscodeStatusCompleted:
			if err := w.saveTranscodingResults(taskID, transcodeTask); err != nil {
				log.Printf("Failed to save transcoding results for task %s: %v", taskID, err)
				w.updateTaskStatusInDB(taskID, domain.TaskStatusError)
			} else {
				log.Printf("Transcoding completed and saved for task %s", taskID)
				w.updateTaskStatusInDB(taskID, domain.TaskStatusReady)
			}
			return
		case domain.TranscodeStatusError:
			log.Printf("Transcoding failed for task %s: %s", taskID, transcodeTask.Metadata["error"])
			w.updateTaskStatusInDB(taskID, domain.TaskStatusError)
			return
		}
	}
}

func (w *Worker) saveTranscodingResults(taskID string, transcodeTask *transcoder.TranscodeTask) error {
	repo := w.taskRepository()
	task, err := repo.GetByTaskID(taskID)
	if err != nil {
		return fmt.Errorf("failed to get task: %v", err)
	}

	task.M3U8FilePath = transcodeTask.M3U8Path

	if len(transcodeTask.Subtitles) > 0 {
		if err := task.SetSrts(transcodeTask.Subtitles); err != nil {
			log.Printf("Failed to set subtitle files: %v", err)
		}
	}

	segments, err := w.readSegmentsFromM3U8(transcodeTask.M3U8Path)
	if err != nil {
		log.Printf("Failed to read segments from M3U8: %v", err)
	} else {
		if err := task.SetSegments(segments); err != nil {
			log.Printf("Failed to set segments: %v", err)
		}
	}

	metadata, _ := task.GetMetadata()
	if metadata == nil {
		metadata = make(map[string]interface{})
	}
	metadata["output_path"] = transcodeTask.OutputPath
	metadata["segment_count"] = len(segments)
	if err := task.SetMetadata(metadata); err != nil {
		log.Printf("Failed to set task metadata: %v", err)
	}

	return repo.Update(task)
}

func (w *Worker) readSegmentsFromM3U8(m3u8Path string) ([]string, error) {
	content, err := os.ReadFile(m3u8Path)
	if err != nil {
		return nil, fmt.Errorf("failed to read M3U8 file: %v", err)
	}

	var segments []string
	lines := strings.Split(string(content), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") && strings.HasSuffix(line, ".ts") {
			segmentPath := filepath.Join(filepath.Dir(m3u8Path), line)
			segments = append(segments, segmentPath)
		}
	}

	return segments, nil
}

func (w *Worker) handleWebRTCICECandidate(sessionID string, candidate *webrtcLib.ICECandidate) {
	log.Printf("Sending ICE candidate for session %s: %s", sessionID, candidate.String())

	candidateJSON := candidate.ToJSON()
	candidateStr := candidateJSON.Candidate

	if err := w.gateway.SendICECandidate(sessionID, candidateStr); err != nil {
		log.Printf("Failed to send ICE candidate: %v", err)
	}
}

func (w *Worker) updateTaskStatusInDB(taskID string, status domain.TaskStatus) {
	repo := w.taskRepository()
	if err := repo.UpdateStatus(taskID, status); err != nil {
		log.Printf("Failed to update task status in database: %v", err)
	}
}

func (w *Worker) taskRepository() database.TaskRepository {
	if w.taskRepoFactory == nil {
		return database.NewTaskRepository()
	}
	return w.taskRepoFactory()
}
