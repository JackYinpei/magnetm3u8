package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// WebSocketConnection 管理与服务A的WebSocket连接
type WebSocketConnection struct {
	url            string
	conn           *websocket.Conn
	isConnected    bool
	mu             sync.RWMutex
	messageHandler func(msgType string, payload map[string]interface{})
	closeCh        chan struct{}
	doneCh         chan struct{}
}

// NewWebSocketConnection 创建新的WebSocket连接
func NewWebSocketConnection(serverURL string) (*WebSocketConnection, error) {
	u, err := url.Parse(serverURL)
	if err != nil {
		return nil, err
	}

	conn := &WebSocketConnection{
		url:         serverURL,
		isConnected: false,
		closeCh:     make(chan struct{}),
		doneCh:      make(chan struct{}),
	}

	// 连接到WebSocket服务器
	if err := conn.connect(u); err != nil {
		return nil, err
	}

	// 开始读取消息
	go conn.readMessages()

	return conn, nil
}

// connect 连接到WebSocket服务器
func (wc *WebSocketConnection) connect(u *url.URL) error {
	log.Printf("正在连接到服务A: %s", u.String())

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	c, _, err := dialer.Dial(u.String(), nil)
	if err != nil {
		return err
	}

	wc.mu.Lock()
	wc.conn = c
	wc.isConnected = true
	wc.mu.Unlock()

	// 设置Ping处理
	wc.conn.SetPingHandler(func(data string) error {
		return wc.conn.WriteMessage(websocket.PongMessage, []byte{})
	})

	return nil
}

// SetMessageHandler 设置消息处理函数
func (wc *WebSocketConnection) SetMessageHandler(handler func(msgType string, payload map[string]interface{})) {
	wc.mu.Lock()
	defer wc.mu.Unlock()
	wc.messageHandler = handler
}

// readMessages 读取来自服务A的消息
func (wc *WebSocketConnection) readMessages() {
	defer close(wc.doneCh)

	for {
		select {
		case <-wc.closeCh:
			return
		default:
			_, message, err := wc.conn.ReadMessage()
			if err != nil {
				log.Printf("读取WebSocket消息错误: %v", err)
				wc.handleDisconnect()
				return
			}

			// 解析消息
			var wsMessage struct {
				Type    string                 `json:"type"`
				Payload map[string]interface{} `json:"payload"`
			}
			if err := json.Unmarshal(message, &wsMessage); err != nil {
				log.Printf("解析WebSocket消息错误: %v", err)
				continue
			}

			// 处理消息
			wc.mu.RLock()
			handler := wc.messageHandler
			wc.mu.RUnlock()

			if handler != nil {
				handler(wsMessage.Type, wsMessage.Payload)
			}
		}
	}
}

// SendMessage 向服务A发送消息
func (wc *WebSocketConnection) SendMessage(messageType string, payload interface{}) error {
	wc.mu.RLock()
	defer wc.mu.RUnlock()

	if !wc.isConnected || wc.conn == nil {
		return errors.New("未连接到服务A")
	}

	message := struct {
		Type    string      `json:"type"`
		Payload interface{} `json:"payload"`
	}{
		Type:    messageType,
		Payload: payload,
	}

	return wc.conn.WriteJSON(message)
}

// Close 关闭连接
func (wc *WebSocketConnection) Close() {
	wc.mu.Lock()
	if wc.isConnected {
		close(wc.closeCh)
		wc.isConnected = false
		if wc.conn != nil {
			wc.conn.Close()
			wc.conn = nil
		}
	}
	wc.mu.Unlock()
}

// Wait 等待连接关闭
func (wc *WebSocketConnection) Wait() {
	<-wc.doneCh
}

// handleDisconnect 处理断开连接
func (wc *WebSocketConnection) handleDisconnect() {
	wc.mu.Lock()
	defer wc.mu.Unlock()

	if wc.isConnected {
		wc.isConnected = false
		if wc.conn != nil {
			wc.conn.Close()
			wc.conn = nil
		}
	}
}
