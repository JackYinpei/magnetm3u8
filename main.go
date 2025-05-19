package main

import (
	"log"
	"os"

	"magnetm3u8/api"
	"magnetm3u8/db"
	"magnetm3u8/services"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func main() {

	godotenv.Load(".env")

	// 获取端口配置，默认为8080
	port := os.Getenv("SERVICE_A_PORT")
	if port == "" {
		port = "7070"
	}

	// 初始化数据库
	db.InitDB()

	// 设置Gin模式
	mode := os.Getenv("GIN_MODE")
	if mode == "release" {
		gin.SetMode(gin.ReleaseMode)
	}

	// 创建Gin路由
	router := gin.Default()

	// 配置CORS中间件
	router.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	})

	// 设置发送消息到客户端的回调函数
	services.SetSendToClientFunc(api.SendMessageToClient)

	// 设置API路由
	api.SetupRoutes(router)

	// 设置WebSocket消息处理
	services.SetupMessageHandling()

	log.Printf("服务器启动在端口 %s...\n", port)
	err := router.Run(":" + port)
	if err != nil {
		log.Fatalf("启动服务器失败: %v", err)
	}
}
