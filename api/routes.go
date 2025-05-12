package api

import (
	"github.com/gin-gonic/gin"
)

// SetupRoutes 配置API路由
func SetupRoutes(router *gin.Engine) {
	controller := NewController()

	// 配置API路由组
	api := router.Group("/api")
	{
		// Torrent相关API
		api.POST("/magnets", controller.SubmitMagnet)
		api.GET("/tasks", controller.GetTaskList)
		api.GET("/tasks/:id", controller.GetTaskDetail)

		// WebRTC信令相关API
		api.POST("/webrtc/offer", controller.WebRTCOffer)
		api.POST("/webrtc/ice", controller.ICECandidate)

		// 服务B状态检查
		api.GET("/service-b/status", controller.CheckServiceBStatus)
	}

	// WebSocket端点
	router.GET("/ws/service-b", controller.HandleServiceBWebSocket)
	router.GET("/ws/client", controller.HandleClientWebSocket)

	// 静态文件服务（前端资源）
	router.Static("/static", "./static")
	router.StaticFile("/", "./static/index.html")
}
