package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"magnetm3u8_service_b/downloader"
	"magnetm3u8_service_b/transcoder"
	"magnetm3u8_service_b/webrtc"
)

var (
	serverA           = flag.String("server", "ws://localhost:8080/ws/service-b", "服务A的WebSocket地址")
	downloadDir       = flag.String("download", "./downloads", "下载目录")
	m3u8Dir           = flag.String("m3u8", "./m3u8", "M3U8文件存储目录")
	reconnectInterval = flag.Int("reconnect", 5, "重连间隔（秒）")
)

func main() {
	flag.Parse()

	// 创建下载和转码目录
	createDirectories()

	// 初始化下载管理器
	dlManager := downloader.NewManager(*downloadDir)

	// 初始化转码管理器
	tcManager := transcoder.NewManager(*downloadDir, *m3u8Dir)

	// 初始化WebRTC管理器
	rtcManager := webrtc.NewManager(*m3u8Dir)

	// 创建连接管理器
	conn := NewConnectionManager(*serverA, dlManager, tcManager, rtcManager)

	// 连接到服务A
	go func() {
		for {
			err := conn.Connect()
			if err != nil {
				log.Printf("连接到服务A失败: %v, %d秒后重试", err, *reconnectInterval)
				time.Sleep(time.Duration(*reconnectInterval) * time.Second)
				continue
			}

			// 连接成功后等待断开
			conn.Wait()
			log.Printf("与服务A的连接已断开, %d秒后重试", *reconnectInterval)
			time.Sleep(time.Duration(*reconnectInterval) * time.Second)
		}
	}()

	// 等待中断信号
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	// 关闭连接
	conn.Close()
	log.Println("服务B已关闭")
}

func createDirectories() {
	dirs := []string{*downloadDir, *m3u8Dir}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			log.Fatalf("创建目录失败 %s: %v", dir, err)
		}
		absPath, err := filepath.Abs(dir)
		if err == nil {
			log.Printf("目录已创建: %s", absPath)
		}
	}
}

// ConnectionManager 管理与服务A的WebSocket连接
type ConnectionManager struct {
	serverURL  string
	conn       *WebSocketConnection
	dlManager  *downloader.Manager
	tcManager  *transcoder.Manager
	rtcManager *webrtc.Manager
	closeCh    chan struct{}
}

// NewConnectionManager 创建新的连接管理器
func NewConnectionManager(serverURL string, dlManager *downloader.Manager, tcManager *transcoder.Manager, rtcManager *webrtc.Manager) *ConnectionManager {
	return &ConnectionManager{
		serverURL:  serverURL,
		dlManager:  dlManager,
		tcManager:  tcManager,
		rtcManager: rtcManager,
		closeCh:    make(chan struct{}),
	}
}

// Connect 连接到服务A
func (cm *ConnectionManager) Connect() error {
	// 创建WebSocket连接
	conn, err := NewWebSocketConnection(cm.serverURL)
	if err != nil {
		return err
	}

	cm.conn = conn

	// 设置消息处理函数
	cm.conn.SetMessageHandler(func(msgType string, payload map[string]interface{}) {
		cm.handleMessage(msgType, payload)
	})

	log.Printf("已连接到服务A: %s", cm.serverURL)
	return nil
}

// Wait 等待连接断开
func (cm *ConnectionManager) Wait() {
	if cm.conn != nil {
		cm.conn.Wait()
	}
}

// Close 关闭连接
func (cm *ConnectionManager) Close() {
	close(cm.closeCh)
	if cm.conn != nil {
		cm.conn.Close()
	}
}

// 处理来自服务A的消息
func (cm *ConnectionManager) handleMessage(msgType string, payload map[string]interface{}) {
	switch msgType {
	case "magnet_submit":
		// 处理磁力链接提交
		cm.handleMagnetSubmit(payload)
	case "webrtc_offer":
		// 处理WebRTC Offer
		cm.handleWebRTCOffer(payload)
	case "ice_candidate":
		// 处理ICE Candidate
		cm.handleICECandidate(payload)
	default:
		log.Printf("未知消息类型: %s", msgType)
	}
}

// 处理磁力链接提交
func (cm *ConnectionManager) handleMagnetSubmit(payload map[string]interface{}) {
	taskID, ok := payload["task_id"].(float64)
	if !ok {
		log.Printf("无效的task_id")
		return
	}

	magnetURL, ok := payload["magnet_url"].(string)
	if !ok {
		log.Printf("无效的magnet_url")
		return
	}

	log.Printf("收到磁力链接任务: ID=%d, URL=%s", int(taskID), magnetURL)

	// 开始处理磁力链接
	go cm.processMagnetTask(uint(taskID), magnetURL)
}

// 处理WebRTC Offer
func (cm *ConnectionManager) handleWebRTCOffer(payload map[string]interface{}) {
	clientID, ok := payload["client_id"].(string)
	if !ok {
		log.Printf("WebRTC Offer中缺少client_id")
		return
	}

	taskID, ok := payload["task_id"].(float64)
	if !ok {
		log.Printf("WebRTC Offer中缺少task_id")
		return
	}

	sdp, ok := payload["sdp"].(string)
	if !ok {
		log.Printf("WebRTC Offer中缺少sdp")
		return
	}

	// 处理WebRTC Offer
	go cm.rtcManager.HandleOffer(cm.conn, uint(taskID), clientID, sdp)
}

// 处理ICE Candidate
func (cm *ConnectionManager) handleICECandidate(payload map[string]interface{}) {
	clientID, ok := payload["client_id"].(string)
	if !ok {
		log.Printf("ICE Candidate中缺少client_id")
		return
	}

	candidate, ok := payload["candidate"].(string)
	if !ok {
		log.Printf("ICE Candidate中缺少candidate")
		return
	}

	isClient, _ := payload["is_client"].(bool)

	// 处理ICE Candidate
	if isClient {
		cm.rtcManager.AddICECandidate(clientID, candidate)
	}
}

// 处理磁力链接任务
func (cm *ConnectionManager) processMagnetTask(taskID uint, magnetURL string) {
	// 1. 下载Torrent元数据，等待两分钟，如失败则报错
	torrentInfo, err := cm.dlManager.GetTorrentInfo(magnetURL)
	if err != nil {
		log.Printf("获取Torrent信息失败: %v", err)
		cm.reportError(taskID, err.Error())
		return
	}

	// 2. 发送Torrent信息给服务A
	cm.sendTorrentInfo(taskID, torrentInfo)

	// 3. 下载文件
	downloadComplete := make(chan bool, 1)
	downloadError := make(chan error, 1)

	go func() {
		err := cm.dlManager.Download(taskID, magnetURL, func(percentage float64, speed int64) {
			// 进度回调
			cm.sendDownloadProgress(taskID, percentage, speed)

			// 当下载进度达到100%时，表示下载完成
			if percentage >= 100.0 {
				downloadComplete <- true
			}
		})

		if err != nil {
			downloadError <- err
		}
	}()

	// 等待下载完成或出错
	select {
	case <-downloadComplete:
		// 下载完成
		log.Printf("任务 %d 下载完成", taskID)
	case err := <-downloadError:
		// 下载出错
		log.Printf("下载失败: %v", err)
		cm.reportError(taskID, err.Error())
		return
	case <-time.After(2 * time.Hour): // 设置超时时间
		// 下载超时
		log.Printf("下载超时")
		cm.reportError(taskID, "下载超时")
		return
	}

	// 4. 下载完成，通知服务A
	cm.sendDownloadComplete(taskID)

	// 5. 转码文件
	filePath := cm.dlManager.GetDownloadedFilePath(taskID)
	if filePath == "" {
		cm.reportError(taskID, "找不到下载的文件")
		return
	}

	// 等待文件系统完全同步，确保文件可以访问
	var fileReady bool
	for i := 0; i < 30; i++ {
		file, err := os.Open(filePath)
		if err == nil {
			file.Close()
			fileReady = true
			log.Printf("文件已准备就绪: %s", filePath)
			break
		}
		log.Printf("等待文件准备就绪(%d/30): %s, 错误: %v", i+1, filePath, err)
		time.Sleep(time.Second)
	}

	if !fileReady {
		cm.reportError(taskID, fmt.Sprintf("文件准备超时，无法访问: %s", filePath))
		return
	}

	log.Printf("开始转码文件: %s", filePath)
	m3u8Path, err := cm.tcManager.Transcode(taskID, filePath)
	if err != nil {
		log.Printf("转码失败: %v", err)
		cm.reportError(taskID, fmt.Sprintf("转码失败: %v", err))
		return
	}

	// 6. 转码完成，通知服务A
	cm.sendTranscodeComplete(taskID, m3u8Path)
}

// 发送Torrent信息给服务A
func (cm *ConnectionManager) sendTorrentInfo(taskID uint, info *downloader.TorrentInfo) {
	// 转换文件信息结构，使字段名与服务A期望的字段名匹配
	var formattedFiles []map[string]interface{}
	for _, file := range info.Files {
		formattedFiles = append(formattedFiles, map[string]interface{}{
			"file_name": filepath.Base(file.Path), // 从路径中提取文件名
			"file_size": file.Size,                // 大小保持不变
			"file_path": file.Path,                // 路径保持不变
		})
	}

	payload := map[string]interface{}{
		"task_id":   taskID,
		"name":      info.Name,
		"size":      info.Size,
		"files":     formattedFiles, // 使用转换后的文件列表
		"info_hash": info.InfoHash,
	}

	err := cm.conn.SendMessage("torrent_info", payload)
	if err != nil {
		log.Printf("发送Torrent信息失败: %v", err)
	} else {
		log.Printf("已发送Torrent信息: %s (大小: %d bytes)", info.Name, info.Size)
	}
}

// 发送下载进度给服务A
func (cm *ConnectionManager) sendDownloadProgress(taskID uint, percentage float64, speed int64) {
	payload := map[string]interface{}{
		"task_id":    taskID,
		"percentage": percentage,
		"speed":      speed,
	}

	err := cm.conn.SendMessage("download_progress", payload)
	if err != nil {
		log.Printf("发送下载进度失败: %v", err)
	} else {
		log.Printf("下载进度: %.2f%%, 速度: %d bytes/s", percentage, speed)
	}
}

// 发送下载完成通知给服务A
func (cm *ConnectionManager) sendDownloadComplete(taskID uint) {
	payload := map[string]interface{}{
		"task_id": taskID,
	}

	err := cm.conn.SendMessage("download_complete", payload)
	if err != nil {
		log.Printf("发送下载完成通知失败: %v", err)
	} else {
		log.Printf("下载任务 %d 已完成", taskID)
	}
}

// 发送转码完成通知给服务A
func (cm *ConnectionManager) sendTranscodeComplete(taskID uint, m3u8Path string) {
	payload := map[string]interface{}{
		"task_id":   taskID,
		"m3u8_path": m3u8Path,
	}

	err := cm.conn.SendMessage("transcode_complete", payload)
	if err != nil {
		log.Printf("发送转码完成通知失败: %v", err)
	} else {
		log.Printf("转码任务 %d 已完成: %s", taskID, m3u8Path)
	}
}

// 报告错误给服务A
func (cm *ConnectionManager) reportError(taskID uint, errorMsg string) {
	payload := map[string]interface{}{
		"task_id": taskID,
		"error":   errorMsg,
	}

	err := cm.conn.SendMessage("error", payload)
	if err != nil {
		log.Printf("报告错误失败: %v", err)
	} else {
		log.Printf("任务 %d 报告错误: %s", taskID, errorMsg)
	}
}
