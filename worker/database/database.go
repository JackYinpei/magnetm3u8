package database

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"time"

	"worker/models"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/driver/sqlite"
	_ "modernc.org/sqlite" // 使用纯Go SQLite实现
)

var (
	// DB 数据库连接实例
	DB *gorm.DB
)

// Initialize 初始化数据库
func Initialize(dataPath string) error {
	// 确保数据目录存在
	var err error
	// 配置GORM
	config := &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent), // 设置为Silent减少日志输出
	}
	
	// 使用纯Go SQLite实现
	dbPath := filepath.Join(dataPath, "worker.db")
	// 可选的模式配置：
	// WAL模式（推荐）：_pragma=journal_mode(WAL) - 高并发性能，会产生.wal和.shm文件
	// DELETE模式：_pragma=journal_mode(DELETE) - 传统模式，只有一个.db文件但性能较差
	dsn := fmt.Sprintf("file:%s?cache=shared&mode=rwc&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(1)", dbPath)
	
	// 先打开原生SQL连接以确保使用modernc.org/sqlite
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return fmt.Errorf("failed to open sqlite database: %v", err)
	}
	
	// 使用现有连接创建GORM实例
	DB, err = gorm.Open(sqlite.Dialector{
		Conn: sqlDB,
	}, config)
	if err != nil {
		return fmt.Errorf("failed to open database: %v", err)
	}

	// 自动迁移数据库表
	err = DB.AutoMigrate(&models.Task{}, &models.WebRTCSession{})
	if err != nil {
		return fmt.Errorf("failed to migrate database: %v", err)
	}

	// 配置数据库连接池
	sqlDBConn, err := DB.DB()
	if err != nil {
		return fmt.Errorf("failed to get underlying sql.DB: %v", err)
	}
	
	// 设置最大空闲连接数
	sqlDBConn.SetMaxIdleConns(10)
	// 设置最大打开连接数
	sqlDBConn.SetMaxOpenConns(100)
	// 设置连接最大生存时间
	sqlDBConn.SetConnMaxLifetime(time.Hour)

	return nil
}

// Close 关闭数据库连接
func Close() error {
	if DB != nil {
		sqlDB, err := DB.DB()
		if err != nil {
			return err
		}
		return sqlDB.Close()
	}
	return nil
}

// GetDB 获取数据库实例
func GetDB() *gorm.DB {
	return DB
}

// TaskRepository 任务数据仓库
type TaskRepository struct {
	db *gorm.DB
}

// NewTaskRepository 创建任务仓库
func NewTaskRepository() *TaskRepository {
	return &TaskRepository{db: DB}
}

// Create 创建任务
func (r *TaskRepository) Create(task *models.Task) error {
	return r.db.Create(task).Error
}

// GetByTaskID 根据TaskID获取任务
func (r *TaskRepository) GetByTaskID(taskID string) (*models.Task, error) {
	var task models.Task
	err := r.db.Where("task_id = ?", taskID).First(&task).Error
	if err != nil {
		return nil, err
	}
	return &task, nil
}

// GetAll 获取所有任务
func (r *TaskRepository) GetAll() ([]models.Task, error) {
	var tasks []models.Task
	err := r.db.Find(&tasks).Error
	return tasks, err
}

// GetByWorkerID 根据WorkerID获取任务列表
func (r *TaskRepository) GetByWorkerID(workerID string) ([]models.Task, error) {
	var tasks []models.Task
	err := r.db.Where("worker_id = ?", workerID).Find(&tasks).Error
	return tasks, err
}

// GetByStatus 根据状态获取任务列表
func (r *TaskRepository) GetByStatus(status string) ([]models.Task, error) {
	var tasks []models.Task
	err := r.db.Where("status = ?", status).Find(&tasks).Error
	return tasks, err
}

// Update 更新任务
func (r *TaskRepository) Update(task *models.Task) error {
	return r.db.Save(task).Error
}

// UpdateStatus 更新任务状态
func (r *TaskRepository) UpdateStatus(taskID string, status string) error {
	return r.db.Model(&models.Task{}).Where("task_id = ?", taskID).Update("status", status).Error
}

// UpdateProgress 更新任务进度
func (r *TaskRepository) UpdateProgress(taskID string, progress int, speed int64, downloaded int64) error {
	updates := map[string]interface{}{
		"progress":         progress,
		"speed":           speed,
		"downloaded":      downloaded,
		"last_update_time": time.Now(),
	}
	return r.db.Model(&models.Task{}).Where("task_id = ?", taskID).Updates(updates).Error
}

// Delete 删除任务
func (r *TaskRepository) Delete(taskID string) error {
	return r.db.Where("task_id = ?", taskID).Delete(&models.Task{}).Error
}

// GetActiveTasksCount 获取活跃任务数量
func (r *TaskRepository) GetActiveTasksCount(workerID string) (int64, error) {
	var count int64
	err := r.db.Model(&models.Task{}).Where(
		"worker_id = ? AND status IN (?)", 
		workerID, 
		[]string{"pending", "downloading", "transcoding"},
	).Count(&count).Error
	return count, err
}

// WebRTCSessionRepository WebRTC会话数据仓库
type WebRTCSessionRepository struct {
	db *gorm.DB
}

// NewWebRTCSessionRepository 创建WebRTC会话仓库
func NewWebRTCSessionRepository() *WebRTCSessionRepository {
	return &WebRTCSessionRepository{db: DB}
}

// Create 创建会话
func (r *WebRTCSessionRepository) Create(session *models.WebRTCSession) error {
	return r.db.Create(session).Error
}

// GetBySessionID 根据SessionID获取会话
func (r *WebRTCSessionRepository) GetBySessionID(sessionID string) (*models.WebRTCSession, error) {
	var session models.WebRTCSession
	err := r.db.Where("session_id = ?", sessionID).First(&session).Error
	if err != nil {
		return nil, err
	}
	return &session, nil
}

// UpdateStatus 更新会话状态
func (r *WebRTCSessionRepository) UpdateStatus(sessionID string, status string) error {
	return r.db.Model(&models.WebRTCSession{}).Where("session_id = ?", sessionID).Update("status", status).Error
}

// Delete 删除会话
func (r *WebRTCSessionRepository) Delete(sessionID string) error {
	return r.db.Where("session_id = ?", sessionID).Delete(&models.WebRTCSession{}).Error
}

// DeleteExpired 删除过期会话
func (r *WebRTCSessionRepository) DeleteExpired() error {
	cutoffTime := time.Now().Add(-1 * time.Hour)
	return r.db.Where("created_at < ?", cutoffTime).Delete(&models.WebRTCSession{}).Error
}