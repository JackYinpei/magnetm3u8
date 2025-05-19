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

	// 设置发送消息到客户端的回调函数
	services.SetSendToClientFunc(SendMessageToClient)

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

		log.Printf("收到客户端消息类型: %s, 内容: %+v", wsMessage.Type, wsMessage.Payload)

		switch wsMessage.Type {
		case services.MsgTypeWebRTCOffer:
			// 处理WebRTC Offer
			if data, ok := wsMessage.Payload.(map[string]interface{}); ok {
				log.Printf("收到WebRTC Offer: %v\n要转发到service_b", data)

				// 支持不同格式的任务ID
				var taskID uint
				if taskIDFloat, ok := data["task_id"].(float64); ok {
					taskID = uint(taskIDFloat)
				} else if taskIDStr, ok := data["task_id"].(string); ok {
					// player.html可能发送字符串类型的任务ID
					log.Printf("收到字符串类型的task_id: %s", taskIDStr)
					taskID = 0 // 简化处理，使用默认值
				} else {
					log.Printf("无法识别的task_id类型")
					taskID = 0
				}

				sdp, _ := data["sdp"].(string)

				// 创建WebRTC会话并发送Offer到服务B
				webrtcService.CreateSession(taskID, clientID)
				webrtcService.SendOffer(clientID, taskID, sdp)
			}
		case services.MsgTypeICECandidate:
			// 处理ICE Candidate
			if data, ok := wsMessage.Payload.(map[string]interface{}); ok {
				var candidate string

				// 处理player.html和index.html可能发送的不同格式
				if payload, ok := data["payload"]; ok && payload != nil {
					// player.html格式: {type: "ice_candidate", payload: {...}}
					if payloadMap, ok := payload.(map[string]interface{}); ok {
						candidate, _ = payloadMap["candidate"].(string)
						log.Printf("收到player.html格式的ICE candidate")
					}
				} else if candidateStr, ok := data["candidate"].(string); ok {
					// index.html格式: {type: "ice_candidate", candidate: "..."}
					candidate = candidateStr
					log.Printf("收到index.html格式的ICE candidate")
				}

				if candidate != "" {
					// 发送ICE Candidate到服务B
					webrtcService.SendICECandidateToServiceB(clientID, candidate)
				}
			}
		}
	}
}
