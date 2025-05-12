package models

import (
	"time"
)

// TorrentTask 表示一个磁力链接下载任务
type TorrentTask struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	MagnetURL string    `json:"magnet_url" gorm:"type:text;not null"`
	Status    string    `json:"status" gorm:"type:varchar(20);not null"` // waiting, downloading, completed, failed, transcoding, ready
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TorrentFile 表示Torrent中的文件信息
type TorrentFile struct {
	ID         uint   `json:"id" gorm:"primaryKey"`
	TaskID     uint   `json:"task_id" gorm:"index"`
	FileName   string `json:"file_name" gorm:"type:varchar(255);not null"`
	FileSize   int64  `json:"file_size"`
	FilePath   string `json:"file_path" gorm:"type:text"`
	IsSelected bool   `json:"is_selected" gorm:"default:false"` // 是否被选中下载
}

// DownloadProgress 表示下载进度
type DownloadProgress struct {
	ID             uint      `json:"id" gorm:"primaryKey"`
	TaskID         uint      `json:"task_id" gorm:"uniqueIndex"`
	Percentage     float64   `json:"percentage"`
	DownloadSpeed  int64     `json:"download_speed"` // bytes per second
	LastUpdateTime time.Time `json:"last_update_time"`
}

// M3U8Info 表示转码后的M3U8信息
type M3U8Info struct {
	ID       uint   `json:"id" gorm:"primaryKey"`
	TaskID   uint   `json:"task_id" gorm:"uniqueIndex"`
	FilePath string `json:"file_path" gorm:"type:text"`
}

// WebRTCSession 表示WebRTC会话信息
type WebRTCSession struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	TaskID    uint      `json:"task_id" gorm:"index"`
	ClientID  string    `json:"client_id" gorm:"type:varchar(50)"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
