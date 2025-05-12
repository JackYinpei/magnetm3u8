package services

import (
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// 消息类型常量
const (
	MsgTypeMagnetSubmit      = "magnet_submit"      // 提交磁力链接
	MsgTypeTorrentInfo       = "torrent_info"       // Torrent信息
	MsgTypeDownloadProgress  = "download_progress"  // 下载进度
	MsgTypeDownloadComplete  = "download_complete"  // 下载完成
	MsgTypeTranscodeStart    = "transcode_start"    // 开始转码
	MsgTypeTranscodeComplete = "transcode_complete" // 转码完成
	MsgTypeError             = "error"              // 错误信息
	MsgTypeWebRTCOffer       = "webrtc_offer"       // WebRTC Offer
	MsgTypeWebRTCAnswer      = "webrtc_answer"      // WebRTC Answer
	MsgTypeICECandidate      = "ice_candidate"      // ICE Candidate
)

// WebSocketMessage 定义WebSocket消息结构
type WebSocketMessage struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
}

// WebSocketManager 管理与服务B的WebSocket连接
type WebSocketManager struct {
	conn           *websocket.Conn
	isConnected    bool
	mu             sync.RWMutex
	messageHandler func(message WebSocketMessage)
}

var (
	wsManager     *WebSocketManager
	wsManagerOnce sync.Once
)

// GetWebSocketManager 获取WebSocket管理器单例
func GetWebSocketManager() *WebSocketManager {
	wsManagerOnce.Do(func() {
		wsManager = &WebSocketManager{
			isConnected: false,
		}
	})
	return wsManager
}

// SetMessageHandler 设置消息处理函数
func (wm *WebSocketManager) SetMessageHandler(handler func(message WebSocketMessage)) {
	wm.mu.Lock()
	defer wm.mu.Unlock()
	wm.messageHandler = handler
}

// IsConnected 检查是否已连接
func (wm *WebSocketManager) IsConnected() bool {
	wm.mu.RLock()
	defer wm.mu.RUnlock()
	return wm.isConnected
}

// RegisterConnection 注册新的WebSocket连接
func (wm *WebSocketManager) RegisterConnection(conn *websocket.Conn) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	// 关闭旧连接
	if wm.conn != nil {
		wm.conn.Close()
	}

	wm.conn = conn
	wm.isConnected = true

	// 启动读取消息的goroutine
	go wm.readMessages()

	log.Println("服务B已连接")
}

// readMessages 读取来自服务B的消息
func (wm *WebSocketManager) readMessages() {
	for {
		var message WebSocketMessage
		err := wm.conn.ReadJSON(&message)
		if err != nil {
			log.Printf("读取WebSocket消息错误: %v", err)
			wm.handleDisconnect()
			return
		}

		// 处理接收到的消息
		wm.mu.RLock()
		handler := wm.messageHandler
		wm.mu.RUnlock()

		if handler != nil {
			handler(message)
		}
	}
}

// SendMessage 向服务B发送消息
func (wm *WebSocketManager) SendMessage(messageType string, payload interface{}) error {
	wm.mu.RLock()
	defer wm.mu.RUnlock()

	if !wm.isConnected || wm.conn == nil {
		return ErrNotConnected
	}

	message := WebSocketMessage{
		Type:    messageType,
		Payload: payload,
	}

	return wm.conn.WriteJSON(message)
}

// handleDisconnect 处理断开连接
func (wm *WebSocketManager) handleDisconnect() {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	wm.isConnected = false
	if wm.conn != nil {
		wm.conn.Close()
		wm.conn = nil
	}

	log.Println("服务B断开连接")
}

// StartConnectionChecker 启动连接检查器
func (wm *WebSocketManager) StartConnectionChecker() {
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			wm.mu.RLock()
			if wm.isConnected && wm.conn != nil {
				err := wm.conn.WriteMessage(websocket.PingMessage, []byte{})
				if err != nil {
					log.Printf("Ping失败: %v", err)
					wm.mu.RUnlock()
					wm.handleDisconnect()
					continue
				}
			}
			wm.mu.RUnlock()
		}
	}()
}
