package api

import (
	"log"
	"net/http"
	"strconv"
	"time"

	"magnetm3u8/models"
	"magnetm3u8/services"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type TaskController struct {
	torrentService *services.TorrentService
	webrtcService  *services.WebRTCService
}

func NewTaskController() *TaskController {
	return &TaskController{
		torrentService: services.NewTorrentService(),
		webrtcService:  services.NewWebRTCService(),
	}
}

// SubmitMagnet 提交磁力链接
func (c *TaskController) SubmitMagnet(ctx *gin.Context) {
	var request struct {
		MagnetURL string `json:"magnet_url" binding:"required"`
	}

	if err := ctx.ShouldBindJSON(&request); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error": "请求参数错误: " + err.Error(),
		})
		return
	}

	// 验证磁力链接格式
	if err := c.torrentService.ValidateMagnetURL(request.MagnetURL); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error": "磁力链接格式无效: " + err.Error(),
		})
		return
	}

	// 创建任务
	task, err := c.torrentService.CreateTask(request.MagnetURL)
	if err != nil {
		log.Printf("创建任务失败: %v", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": "创建任务失败: " + err.Error(),
		})
		return
	}

	// 发送任务到服务B
	wsManager := services.GetWebSocketManager()
	if !wsManager.IsConnected() {
		// 更新任务状态为失败
		c.torrentService.UpdateTaskStatus(task.ID, "failed")
		ctx.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "服务B未连接",
		})
		return
	}

	// 发送磁力链接到服务B
	err = wsManager.SendMessage(services.MsgTypeMagnetSubmit, map[string]interface{}{
		"task_id":    task.ID,
		"magnet_url": request.MagnetURL,
	})

	if err != nil {
		log.Printf("发送任务到服务B失败: %v", err)
		c.torrentService.UpdateTaskStatus(task.ID, "failed")
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": "发送任务失败: " + err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"message": "任务提交成功",
		"task":    task,
	})
}

// GetAllTasks 获取所有任务
func (c *TaskController) GetAllTasks(ctx *gin.Context) {
	tasks, err := c.torrentService.GetAllTasks()
	if err != nil {
		log.Printf("获取任务列表失败: %v", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": "获取任务列表失败: " + err.Error(),
		})
		return
	}

	// 构建返回数据
	var taskList []map[string]interface{}
	for _, task := range tasks {
		// 获取文件信息
		files, err := task.GetTorrentFiles()
		if err != nil {
			log.Printf("获取任务 %d 文件信息失败: %v", task.ID, err)
			files = []models.TorrentFileInfo{}
		}

		// 构建任务信息
		taskInfo := map[string]interface{}{
			"id":               task.ID,
			"magnet_url":       task.MagnetURL,
			"status":           task.Status,
			"percentage":       task.Percentage,
			"download_speed":   task.DownloadSpeed,
			"last_update_time": task.LastUpdateTime,
			"created_at":       task.CreatedAt,
			"updated_at":       task.UpdatedAt,
			"files":            files,
		}

		// 如果有M3U8文件路径，添加到返回数据中
		if task.M3U8FilePath != "" {
			taskInfo["m3u8_file_path"] = task.M3U8FilePath
		}

		taskList = append(taskList, taskInfo)
	}

	ctx.JSON(http.StatusOK, gin.H{
		"tasks": taskList,
	})
}

// GetTaskDetail 获取任务详情
func (c *TaskController) GetTaskDetail(ctx *gin.Context) {
	taskIDStr := ctx.Param("id")
	taskID, err := strconv.ParseUint(taskIDStr, 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的任务ID",
		})
		return
	}

	task, err := c.torrentService.GetTaskByID(uint(taskID))
	if err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{
			"error": "任务不存在",
		})
		return
	}

	// 获取文件信息
	files, err := task.GetTorrentFiles()
	if err != nil {
		log.Printf("获取任务 %d 文件信息失败: %v", task.ID, err)
		files = []models.TorrentFileInfo{}
	}

	// 构建返回数据
	taskDetail := map[string]interface{}{
		"id":               task.ID,
		"magnet_url":       task.MagnetURL,
		"status":           task.Status,
		"percentage":       task.Percentage,
		"download_speed":   task.DownloadSpeed,
		"last_update_time": task.LastUpdateTime,
		"created_at":       task.CreatedAt,
		"updated_at":       task.UpdatedAt,
		"files":            files,
	}

	// 如果有M3U8文件路径，添加到返回数据中
	if task.M3U8FilePath != "" {
		taskDetail["m3u8_file_path"] = task.M3U8FilePath
	}

	ctx.JSON(http.StatusOK, gin.H{
		"task": taskDetail,
	})
}

// GetTaskFiles 获取任务文件列表
func (c *TaskController) GetTaskFiles(ctx *gin.Context) {
	taskIDStr := ctx.Param("id")
	taskID, err := strconv.ParseUint(taskIDStr, 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的任务ID",
		})
		return
	}

	files, err := c.torrentService.GetTorrentFiles(uint(taskID))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": "获取文件列表失败: " + err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"files": files,
	})
}

// DeleteTask 删除任务
func (c *TaskController) DeleteTask(ctx *gin.Context) {
	taskIDStr := ctx.Param("id")
	taskID, err := strconv.ParseUint(taskIDStr, 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的任务ID",
		})
		return
	}

	// 检查任务是否存在
	task, err := c.torrentService.GetTaskByID(uint(taskID))
	if err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{
			"error": "任务不存在",
		})
		return
	}

	// 如果任务正在进行中，不允许删除
	if task.Status == "downloading" || task.Status == "transcoding" {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error": "任务正在进行中，不能删除",
		})
		return
	}

	// 删除任务（这里需要实现删除方法）
	// TODO: 实现删除任务的方法
	ctx.JSON(http.StatusOK, gin.H{
		"message": "任务删除成功",
	})
}

// GetConnectionStatus 获取与服务B的连接状态
func (c *TaskController) GetConnectionStatus(ctx *gin.Context) {
	wsManager := services.GetWebSocketManager()

	ctx.JSON(http.StatusOK, gin.H{
		"connected":   wsManager.IsConnected(),
		"server_time": time.Now(),
	})
}

// RetryTask 重试失败的任务
func (c *TaskController) RetryTask(ctx *gin.Context) {
	taskIDStr := ctx.Param("id")
	taskID, err := strconv.ParseUint(taskIDStr, 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的任务ID",
		})
		return
	}

	task, err := c.torrentService.GetTaskByID(uint(taskID))
	if err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{
			"error": "任务不存在",
		})
		return
	}

	// 只有失败的任务才能重试
	if task.Status != "failed" {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error": "只有失败的任务才能重试",
		})
		return
	}

	// 检查服务B连接状态
	wsManager := services.GetWebSocketManager()
	if !wsManager.IsConnected() {
		ctx.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "服务B未连接",
		})
		return
	}

	// 重置任务状态
	err = c.torrentService.UpdateTaskStatus(task.ID, "waiting")
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": "更新任务状态失败: " + err.Error(),
		})
		return
	}

	// 重新发送任务到服务B
	err = wsManager.SendMessage(services.MsgTypeMagnetSubmit, map[string]interface{}{
		"task_id":    task.ID,
		"magnet_url": task.MagnetURL,
	})

	if err != nil {
		log.Printf("重新发送任务到服务B失败: %v", err)
		c.torrentService.UpdateTaskStatus(task.ID, "failed")
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": "重试任务失败: " + err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"message": "任务重试成功",
	})
}

// WebRTC 相关方法
// WebRTCOffer 处理WebRTC Offer
func (c *TaskController) WebRTCOffer(ctx *gin.Context) {
	var request struct {
		TaskID   uint   `json:"task_id" binding:"required"`
		ClientID string `json:"client_id" binding:"required"`
		SDP      string `json:"sdp" binding:"required"`
	}

	if err := ctx.ShouldBindJSON(&request); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的请求格式",
		})
		return
	}

	// 创建WebRTC会话
	_, err := c.webrtcService.CreateSession(request.TaskID, request.ClientID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": "创建WebRTC会话失败",
		})
		return
	}

	// 发送Offer到服务B
	err = c.webrtcService.SendOffer(request.ClientID, request.TaskID, request.SDP)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": "发送WebRTC Offer失败",
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
	})
}

// ICECandidate 处理ICE Candidate
func (c *TaskController) ICECandidate(ctx *gin.Context) {
	var request struct {
		ClientID  string `json:"client_id" binding:"required"`
		Candidate string `json:"candidate" binding:"required"`
	}

	if err := ctx.ShouldBindJSON(&request); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的请求格式",
		})
		return
	}

	// 发送ICE Candidate到服务B
	err := c.webrtcService.SendICECandidateToServiceB(request.ClientID, request.Candidate)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": "发送ICE Candidate失败",
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
	})
}

// 设置WebSocket升级器
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // 允许所有来源，生产环境中应该限制来源
	},
}

// HandleServiceBWebSocket 处理服务B的WebSocket连接
func (c *TaskController) HandleServiceBWebSocket(ctx *gin.Context) {
	// 获取客户端IP
	clientIP := ctx.ClientIP()

	// 升级HTTP连接为WebSocket连接
	conn, err := upgrader.Upgrade(ctx.Writer, ctx.Request, nil)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": "升级WebSocket连接失败",
		})
		return
	}

	// 检查当前是否已有服务B连接
	wsManager := services.GetWebSocketManager()
	if wsManager.IsConnected() {
		// 如果已经有连接，说明这是恶意连接尝试
		log.Printf("检测到恶意连接尝试，来自IP: %s", clientIP)

		// 发送拒绝消息
		rejectMsg := struct {
			Type    string `json:"type"`
			Payload string `json:"payload"`
		}{
			Type:    "reject",
			Payload: "Fuck you",
		}
		conn.WriteJSON(rejectMsg)

		// 关闭连接
		time.Sleep(200 * time.Millisecond) // 给一点时间发送消息
		conn.Close()
		return
	}

	// 注册WebSocket连接
	wsManager.RegisterConnection(conn)
	log.Printf("服务B已连接，IP: %s", clientIP)
}

// HandleClientWebSocket 处理客户端的WebSocket连接
func (c *TaskController) HandleClientWebSocket(ctx *gin.Context) {
	// 升级HTTP连接为WebSocket连接
	conn, err := upgrader.Upgrade(ctx.Writer, ctx.Request, nil)
	if err != nil {
		log.Printf("升级客户端WebSocket连接失败: %v", err)
		return
	}

	// 处理客户端连接
	HandleClientConnection(conn, ctx.Query("client_id"), c.webrtcService)
}
