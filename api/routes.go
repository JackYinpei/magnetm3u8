package api

import (
	"github.com/gin-gonic/gin"
)

// SetupRoutes 设置API路由
func SetupRoutes(router *gin.Engine) {
	// 创建控制器
	taskController := NewTaskController()

	// API路由组
	api := router.Group("/api")
	{
		// 任务相关路由
		api.POST("/tasks", taskController.SubmitMagnet)          // 提交磁力链接
		api.GET("/tasks", taskController.GetAllTasks)            // 获取所有任务
		api.GET("/tasks/:id", taskController.GetTaskDetail)      // 获取任务详情
		api.GET("/tasks/:id/files", taskController.GetTaskFiles) // 获取任务文件列表
		api.POST("/tasks/:id/retry", taskController.RetryTask)   // 重试任务
		api.DELETE("/tasks/:id", taskController.DeleteTask)      // 删除任务

		// 系统状态路由
		api.GET("/status", taskController.GetConnectionStatus) // 获取服务B连接状态
	}

	// WebSocket路由
	router.GET("/ws/service-b", taskController.HandleServiceBWebSocket) // 服务B WebSocket连接

	// 静态文件服务（前端资源）
	router.Static("/static", "./static")
	router.StaticFile("/", "./static/index.html")
	router.StaticFile("/player", "./static/player.html")
}
