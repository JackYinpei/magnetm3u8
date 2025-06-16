package services

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"

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

// WSMessage WebSocket消息结构
type WSMessage struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
}

// HandleMessage 处理来自服务B的消息
func (h *MessageHandler) HandleMessage(messageData []byte) error {
	var message WSMessage
	if err := json.Unmarshal(messageData, &message); err != nil {
		return fmt.Errorf("解析消息失败: %v", err)
	}

	log.Printf("收到消息类型: %s, %s", message.Type, message.Payload)

	switch message.Type {
	case MsgTypeDownloadProgress:
		h.handleDownloadProgress(message.Payload)
	case MsgTypeTorrentInfo:
		h.handleTorrentInfo(message.Payload)
	case MsgTypeDownloadComplete:
		h.handleDownloadComplete(message.Payload)
	case MsgTypeTranscodeProgress:
		h.handleTranscodeProgress(message.Payload)
	case MsgTypeTranscodeComplete:
		h.handleTranscodeComplete(message.Payload)
	case MsgTypeError:
		h.handleError(message.Payload)
	case MsgTypeWebRTCAnswer:
		h.handleWebRTCAnswer(message.Payload)
	case MsgTypeICECandidate:
		h.handleICECandidate(message.Payload)
	default:
		log.Printf("未知消息类型: %s", message.Type)
	}

	return nil
}

// 处理Torrent信息消息
func (h *MessageHandler) handleTorrentInfo(payload interface{}) {

	log.Printf("处理Torrent信息消息: %s\n", payload)

	payloadMap, ok := payload.(map[string]interface{})
	if !ok {
		log.Printf("无效的Torrent信息载荷")
		return
	}

	taskIDFloat, ok := payloadMap["task_id"].(float64)
	if !ok {
		log.Printf("无效的任务ID")
		return
	}
	taskID := uint(taskIDFloat)

	filesInterface, ok := payloadMap["files"].([]interface{})
	if !ok {
		log.Printf("无效的文件列表")
		return
	}

	var files []models.TorrentFileInfo
	for _, fileInterface := range filesInterface {
		fileMap, ok := fileInterface.(map[string]interface{})
		if !ok {
			continue
		}

		fileName, _ := fileMap["name"].(string)
		fileSizeFloat, _ := fileMap["size"].(float64)
		filePath, _ := fileMap["path"].(string)

		files = append(files, models.TorrentFileInfo{
			FileName:   fileName,
			FileSize:   int64(fileSizeFloat),
			FilePath:   filePath,
			IsSelected: true, // 默认选中
		})
	}

	// 保存文件信息
	if err := h.torrentService.SaveTorrentFiles(taskID, files); err != nil {
		log.Printf("保存Torrent文件信息失败: %v", err)
		return
	}

	// 更新任务状态
	if err := h.torrentService.UpdateTaskStatus(taskID, "downloading"); err != nil {
		log.Printf("更新任务状态失败: %v", err)
	}

	log.Printf("已保存任务 %d 的Torrent文件信息，共 %d 个文件", taskID, len(files))
}

// 处理下载进度消息
func (h *MessageHandler) handleDownloadProgress(payload interface{}) {
	payloadMap, ok := payload.(map[string]interface{})
	if !ok {
		log.Printf("无效的下载进度载荷")
		return
	}

	taskIDFloat, ok := payloadMap["task_id"].(float64)
	if !ok {
		log.Printf("无效的任务ID")
		return
	}
	taskID := uint(taskIDFloat)

	percentageStr, ok := payloadMap["percentage"].(string)
	if !ok {
		log.Printf("无效的下载百分比")
		return
	}

	speedStr, ok := payloadMap["speed"].(string)
	if !ok {
		log.Printf("无效的下载速度")
		return
	}

	percentageFloat, err := strconv.ParseFloat(percentageStr, 64)
	if err != nil {
		log.Printf("解析下载百分比失败: %v", err)
		return
	}

	speedFloat, err := strconv.ParseFloat(speedStr, 64)
	if err != nil {
		log.Printf("解析下载速度失败: %v", err)
		return
	}

	if err := h.torrentService.UpdateDownloadProgress(taskID, percentageFloat, int64(speedFloat)); err != nil {
		log.Printf("更新下载进度失败: %v", err)
		return
	}

	log.Printf("更新任务 %d 下载进度: %.2f%%, 速度: %.0f bytes/s", taskID, percentageFloat, speedFloat)
}

// 处理下载完成消息
func (h *MessageHandler) handleDownloadComplete(payload interface{}) {
	payloadMap, ok := payload.(map[string]interface{})
	if !ok {
		log.Printf("无效的下载完成载荷")
		return
	}

	taskIDFloat, ok := payloadMap["task_id"].(float64)
	if !ok {
		log.Printf("无效的任务ID")
		return
	}
	taskID := uint(taskIDFloat)

	// 更新下载进度为100%
	if err := h.torrentService.UpdateDownloadProgress(taskID, 100.0, 0); err != nil {
		log.Printf("更新下载进度失败: %v", err)
	}

	// 更新任务状态为已完成
	if err := h.torrentService.UpdateTaskStatus(taskID, "completed"); err != nil {
		log.Printf("更新任务状态失败: %v", err)
		return
	}

	log.Printf("任务 %d 下载完成", taskID)
}

// 处理转码进度消息
func (h *MessageHandler) handleTranscodeProgress(payload interface{}) {
	payloadMap, ok := payload.(map[string]interface{})
	if !ok {
		log.Printf("无效的转码进度载荷")
		return
	}

	taskIDFloat, ok := payloadMap["task_id"].(float64)
	if !ok {
		log.Printf("无效的任务ID")
		return
	}
	taskID := uint(taskIDFloat)

	// 更新任务状态为转码中
	if err := h.torrentService.UpdateTaskStatus(taskID, "transcoding"); err != nil {
		log.Printf("更新任务状态失败: %v", err)
	}

	log.Printf("任务 %d 正在转码", taskID)
}

// 处理转码完成消息
func (h *MessageHandler) handleTranscodeComplete(payload interface{}) {
	payloadMap, ok := payload.(map[string]interface{})
	if !ok {
		log.Printf("无效的转码完成载荷")
		return
	}

	taskIDFloat, ok := payloadMap["task_id"].(float64)
	if !ok {
		log.Printf("无效的任务ID")
		return
	}
	taskID := uint(taskIDFloat)

	m3u8Path, ok := payloadMap["m3u8_path"].(string)
	if !ok {
		log.Printf("无效的M3U8路径")
		return
	}

	log.Printf("转码完成，M3U8路径: %s", m3u8Path)

	// 保存M3U8信息
	if err := h.torrentService.SaveM3U8Info(taskID, m3u8Path); err != nil {
		log.Printf("保存M3U8信息失败: %v", err)
		return
	}

	// 更新任务状态为可播放
	if err := h.torrentService.UpdateTaskStatus(taskID, "ready"); err != nil {
		log.Printf("更新任务状态失败: %v", err)
		return
	}

	log.Printf("任务 %d 转码完成，M3U8路径: %s", taskID, m3u8Path)
}

// 处理错误消息
func (h *MessageHandler) handleError(payload interface{}) {
	payloadMap, ok := payload.(map[string]interface{})
	if !ok {
		log.Printf("无效的错误载荷")
		return
	}

	taskIDFloat, ok := payloadMap["task_id"].(float64)
	if !ok {
		log.Printf("无效的任务ID")
		return
	}
	taskID := uint(taskIDFloat)

	errorMsg, ok := payloadMap["error"].(string)
	if !ok {
		log.Printf("无效的错误消息")
		return
	}

	// 更新任务状态为失败
	if err := h.torrentService.UpdateTaskStatus(taskID, "failed"); err != nil {
		log.Printf("更新任务状态失败: %v", err)
	}

	log.Printf("任务 %d 出现错误: %s", taskID, errorMsg)
}

// 处理WebRTC Answer消息
func (h *MessageHandler) handleWebRTCAnswer(payload interface{}) {
	payloadMap, ok := payload.(map[string]interface{})
	if !ok {
		log.Printf("无效的WebRTC Answer消息格式")
		return
	}

	clientID, ok := payloadMap["client_id"].(string)
	if !ok {
		log.Printf("WebRTC Answer消息中缺少client_id字段")
		return
	}

	sdp, ok := payloadMap["sdp"].(string)
	if !ok {
		log.Printf("WebRTC Answer消息中缺少sdp字段")
		return
	}

	// 将Answer发送到客户端
	if err := h.webrtcService.SendAnswer(clientID, sdp); err != nil {
		log.Printf("发送WebRTC Answer到客户端失败: %v", err)
	}
}

// 处理ICE Candidate消息
func (h *MessageHandler) handleICECandidate(payload interface{}) {
	payloadMap, ok := payload.(map[string]interface{})
	if !ok {
		log.Printf("无效的ICE Candidate消息格式")
		return
	}

	clientID, ok := payloadMap["client_id"].(string)
	if !ok {
		log.Printf("ICE Candidate消息中缺少client_id字段")
		return
	}

	candidate, ok := payloadMap["candidate"].(string)
	if !ok {
		log.Printf("ICE Candidate消息中缺少candidate字段")
		return
	}

	isClient, ok := payloadMap["is_client"].(bool)
	if !ok || !isClient {
		// 这是从服务B发来的Candidate，需要转发给客户端
		if err := h.webrtcService.SendICECandidateToClient(clientID, candidate); err != nil {
			log.Printf("发送ICE Candidate到客户端失败: %v", err)
		}
	}
}

// GetHandler 获取消息处理器实例
func GetHandler() *MessageHandler {
	if handler == nil {
		handler = NewMessageHandler()
	}
	return handler
}

// SetupMessageHandling 设置消息处理
func SetupMessageHandling() {
	handler := GetHandler()
	wsManager := GetWebSocketManager()

	// 设置消息处理函数
	wsManager.SetMessageHandler(func(message WebSocketMessage) {
		messageData, err := json.Marshal(message)
		if err != nil {
			log.Printf("序列化消息失败: %v", err)
			return
		}

		if err := handler.HandleMessage(messageData); err != nil {
			log.Printf("处理消息失败: %v", err)
		}
	})

	// 启动WebSocket连接检查器
	wsManager.StartConnectionChecker()

	// 启动WebRTC会话清理
	handler.webrtcService.StartSessionCleanup()
}

var handler *MessageHandler
