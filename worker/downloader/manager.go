package downloader

import (
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"worker/database"
	"worker/models"

	"github.com/anacrolix/torrent"
)

// TaskStatus 任务状态
type TaskStatus string

const (
	StatusPending     TaskStatus = "pending"
	StatusDownloading TaskStatus = "downloading"
	StatusCompleted   TaskStatus = "completed"
	StatusError       TaskStatus = "error"
	StatusPaused      TaskStatus = "paused"
	StatusTranscoding TaskStatus = "transcoding"
	StatusReady       TaskStatus = "ready"
)

// Manager 下载管理器
type Manager struct {
	client                *torrent.Client
	activeTasks           map[string]*torrent.Torrent // 内存中的活跃任务（torrent实例）
	downloadPath          string
	workerID              string
	mutex                 sync.RWMutex
	statusChan            chan *models.Task
	maxTasks              int
	taskRepo              *database.TaskRepository
	externalStatusHandler func(*models.Task) // 外部状态处理器
}

// New 创建新的下载管理器
func New(downloadPath, workerID string) *Manager {
	return &Manager{
		activeTasks:           make(map[string]*torrent.Torrent),
		downloadPath:          downloadPath,
		workerID:              workerID,
		statusChan:            make(chan *models.Task, 100),
		maxTasks:              5,
		taskRepo:              database.NewTaskRepository(),
		externalStatusHandler: nil,
	}
}

// Start 启动下载管理器
func (m *Manager) Start() error {
	// 创建下载目录
	if err := os.MkdirAll(m.downloadPath, 0755); err != nil {
		return fmt.Errorf("failed to create download path: %v", err)
	}

	// 配置torrent客户端
	config := torrent.NewDefaultClientConfig()
	config.DataDir = m.downloadPath
	config.NoUpload = false
	config.Seed = true

	client, err := torrent.NewClient(config)
	if err != nil {
		return fmt.Errorf("failed to create torrent client: %v", err)
	}

	m.client = client

	// 启动状态监控
	go m.statusMonitor()

	// 恢复之前未完成的任务
	if err := m.restoreActiveTasks(); err != nil {
		log.Printf("Failed to restore active tasks: %v", err)
	}

	log.Printf("Download manager started, download path: %s", m.downloadPath)
	return nil
}

// Stop 停止下载管理器
func (m *Manager) Stop() {
	if m.client != nil {
		m.client.Close()
	}
	close(m.statusChan)
	log.Printf("Download manager stopped")
}

// StartDownload 开始下载任务
func (m *Manager) StartDownload(magnetURL string) (string, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// 检查任务数量限制
	activeCount, err := m.taskRepo.GetActiveTasksCount(m.workerID)
	if err != nil {
		return "", fmt.Errorf("failed to check active tasks: %v", err)
	}

	if activeCount >= int64(m.maxTasks) {
		return "", fmt.Errorf("maximum active downloads reached (%d)", m.maxTasks)
	}

	// 创建数据库任务记录
	task := &models.Task{
		TaskID:    generateTaskID(),
		MagnetURL: magnetURL,
		Status:    string(StatusPending),
		Progress:  0,
		WorkerID:  m.workerID,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// 设置空的元数据
	if err := task.SetMetadata(make(map[string]interface{})); err != nil {
		return "", fmt.Errorf("failed to set metadata: %v", err)
	}

	// 保存到数据库
	if err := m.taskRepo.Create(task); err != nil {
		return "", fmt.Errorf("failed to create task in database: %v", err)
	}

	// 开始下载
	go m.downloadTask(task)

	log.Printf("Started download task: %s", task.TaskID)
	return task.TaskID, nil
}

// GetTask 获取任务信息
func (m *Manager) GetTask(taskID string) (*models.Task, bool) {
	task, err := m.taskRepo.GetByTaskID(taskID)
	if err != nil {
		return nil, false
	}
	return task, true
}

// GetAllTasks 获取所有任务
func (m *Manager) GetAllTasks() []*models.Task {
	tasks, err := m.taskRepo.GetByWorkerID(m.workerID)
	if err != nil {
		log.Printf("Failed to get tasks from database: %v", err)
		return []*models.Task{}
	}

	// 转换为指针切片
	taskPtrs := make([]*models.Task, len(tasks))
	for i := range tasks {
		taskPtrs[i] = &tasks[i]
	}
	return taskPtrs
}

// PauseTask 暂停任务
func (m *Manager) PauseTask(taskID string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// 从内存中移除torrent实例
	if torrentInstance, exists := m.activeTasks[taskID]; exists {
		torrentInstance.Drop()
		delete(m.activeTasks, taskID)
	}

	// 更新数据库状态
	return m.taskRepo.UpdateStatus(taskID, string(StatusPaused))
}

// ResumeTask 恢复任务
func (m *Manager) ResumeTask(taskID string) error {
	task, err := m.taskRepo.GetByTaskID(taskID)
	if err != nil {
		return fmt.Errorf("task not found: %s", taskID)
	}

	if task.Status == string(StatusPaused) {
		go m.downloadTask(task)
	}

	return nil
}

// RemoveTask 删除任务
func (m *Manager) RemoveTask(taskID string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// 从内存中移除torrent实例
	if torrentInstance, exists := m.activeTasks[taskID]; exists {
		torrentInstance.Drop()
		delete(m.activeTasks, taskID)
	}

	// 从数据库删除
	return m.taskRepo.Delete(taskID)
}

// downloadTask 执行下载任务
func (m *Manager) downloadTask(task *models.Task) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Download task %s panicked: %v", task.TaskID, r)
			task.Status = string(StatusError)
			metadata, _ := task.GetMetadata()
			metadata["error"] = fmt.Sprintf("panic: %v", r)
			task.SetMetadata(metadata)
			m.taskRepo.Update(task)
			m.statusChan <- task
		}
	}()

	log.Printf("Starting download for task %s: %s", task.TaskID, task.MagnetURL)

	// 添加torrent
	t, err := m.client.AddMagnet(task.MagnetURL)
	if err != nil {
		log.Printf("Failed to add magnet for task %s: %v", task.TaskID, err)
		task.Status = string(StatusError)
		metadata, _ := task.GetMetadata()
		metadata["error"] = err.Error()
		task.SetMetadata(metadata)
		m.taskRepo.Update(task)
		m.statusChan <- task
		return
	}

	// 为种子添加更多的 trackers 以提高发现速度
	publicTrackers := []string{
		"udp://tracker.opentrackr.org:1337/announce",
		"udp://tracker.openbittorrent.com:6969/announce",
		"udp://open.stealth.si:80/announce",
		"udp://exodus.desync.com:6969/announce",
		"udp://explodie.org:6969/announce",
		"http://tracker.opentrackr.org:1337/announce",
		"http://tracker.openbittorrent.com:80/announce",
		"udp://tracker.torrent.eu.org:451/announce",
		"udp://tracker.moeking.me:6969/announce",
		"udp://bt.oiyo.tk:6969/announce",
		"https://tracker.nanoha.org:443/announce",
		"https://tracker.lilithraws.org:443/announce",
	}
	for _, tracker := range publicTrackers {
		t.AddTrackers([][]string{{tracker}})
	}

	// 保存torrent实例到内存
	m.mutex.Lock()
	m.activeTasks[task.TaskID] = t
	m.mutex.Unlock()

	// 更新任务状态为下载中
	task.Status = string(StatusDownloading)
	task.UpdatedAt = time.Now()
	m.taskRepo.Update(task)
	m.statusChan <- task

	// 等待torrent信息
	<-t.GotInfo()

	// 更新任务信息
	task.Size = t.Length()
	task.TorrentName = t.Name()
	
	// 保存文件信息
	files := make([]models.TorrentFileInfo, len(t.Files()))
	fileNames := make([]string, len(t.Files()))
	for i, file := range t.Files() {
		files[i] = models.TorrentFileInfo{
			FileName:   file.DisplayPath(),
			FileSize:   file.Length(),
			FilePath:   file.Path(),
			IsSelected: true,
		}
		fileNames[i] = file.Path()
	}
	task.SetTorrentFiles(files)
	m.taskRepo.Update(task)
	
	log.Printf("Got torrent info for task %s: %s, size: %d bytes", task.TaskID, t.Name(), task.Size)

	// 开始下载所有文件
	t.DownloadAll()

	// 监控下载进度
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// 从数据库重新获取任务状态，以防被外部暂停
			currentTask, err := m.taskRepo.GetByTaskID(task.TaskID)
			if err != nil {
				log.Printf("Failed to get task status: %v", err)
				return
			}

			if currentTask.Status != string(StatusDownloading) {
				return
			}

			// 更新进度
			downloaded := t.BytesCompleted()
			progress := 0
			if task.Size > 0 {
				progress = int((downloaded * 100) / task.Size)
			}
			
			// 计算速度（简单实现）
			stats := t.Stats()
			speed := int64(stats.BytesReadData.Int64())

			// 更新数据库
			m.taskRepo.UpdateProgress(task.TaskID, progress, speed, downloaded)

			// 更新任务对象用于发送状态
			task.Progress = progress
			task.Speed = speed
			task.Downloaded = downloaded
			task.UpdatedAt = time.Now()

			// 检查是否完成
			if progress >= 100 {
				task.Status = string(StatusCompleted)
				task.UpdatedAt = time.Now()
				m.taskRepo.Update(task)
				log.Printf("Download completed for task %s", task.TaskID)
				
				// 从活跃任务中移除
				m.mutex.Lock()
				delete(m.activeTasks, task.TaskID)
				m.mutex.Unlock()
				
				m.statusChan <- task
				return
			}

			// 发送状态更新
			m.statusChan <- task

		default:
			time.Sleep(100 * time.Millisecond)
		}
	}
}

// restoreActiveTasks 恢复之前未完成的任务
func (m *Manager) restoreActiveTasks() error {
	tasks, err := m.taskRepo.GetByStatus(string(StatusDownloading))
	if err != nil {
		return err
	}

	for _, task := range tasks {
		log.Printf("Restoring active task: %s", task.TaskID)
		go m.downloadTask(&task)
	}

	return nil
}

// statusMonitor 状态监控
func (m *Manager) statusMonitor() {
	for task := range m.statusChan {
		// 记录状态变化
		log.Printf("Task %s status: %s, progress: %d%%", task.TaskID, task.Status, task.Progress)
		
		// 如果有外部的状态处理器，调用它
		if m.externalStatusHandler != nil {
			m.externalStatusHandler(task)
		}
	}
}

// GetStatusChannel 获取状态通道
func (m *Manager) GetStatusChannel() <-chan *models.Task {
	return m.statusChan
}

// SetExternalStatusHandler 设置外部状态处理器
func (m *Manager) SetExternalStatusHandler(handler func(*models.Task)) {
	m.externalStatusHandler = handler
}

// generateTaskID 生成任务ID
func generateTaskID() string {
	return fmt.Sprintf("task_%d", time.Now().UnixNano())
}