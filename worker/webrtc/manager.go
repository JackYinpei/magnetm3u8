package webrtc

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/pion/webrtc/v3"
)

// Service 抽象WebRTC管理器行为，以便依赖注入。
type Service interface {
	Start() error
	Stop()
	HandleOffer(sessionID, sdp string) (string, error)
	AddICECandidate(sessionID, candidateStr string) error
	GetSession(sessionID string) (*Session, bool)
	GetAllSessions() []*Session
	SetICECandidateHandler(handler func(sessionID string, candidate *webrtc.ICECandidate))
	UpdateConfiguration(config webrtc.Configuration)
	SendData(sessionID string, data []byte) error
	BroadcastData(data []byte)
}

// Session WebRTC会话
type Session struct {
	ID        string                     `json:"id"`
	PeerConn  *webrtc.PeerConnection     `json:"-"`
	DataChan  *webrtc.DataChannel        `json:"-"`
	State     webrtc.PeerConnectionState `json:"state"`
	CreatedAt int64                      `json:"created_at"`
}

// Manager WebRTC管理器
type Manager struct {
	sessions            map[string]*Session
	mutex               sync.RWMutex
	config              webrtc.Configuration
	configMu            sync.RWMutex
	iceCandidateHandler func(sessionID string, candidate *webrtc.ICECandidate) // ICE候选者处理回调
}

// New 创建新的WebRTC管理器
func New() *Manager {
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	}

	return &Manager{
		sessions:            make(map[string]*Session),
		config:              config,
		iceCandidateHandler: nil,
	}
}

// Start 启动WebRTC管理器
func (m *Manager) Start() error {
	log.Printf("WebRTC manager started")
	return nil
}

// Stop 停止WebRTC管理器
func (m *Manager) Stop() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// 关闭所有会话
	for _, session := range m.sessions {
		if session.PeerConn != nil {
			session.PeerConn.Close()
		}
	}

	m.sessions = make(map[string]*Session)
	log.Printf("WebRTC manager stopped")
}

// HandleOffer 处理WebRTC Offer
func (m *Manager) HandleOffer(sessionID, sdp string) (string, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	log.Printf("Handling WebRTC offer for session: %s", sessionID)

	// 创建新的PeerConnection
	peerConn, err := webrtc.NewPeerConnection(m.getConfiguration())
	if err != nil {
		return "", fmt.Errorf("failed to create peer connection: %v", err)
	}

	// 创建会话
	session := &Session{
		ID:       sessionID,
		PeerConn: peerConn,
		State:    peerConn.ConnectionState(),
	}

	m.sessions[sessionID] = session

	// 设置连接状态变化回调
	peerConn.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		log.Printf("WebRTC connection state changed for session %s: %s", sessionID, state.String())
		session.State = state

		if state == webrtc.PeerConnectionStateFailed || state == webrtc.PeerConnectionStateClosed {
			m.removeSession(sessionID)
		}
	})

	// 设置ICE候选者回调
	peerConn.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate != nil {
			log.Printf("New ICE candidate for session %s: %s", sessionID, candidate.String())
			// 通过回调发送ICE候选者到客户端
			if m.iceCandidateHandler != nil {
				m.iceCandidateHandler(sessionID, candidate)
			}
		}
	})

	// 监听客户端创建的数据通道
	peerConn.OnDataChannel(func(dataChannel *webrtc.DataChannel) {
		if dataChannel.Label() == "filePathChannel" {
			log.Printf("Received data channel from client for session %s: %s", sessionID, dataChannel.Label())
			session.DataChan = dataChannel

			// 设置数据通道回调
			dataChannel.OnOpen(func() {
				log.Printf("Data channel opened for session: %s", sessionID)
			})

			dataChannel.OnMessage(func(msg webrtc.DataChannelMessage) {
				log.Printf("Received message on data channel for session %s: %s", sessionID, string(msg.Data))
				// 处理文件请求消息
				go m.handleFileRequest(sessionID, msg.Data)
			})

			dataChannel.OnClose(func() {
				log.Printf("Data channel closed for session: %s", sessionID)
			})
		}
	})

	// 解析并设置远程描述
	offer := webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  sdp,
	}

	if err := peerConn.SetRemoteDescription(offer); err != nil {
		peerConn.Close()
		delete(m.sessions, sessionID)
		return "", fmt.Errorf("failed to set remote description: %v", err)
	}

	// 创建应答
	answer, err := peerConn.CreateAnswer(nil)
	if err != nil {
		peerConn.Close()
		delete(m.sessions, sessionID)
		return "", fmt.Errorf("failed to create answer: %v", err)
	}

	// 设置本地描述
	if err := peerConn.SetLocalDescription(answer); err != nil {
		peerConn.Close()
		delete(m.sessions, sessionID)
		return "", fmt.Errorf("failed to set local description: %v", err)
	}

	log.Printf("Created WebRTC answer for session: %s", sessionID)
	return answer.SDP, nil
}

// AddICECandidate 添加ICE候选者
func (m *Manager) AddICECandidate(sessionID, candidateStr string) error {
	m.mutex.RLock()
	session, exists := m.sessions[sessionID]
	m.mutex.RUnlock()

	if !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	// 尝试直接使用候选者字符串（浏览器可能直接发送候选者字符串）
	candidate := webrtc.ICECandidateInit{
		Candidate: candidateStr,
	}

	// 如果候选者字符串是JSON格式，则解析它
	if strings.HasPrefix(candidateStr, "{") {
		var candidateData map[string]interface{}
		if err := json.Unmarshal([]byte(candidateStr), &candidateData); err == nil {
			if cand, ok := candidateData["candidate"].(string); ok {
				candidate.Candidate = cand
			}

			if sdpMid, ok := candidateData["sdpMid"]; ok {
				if mid, ok := sdpMid.(string); ok {
					candidate.SDPMid = &mid
				}
			}

			if sdpMLineIndex, ok := candidateData["sdpMLineIndex"]; ok {
				if index, ok := sdpMLineIndex.(float64); ok {
					idx := uint16(index)
					candidate.SDPMLineIndex = &idx
				}
			}
		}
	}

	// 添加ICE候选者
	if err := session.PeerConn.AddICECandidate(candidate); err != nil {
		return fmt.Errorf("failed to add ICE candidate: %v", err)
	}

	log.Printf("Added ICE candidate for session %s", sessionID)
	return nil
}

// GetSession 获取会话
func (m *Manager) GetSession(sessionID string) (*Session, bool) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	session, exists := m.sessions[sessionID]
	return session, exists
}

// GetAllSessions 获取所有会话
func (m *Manager) GetAllSessions() []*Session {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	sessions := make([]*Session, 0, len(m.sessions))
	for _, session := range m.sessions {
		sessions = append(sessions, session)
	}
	return sessions
}

// removeSession 移除会话（内部方法）
func (m *Manager) removeSession(sessionID string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if session, exists := m.sessions[sessionID]; exists {
		if session.PeerConn != nil {
			session.PeerConn.Close()
		}
		delete(m.sessions, sessionID)
		log.Printf("Removed WebRTC session: %s", sessionID)
	}
}

// SendData 通过数据通道发送数据
func (m *Manager) SendData(sessionID string, data []byte) error {
	m.mutex.RLock()
	session, exists := m.sessions[sessionID]
	m.mutex.RUnlock()

	if !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	if session.DataChan == nil {
		return fmt.Errorf("data channel not available for session: %s", sessionID)
	}

	if session.DataChan.ReadyState() != webrtc.DataChannelStateOpen {
		return fmt.Errorf("data channel not open for session: %s", sessionID)
	}

	return session.DataChan.Send(data)
}

// SetICECandidateHandler 设置ICE候选者处理回调
func (m *Manager) SetICECandidateHandler(handler func(sessionID string, candidate *webrtc.ICECandidate)) {
	m.iceCandidateHandler = handler
}

// BroadcastData 向所有会话广播数据
func (m *Manager) BroadcastData(data []byte) {
	m.mutex.RLock()
	sessions := make([]*Session, 0, len(m.sessions))
	for _, session := range m.sessions {
		sessions = append(sessions, session)
	}
	m.mutex.RUnlock()

	for _, session := range sessions {
		if err := m.SendData(session.ID, data); err != nil {
			log.Printf("Failed to send data to session %s: %v", session.ID, err)
		}
	}
}

// UpdateConfiguration 更新WebRTC配置
func (m *Manager) UpdateConfiguration(config webrtc.Configuration) {
	m.configMu.Lock()
	defer m.configMu.Unlock()

	m.config = config
}

// 获取当前配置（内部使用）
func (m *Manager) getConfiguration() webrtc.Configuration {
	m.configMu.RLock()
	defer m.configMu.RUnlock()

	return m.config
}

// FileRequest 文件请求结构
type FileRequest struct {
	Type string `json:"type"`
	TS   string `json:"ts"`
	ID   string `json:"id"`
}

// FileResponse 文件响应结构
type FileResponse struct {
	Type          string `json:"type"`
	ID            string `json:"id"`
	SliceNum      int    `json:"sliceNum"`
	TotalSliceNum int    `json:"totalSliceNum"`
	TotalLength   int    `json:"totalLength"`
	Payload       string `json:"payload"`
}

const (
	ServerChunkSize = 16 * 1024 // 16KB chunks
)

// handleFileRequest 处理文件请求
func (m *Manager) handleFileRequest(sessionID string, data []byte) {
	var request FileRequest
	if err := json.Unmarshal(data, &request); err != nil {
		log.Printf("Failed to parse file request: %v", err)
		return
	}

	log.Printf("Processing file request for session %s: type=%s, ts=%s, id=%s",
		sessionID, request.Type, request.TS, request.ID)

	if request.Type != "hijackReq" {
		log.Printf("Unknown request type: %s", request.Type)
		return
	}

	// 解析URL并提取路径
	var filePath string
	if u, err := url.Parse(request.TS); err == nil && u.Scheme != "" && u.Host != "" {
		filePath = u.Path
	} else {
		filePath = request.TS
	}

	// 处理文件路径，移除前缀
	filePath = strings.TrimPrefix(filePath, "/video/")

	// 解析任务ID和文件名
	parts := strings.Split(filePath, "/")
	if len(parts) < 2 {
		log.Printf("Invalid file path format: %s", filePath)
		m.sendFileError(sessionID, request.ID, "Invalid file path")
		return
	}

	taskID := parts[0]
	fileName := parts[1]

	log.Printf("Parsed request: taskID=%s, fileName=%s", taskID, fileName)

	// 构建实际文件路径 - 先尝试直接匹配taskID目��
	var actualPath string
	var found bool

	// 方法1：尝试直接匹配taskID目录
	if strings.HasSuffix(fileName, ".m3u8") {
		actualPath = filepath.Join("data", "m3u8", taskID, fileName)
	} else if strings.HasSuffix(fileName, ".ts") || strings.HasSuffix(fileName, ".vtt") {
		actualPath = filepath.Join("data", "m3u8", taskID, fileName)
	}

	// 检查文件是否存在
	if _, err := os.Stat(actualPath); err == nil {
		found = true
	} else {
		// 方法2：如果直接匹配失败，搜索m3u8目录下的所有子目录
		m3u8BaseDir := "data/m3u8"
		entries, err := os.ReadDir(m3u8BaseDir)
		if err != nil {
			log.Printf("Failed to read m3u8 directory: %v", err)
			m.sendFileError(sessionID, request.ID, "M3U8 directory not accessible")
			return
		}

		// 遍历所有目录，寻找包含目标文件的目录
		for _, entry := range entries {
			if entry.IsDir() {
				testPath := filepath.Join(m3u8BaseDir, entry.Name(), fileName)
				if _, err := os.Stat(testPath); err == nil {
					actualPath = testPath
					found = true
					log.Printf("Found file in directory: %s -> %s", entry.Name(), actualPath)
					break
				}
			}
		}
	}

	if !found {
		log.Printf("File not found after searching: taskID=%s, fileName=%s", taskID, fileName)
		m.sendFileError(sessionID, request.ID, "File not found")
		return
	}

	// 读取文件内容
	fileData, err := os.ReadFile(actualPath)
	if err != nil {
		log.Printf("Failed to read file %s: %v", actualPath, err)
		m.sendFileError(sessionID, request.ID, "Failed to read file")
		return
	}

	// 发送文件数据
	if err := m.sendFileData(sessionID, request.ID, fileData, fileName); err != nil {
		log.Printf("Failed to send file data: %v", err)
	} else {
		log.Printf("Successfully sent file %s to session %s", actualPath, sessionID)
	}
}

// sendFileData 发送文件数据
func (m *Manager) sendFileData(sessionID, requestID string, data []byte, fileName string) error {
	totalLength := len(data)
	totalSlices := (totalLength + ServerChunkSize - 1) / ServerChunkSize

	log.Printf("Sending file data: size=%d bytes, slices=%d", totalLength, totalSlices)

	// 确定响应类型
	responseType := "hijackRespData"
	if strings.HasSuffix(fileName, ".m3u8") || strings.HasSuffix(fileName, ".vtt") {
		responseType = "hijackRespText"
	}

	// 分片发送
	for i := 0; i < totalSlices; i++ {
		start := i * ServerChunkSize
		end := start + ServerChunkSize
		if end > totalLength {
			end = totalLength
		}

		chunk := data[start:end]

		// Base64编码
		payload := base64.StdEncoding.EncodeToString(chunk)

		response := FileResponse{
			Type:          responseType,
			ID:            requestID,
			SliceNum:      i,
			TotalSliceNum: totalSlices,
			TotalLength:   totalLength,
			Payload:       payload,
		}

		responseData, err := json.Marshal(response)
		if err != nil {
			return fmt.Errorf("failed to marshal response: %v", err)
		}

		if err := m.SendData(sessionID, responseData); err != nil {
			return fmt.Errorf("failed to send chunk %d: %v", i, err)
		}

		log.Printf("Sent chunk %d/%d for request %s", i+1, totalSlices, requestID)
	}

	return nil
}

// sendFileError 发送文件错误响应
func (m *Manager) sendFileError(sessionID, requestID, errorMsg string) {
	errorResponse := map[string]interface{}{
		"type":  "hijackError",
		"id":    requestID,
		"error": errorMsg,
	}

	responseData, err := json.Marshal(errorResponse)
	if err != nil {
		log.Printf("Failed to marshal error response: %v", err)
		return
	}

	if err := m.SendData(sessionID, responseData); err != nil {
		log.Printf("Failed to send error response: %v", err)
	}
}

var _ Service = (*Manager)(nil)
