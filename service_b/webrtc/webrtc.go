package webrtc

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/pion/webrtc/v3"
)

// Connection 表示与单个客户端的WebRTC连接
type Connection struct {
	taskID      uint
	clientID    string
	peerConn    *webrtc.PeerConnection
	dataChannel *webrtc.DataChannel
	candidates  []string
	mu          sync.Mutex
	isConnected bool
}

// Manager 管理WebRTC连接
type Manager struct {
	m3u8Dir     string
	connections map[string]*Connection
	mu          sync.RWMutex
}

// NewManager 创建新的WebRTC管理器
func NewManager(m3u8Dir string) *Manager {
	return &Manager{
		m3u8Dir:     m3u8Dir,
		connections: make(map[string]*Connection),
	}
}

// HandleOffer 处理客户端的WebRTC Offer
func (m *Manager) HandleOffer(wsConn interface {
	SendMessage(string, interface{}) error
}, taskID uint, clientID string, sdp string) {
	m.mu.Lock()
	conn, exists := m.connections[clientID]
	if exists {
		// 如果连接已存在，关闭旧连接
		if conn.peerConn != nil {
			conn.peerConn.Close()
		}
		delete(m.connections, clientID)
	}
	m.mu.Unlock()

	// 创建新连接
	conn = &Connection{
		taskID:   taskID,
		clientID: clientID,
	}

	// 保存新连接
	m.mu.Lock()
	m.connections[clientID] = conn
	m.mu.Unlock()

	// 配置WebRTC
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	}

	// 创建Peer Connection
	peerConn, err := webrtc.NewPeerConnection(config)
	if err != nil {
		log.Printf("创建PeerConnection失败: %v", err)
		return
	}
	conn.peerConn = peerConn

	// 创建数据通道
	dataChannel, err := peerConn.CreateDataChannel("m3u8", nil)
	if err != nil {
		log.Printf("创建DataChannel失败: %v", err)
		peerConn.Close()
		return
	}
	conn.dataChannel = dataChannel

	// 设置数据通道回调
	dataChannel.OnOpen(func() {
		conn.mu.Lock()
		conn.isConnected = true
		conn.mu.Unlock()
		log.Printf("与客户端 %s 的数据通道已打开", clientID)

		// 发送M3U8和ts文件
		go m.sendM3U8Files(conn)
	})

	dataChannel.OnMessage(func(msg webrtc.DataChannelMessage) {
		log.Printf("收到客户端 %s 的消息: %s", clientID, string(msg.Data))
	})

	dataChannel.OnClose(func() {
		log.Printf("与客户端 %s 的数据通道已关闭", clientID)
		conn.mu.Lock()
		conn.isConnected = false
		conn.mu.Unlock()
	})

	// 处理ICE候选
	peerConn.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate == nil {
			return
		}

		// 发送ICE候选给客户端
		candidateJSON := candidate.ToJSON()
		payload := map[string]interface{}{
			"client_id": clientID,
			"candidate": candidateJSON.Candidate,
			"is_client": false,
		}
		wsConn.SendMessage("ice_candidate", payload)
	})

	// 设置远程描述
	offer := webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  sdp,
	}

	if err := peerConn.SetRemoteDescription(offer); err != nil {
		log.Printf("设置远程描述失败: %v", err)
		peerConn.Close()
		return
	}

	// 添加之前保存的ICE候选
	conn.mu.Lock()
	for _, candidate := range conn.candidates {
		if err := conn.peerConn.AddICECandidate(webrtc.ICECandidateInit{Candidate: candidate}); err != nil {
			log.Printf("添加ICE候选失败: %v", err)
		}
	}
	conn.candidates = nil
	conn.mu.Unlock()

	// 创建应答
	answer, err := peerConn.CreateAnswer(nil)
	if err != nil {
		log.Printf("创建Answer失败: %v", err)
		peerConn.Close()
		return
	}

	// 设置本地描述
	if err := peerConn.SetLocalDescription(answer); err != nil {
		log.Printf("设置本地描述失败: %v", err)
		peerConn.Close()
		return
	}

	// 发送Answer给客户端
	payload := map[string]interface{}{
		"client_id": clientID,
		"sdp":       answer.SDP,
	}
	wsConn.SendMessage("webrtc_answer", payload)
}

// AddICECandidate 添加ICE候选
func (m *Manager) AddICECandidate(clientID string, candidate string) {
	m.mu.RLock()
	conn, exists := m.connections[clientID]
	m.mu.RUnlock()

	if !exists {
		return
	}

	conn.mu.Lock()
	defer conn.mu.Unlock()

	// 如果PeerConnection已创建，直接添加候选
	if conn.peerConn != nil {
		if err := conn.peerConn.AddICECandidate(webrtc.ICECandidateInit{Candidate: candidate}); err != nil {
			log.Printf("添加ICE候选失败: %v", err)
		}
	} else {
		// 否则保存候选等待PeerConnection创建后添加
		conn.candidates = append(conn.candidates, candidate)
	}
}

// sendM3U8Files 发送M3U8和TS文件
func (m *Manager) sendM3U8Files(conn *Connection) {
	// 获取M3U8文件路径
	taskDir := filepath.Join(m.m3u8Dir, fmt.Sprintf("task_%d", conn.taskID))
	m3u8Path := filepath.Join(taskDir, "index.m3u8")

	// 检查M3U8文件是否存在
	if _, err := os.Stat(m3u8Path); os.IsNotExist(err) {
		log.Printf("M3U8文件不存在: %s", m3u8Path)
		return
	}

	// 读取M3U8文件
	m3u8Data, err := ioutil.ReadFile(m3u8Path)
	if err != nil {
		log.Printf("读取M3U8文件失败: %v", err)
		return
	}

	// 发送M3U8文件内容
	conn.mu.Lock()
	if conn.isConnected && conn.dataChannel != nil {
		err = conn.dataChannel.SendText("m3u8:" + string(m3u8Data))
		if err != nil {
			log.Printf("发送M3U8文件失败: %v", err)
		}
	}
	conn.mu.Unlock()

	// 扫描目录中的TS文件
	files, err := ioutil.ReadDir(taskDir)
	if err != nil {
		log.Printf("读取TS文件目录失败: %v", err)
		return
	}

	// 发送每个TS文件
	for _, file := range files {
		if filepath.Ext(file.Name()) == ".ts" {
			tsPath := filepath.Join(taskDir, file.Name())
			tsData, err := ioutil.ReadFile(tsPath)
			if err != nil {
				log.Printf("读取TS文件失败: %v", err)
				continue
			}

			// 分块发送TS文件
			chunkSize := 16 * 1024 // 16KB chunks
			for i := 0; i < len(tsData); i += chunkSize {
				end := i + chunkSize
				if end > len(tsData) {
					end = len(tsData)
				}

				chunk := tsData[i:end]
				header := fmt.Sprintf("ts:%s:%d:%d", file.Name(), i, end-i)

				conn.mu.Lock()
				if !conn.isConnected || conn.dataChannel == nil {
					conn.mu.Unlock()
					return
				}

				// 发送TS文件块
				err = conn.dataChannel.Send(append([]byte(header+":"), chunk...))
				conn.mu.Unlock()

				if err != nil {
					log.Printf("发送TS文件块失败: %v", err)
					return
				}
			}
		}
	}
}

// Close 关闭所有WebRTC连接
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, conn := range m.connections {
		if conn.peerConn != nil {
			conn.peerConn.Close()
		}
	}
}
