package services

import (
	"log"
	"sync"
	"time"

	"magnetm3u8/db"
	"magnetm3u8/models"
)

// WebRTCService 处理WebRTC相关操作
type WebRTCService struct {
	// 会话映射表
	sessions     map[string]*models.WebRTCSession
	sessionMutex sync.RWMutex
}

// NewWebRTCService 创建新的WebRTCService
func NewWebRTCService() *WebRTCService {
	return &WebRTCService{
		sessions: make(map[string]*models.WebRTCSession),
	}
}

// 创建新的WebRTC会话
func (s *WebRTCService) CreateSession(taskID uint, clientID string) (*models.WebRTCSession, error) {
	// 创建会话记录
	session := &models.WebRTCSession{
		TaskID:    taskID,
		ClientID:  clientID,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// 保存到数据库
	if err := db.DB.Create(session).Error; err != nil {
		log.Printf("创建WebRTC会话失败: %v", err)
		return nil, err
	}

	// 添加到会话映射表
	s.sessionMutex.Lock()
	s.sessions[clientID] = session
	s.sessionMutex.Unlock()

	return session, nil
}

// 发送WebRTC Offer到服务B
func (s *WebRTCService) SendOffer(clientID string, taskID uint, offerSDP string) error {
	wsManager := GetWebSocketManager()
	if !wsManager.IsConnected() {
		return ErrNotConnected
	}

	return wsManager.SendMessage(MsgTypeWebRTCOffer, map[string]interface{}{
		"client_id": clientID,
		"task_id":   taskID,
		"sdp":       offerSDP,
	})
}

// 发送WebRTC Answer到客户端
func (s *WebRTCService) SendAnswer(clientID string, answerSDP string) error {
	// 通过API包中的SendMessageToClient函数发送Answer到客户端
	// 这里需要避免循环导入，所以我们先定义一个回调函数
	if sendToClientFunc != nil {
		log.Printf("向客户端 %s 发送 WebRTC Answer", clientID)
		return sendToClientFunc(clientID, MsgTypeWebRTCAnswer, map[string]interface{}{
			"sdp": answerSDP,
		})
	}
	return nil
}

// 发送ICE Candidate到服务B
func (s *WebRTCService) SendICECandidateToServiceB(clientID string, candidate string) error {
	wsManager := GetWebSocketManager()
	if !wsManager.IsConnected() {
		return ErrNotConnected
	}

	return wsManager.SendMessage(MsgTypeICECandidate, map[string]interface{}{
		"client_id": clientID,
		"candidate": candidate,
		"is_client": true,
	})
}

// 发送ICE Candidate到客户端
func (s *WebRTCService) SendICECandidateToClient(clientID string, candidate string) error {
	// 通过API包中的SendMessageToClient函数发送ICE Candidate到客户端
	if sendToClientFunc != nil {
		log.Printf("向客户端 %s 发送 ICE Candidate", clientID)
		return sendToClientFunc(clientID, MsgTypeICECandidate, map[string]interface{}{
			"candidate": candidate,
		})
	}
	return nil
}

// 根据客户端ID获取会话
func (s *WebRTCService) GetSessionByClientID(clientID string) *models.WebRTCSession {
	s.sessionMutex.RLock()
	defer s.sessionMutex.RUnlock()
	return s.sessions[clientID]
}

// 删除会话
func (s *WebRTCService) RemoveSession(clientID string) error {
	s.sessionMutex.Lock()
	defer s.sessionMutex.Unlock()

	session, exists := s.sessions[clientID]
	if !exists {
		return nil
	}

	// 从数据库中删除
	if err := db.DB.Delete(session).Error; err != nil {
		return err
	}

	// 从映射表中删除
	delete(s.sessions, clientID)
	return nil
}

// 清理过期会话
func (s *WebRTCService) CleanupExpiredSessions() {
	threshold := time.Now().Add(-24 * time.Hour) // 24小时前的会话视为过期

	var expiredSessions []models.WebRTCSession
	if err := db.DB.Where("updated_at < ?", threshold).Find(&expiredSessions).Error; err != nil {
		log.Printf("查询过期会话失败: %v", err)
		return
	}

	s.sessionMutex.Lock()
	defer s.sessionMutex.Unlock()

	for _, session := range expiredSessions {
		// 从映射表中删除
		delete(s.sessions, session.ClientID)
	}

	// 从数据库中批量删除
	if len(expiredSessions) > 0 {
		db.DB.Where("updated_at < ?", threshold).Delete(&models.WebRTCSession{})
	}
}

// 启动定期清理过期会话的定时器
func (s *WebRTCService) StartSessionCleanup() {
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()

		for range ticker.C {
			s.CleanupExpiredSessions()
		}
	}()
}

// 定义发送消息到客户端的回调函数类型
type SendToClientFunc func(clientID string, messageType string, payload interface{}) error

// 全局变量，用于存储发送消息到客户端的回调函数
var sendToClientFunc SendToClientFunc

// SetSendToClientFunc 设置发送消息到客户端的回调函数
func SetSendToClientFunc(fn SendToClientFunc) {
	sendToClientFunc = fn
}
