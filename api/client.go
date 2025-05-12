package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"magnetm3u8/services"

	"github.com/gorilla/websocket"
)

// ClientConnection 表示客户端WebSocket连接
type ClientConnection struct {
	ID   string
	Conn *websocket.Conn
}

var (
	clients      = make(map[string]*ClientConnection)
	clientsMutex sync.RWMutex
)

// 生成客户端ID
func generateClientID() string {
	// 生成一个随机的客户端ID
	return fmt.Sprintf("client-%d", time.Now().UnixNano())
}

// 注册客户端连接
func registerClientConnection(client *ClientConnection) {
	clientsMutex.Lock()
	defer clientsMutex.Unlock()

	// 如果已存在同ID的连接，关闭旧连接
	if oldClient, exists := clients[client.ID]; exists && oldClient.Conn != nil {
		oldClient.Conn.Close()
	}

	clients[client.ID] = client
	log.Printf("客户端 %s 已连接", client.ID)
}

// 处理客户端消息的函数类型
type ClientMessageHandler func(clientID string, messageType string, payload interface{})

// HandleClientMessages 处理来自客户端的消息
func HandleClientMessages(client *ClientConnection, handler ClientMessageHandler) {
	defer func() {
		// 连接关闭时，清理资源
		client.Conn.Close()
		clientsMutex.Lock()
		delete(clients, client.ID)
		clientsMutex.Unlock()
		log.Printf("客户端 %s 已断开连接", client.ID)
	}()

	for {
		// 读取消息
		messageType, message, err := client.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("读取客户端消息错误: %v", err)
			}
			break
		}

		// 只处理文本消息
		if messageType != websocket.TextMessage {
			continue
		}

		// 解析消息
		var wsMessage struct {
			Type    string      `json:"type"`
			Payload interface{} `json:"payload"`
		}
		if err := json.Unmarshal(message, &wsMessage); err != nil {
			log.Printf("解析客户端消息失败: %v", err)
			continue
		}

		// 调用处理函数
		if handler != nil {
			handler(client.ID, wsMessage.Type, wsMessage.Payload)
		}
	}
}

// SendMessageToClient 向客户端发送消息
func SendMessageToClient(clientID string, messageType string, payload interface{}) error {
	clientsMutex.RLock()
	client, exists := clients[clientID]
	clientsMutex.RUnlock()

	if !exists || client.Conn == nil {
		return errors.New("客户端未连接")
	}

	message := struct {
		Type    string      `json:"type"`
		Payload interface{} `json:"payload"`
	}{
		Type:    messageType,
		Payload: payload,
	}

	return client.Conn.WriteJSON(message)
}

// HandleClientConnection 处理客户端WebSocket连接
func HandleClientConnection(conn *websocket.Conn, clientIDParam string, webrtcService *services.WebRTCService) {
	// 获取或生成客户端ID
	clientID := clientIDParam
	if clientID == "" {
		clientID = generateClientID()
	}

	// 创建一个客户端连接对象
	client := &ClientConnection{
		ID:   clientID,
		Conn: conn,
	}

	// 注册客户端连接
	registerClientConnection(client)

	// 定义消息处理函数
	messageHandler := func(clientID string, messageType string, payload interface{}) {
		switch messageType {
		case services.MsgTypeWebRTCOffer:
			// 处理WebRTC Offer
			if data, ok := payload.(map[string]interface{}); ok {
				taskIDFloat, _ := data["task_id"].(float64)
				sdp, _ := data["sdp"].(string)

				// 创建WebRTC会话并发送Offer到服务B
				webrtcService.CreateSession(uint(taskIDFloat), clientID)
				webrtcService.SendOffer(clientID, uint(taskIDFloat), sdp)
			}
		case services.MsgTypeICECandidate:
			// 处理ICE Candidate
			if data, ok := payload.(map[string]interface{}); ok {
				candidate, _ := data["candidate"].(string)

				// 发送ICE Candidate到服务B
				webrtcService.SendICECandidateToServiceB(clientID, candidate)
			}
		}
	}

	// 处理客户端消息
	HandleClientMessages(client, messageHandler)
}
