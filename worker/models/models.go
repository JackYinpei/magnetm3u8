package models

import (
	"encoding/json"
	"time"

	"worker/domain"

	"gorm.io/gorm"
)

// TorrentFileInfo 表示单个torrent文件的信息
type TorrentFileInfo struct {
	FileName   string `json:"file_name"`
	FileSize   int64  `json:"file_size"`
	FilePath   string `json:"file_path"`
	IsSelected bool   `json:"is_selected"`
}

// Task 表示一个磁力链接下载任务
type Task struct {
	ID             uint              `json:"id" gorm:"primaryKey"`
	TaskID         string            `json:"task_id" gorm:"uniqueIndex;not null"` // UUID for task identification
	MagnetURL      string            `json:"magnet_url" gorm:"not null"`
	Status         domain.TaskStatus `json:"status" gorm:"default:pending"`  // pending, downloading, completed, error, transcoding, ready
	Progress       int               `json:"progress" gorm:"default:0"`      // 0-100
	Speed          int64             `json:"speed" gorm:"default:0"`         // bytes per second
	Size           int64             `json:"size" gorm:"default:0"`          // total size in bytes
	Downloaded     int64             `json:"downloaded" gorm:"default:0"`    // downloaded bytes
	TorrentFiles   string            `json:"torrent_files" gorm:"type:text"` // JSON序列化的文件信息
	TorrentName    string            `json:"torrent_name"`                   // 种子名称
	M3U8FilePath   string            `json:"m3u8_file_path"`                 // M3U8文件路径
	Srts           string            `json:"srts" gorm:"type:text"`          // JSON序列化的字幕文件列表
	Segments       string            `json:"segments" gorm:"type:text"`      // JSON序列化的视频分片信息
	WorkerID       string            `json:"worker_id"`                      // 执行任务的worker节点ID
	Metadata       string            `json:"metadata" gorm:"type:text"`      // JSON序列化的额外元数据
	LastUpdateTime time.Time         `json:"last_update_time"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
	DeletedAt      gorm.DeletedAt    `json:"deleted_at" gorm:"index"`
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
	if t.Srts == "" {
		return []string{}, nil
	}

	var srts []string
	err := json.Unmarshal([]byte(t.Srts), &srts)
	return srts, err
}

// SetSrts 设置序列化的字幕文件列表
func (t *Task) SetSrts(srts []string) error {
	data, err := json.Marshal(srts)
	if err != nil {
		return err
	}
	t.Srts = string(data)
	return nil
}

// GetMetadata 获取反序列化的元数据
func (t *Task) GetMetadata() (map[string]interface{}, error) {
	if t.Metadata == "" {
		return make(map[string]interface{}), nil
	}

	var metadata map[string]interface{}
	err := json.Unmarshal([]byte(t.Metadata), &metadata)
	return metadata, err
}

// SetMetadata 设置序列化的元数据
func (t *Task) SetMetadata(metadata map[string]interface{}) error {
	data, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	t.Metadata = string(data)
	return nil
}

// GetSegments 获取反序列化的视频分片信息
func (t *Task) GetSegments() ([]string, error) {
	if t.Segments == "" {
		return []string{}, nil
	}

	var segments []string
	err := json.Unmarshal([]byte(t.Segments), &segments)
	return segments, err
}

// SetSegments 设置序列化的视频分片信息
func (t *Task) SetSegments(segments []string) error {
	data, err := json.Marshal(segments)
	if err != nil {
		return err
	}
	t.Segments = string(data)
	return nil
}

// WebRTCSession 表示WebRTC会话信息
type WebRTCSession struct {
	ID        uint           `json:"id" gorm:"primaryKey"`
	SessionID string         `json:"session_id" gorm:"uniqueIndex;not null"` // 会话ID
	TaskID    uint           `json:"task_id"`                                // 关联的任务ID
	ClientID  string         `json:"client_id"`                              // 客户端ID
	WorkerID  string         `json:"worker_id"`                              // Worker节点ID
	Status    string         `json:"status" gorm:"default:negotiating"`      // negotiating, established, closed
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`
}
