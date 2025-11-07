package cluster

import (
	"sync"
	"time"
)

// WorkerNode represents a worker that can register with the gateway.
type WorkerNode struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Address      string            `json:"address"`
	Status       string            `json:"status"`
	LastSeen     time.Time         `json:"last_seen"`
	Capabilities []string          `json:"capabilities"`
	Resources    map[string]int    `json:"resources"`
	Metadata     map[string]string `json:"metadata"`
}

// SignalingSession captures metadata for active WebRTC sessions.
type SignalingSession struct {
	SessionID string    `json:"session_id"`
	ClientID  string    `json:"client_id"`
	WorkerID  string    `json:"worker_id"`
	CreatedAt time.Time `json:"created_at"`
	Status    string    `json:"status"`
}

// Manager orchestrates registered worker nodes and WebRTC sessions.
type Manager struct {
	nodes    map[string]*WorkerNode
	sessions map[string]*SignalingSession
	mutex    sync.RWMutex
}

// NewManager constructs a Manager and starts background cleanup tasks.
func NewManager() *Manager {
	m := &Manager{
		nodes:    make(map[string]*WorkerNode),
		sessions: make(map[string]*SignalingSession),
	}

	go m.startCleanupTask()

	return m
}

// RegisterNode stores or updates a worker record.
func (m *Manager) RegisterNode(node *WorkerNode) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	node.LastSeen = time.Now()
	node.Status = "online"
	m.nodes[node.ID] = node
}

// UpdateNodeHeartbeat refreshes the LastSeen timestamp of a worker.
func (m *Manager) UpdateNodeHeartbeat(nodeID string) bool {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if node, exists := m.nodes[nodeID]; exists {
		node.LastSeen = time.Now()
		node.Status = "online"
		return true
	}
	return false
}

// GetOnlineNodes returns all nodes whose status is "online".
func (m *Manager) GetOnlineNodes() []*WorkerNode {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	var online []*WorkerNode
	for _, node := range m.nodes {
		if node.Status == "online" {
			online = append(online, node)
		}
	}
	return online
}

// GetNode fetches a single worker by ID.
func (m *Manager) GetNode(nodeID string) (*WorkerNode, bool) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	node, exists := m.nodes[nodeID]
	return node, exists
}

// RemoveNode deletes a worker.
func (m *Manager) RemoveNode(nodeID string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	delete(m.nodes, nodeID)
}

// CreateSignalingSession registers a WebRTC signaling session.
func (m *Manager) CreateSignalingSession(sessionID, clientID, workerID string) *SignalingSession {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	session := &SignalingSession{
		SessionID: sessionID,
		ClientID:  clientID,
		WorkerID:  workerID,
		CreatedAt: time.Now(),
		Status:    "negotiating",
	}

	m.sessions[sessionID] = session
	return session
}

// CreateWebRTCSession is an alias for CreateSignalingSession.
func (m *Manager) CreateWebRTCSession(sessionID, clientID, workerID string) *SignalingSession {
	return m.CreateSignalingSession(sessionID, clientID, workerID)
}

// GetSignalingSession returns a signaling session by ID.
func (m *Manager) GetSignalingSession(sessionID string) (*SignalingSession, bool) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	session, exists := m.sessions[sessionID]
	return session, exists
}

// GetWebRTCSession is an alias for GetSignalingSession.
func (m *Manager) GetWebRTCSession(sessionID string) (*SignalingSession, bool) {
	return m.GetSignalingSession(sessionID)
}

// UpdateSessionStatus sets the status of a session if it exists.
func (m *Manager) UpdateSessionStatus(sessionID, status string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if session, exists := m.sessions[sessionID]; exists {
		session.Status = status
	}
}

// RemoveSignalingSession deletes a signaling session by ID.
func (m *Manager) RemoveSignalingSession(sessionID string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	delete(m.sessions, sessionID)
}

// Stats returns counts for total nodes, currently online nodes, and active sessions.
func (m *Manager) Stats() (totalNodes int, onlineNodes int, activeSessions int) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	totalNodes = len(m.nodes)
	activeSessions = len(m.sessions)
	for _, node := range m.nodes {
		if node.Status == "online" {
			onlineNodes++
		}
	}
	return
}

func (m *Manager) startCleanupTask() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		m.cleanupOfflineNodes()
		m.cleanupExpiredSessions()
	}
}

func (m *Manager) cleanupOfflineNodes() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	now := time.Now()
	for nodeID, node := range m.nodes {
		if now.Sub(node.LastSeen) > 2*time.Minute {
			if node.Status != "offline" {
				node.Status = "offline"
			}
			if now.Sub(node.LastSeen) > 10*time.Minute {
				delete(m.nodes, nodeID)
			}
		}
	}
}

func (m *Manager) cleanupExpiredSessions() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	now := time.Now()
	for sessionID, session := range m.sessions {
		if now.Sub(session.CreatedAt) > time.Hour {
			delete(m.sessions, sessionID)
		}
	}
}
