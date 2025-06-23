package models

import (
	"encoding/json"
	"time"
)

// TorrentFileInfo 表示单个torrent文件的信息
type TorrentFileInfo struct {
	FileName   string `json:"file_name"`
	FileSize   int64  `json:"file_size"`
	FilePath   string `json:"file_path"`
	IsSelected bool   `json:"is_selected"`
}

// Task 表示一个磁力链接下载任务（合并了之前的多个表）
type Task struct {
	ID             uint      `json:"id" gorm:"primaryKey"`
	MagnetURL      string    `json:"magnet_url" gorm:"type:text;not null"`
	Status         string    `json:"status" gorm:"type:varchar(20);not null"` // waiting, downloading, completed, failed, transcoding, ready
	Percentage     float64   `json:"percentage" gorm:"default:0"`
	DownloadSpeed  int64     `json:"download_speed" gorm:"default:0"`                        // bytes per second
	TorrentFiles   string    `json:"-" gorm:"type:text"`                                     // JSON序列化的文件信息
	M3U8FilePath   string    `json:"m3u8_file_path" gorm:"column:m3_u8_file_path;type:text"` // M3U8文件路径
	SrtsJSON       string    `json:"-" gorm:"column:srts;type:text"`                         // JSON序列化的字幕文件列表
	LastUpdateTime time.Time `json:"last_update_time"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// GetTorrentFiles 获取反序列化的文件信息
func (t *Task) GetTorrentFiles() ([]TorrentFileInfo, error) {
	if t.TorrentFiles == "" {
		return []TorrentFileInfo{}, nil
	}

	var files []TorrentFileInfo
	err := json.Unmarshal([]byte(t.TorrentFiles), &files)
	return files, err
}

// SetTorrentFiles 设置序列化的文件信息
func (t *Task) SetTorrentFiles(files []TorrentFileInfo) error {
	data, err := json.Marshal(files)
	if err != nil {
		return err
	}
	t.TorrentFiles = string(data)
	return nil
}

// GetSrts 获取反序列化的字幕文件列表
func (t *Task) GetSrts() ([]string, error) {
	if t.SrtsJSON == "" {
		return []string{}, nil
	}

	var srts []string
	err := json.Unmarshal([]byte(t.SrtsJSON), &srts)
	return srts, err
}

// SetSrts 设置序列化的字幕文件列表
func (t *Task) SetSrts(srts []string) error {
	data, err := json.Marshal(srts)
	if err != nil {
		return err
	}
	t.SrtsJSON = string(data)
	return nil
}

// Srts 获取字幕文件列表（用于 JSON 响应）
func (t *Task) Srts() []string {
	srts, _ := t.GetSrts()
	return srts
}

// MarshalJSON 自定义 JSON 序列化，确保 srts 字段正确输出
func (t *Task) MarshalJSON() ([]byte, error) {
	type TaskAlias Task
	srts, _ := t.GetSrts()

	return json.Marshal(&struct {
		*TaskAlias
		Srts []string `json:"srts"`
	}{
		TaskAlias: (*TaskAlias)(t),
		Srts:      srts,
	})
}

// WebRTCSession 表示WebRTC会话信息
type WebRTCSession struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	TaskID    uint      `json:"task_id" gorm:"index"`
	ClientID  string    `json:"client_id" gorm:"type:varchar(50)"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
