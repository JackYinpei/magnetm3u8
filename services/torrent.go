package services

import (
	"fmt"
	"time"

	"gorm.io/gorm"

	"magnetm3u8/db"
	"magnetm3u8/models"
)

// TorrentService 处理Torrent任务相关操作
type TorrentService struct {
	DB *gorm.DB
}

// NewTorrentService 创建新的TorrentService
func NewTorrentService() *TorrentService {
	return &TorrentService{
		DB: db.DB,
	}
}

// CreateTask 创建一个新的磁力下载任务
func (s *TorrentService) CreateTask(magnetURL string) (*models.Task, error) {
	if magnetURL == "" {
		return nil, fmt.Errorf("magnet URL cannot be empty")
	}

	task := &models.Task{
		MagnetURL:      magnetURL,
		Status:         "waiting",
		Percentage:     0,
		DownloadSpeed:  0,
		LastUpdateTime: time.Now(),
	}

	if err := s.DB.Create(task).Error; err != nil {
		return nil, fmt.Errorf("failed to create task: %v", err)
	}

	return task, nil
}

// ValidateMagnetURL 验证磁力链接格式（简单验证）
func (s *TorrentService) ValidateMagnetURL(magnetURL string) error {
	if magnetURL == "" {
		return fmt.Errorf("magnet URL cannot be empty")
	}

	if len(magnetURL) < 8 || magnetURL[:8] != "magnet:?" {
		return fmt.Errorf("invalid magnet URL format")
	}

	return nil
}

// GetTaskByID 根据ID获取任务
func (s *TorrentService) GetTaskByID(taskID uint) (*models.Task, error) {
	var task models.Task
	if err := s.DB.First(&task, taskID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("task not found")
		}
		return nil, fmt.Errorf("failed to get task: %v", err)
	}
	return &task, nil
}

// GetAllTasks 获取所有任务
func (s *TorrentService) GetAllTasks() ([]models.Task, error) {
	var tasks []models.Task
	if err := s.DB.Order("created_at desc").Find(&tasks).Error; err != nil {
		return nil, fmt.Errorf("failed to get tasks: %v", err)
	}
	return tasks, nil
}

// UpdateTaskStatus 更新任务状态
func (s *TorrentService) UpdateTaskStatus(taskID uint, status string) error {
	result := s.DB.Model(&models.Task{}).Where("id = ?", taskID).Update("status", status)
	if result.Error != nil {
		return fmt.Errorf("failed to update task status: %v", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("task not found")
	}
	return nil
}

// GetTorrentFiles 获取Torrent文件列表
func (s *TorrentService) GetTorrentFiles(taskID uint) ([]models.TorrentFileInfo, error) {
	task, err := s.GetTaskByID(taskID)
	if err != nil {
		return nil, err
	}

	return task.GetTorrentFiles()
}

// SaveTorrentFiles 保存Torrent文件信息
func (s *TorrentService) SaveTorrentFiles(taskID uint, files []models.TorrentFileInfo) error {
	task, err := s.GetTaskByID(taskID)
	if err != nil {
		return err
	}

	if err := task.SetTorrentFiles(files); err != nil {
		return fmt.Errorf("failed to serialize torrent files: %v", err)
	}

	if err := s.DB.Model(task).Update("torrent_files", task.TorrentFiles).Error; err != nil {
		return fmt.Errorf("failed to save torrent files: %v", err)
	}

	return nil
}

// UpdateDownloadProgress 更新下载进度
func (s *TorrentService) UpdateDownloadProgress(taskID uint, percentage float64, speed int64) error {
	updates := map[string]interface{}{
		"percentage":       percentage,
		"download_speed":   speed,
		"last_update_time": time.Now(),
	}

	result := s.DB.Model(&models.Task{}).Where("id = ?", taskID).Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("failed to update download progress: %v", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("task not found")
	}
	return nil
}

// GetDownloadProgress 获取下载进度
func (s *TorrentService) GetDownloadProgress(taskID uint) (map[string]interface{}, error) {
	task, err := s.GetTaskByID(taskID)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"percentage":       task.Percentage,
		"download_speed":   task.DownloadSpeed,
		"last_update_time": task.LastUpdateTime,
	}, nil
}

// SaveM3U8Info 保存M3U8信息
func (s *TorrentService) SaveM3U8Info(taskID uint, filePath string) error {
	result := s.DB.Model(&models.Task{}).Where("id = ?", taskID).Update("m3u8_file_path", filePath)
	if result.Error != nil {
		return fmt.Errorf("failed to save M3U8 info: %v", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("task not found")
	}
	return nil
}

// GetM3U8Info 获取M3U8信息
func (s *TorrentService) GetM3U8Info(taskID uint) (map[string]interface{}, error) {
	task, err := s.GetTaskByID(taskID)
	if err != nil {
		return nil, err
	}

	if task.M3U8FilePath == "" {
		return nil, fmt.Errorf("M3U8 file not found for task")
	}

	return map[string]interface{}{
		"file_path": task.M3U8FilePath,
	}, nil
}
