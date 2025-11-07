package main

import (
	"context"
	"flag"
	"log"

	"github.com/joho/godotenv"

	"magnetm3u8-gateway/internal/auth"
	"magnetm3u8-gateway/internal/cluster"
	"magnetm3u8-gateway/internal/config"
	"magnetm3u8-gateway/internal/database"
	"magnetm3u8-gateway/internal/http/router"
	"magnetm3u8-gateway/internal/ice"
	"magnetm3u8-gateway/internal/session"
	"magnetm3u8-gateway/internal/user"
)

var port = flag.String("port", "8080", "Gateway server port")

func main() {
	flag.Parse()
	_ = godotenv.Load(".env")

	cfg := config.Load(*port)

	manager := cluster.NewManager()
	iceProvider := ice.NewIceServerProviderFromEnv()

	db, err := database.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("无法连接数据库: %v", err)
	}
	defer db.Close()

	if err := database.Migrate(db); err != nil {
		log.Fatalf("数据库迁移失败: %v", err)
	}

	userRepo := user.NewRepository(db)
	sessionStore := session.NewStore(db)
	authService := auth.NewService(userRepo, sessionStore, cfg.SessionTTL)

	if err := authService.EnsureDefaultAdmin(context.Background(), cfg.AdminUsername, cfg.AdminPassword); err != nil {
		log.Fatalf("初始化管理员账户失败: %v", err)
	}

	engine := router.New(router.Dependencies{
		Config:      cfg,
		Manager:     manager,
		Ice:         iceProvider,
		AuthService: authService,
		UserRepo:    userRepo,
	})

	log.Printf("Gateway Server 启动在端口 %s...", cfg.Port)
	if err := engine.Run(":" + cfg.Port); err != nil {
		log.Fatalf("启动Gateway Server失败: %v", err)
	}
}
