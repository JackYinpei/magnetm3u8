package main

import (
	"flag"
	"log"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

var (
	port = flag.String("port", "8080", "Gateway server port")
)

func main() {
	flag.Parse()

	godotenv.Load(".env")

	// 获取端口配置
	if envPort := os.Getenv("GATEWAY_PORT"); envPort != "" {
		*port = envPort
	}

	// 设置Gin模式
	if os.Getenv("GIN_MODE") == "release" {
		gin.SetMode(gin.ReleaseMode)
	}

	// 创建网关管理器
	gateway := NewGatewayManager()

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

	// 设置路由
	setupRoutes(router, gateway)

	log.Printf("Gateway Server 启动在端口 %s...", *port)
	err := router.Run(":" + *port)
	if err != nil {
		log.Fatalf("启动Gateway Server失败: %v", err)
	}
}