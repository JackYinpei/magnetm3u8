package webrtc

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"log"
	"magnetm3u8_service_b/utils"
	"os"
	"strings"
	"sync"

	"github.com/pion/webrtc/v3"
)

const chunkSize = 16 * 1024

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

	// 监听前端创建的数据通道
	peerConn.OnDataChannel(func(dataChannel *webrtc.DataChannel) {
		if dataChannel.Label() == "filePathChannel" {
			conn.dataChannel = dataChannel

			// 设置数据通道回调
			dataChannel.OnOpen(func() {
				conn.mu.Lock()
				conn.isConnected = true
				conn.mu.Unlock()
				log.Printf("与客户端 %s 的数据通道已打开", clientID)
			})

			dataChannel.OnMessage(func(msg webrtc.DataChannelMessage) {
				log.Printf("收到客户端 %s 的消息: %s", clientID, string(msg.Data))
				log.Println("收到消息 hijack:", msg.Data)
				var req struct {
					Type string `json:"type"`
					Ts   string `json:"ts"`
					Id   string `json:"id"`
				}
				_ = json.Unmarshal(msg.Data, &req)

				if req.Type == "hijackReq" {
					log.Println("拦截请求:", req.Ts)
					if !checkPath(req.Ts) {
						log.Println("路径不合法:", req.Ts)
						return
					}
					realPath := utils.ExtractPath(req.Ts)
					path := "./m3u8/" + realPath
					file, err := os.Open(path)
					if err != nil {
						log.Println("读取失败:", err)
						return
					}
					defer file.Close()

					info, err := file.Stat()
					if err != nil {
						log.Println("获取文件信息失败:", err)
						return
					}
					totalSliceNum := int((info.Size() + int64(chunkSize) - 1) / int64(chunkSize))
					thisSendNum := 0

					buf := make([]byte, chunkSize) // 16KB
					for {
						n, err := file.Read(buf)
						if err != nil {
							if err == io.EOF {
								break
							}
							log.Println("Read error:", err)
							return
						}
						var resp map[string]interface{}
						if strings.HasSuffix(req.Ts, ".m3u8") || strings.HasSuffix(req.Ts, ".vtt") {
							resp = map[string]interface{}{
								"type":          "hijackRespText",
								"id":            req.Id,
								"payload":       base64.StdEncoding.EncodeToString(buf[:n]),
								"sliceNum":      thisSendNum,
								"totalSliceNum": totalSliceNum,
								"totalLength":   info.Size(),
							}
						} else {
							resp = map[string]interface{}{
								"type":          "hijackRespData",
								"id":            req.Id,
								"payload":       base64.StdEncoding.EncodeToString(buf[:n]),
								"sliceNum":      thisSendNum,
								"totalSliceNum": totalSliceNum,
								"totalLength":   info.Size(),
							}
						}
						respByte, err := json.Marshal(resp)
						if err != nil {
							log.Println("发送失败:", err)
							return
						}
						conn.dataChannel.Send(respByte)
						thisSendNum++
					}
					// 发送 ts 数据
					log.Println("发送成功 for")
				} else {
					panic("not supported type: " + req.Type)
				}
			})

			dataChannel.OnClose(func() {
				log.Printf("与客户端 %s 的数据通道已关闭", clientID)
				conn.mu.Lock()
				conn.isConnected = false
				conn.mu.Unlock()
			})
		}
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

func checkPath(path string) bool {
	// 禁止父级目录跳转
	if strings.Contains(path, "../") {
		log.Println("路径不合法: 包含上级目录", path)
		return false
	}
	if strings.Contains(path, "..\\") {
		log.Println("路径不合法: 包含上级目录", path)
		return false
	}
	return true
}
