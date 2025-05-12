package api

import (
	"log"
	"net/http"
	"strconv"

	"magnetm3u8/services"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// Controller 处理API请求的控制器
type Controller struct {
	torrentService *services.TorrentService
	webrtcService  *services.WebRTCService
}

// NewController 创建新的控制器
func NewController() *Controller {
	return &Controller{
		torrentService: services.NewTorrentService(),
		webrtcService:  services.NewWebRTCService(),
	}
}

// SubmitMagnet 处理提交磁力链接请求
func (c *Controller) SubmitMagnet(ctx *gin.Context) {
	var request struct {
		MagnetURL string `json:"magnet_url" binding:"required"`
	}

	if err := ctx.ShouldBindJSON(&request); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的请求格式",
		})
		return
	}

	// 创建下载任务
	task, err := c.torrentService.CreateTask(request.MagnetURL)
	if err != nil {
		if err == services.ErrInvalidMagnetURL {
			ctx.JSON(http.StatusBadRequest, gin.H{
				"error": "无效的磁力链接",
			})
			return
		}

		if err == services.ErrNotConnected {
			ctx.JSON(http.StatusServiceUnavailable, gin.H{
				"error": "服务B未连接",
			})
			return
		}

		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": "创建任务失败",
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"task_id": task.ID,
		"status":  task.Status,
	})
}

// GetTaskList 获取任务列表
func (c *Controller) GetTaskList(ctx *gin.Context) {
	tasks, err := c.torrentService.GetAllTasks()
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": "获取任务列表失败",
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"tasks": tasks,
	})
}

// GetTaskDetail 获取任务详情
func (c *Controller) GetTaskDetail(ctx *gin.Context) {
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
		if err == services.ErrTaskNotFound {
			ctx.JSON(http.StatusNotFound, gin.H{
				"error": "任务未找到",
			})
			return
		}

		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": "获取任务失败",
		})
		return
	}

	// 获取下载进度
	progress, err := c.torrentService.GetDownloadProgress(task.ID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": "获取下载进度失败",
		})
		return
	}

	// 获取文件列表
	files, err := c.torrentService.GetTorrentFiles(task.ID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": "获取文件列表失败",
		})
		return
	}

	// 获取M3U8信息
	m3u8Info, err := c.torrentService.GetM3U8Info(task.ID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": "获取M3U8信息失败",
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"task":     task,
		"progress": progress,
		"files":    files,
		"m3u8":     m3u8Info,
	})
}

// WebRTCOffer 处理WebRTC Offer
func (c *Controller) WebRTCOffer(ctx *gin.Context) {
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
		if err == services.ErrNotConnected {
			ctx.JSON(http.StatusServiceUnavailable, gin.H{
				"error": "服务B未连接",
			})
			return
		}

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
func (c *Controller) ICECandidate(ctx *gin.Context) {
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
		if err == services.ErrNotConnected {
			ctx.JSON(http.StatusServiceUnavailable, gin.H{
				"error": "服务B未连接",
			})
			return
		}

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
func (c *Controller) HandleServiceBWebSocket(ctx *gin.Context) {
	// 升级HTTP连接为WebSocket连接
	conn, err := upgrader.Upgrade(ctx.Writer, ctx.Request, nil)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": "升级WebSocket连接失败",
		})
		return
	}

	// 注册WebSocket连接
	wsManager := services.GetWebSocketManager()
	wsManager.RegisterConnection(conn)
}

// HandleClientWebSocket 处理客户端的WebSocket连接
func (c *Controller) HandleClientWebSocket(ctx *gin.Context) {
	// 升级HTTP连接为WebSocket连接
	conn, err := upgrader.Upgrade(ctx.Writer, ctx.Request, nil)
	if err != nil {
		log.Printf("升级客户端WebSocket连接失败: %v", err)
		return
	}

	// 处理客户端连接
	HandleClientConnection(conn, ctx.Query("client_id"), c.webrtcService)
}

// CheckServiceBStatus 检查与服务B的连接状态
func (c *Controller) CheckServiceBStatus(ctx *gin.Context) {
	wsManager := services.GetWebSocketManager()
	isConnected := wsManager.IsConnected()

	ctx.JSON(http.StatusOK, gin.H{
		"connected": isConnected,
	})
}
