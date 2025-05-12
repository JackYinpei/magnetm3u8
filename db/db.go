package db

import (
	"log"

	"magnetm3u8/models"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

var DB *gorm.DB

// 初始化数据库连接
func InitDB() {
	var err error
	DB, err = gorm.Open(sqlite.Open("magnetm3u8.db"), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// 自动迁移数据库结构
	err = DB.AutoMigrate(
		&models.TorrentTask{},
		&models.TorrentFile{},
		&models.DownloadProgress{},
		&models.M3U8Info{},
		&models.WebRTCSession{},
	)
	if err != nil {
		log.Fatalf("Failed to migrate database: %v", err)
	}

	log.Println("Database initialized successfully")
}
