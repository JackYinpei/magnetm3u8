package services

import (
	"log"

	"magnetm3u8/models"
)

// MessageHandler 处理从服务B接收到的消息
type MessageHandler struct {
	torrentService *TorrentService
	webrtcService  *WebRTCService
}

// NewMessageHandler 创建新的MessageHandler
func NewMessageHandler() *MessageHandler {
	return &MessageHandler{
		torrentService: NewTorrentService(),
		webrtcService:  NewWebRTCService(),
	}
}

// HandleMessage 处理接收到的消息
func (h *MessageHandler) HandleMessage(message WebSocketMessage) {
	log.Printf("收到消息类型: %s", message.Type)

	switch message.Type {
	case MsgTypeTorrentInfo:
		h.handleTorrentInfo(message.Payload)
	case MsgTypeDownloadProgress:
		h.handleDownloadProgress(message.Payload)
	case MsgTypeDownloadComplete:
		h.handleDownloadComplete(message.Payload)
	case MsgTypeTranscodeComplete:
		h.handleTranscodeComplete(message.Payload)
	case MsgTypeError:
		h.handleError(message.Payload)
	case MsgTypeWebRTCAnswer:
		h.handleWebRTCAnswer(message.Payload)
	case MsgTypeICECandidate:
		h.handleICECandidate(message.Payload)
	default:
		log.Printf("未知的消息类型: %s", message.Type)
	}
}

// 处理Torrent信息消息
func (h *MessageHandler) handleTorrentInfo(payload interface{}) {
	// 将payload解析为map
	data, ok := payload.(map[string]interface{})
	if !ok {
		log.Printf("无效的Torrent信息消息格式")
		return
	}

	// 提取任务ID
	taskIDFloat, ok := data["task_id"].(float64)
	if !ok {
		log.Printf("Torrent信息中缺少task_id字段")
		return
	}
	taskID := uint(taskIDFloat)

	// 提取文件列表
	filesData, ok := data["files"].([]interface{})
	if !ok {
		log.Printf("Torrent信息中缺少files字段")
		return
	}

	// 构建文件对象列表
	var files []models.TorrentFile
	for _, fileData := range filesData {
		fileMap, ok := fileData.(map[string]interface{})
		if !ok {
			continue
		}

		fileName, _ := fileMap["file_name"].(string)
		fileSizeFloat, _ := fileMap["file_size"].(float64)
		filePath, _ := fileMap["file_path"].(string)

		files = append(files, models.TorrentFile{
			TaskID:     taskID,
			FileName:   fileName,
			FileSize:   int64(fileSizeFloat),
			FilePath:   filePath,
			IsSelected: true, // 默认选中所有文件
		})
	}

	// 更新任务状态为downloading
	err := h.torrentService.UpdateTaskStatus(taskID, "downloading")
	if err != nil {
		log.Printf("更新任务状态失败: %v", err)
	}

	// 保存文件信息
	err = h.torrentService.SaveTorrentFiles(taskID, files)
	if err != nil {
		log.Printf("保存文件信息失败: %v", err)
	}
}

// 处理下载进度消息
func (h *MessageHandler) handleDownloadProgress(payload interface{}) {
	// 将payload解析为map
	data, ok := payload.(map[string]interface{})
	if !ok {
		log.Printf("无效的下载进度消息格式")
		return
	}

	// 提取信息
	taskIDFloat, ok := data["task_id"].(float64)
	if !ok {
		log.Printf("下载进度中缺少task_id字段")
		return
	}
	taskID := uint(taskIDFloat)

	percentageFloat, ok := data["percentage"].(float64)
	if !ok {
		log.Printf("下载进度中缺少percentage字段")
		return
	}

	speedFloat, ok := data["speed"].(float64)
	if !ok {
		log.Printf("下载进度中缺少speed字段")
		return
	}

	// 更新下载进度
	err := h.torrentService.UpdateDownloadProgress(taskID, percentageFloat, int64(speedFloat))
	if err != nil {
		log.Printf("更新下载进度失败: %v", err)
	}
}

// 处理下载完成消息
func (h *MessageHandler) handleDownloadComplete(payload interface{}) {
	// 将payload解析为map
	data, ok := payload.(map[string]interface{})
	if !ok {
		log.Printf("无效的下载完成消息格式")
		return
	}

	// 提取任务ID
	taskIDFloat, ok := data["task_id"].(float64)
	if !ok {
		log.Printf("下载完成消息中缺少task_id字段")
		return
	}
	taskID := uint(taskIDFloat)

	// 更新任务状态为transcoding（表示正在转码）
	err := h.torrentService.UpdateTaskStatus(taskID, "transcoding")
	if err != nil {
		log.Printf("更新任务状态失败: %v", err)
	}

	// 更新下载进度为100%
	err = h.torrentService.UpdateDownloadProgress(taskID, 100.0, 0)
	if err != nil {
		log.Printf("更新下载进度失败: %v", err)
	}
}

// 处理转码完成消息
func (h *MessageHandler) handleTranscodeComplete(payload interface{}) {
	// 将payload解析为map
	data, ok := payload.(map[string]interface{})
	if !ok {
		log.Printf("无效的转码完成消息格式")
		return
	}

	// 提取信息
	taskIDFloat, ok := data["task_id"].(float64)
	if !ok {
		log.Printf("转码完成消息中缺少task_id字段")
		return
	}
	taskID := uint(taskIDFloat)

	m3u8Path, ok := data["m3u8_path"].(string)
	if !ok {
		log.Printf("转码完成消息中缺少m3u8_path字段")
		return
	}

	// 保存M3U8信息
	err := h.torrentService.SaveM3U8Info(taskID, m3u8Path)
	if err != nil {
		log.Printf("保存M3U8信息失败: %v", err)
	}

	// 更新任务状态为ready（表示可以播放）
	err = h.torrentService.UpdateTaskStatus(taskID, "ready")
	if err != nil {
		log.Printf("更新任务状态失败: %v", err)
	}
}

// 处理错误消息
func (h *MessageHandler) handleError(payload interface{}) {
	// 将payload解析为map
	data, ok := payload.(map[string]interface{})
	if !ok {
		log.Printf("无效的错误消息格式")
		return
	}

	// 提取信息
	taskIDFloat, ok := data["task_id"].(float64)
	if !ok {
		log.Printf("错误消息中缺少task_id字段")
		return
	}
	taskID := uint(taskIDFloat)

	errorMsg, ok := data["error"].(string)
	if !ok {
		log.Printf("错误消息中缺少error字段")
		return
	}

	log.Printf("任务 %d 发生错误: %s", taskID, errorMsg)

	// 更新任务状态为failed
	err := h.torrentService.UpdateTaskStatus(taskID, "failed")
	if err != nil {
		log.Printf("更新任务状态失败: %v", err)
	}

	// 记录错误到数据库日志表
	// 这里可以添加一个错误日志表来记录详细信息
}

// 处理WebRTC Answer消息
func (h *MessageHandler) handleWebRTCAnswer(payload interface{}) {
	// 将payload解析为map
	data, ok := payload.(map[string]interface{})
	if !ok {
		log.Printf("无效的WebRTC Answer消息格式")
		return
	}

	// 提取信息
	clientID, ok := data["client_id"].(string)
	if !ok {
		log.Printf("WebRTC Answer消息中缺少client_id字段")
		return
	}

	sdp, ok := data["sdp"].(string)
	if !ok {
		log.Printf("WebRTC Answer消息中缺少sdp字段")
		return
	}

	// 将Answer发送到客户端
	err := h.webrtcService.SendAnswer(clientID, sdp)
	if err != nil {
		log.Printf("发送WebRTC Answer到客户端失败: %v", err)
	}
}

// 处理ICE Candidate消息
func (h *MessageHandler) handleICECandidate(payload interface{}) {
	// 将payload解析为map
	data, ok := payload.(map[string]interface{})
	if !ok {
		log.Printf("无效的ICE Candidate消息格式")
		return
	}

	// 提取信息
	clientID, ok := data["client_id"].(string)
	if !ok {
		log.Printf("ICE Candidate消息中缺少client_id字段")
		return
	}

	candidate, ok := data["candidate"].(string)
	if !ok {
		log.Printf("ICE Candidate消息中缺少candidate字段")
		return
	}

	isClient, ok := data["is_client"].(bool)
	if !ok || !isClient {
		// 这是从服务B发来的Candidate，需要转发给客户端
		err := h.webrtcService.SendICECandidateToClient(clientID, candidate)
		if err != nil {
			log.Printf("发送ICE Candidate到客户端失败: %v", err)
		}
	}
}

// SetupMessageHandling 设置消息处理
func SetupMessageHandling() {
	handler := NewMessageHandler()
	wsManager := GetWebSocketManager()

	// 设置消息处理函数
	wsManager.SetMessageHandler(func(message WebSocketMessage) {
		handler.HandleMessage(message)
	})

	// 启动WebRTC会话清理
	handler.webrtcService.StartSessionCleanup()

	// 启动WebSocket连接检查器
	wsManager.StartConnectionChecker()
}
