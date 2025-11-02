package main

import (
	"sync"
	"time"
)

// WorkerNode 表示一个工作节点
type WorkerNode struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Address      string            `json:"address"`
	Status       string            `json:"status"` // online, offline, busy
	LastSeen     time.Time         `json:"last_seen"`
	Capabilities []string          `json:"capabilities"`
	Resources    map[string]int    `json:"resources"` // 可用资源统计
	Metadata     map[string]string `json:"metadata"`  // 额外信息
}

// SignalingSession 表示信令会话（用于WebRTC信令中继）
type SignalingSession struct {
	SessionID string    `json:"session_id"`
	ClientID  string    `json:"client_id"`
	WorkerID  string    `json:"worker_id"`
	CreatedAt time.Time `json:"created_at"`
	Status    string    `json:"status"` // negotiating, established, closed
}

// GatewayManager 网关管理器
type GatewayManager struct {
	nodes    map[string]*WorkerNode      // 工作节点注册表
	sessions map[string]*SignalingSession // 信令会话表
	mutex    sync.RWMutex                // 并发控制
}

// NewGatewayManager 创建新的网关管理器
func NewGatewayManager() *GatewayManager {
	manager := &GatewayManager{
		nodes:    make(map[string]*WorkerNode),
		sessions: make(map[string]*SignalingSession),
	}

	// 启动清理任务
	go manager.startCleanupTask()

	return manager
}

// RegisterNode 注册工作节点
func (gm *GatewayManager) RegisterNode(node *WorkerNode) {
	gm.mutex.Lock()
	defer gm.mutex.Unlock()

	node.LastSeen = time.Now()
	node.Status = "online"
	gm.nodes[node.ID] = node
}

// UpdateNodeHeartbeat 更新节点心跳
func (gm *GatewayManager) UpdateNodeHeartbeat(nodeID string) bool {
	gm.mutex.Lock()
	defer gm.mutex.Unlock()

	if node, exists := gm.nodes[nodeID]; exists {
		node.LastSeen = time.Now()
		node.Status = "online"
		return true
	}
	return false
}

// GetOnlineNodes 获取在线节点列表
func (gm *GatewayManager) GetOnlineNodes() []*WorkerNode {
	gm.mutex.RLock()
	defer gm.mutex.RUnlock()

	var onlineNodes []*WorkerNode
	for _, node := range gm.nodes {
		if node.Status == "online" {
			onlineNodes = append(onlineNodes, node)
		}
	}
	return onlineNodes
}

// GetNode 获取指定节点
func (gm *GatewayManager) GetNode(nodeID string) (*WorkerNode, bool) {
	gm.mutex.RLock()
	defer gm.mutex.RUnlock()

	node, exists := gm.nodes[nodeID]
	return node, exists
}

// RemoveNode 移除节点
func (gm *GatewayManager) RemoveNode(nodeID string) {
	gm.mutex.Lock()
	defer gm.mutex.Unlock()

	delete(gm.nodes, nodeID)
}

// CreateSignalingSession 创建信令会话
func (gm *GatewayManager) CreateSignalingSession(sessionID, clientID, workerID string) *SignalingSession {
	gm.mutex.Lock()
	defer gm.mutex.Unlock()

	session := &SignalingSession{
		SessionID: sessionID,
		ClientID:  clientID,
		WorkerID:  workerID,
		CreatedAt: time.Now(),
		Status:    "negotiating",
	}

	gm.sessions[sessionID] = session
	return session
}

// GetSignalingSession 获取信令会话
func (gm *GatewayManager) GetSignalingSession(sessionID string) (*SignalingSession, bool) {
	gm.mutex.RLock()
	defer gm.mutex.RUnlock()

	session, exists := gm.sessions[sessionID]
	return session, exists
}

// UpdateSessionStatus 更新会话状态
func (gm *GatewayManager) UpdateSessionStatus(sessionID, status string) {
	gm.mutex.Lock()
	defer gm.mutex.Unlock()

	if session, exists := gm.sessions[sessionID]; exists {
		session.Status = status
	}
}

// RemoveSignalingSession 移除信令会话
func (gm *GatewayManager) RemoveSignalingSession(sessionID string) {
	gm.mutex.Lock()
	defer gm.mutex.Unlock()

	delete(gm.sessions, sessionID)
}

// CreateWebRTCSession 创建WebRTC会话 (别名方法，与SignalingSession相同)
func (gm *GatewayManager) CreateWebRTCSession(sessionID, clientID, workerID string) *SignalingSession {
	return gm.CreateSignalingSession(sessionID, clientID, workerID)
}

// GetWebRTCSession 获取WebRTC会话 (别名方法，与SignalingSession相同)
func (gm *GatewayManager) GetWebRTCSession(sessionID string) (*SignalingSession, bool) {
	return gm.GetSignalingSession(sessionID)
}

// startCleanupTask 启动清理任务
func (gm *GatewayManager) startCleanupTask() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		gm.cleanupOfflineNodes()
		gm.cleanupExpiredSessions()
	}
}

// cleanupOfflineNodes 清理离线节点
func (gm *GatewayManager) cleanupOfflineNodes() {
	gm.mutex.Lock()
	defer gm.mutex.Unlock()

	now := time.Now()
	for nodeID, node := range gm.nodes {
		// 如果节点超过2分钟没有心跳，标记为离线
		if now.Sub(node.LastSeen) > 2*time.Minute {
			if node.Status != "offline" {
				node.Status = "offline"
			}
			// 如果离线超过10分钟，从注册表移除
			if now.Sub(node.LastSeen) > 10*time.Minute {
				delete(gm.nodes, nodeID)
			}
		}
	}
}

// cleanupExpiredSessions 清理过期会话
func (gm *GatewayManager) cleanupExpiredSessions() {
	gm.mutex.Lock()
	defer gm.mutex.Unlock()

	now := time.Now()
	for sessionID, session := range gm.sessions {
		// 如果会话超过1小时，自动清理
		if now.Sub(session.CreatedAt) > time.Hour {
			delete(gm.sessions, sessionID)
		}
	}
}