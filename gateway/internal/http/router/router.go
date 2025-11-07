package router

import (
	"net/http"
	"path/filepath"

	"github.com/gin-gonic/gin"

	"magnetm3u8-gateway/internal/auth"
	"magnetm3u8-gateway/internal/cluster"
	"magnetm3u8-gateway/internal/config"
	"magnetm3u8-gateway/internal/http/handlers"
	"magnetm3u8-gateway/internal/http/middleware"
	"magnetm3u8-gateway/internal/ice"
	"magnetm3u8-gateway/internal/user"
)

// Dependencies aggregates the components required to build the HTTP server.
type Dependencies struct {
	Config      config.Config
	Manager     *cluster.Manager
	Ice         *ice.IceServerProvider
	AuthService *auth.Service
	UserRepo    *user.Repository
}

// New builds a fully configured Gin engine.
func New(deps Dependencies) *gin.Engine {
	engine := gin.Default()
	engine.Use(corsMiddleware())
	engine.Use(middleware.Session(deps.AuthService, deps.Config.SessionCookieName))

	authHandler := handlers.NewAuthHandler(deps.AuthService, deps.Config.SessionCookieName, deps.Config.SessionTTL)
	adminHandler := handlers.NewAdminHandler(deps.UserRepo)

	handlers.RegisterGatewayRoutes(engine, deps.Manager, deps.Ice)
	registerAuthRoutes(engine, authHandler)
	registerAdminRoutes(engine, adminHandler)

	staticDir := deps.Config.StaticDir
	engine.Static("/static", staticDir)
	engine.StaticFile("/", filepath.Join(staticDir, "index.html"))
	engine.StaticFile("/player", filepath.Join(staticDir, "player.html"))

	return engine
}

func registerAuthRoutes(router *gin.Engine, handler *handlers.AuthHandler) {
	authGroup := router.Group("/api/auth")
	{
		authGroup.POST("/register", handler.Register)
		authGroup.POST("/login", handler.Login)
		authGroup.POST("/logout", handler.Logout)
		authGroup.GET("/me", handler.Profile)
	}
}

func registerAdminRoutes(router *gin.Engine, handler *handlers.AdminHandler) {
	adminGroup := router.Group("/api/admin")
	adminGroup.Use(middleware.RequireAdmin())
	{
		adminGroup.GET("/users", handler.ListUsers)
		adminGroup.PATCH("/users/:id/ban", handler.UpdateBanState)
	}
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" {
			c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
			c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		} else {
			c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		}

		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS")
		c.Writer.Header().Set("Vary", "Origin")

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}
