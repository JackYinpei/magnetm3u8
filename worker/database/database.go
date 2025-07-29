package database

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"worker/models"

	"go.etcd.io/bbolt"
)

var (
	// DB 数据库连接实例
	DB *bbolt.DB
	
	// Bucket names
	TasksBucket    = []byte("tasks")
	SessionsBucket = []byte("sessions")
)

// Initialize 初始化数据库
func Initialize(dataPath string) error {
	// 确保数据目录存在
	dbPath := filepath.Join(dataPath, "worker.db")
	
	var err error
	DB, err = bbolt.Open(dbPath, 0600, &bbolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		return fmt.Errorf("failed to open database: %v", err)
	}

	// 创建必要的buckets
	err = DB.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(TasksBucket)
		if err != nil {
			return fmt.Errorf("create tasks bucket: %s", err)
		}
		
		_, err = tx.CreateBucketIfNotExists(SessionsBucket)
		if err != nil {
			return fmt.Errorf("create sessions bucket: %s", err)
		}
		
		return nil
	})
	
	if err != nil {
		return fmt.Errorf("failed to create buckets: %v", err)
	}

	return nil
}

// Close 关闭数据库连接
func Close() error {
	if DB != nil {
		return DB.Close()
	}
	return nil
}

// GetDB 获取数据库实例
func GetDB() *bbolt.DB {
	return DB
}

// TaskRepository 任务数据仓库
type TaskRepository struct {
	db *bbolt.DB
}

// NewTaskRepository 创建任务仓库
func NewTaskRepository() *TaskRepository {
	return &TaskRepository{db: DB}
}

// Create 创建任务
func (r *TaskRepository) Create(task *models.Task) error {
	return r.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(TasksBucket)
		
		// 设置创建时间
		task.CreatedAt = time.Now()
		task.UpdatedAt = time.Now()
		
		// 序列化任务
		data, err := json.Marshal(task)
		if err != nil {
			return err
		}
		
		return b.Put([]byte(task.TaskID), data)
	})
}

// GetByTaskID 根据TaskID获取任务
func (r *TaskRepository) GetByTaskID(taskID string) (*models.Task, error) {
	var task models.Task
	
	err := r.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(TasksBucket)
		data := b.Get([]byte(taskID))
		if data == nil {
			return fmt.Errorf("task not found")
		}
		
		return json.Unmarshal(data, &task)
	})
	
	if err != nil {
		return nil, err
	}
	
	return &task, nil
}

// GetAll 获取所有任务
func (r *TaskRepository) GetAll() ([]models.Task, error) {
	var tasks []models.Task
	
	err := r.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(TasksBucket)
		
		return b.ForEach(func(k, v []byte) error {
			var task models.Task
			if err := json.Unmarshal(v, &task); err != nil {
				return err
			}
			tasks = append(tasks, task)
			return nil
		})
	})
	
	if err != nil {
		return nil, err
	}
	
	return tasks, nil
}

// GetByWorkerID 根据WorkerID获取任务列表
func (r *TaskRepository) GetByWorkerID(workerID string) ([]models.Task, error) {
	var tasks []models.Task
	
	err := r.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(TasksBucket)
		
		return b.ForEach(func(k, v []byte) error {
			var task models.Task
			if err := json.Unmarshal(v, &task); err != nil {
				return err
			}
			
			if task.WorkerID == workerID {
				tasks = append(tasks, task)
			}
			
			return nil
		})
	})
	
	if err != nil {
		return nil, err
	}
	
	return tasks, nil
}

// GetByStatus 根据状态获取任务列表
func (r *TaskRepository) GetByStatus(status string) ([]models.Task, error) {
	var tasks []models.Task
	
	err := r.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(TasksBucket)
		
		return b.ForEach(func(k, v []byte) error {
			var task models.Task
			if err := json.Unmarshal(v, &task); err != nil {
				return err
			}
			
			if task.Status == status {
				tasks = append(tasks, task)
			}
			
			return nil
		})
	})
	
	if err != nil {
		return nil, err
	}
	
	return tasks, nil
}

// Update 更新任务
func (r *TaskRepository) Update(task *models.Task) error {
	return r.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(TasksBucket)
		
		// 更新时间
		task.UpdatedAt = time.Now()
		
		// 序列化任务
		data, err := json.Marshal(task)
		if err != nil {
			return err
		}
		
		return b.Put([]byte(task.TaskID), data)
	})
}

// UpdateStatus 更新任务状态
func (r *TaskRepository) UpdateStatus(taskID string, status string) error {
	return r.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(TasksBucket)
		data := b.Get([]byte(taskID))
		if data == nil {
			return fmt.Errorf("task not found")
		}
		
		var task models.Task
		if err := json.Unmarshal(data, &task); err != nil {
			return err
		}
		
		task.Status = status
		task.UpdatedAt = time.Now()
		
		data, err := json.Marshal(task)
		if err != nil {
			return err
		}
		
		return b.Put([]byte(taskID), data)
	})
}

// UpdateProgress 更新任务进度
func (r *TaskRepository) UpdateProgress(taskID string, progress int, speed int64, downloaded int64) error {
	return r.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(TasksBucket)
		data := b.Get([]byte(taskID))
		if data == nil {
			return fmt.Errorf("task not found")
		}
		
		var task models.Task
		if err := json.Unmarshal(data, &task); err != nil {
			return err
		}
		
		task.Progress = progress
		task.Speed = speed
		task.Downloaded = downloaded
		task.LastUpdateTime = time.Now()
		task.UpdatedAt = time.Now()
		
		data, err := json.Marshal(task)
		if err != nil {
			return err
		}
		
		return b.Put([]byte(taskID), data)
	})
}

// Delete 删除任务
func (r *TaskRepository) Delete(taskID string) error {
	return r.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(TasksBucket)
		return b.Delete([]byte(taskID))
	})
}

// GetActiveTasksCount 获取活跃任务数量
func (r *TaskRepository) GetActiveTasksCount(workerID string) (int64, error) {
	var count int64
	
	err := r.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(TasksBucket)
		
		return b.ForEach(func(k, v []byte) error {
			var task models.Task
			if err := json.Unmarshal(v, &task); err != nil {
				return err
			}
			
			if task.WorkerID == workerID && 
			   (task.Status == "pending" || task.Status == "downloading" || task.Status == "transcoding") {
				count++
			}
			
			return nil
		})
	})
	
	return count, err
}

// WebRTCSessionRepository WebRTC会话数据仓库
type WebRTCSessionRepository struct {
	db *bbolt.DB
}

// NewWebRTCSessionRepository 创建WebRTC会话仓库
func NewWebRTCSessionRepository() *WebRTCSessionRepository {
	return &WebRTCSessionRepository{db: DB}
}

// Create 创建会话
func (r *WebRTCSessionRepository) Create(session *models.WebRTCSession) error {
	return r.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(SessionsBucket)
		
		// 设置创建时间
		session.CreatedAt = time.Now()
		session.UpdatedAt = time.Now()
		
		// 序列化会话
		data, err := json.Marshal(session)
		if err != nil {
			return err
		}
		
		return b.Put([]byte(session.SessionID), data)
	})
}

// GetBySessionID 根据SessionID获取会话
func (r *WebRTCSessionRepository) GetBySessionID(sessionID string) (*models.WebRTCSession, error) {
	var session models.WebRTCSession
	
	err := r.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(SessionsBucket)
		data := b.Get([]byte(sessionID))
		if data == nil {
			return fmt.Errorf("session not found")
		}
		
		return json.Unmarshal(data, &session)
	})
	
	if err != nil {
		return nil, err
	}
	
	return &session, nil
}

// UpdateStatus 更新会话状态
func (r *WebRTCSessionRepository) UpdateStatus(sessionID string, status string) error {
	return r.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(SessionsBucket)
		data := b.Get([]byte(sessionID))
		if data == nil {
			return fmt.Errorf("session not found")
		}
		
		var session models.WebRTCSession
		if err := json.Unmarshal(data, &session); err != nil {
			return err
		}
		
		session.Status = status
		session.UpdatedAt = time.Now()
		
		data, err := json.Marshal(session)
		if err != nil {
			return err
		}
		
		return b.Put([]byte(sessionID), data)
	})
}

// Delete 删除会话
func (r *WebRTCSessionRepository) Delete(sessionID string) error {
	return r.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(SessionsBucket)
		return b.Delete([]byte(sessionID))
	})
}

// DeleteExpired 删除过期会话
func (r *WebRTCSessionRepository) DeleteExpired() error {
	cutoffTime := time.Now().Add(-1 * time.Hour)
	
	return r.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(SessionsBucket)
		var toDelete [][]byte
		
		// 收集过期的会话
		err := b.ForEach(func(k, v []byte) error {
			var session models.WebRTCSession
			if err := json.Unmarshal(v, &session); err != nil {
				return err
			}
			
			if session.CreatedAt.Before(cutoffTime) {
				toDelete = append(toDelete, k)
			}
			
			return nil
		})
		
		if err != nil {
			return err
		}
		
		// 删除过期的会话
		for _, key := range toDelete {
			if err := b.Delete(key); err != nil {
				return err
			}
		}
		
		return nil
	})
}