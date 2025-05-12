package services

import (
	"log"
	"strings"
	"time"

	"magnetm3u8/db"
	"magnetm3u8/models"

	"gorm.io/gorm"
)

// TorrentService 处理Torrent任务相关操作
type TorrentService struct{}

// NewTorrentService 创建新的TorrentService
func NewTorrentService() *TorrentService {
	return &TorrentService{}
}

// ValidateMagnetURL 验证磁力链接格式
func (s *TorrentService) ValidateMagnetURL(magnetURL string) bool {
	return strings.HasPrefix(magnetURL, "magnet:?xt=urn:btih:")
}

// CreateTask 创建新的下载任务
func (s *TorrentService) CreateTask(magnetURL string) (*models.TorrentTask, error) {
	// 验证磁力链接
	if !s.ValidateMagnetURL(magnetURL) {
		return nil, ErrInvalidMagnetURL
	}

	// 创建任务记录
	task := &models.TorrentTask{
		MagnetURL: magnetURL,
		Status:    "waiting", // 初始状态为等待
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// 保存到数据库
	if err := db.DB.Create(task).Error; err != nil {
		log.Printf("创建任务失败: %v", err)
		return nil, err
	}

	// 发送到服务B
	wsManager := GetWebSocketManager()
	if !wsManager.IsConnected() {
		// 更新任务状态为失败
		db.DB.Model(task).Update("status", "failed")
		return task, ErrNotConnected
	}

	// 发送磁力链接到服务B
	err := wsManager.SendMessage(MsgTypeMagnetSubmit, map[string]interface{}{
		"task_id":    task.ID,
		"magnet_url": magnetURL,
	})

	if err != nil {
		log.Printf("发送任务到服务B失败: %v", err)
		db.DB.Model(task).Update("status", "failed")
		return task, err
	}

	return task, nil
}

// GetTaskByID 根据ID获取任务
func (s *TorrentService) GetTaskByID(taskID uint) (*models.TorrentTask, error) {
	var task models.TorrentTask
	if err := db.DB.First(&task, taskID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrTaskNotFound
		}
		return nil, err
	}
	return &task, nil
}

// GetAllTasks 获取所有任务
func (s *TorrentService) GetAllTasks() ([]models.TorrentTask, error) {
	var tasks []models.TorrentTask
	if err := db.DB.Order("created_at desc").Find(&tasks).Error; err != nil {
		return nil, err
	}
	return tasks, nil
}

// UpdateTaskStatus 更新任务状态
func (s *TorrentService) UpdateTaskStatus(taskID uint, status string) error {
	result := db.DB.Model(&models.TorrentTask{}).Where("id = ?", taskID).Update("status", status)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrTaskNotFound
	}
	return nil
}

// GetTorrentFiles 获取Torrent文件列表
func (s *TorrentService) GetTorrentFiles(taskID uint) ([]models.TorrentFile, error) {
	var files []models.TorrentFile
	if err := db.DB.Where("task_id = ?", taskID).Find(&files).Error; err != nil {
		return nil, err
	}
	return files, nil
}

// SaveTorrentFiles 保存Torrent文件信息
func (s *TorrentService) SaveTorrentFiles(taskID uint, files []models.TorrentFile) error {
	// 开始事务
	tx := db.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 删除旧的文件记录
	if err := tx.Where("task_id = ?", taskID).Delete(&models.TorrentFile{}).Error; err != nil {
		tx.Rollback()
		return err
	}

	// 添加新的文件记录
	for i := range files {
		files[i].TaskID = taskID
		if err := tx.Create(&files[i]).Error; err != nil {
			tx.Rollback()
			return err
		}
	}

	// 提交事务
	return tx.Commit().Error
}

// UpdateDownloadProgress 更新下载进度
func (s *TorrentService) UpdateDownloadProgress(taskID uint, percentage float64, speed int64) error {
	var progress models.DownloadProgress
	result := db.DB.Where("task_id = ?", taskID).First(&progress)

	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			// 创建新的进度记录
			progress = models.DownloadProgress{
				TaskID:         taskID,
				Percentage:     percentage,
				DownloadSpeed:  speed,
				LastUpdateTime: time.Now(),
			}
			return db.DB.Create(&progress).Error
		}
		return result.Error
	}

	// 更新现有记录
	progress.Percentage = percentage
	progress.DownloadSpeed = speed
	progress.LastUpdateTime = time.Now()
	return db.DB.Save(&progress).Error
}

// GetDownloadProgress 获取下载进度
func (s *TorrentService) GetDownloadProgress(taskID uint) (*models.DownloadProgress, error) {
	var progress models.DownloadProgress
	if err := db.DB.Where("task_id = ?", taskID).First(&progress).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil // 返回nil表示没有进度记录
		}
		return nil, err
	}
	return &progress, nil
}

// SaveM3U8Info 保存M3U8信息
func (s *TorrentService) SaveM3U8Info(taskID uint, filePath string) error {
	var m3u8Info models.M3U8Info
	result := db.DB.Where("task_id = ?", taskID).First(&m3u8Info)

	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			// 创建新的M3U8记录
			m3u8Info = models.M3U8Info{
				TaskID:   taskID,
				FilePath: filePath,
			}
			return db.DB.Create(&m3u8Info).Error
		}
		return result.Error
	}

	// 更新现有记录
	m3u8Info.FilePath = filePath
	return db.DB.Save(&m3u8Info).Error
}

// GetM3U8Info 获取M3U8信息
func (s *TorrentService) GetM3U8Info(taskID uint) (*models.M3U8Info, error) {
	var m3u8Info models.M3U8Info
	if err := db.DB.Where("task_id = ?", taskID).First(&m3u8Info).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil // 返回nil表示没有M3U8记录
		}
		return nil, err
	}
	return &m3u8Info, nil
}
