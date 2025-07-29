package client

import (
	"fmt"
	"log"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// NodeInfo 节点信息
type NodeInfo struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Address      string            `json:"address"`
	Status       string            `json:"status"`
	Capabilities []string          `json:"capabilities"`
	Resources    map[string]int    `json:"resources"`
	Metadata     map[string]string `json:"metadata"`
}

// Message 消息结构
type Message struct {
	Type    string                 `json:"type"`
	Payload map[string]interface{} `json:"payload"`
}

// MessageHandler 消息处理器类型
type MessageHandler func(msgType string, payload map[string]interface{})

// GatewayClient 网关客户端
type GatewayClient struct {
	gatewayURL     string
	nodeID         string
	conn           *websocket.Conn
	messageHandler MessageHandler
	reconnectDelay time.Duration
	connected      bool
	mutex          sync.RWMutex
	stopChan       chan struct{}
}

// New 创建新的网关客户端
func New(gatewayURL, nodeID string) *GatewayClient {
	return &GatewayClient{
		gatewayURL:     gatewayURL,
		nodeID:         nodeID,
		reconnectDelay: 5 * time.Second,
		stopChan:       make(chan struct{}),
	}
}

// SetMessageHandler 设置消息处理器
func (gc *GatewayClient) SetMessageHandler(handler MessageHandler) {
	gc.messageHandler = handler
}

// Connect 连接到网关
func (gc *GatewayClient) Connect(nodeInfo NodeInfo) error {
	u, err := url.Parse(gc.gatewayURL)
	if err != nil {
		return err
	}

	log.Printf("Connecting to gateway: %s", gc.gatewayURL)

	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return err
	}

	gc.mutex.Lock()
	gc.conn = conn
	gc.connected = true
	gc.mutex.Unlock()

	// 发送节点注册信息
	if err := gc.conn.WriteJSON(nodeInfo); err != nil {
		gc.conn.Close()
		return err
	}

	// 启动消息接收循环
	go gc.readLoop()
	
	// 启动重连监控
	go gc.reconnectLoop(nodeInfo)

	log.Printf("Connected to gateway successfully")
	return nil
}

// Disconnect 断开连接
func (gc *GatewayClient) Disconnect() {
	close(gc.stopChan)
	
	gc.mutex.Lock()
	if gc.conn != nil {
		gc.conn.Close()
		gc.conn = nil
	}
	gc.connected = false
	gc.mutex.Unlock()
	
	log.Printf("Disconnected from gateway")
}

// IsConnected 检查连接状态
func (gc *GatewayClient) IsConnected() bool {
	gc.mutex.RLock()
	defer gc.mutex.RUnlock()
	return gc.connected
}

// SendMessage 发送消息到网关
func (gc *GatewayClient) SendMessage(msgType string, payload map[string]interface{}) error {
	gc.mutex.RLock()
	conn := gc.conn
	connected := gc.connected
	gc.mutex.RUnlock()

	if !connected || conn == nil {
		return ErrNotConnected
	}

	message := Message{
		Type:    msgType,
		Payload: payload,
	}

	return conn.WriteJSON(message)
}

// SendHeartbeat 发送心跳
func (gc *GatewayClient) SendHeartbeat() error {
	return gc.SendMessage("heartbeat", map[string]interface{}{
		"timestamp": time.Now().Unix(),
		"node_id":   gc.nodeID,
	})
}

// SendTaskStatus 发送任务状态更新
func (gc *GatewayClient) SendTaskStatus(taskID, status string, progress int, metadata map[string]interface{}) error {
	payload := map[string]interface{}{
		"task_id":  taskID,
		"status":   status,
		"progress": progress,
		"timestamp": time.Now().Unix(),
	}

	if metadata != nil {
		for k, v := range metadata {
			payload[k] = v
		}
	}

	return gc.SendMessage("task_status", payload)
}

// SendWebRTCAnswer 发送WebRTC Answer
func (gc *GatewayClient) SendWebRTCAnswer(sessionID, sdp string) error {
	return gc.SendMessage("webrtc_answer", map[string]interface{}{
		"session_id": sessionID,
		"sdp":        sdp,
	})
}

// SendICECandidate 发送ICE候选者
func (gc *GatewayClient) SendICECandidate(sessionID, candidate string) error {
	return gc.SendMessage("ice_candidate", map[string]interface{}{
		"session_id": sessionID,
		"candidate":  candidate,
	})
}

// readLoop 消息接收循环
func (gc *GatewayClient) readLoop() {
	defer func() {
		gc.mutex.Lock()
		gc.connected = false
		if gc.conn != nil {
			gc.conn.Close()
			gc.conn = nil
		}
		gc.mutex.Unlock()
	}()

	for {
		select {
		case <-gc.stopChan:
			return
		default:
		}

		gc.mutex.RLock()
		conn := gc.conn
		gc.mutex.RUnlock()

		if conn == nil {
			return
		}

		var message Message
		err := conn.ReadJSON(&message)
		if err != nil {
			log.Printf("Failed to read message from gateway: %v", err)
			return
		}

		// 处理接收到的消息
		if gc.messageHandler != nil {
			go gc.messageHandler(message.Type, message.Payload)
		}
	}
}

// reconnectLoop 重连循环
func (gc *GatewayClient) reconnectLoop(nodeInfo NodeInfo) {
	ticker := time.NewTicker(gc.reconnectDelay)
	defer ticker.Stop()

	for {
		select {
		case <-gc.stopChan:
			return
		case <-ticker.C:
			if !gc.IsConnected() {
				log.Printf("Attempting to reconnect to gateway...")
				if err := gc.Connect(nodeInfo); err != nil {
					log.Printf("Reconnection failed: %v", err)
				} else {
					log.Printf("Reconnected to gateway successfully")
				}
			}
		}
	}
}

// 错误定义
var (
	ErrNotConnected = fmt.Errorf("not connected to gateway")
)