package main

import (
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // 允许所有来源的WebSocket连接
	},
}

// Message 定义通用消息结构
type Message struct {
	Type    string                 `json:"type"`
	Payload map[string]interface{} `json:"payload"`
}

// setupRoutes 设置路由
func setupRoutes(router *gin.Engine, gateway *GatewayManager) {
	// 创建控制器
	controller := NewGatewayController(gateway)

	// API路由组
	api := router.Group("/api")
	{
		// 节点管理API
		api.GET("/nodes", controller.GetOnlineNodes)
		api.GET("/nodes/:id", controller.GetNodeDetail)

		// WebRTC信令API
		api.POST("/webrtc/offer", controller.HandleWebRTCOffer)
		api.POST("/webrtc/answer", controller.HandleWebRTCAnswer)
		api.POST("/webrtc/ice", controller.HandleICECandidate)

		// 任务路由API
		api.POST("/tasks/submit", controller.SubmitTask)
		api.GET("/tasks", controller.GetAllTasks)
		api.GET("/tasks/:id", controller.GetTaskDetail)

		// 系统状态API
		api.GET("/status", controller.GetSystemStatus)
	}

	// WebSocket路由
	router.GET("/ws/nodes", controller.HandleNodeWebSocket)    // 工作节点连接
	router.GET("/ws/clients", controller.HandleClientWebSocket) // 客户端连接

	// 静态文件服务
	router.Static("/static", "./static")
	router.StaticFile("/", "./static/index.html")
	router.StaticFile("/player", "./static/player.html")
}

// GatewayController 网关控制器
type GatewayController struct {
	gateway         *GatewayManager
	nodeConns       map[string]*websocket.Conn // 节点WebSocket连接
	clientConns     map[string]*websocket.Conn // 客户端WebSocket连接
	pendingRequests map[string]*PendingRequest  // 等待响应的请求
	mutex           sync.RWMutex                // 并发控制
}

// PendingRequest 等待中的请求
type PendingRequest struct {
	RequestID    string                   `json:"request_id"`
	RequestType  string                   `json:"request_type"`
	Responses    []map[string]interface{} `json:"responses"`
	ExpectedNodes int                     `json:"expected_nodes"`
	ResponseChan chan []map[string]interface{} `json:"-"`
	CreatedAt    time.Time                `json:"created_at"`
	mutex        sync.Mutex               `json:"-"`
}

// NewGatewayController 创建新的网关控制器
func NewGatewayController(gateway *GatewayManager) *GatewayController {
	controller := &GatewayController{
		gateway:         gateway,
		nodeConns:       make(map[string]*websocket.Conn),
		clientConns:     make(map[string]*websocket.Conn),
		pendingRequests: make(map[string]*PendingRequest),
	}
	
	// 启动清理任务
	go controller.cleanupExpiredRequests()
	
	return controller
}

// GetOnlineNodes 获取在线节点列表
func (gc *GatewayController) GetOnlineNodes(c *gin.Context) {
	nodes := gc.gateway.GetOnlineNodes()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    nodes,
	})
}

// GetNodeDetail 获取节点详情
func (gc *GatewayController) GetNodeDetail(c *gin.Context) {
	nodeID := c.Param("id")
	node, exists := gc.gateway.GetNode(nodeID)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Node not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    node,
	})
}

// HandleWebRTCOffer 处理WebRTC Offer
func (gc *GatewayController) HandleWebRTCOffer(c *gin.Context) {
	var request struct {
		WorkerID  string `json:"worker_id"`
		ClientID  string `json:"client_id"`
		SessionID string `json:"session_id"`
		SDP       string `json:"sdp"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request format",
		})
		return
	}

	// 创建WebRTC会话
	session := gc.gateway.CreateWebRTCSession(request.SessionID, request.ClientID, request.WorkerID)

	// 转发Offer到对应的工作节点
	if conn, exists := gc.nodeConns[request.WorkerID]; exists {
		message := Message{
			Type: "webrtc_offer",
			Payload: map[string]interface{}{
				"session_id": session.SessionID,
				"client_id":  session.ClientID,
				"sdp":        request.SDP,
			},
		}

		if err := conn.WriteJSON(message); err != nil {
			log.Printf("Failed to forward offer to worker %s: %v", request.WorkerID, err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "Failed to forward offer to worker",
			})
			return
		}
	} else {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Worker node not connected",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"session_id": session.SessionID,
	})
}

// HandleWebRTCAnswer 处理WebRTC Answer
func (gc *GatewayController) HandleWebRTCAnswer(c *gin.Context) {
	var request struct {
		SessionID string `json:"session_id"`
		SDP       string `json:"sdp"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request format",
		})
		return
	}

	// 获取会话信息
	session, exists := gc.gateway.GetWebRTCSession(request.SessionID)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Session not found",
		})
		return
	}

	// 转发Answer到对应的客户端
	if conn, exists := gc.clientConns[session.ClientID]; exists {
		message := Message{
			Type: "webrtc_answer",
			Payload: map[string]interface{}{
				"session_id": session.SessionID,
				"sdp":        request.SDP,
			},
		}

		if err := conn.WriteJSON(message); err != nil {
			log.Printf("Failed to forward answer to client %s: %v", session.ClientID, err)
		}
	}

	// 更新会话状态
	gc.gateway.UpdateSessionStatus(request.SessionID, "connected")

	c.JSON(http.StatusOK, gin.H{
		"success": true,
	})
}

// HandleICECandidate 处理ICE候选者
func (gc *GatewayController) HandleICECandidate(c *gin.Context) {
	var request struct {
		SessionID string `json:"session_id"`
		Candidate string `json:"candidate"`
		IsClient  bool   `json:"is_client"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request format",
		})
		return
	}

	// 获取会话信息
	session, exists := gc.gateway.GetWebRTCSession(request.SessionID)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Session not found",
		})
		return
	}

	// 根据来源转发ICE候选者
	var targetConn *websocket.Conn
	var targetID string

	if request.IsClient {
		// 来自客户端，转发到工作节点
		targetConn = gc.nodeConns[session.WorkerID]
		targetID = session.WorkerID
	} else {
		// 来自工作节点，转发到客户端
		targetConn = gc.clientConns[session.ClientID]
		targetID = session.ClientID
	}

	if targetConn != nil {
		message := Message{
			Type: "ice_candidate",
			Payload: map[string]interface{}{
				"session_id": session.SessionID,
				"candidate":  request.Candidate,
			},
		}

		if err := targetConn.WriteJSON(message); err != nil {
			log.Printf("Failed to forward ICE candidate to %s: %v", targetID, err)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
	})
}

// SubmitTask 提交任务到指定节点
func (gc *GatewayController) SubmitTask(c *gin.Context) {
	var request struct {
		WorkerID  string `json:"worker_id"`
		MagnetURL string `json:"magnet_url"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request format",
		})
		return
	}

	// 检查节点是否在线
	node, exists := gc.gateway.GetNode(request.WorkerID)
	if !exists || node.Status != "online" {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Worker node not available",
		})
		return
	}

	// 转发任务到工作节点
	if conn, exists := gc.nodeConns[request.WorkerID]; exists {
		message := Message{
			Type: "task_submit",
			Payload: map[string]interface{}{
				"magnet_url": request.MagnetURL,
				"timestamp":  time.Now().Unix(),
			},
		}

		if err := conn.WriteJSON(message); err != nil {
			log.Printf("Failed to submit task to worker %s: %v", request.WorkerID, err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "Failed to submit task to worker",
			})
			return
		}
	} else {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Worker node not connected",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Task submitted successfully",
	})
}

// GetAllTasks 获取所有任务列表
func (gc *GatewayController) GetAllTasks(c *gin.Context) {
	// 从所有连接的worker节点获取任务状态
	nodes := gc.gateway.GetOnlineNodes()
	if len(nodes) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data": gin.H{
				"tasks": []map[string]interface{}{},
			},
		})
		return
	}
	
	// 创建请求ID和等待响应的通道
	requestID := generateRequestID()
	responseChan := make(chan []map[string]interface{}, 1)
	
	// 注册待响应的请求
	gc.mutex.Lock()
	gc.pendingRequests[requestID] = &PendingRequest{
		RequestID:     requestID,
		RequestType:   "get_tasks",
		Responses:     make([]map[string]interface{}, 0),
		ExpectedNodes: len(nodes),
		ResponseChan:  responseChan,
		CreatedAt:     time.Now(),
	}
	gc.mutex.Unlock()
	
	// 向所有在线节点发送任务列表请求
	sentCount := 0
	for _, node := range nodes {
		if conn, exists := gc.nodeConns[node.ID]; exists {
			message := Message{
				Type: "get_tasks",
				Payload: map[string]interface{}{
					"request_id": requestID,
					"timestamp":  time.Now().Unix(),
				},
			}
			
			if err := conn.WriteJSON(message); err != nil {
				log.Printf("Failed to request tasks from worker %s: %v", node.ID, err)
				continue
			}
			sentCount++
		}
	}
	
	// 如果没有成功发送任何请求，直接返回空结果
	if sentCount == 0 {
		gc.mutex.Lock()
		delete(gc.pendingRequests, requestID)
		gc.mutex.Unlock()
		
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data": gin.H{
				"tasks": []map[string]interface{}{},
			},
		})
		return
	}
	
	// 更新期待的节点数量
	gc.mutex.Lock()
	if req, exists := gc.pendingRequests[requestID]; exists {
		req.ExpectedNodes = sentCount
	}
	gc.mutex.Unlock()
	
	// 等待响应或超时
	select {
	case allTasks := <-responseChan:
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data": gin.H{
				"tasks": allTasks,
			},
		})
	case <-time.After(10 * time.Second):
		// 超时处理
		gc.mutex.Lock()
		delete(gc.pendingRequests, requestID)
		gc.mutex.Unlock()
		
		c.JSON(http.StatusRequestTimeout, gin.H{
			"success": false,
			"error":   "Request timeout while waiting for worker responses",
		})
	}
}

// GetTaskDetail 获取任务详情
func (gc *GatewayController) GetTaskDetail(c *gin.Context) {
	taskID := c.Param("id")
	
	// 从worker节点获取任务详情
	nodes := gc.gateway.GetOnlineNodes()
	for _, node := range nodes {
		if conn, exists := gc.nodeConns[node.ID]; exists {
			message := Message{
				Type: "get_task_detail",
				Payload: map[string]interface{}{
					"task_id":   taskID,
					"timestamp": time.Now().Unix(),
				},
			}
			
			if err := conn.WriteJSON(message); err != nil {
				log.Printf("Failed to request task detail from worker %s: %v", node.ID, err)
				continue
			}
		}
	}
	
	// 暂时返回未找到
	c.JSON(http.StatusNotFound, gin.H{
		"success": false,
		"error":   "Task not found",
	})
}

// GetSystemStatus 获取系统状态
func (gc *GatewayController) GetSystemStatus(c *gin.Context) {
	onlineNodes := gc.gateway.GetOnlineNodes()

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"online_nodes":  len(onlineNodes),
			"total_nodes":   len(gc.gateway.nodes),
			"active_sessions": len(gc.gateway.sessions),
		},
	})
}

// HandleNodeWebSocket 处理工作节点WebSocket连接
func (gc *GatewayController) HandleNodeWebSocket(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	// 等待节点注册消息
	var nodeInfo WorkerNode
	if err := conn.ReadJSON(&nodeInfo); err != nil {
		log.Printf("Failed to read node registration: %v", err)
		return
	}

	// 注册节点
	gc.gateway.RegisterNode(&nodeInfo)
	gc.nodeConns[nodeInfo.ID] = conn

	log.Printf("Worker node %s connected: %s", nodeInfo.ID, nodeInfo.Name)

	// 发送注册确认
	confirmMsg := Message{
		Type: "registration_confirmed",
		Payload: map[string]interface{}{
			"node_id": nodeInfo.ID,
			"status":  "registered",
		},
	}
	conn.WriteJSON(confirmMsg)

	// 处理来自节点的消息
	for {
		var message Message
		if err := conn.ReadJSON(&message); err != nil {
			log.Printf("Worker node %s disconnected: %v", nodeInfo.ID, err)
			break
		}

		gc.handleNodeMessage(nodeInfo.ID, &message)
	}

	// 清理连接
	delete(gc.nodeConns, nodeInfo.ID)
	gc.gateway.RemoveNode(nodeInfo.ID)
}

// HandleClientWebSocket 处理客户端WebSocket连接
func (gc *GatewayController) HandleClientWebSocket(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("Client WebSocket upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	clientID := c.Query("client_id")
	if clientID == "" {
		log.Printf("Client ID is required")
		return
	}

	gc.clientConns[clientID] = conn
	log.Printf("Client %s connected", clientID)

	// 处理来自客户端的消息
	for {
		var message Message
		if err := conn.ReadJSON(&message); err != nil {
			log.Printf("Client %s disconnected: %v", clientID, err)
			break
		}

		gc.handleClientMessage(clientID, &message)
	}

	// 清理连接
	delete(gc.clientConns, clientID)
}

// handleNodeMessage 处理来自工作节点的消息
func (gc *GatewayController) handleNodeMessage(nodeID string, message *Message) {
	switch message.Type {
	case "heartbeat":
		gc.gateway.UpdateNodeHeartbeat(nodeID)

	case "webrtc_answer":
		// 转发WebRTC Answer到客户端
		log.Printf("Received webrtc_answer from node %s: %v", nodeID, message.Payload)
		if sessionID, ok := message.Payload["session_id"].(string); ok {
			log.Printf("Looking for session: %s", sessionID)
			if session, exists := gc.gateway.GetWebRTCSession(sessionID); exists {
				log.Printf("Found session %s, client: %s", sessionID, session.ClientID)
				if clientConn, exists := gc.clientConns[session.ClientID]; exists {
					log.Printf("Forwarding webrtc_answer to client %s", session.ClientID)
					if err := clientConn.WriteJSON(message); err != nil {
						log.Printf("Failed to forward webrtc_answer: %v", err)
					}
				} else {
					log.Printf("Client connection not found for: %s", session.ClientID)
				}
			} else {
				log.Printf("Session not found: %s", sessionID)
			}
		} else {
			log.Printf("No session_id in webrtc_answer payload")
		}

	case "ice_candidate":
		// 转发ICE候选者到客户端
		log.Printf("Received ice_candidate from node %s: %v", nodeID, message.Payload)
		if sessionID, ok := message.Payload["session_id"].(string); ok {
			log.Printf("Looking for session: %s", sessionID)
			if session, exists := gc.gateway.GetWebRTCSession(sessionID); exists {
				log.Printf("Found session %s, client: %s", sessionID, session.ClientID)
				if clientConn, exists := gc.clientConns[session.ClientID]; exists {
					log.Printf("Forwarding ice_candidate to client %s", session.ClientID)
					if err := clientConn.WriteJSON(message); err != nil {
						log.Printf("Failed to forward ice_candidate: %v", err)
					}
				} else {
					log.Printf("Client connection not found for: %s", session.ClientID)
				}
			} else {
				log.Printf("Session not found: %s", sessionID)
			}
		} else {
			log.Printf("No session_id in ice_candidate payload")
		}

	case "task_status":
		// 任务状态更新，可以存储或转发给相关客户端
		log.Printf("Task status update from node %s: %v", nodeID, message.Payload)

	case "tasks_response":
		// 处理任务列表响应
		gc.handleTasksResponse(nodeID, message.Payload)

	case "task_detail_response":
		// 处理任务详情响应
		gc.handleTaskDetailResponse(nodeID, message.Payload)

	default:
		log.Printf("Unknown message type from node %s: %s", nodeID, message.Type)
	}
}

// handleClientMessage 处理来自客户端的消息
func (gc *GatewayController) handleClientMessage(clientID string, message *Message) {
	switch message.Type {
	case "webrtc_offer":
		// 转发WebRTC Offer到指定工作节点
		if workerID, ok := message.Payload["worker_id"].(string); ok {
			if workerConn, exists := gc.nodeConns[workerID]; exists {
				// 使用客户端提供的session_id，而不是创建新的
				sessionID, _ := message.Payload["session_id"].(string)
				if sessionID == "" {
					sessionID = fmt.Sprintf("session_%s_%s_%d", clientID, workerID, time.Now().UnixNano())
				}
				
				// 创建WebRTC会话
				session := gc.gateway.CreateWebRTCSession(sessionID, clientID, workerID)
				
				// 确保消息中的session_id是正确的
				message.Payload["session_id"] = session.SessionID
				message.Payload["client_id"] = clientID
				
				log.Printf("Created WebRTC session %s between client %s and worker %s", 
					session.SessionID, clientID, workerID)
				
				if err := workerConn.WriteJSON(message); err != nil {
					log.Printf("Failed to forward offer to worker %s: %v", workerID, err)
				}
			} else {
				log.Printf("Worker %s is not connected", workerID)
			}
		} else {
			log.Printf("No worker_id specified in webrtc_offer from client %s", clientID)
		}

	case "ice_candidate":
		// 转发ICE候选者到工作节点
		if sessionID, ok := message.Payload["session_id"].(string); ok {
			if session, exists := gc.gateway.GetWebRTCSession(sessionID); exists {
				if workerConn, exists := gc.nodeConns[session.WorkerID]; exists {
					workerConn.WriteJSON(message)
				}
			}
		}

	default:
		log.Printf("Unknown message type from client %s: %s", clientID, message.Type)
	}
}

// handleTasksResponse 处理任务列表响应
func (gc *GatewayController) handleTasksResponse(nodeID string, payload map[string]interface{}) {
	requestIDIntf, ok := payload["request_id"]
	if !ok {
		// 处理老版本的响应，没有request_id
		log.Printf("Received tasks response from %s without request_id", nodeID)
		return
	}
	
	requestID, ok := requestIDIntf.(string)
	if !ok {
		log.Printf("Invalid request_id type from %s", nodeID)
		return
	}
	
	gc.mutex.Lock()
	defer gc.mutex.Unlock()
	
	req, exists := gc.pendingRequests[requestID]
	if !exists {
		log.Printf("Received response for unknown request %s from %s", requestID, nodeID)
		return
	}
	
	req.mutex.Lock()
	defer req.mutex.Unlock()
	
	// 添加节点信息到响应中
	responseData := make(map[string]interface{})
	for k, v := range payload {
		responseData[k] = v
	}
	responseData["node_id"] = nodeID
	
	req.Responses = append(req.Responses, responseData)
	
	// 检查是否收集到所有响应
	if len(req.Responses) >= req.ExpectedNodes {
		// 合并所有任务
		allTasks := make([]map[string]interface{}, 0)
		for _, response := range req.Responses {
			if tasks, ok := response["tasks"].([]interface{}); ok {
				for _, task := range tasks {
					if taskMap, ok := task.(map[string]interface{}); ok {
						allTasks = append(allTasks, taskMap)
					}
				}
			}
		}
		
		// 发送合并后的结果
		select {
		case req.ResponseChan <- allTasks:
			// 成功发送
		default:
			// 通道已关闭或缓冲区满
		}
		
		// 清理请求
		delete(gc.pendingRequests, requestID)
	}
}

// handleTaskDetailResponse 处理任务详情响应
func (gc *GatewayController) handleTaskDetailResponse(nodeID string, payload map[string]interface{}) {
	// 简单实现：找到第一个匹配的任务并返回
	// 在实际应用中，可能需要更复杂的逻辑来处理多个worker节点
	log.Printf("Received task detail response from %s: %v", nodeID, payload)
}

// generateRequestID 生成请求ID
func generateRequestID() string {
	return fmt.Sprintf("req_%d_%d", time.Now().UnixNano(), time.Now().Unix())
}

// cleanupExpiredRequests 清理过期请求
func (gc *GatewayController) cleanupExpiredRequests() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	for range ticker.C {
		gc.mutex.Lock()
		now := time.Now()
		
		for requestID, req := range gc.pendingRequests {
			// 清理超过30秒的请求
			if now.Sub(req.CreatedAt) > 30*time.Second {
				close(req.ResponseChan)
				delete(gc.pendingRequests, requestID)
				log.Printf("Cleaned up expired request: %s", requestID)
			}
		}
		
		gc.mutex.Unlock()
	}
}