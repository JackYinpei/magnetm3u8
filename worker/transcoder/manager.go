package transcoder

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// TaskStatus 转码任务状态
type TaskStatus string

const (
	TranscodeStatusPending    TaskStatus = "pending"
	TranscodeStatusProcessing TaskStatus = "processing"
	TranscodeStatusCompleted  TaskStatus = "completed"
	TranscodeStatusError      TaskStatus = "error"
)

// TranscodeTask 转码任务
type TranscodeTask struct {
	ID         string            `json:"id"`
	InputPath  string            `json:"input_path"`
	OutputPath string            `json:"output_path"`
	Status     TaskStatus        `json:"status"`
	Progress   int               `json:"progress"`
	M3U8Path   string            `json:"m3u8_path"`
	Subtitles  []string          `json:"subtitles"`
	CreatedAt  time.Time         `json:"created_at"`
	UpdatedAt  time.Time         `json:"updated_at"`
	Metadata   map[string]string `json:"metadata"`
}

// Manager 转码管理器 - 重构后的版本
type Manager struct {
	inputDir    string
	outputDir   string
	tasks       map[string]*TranscodeTask
	mutex       sync.RWMutex
	statusChan  chan *TranscodeTask
	maxTasks    int
	// 引用原有的转码器
	legacyManager *LegacyManager
}

// LegacyManager 包装原有的转码器
type LegacyManager struct {
	inputDir   string
	outputDir  string
	activeJobs map[uint]bool
	mu         sync.RWMutex
}

// New 创建新的转码管理器
func New(inputDir, outputDir string) *Manager {
	// 创建输出目录
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		log.Printf("Failed to create output directory: %v", err)
	}

	legacyMgr := &LegacyManager{
		inputDir:   inputDir,
		outputDir:  outputDir,
		activeJobs: make(map[uint]bool),
	}

	return &Manager{
		inputDir:      inputDir,
		outputDir:     outputDir,
		tasks:         make(map[string]*TranscodeTask),
		statusChan:    make(chan *TranscodeTask, 100),
		maxTasks:      3,
		legacyManager: legacyMgr,
	}
}

// Start 启动转码管理器
func (m *Manager) Start() error {
	log.Printf("Transcoder manager started, input: %s, output: %s", m.inputDir, m.outputDir)
	return nil
}

// Stop 停止转码管理器
func (m *Manager) Stop() {
	close(m.statusChan)
	log.Printf("Transcoder manager stopped")
}

// StartTranscode 开始转码任务
func (m *Manager) StartTranscode(inputPath string) (string, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// 检查任务数量限制
	activeCount := 0
	for _, task := range m.tasks {
		if task.Status == TranscodeStatusProcessing || task.Status == TranscodeStatusPending {
			activeCount++
		}
	}

	if activeCount >= m.maxTasks {
		return "", fmt.Errorf("maximum active transcodes reached (%d)", m.maxTasks)
	}

	// 创建任务
	taskID := uuid.New().String()
	task := &TranscodeTask{
		ID:        taskID,
		InputPath: inputPath,
		Status:    TranscodeStatusPending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Metadata:  make(map[string]string),
	}

	m.tasks[taskID] = task

	// 开始转码
	go m.transcodeTask(task)

	log.Printf("Started transcode task: %s for file: %s", taskID, inputPath)
	return taskID, nil
}

// GetTask 获取任务信息
func (m *Manager) GetTask(taskID string) (*TranscodeTask, bool) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	task, exists := m.tasks[taskID]
	return task, exists
}

// GetAllTasks 获取所有任务
func (m *Manager) GetAllTasks() []*TranscodeTask {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	tasks := make([]*TranscodeTask, 0, len(m.tasks))
	for _, task := range m.tasks {
		tasks = append(tasks, task)
	}
	return tasks
}

// transcodeTask 执行转码任务
func (m *Manager) transcodeTask(task *TranscodeTask) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Transcode task %s panicked: %v", task.ID, r)
			task.Status = TranscodeStatusError
			task.Metadata["error"] = fmt.Sprintf("panic: %v", r)
			task.UpdatedAt = time.Now()
			m.statusChan <- task
		}
	}()

	log.Printf("Starting transcode for task %s: %s", task.ID, task.InputPath)

	task.Status = TranscodeStatusProcessing
	task.UpdatedAt = time.Now()
	m.statusChan <- task

	// 使用legacy manager进行转码
	// 生成一个临时的uint ID给legacy系统使用
	legacyID := uint(time.Now().Unix() % 1000000)

	m3u8Path, outputDir, err := m.legacyManager.Transcode(legacyID, task.InputPath)
	if err != nil {
		log.Printf("Transcode failed for task %s: %v", task.ID, err)
		task.Status = TranscodeStatusError
		task.Metadata["error"] = err.Error()
		task.UpdatedAt = time.Now()
		m.statusChan <- task
		return
	}

	// 更新任务信息
	task.M3U8Path = m3u8Path
	task.OutputPath = outputDir
	task.Progress = 100
	task.Status = TranscodeStatusCompleted
	task.UpdatedAt = time.Now()

	// 查找字幕文件
	subtitles, err := m.findSubtitleFiles(outputDir)
	if err != nil {
		log.Printf("Failed to find subtitle files for task %s: %v", task.ID, err)
	} else {
		task.Subtitles = subtitles
	}

	log.Printf("Transcode completed for task %s: %s", task.ID, m3u8Path)
	m.statusChan <- task
}

// findSubtitleFiles 查找字幕文件
func (m *Manager) findSubtitleFiles(dir string) ([]string, error) {
	var subtitles []string

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() {
			ext := filepath.Ext(path)
			if ext == ".srt" || ext == ".vtt" {
				subtitles = append(subtitles, path)
			}
		}
		return nil
	})

	return subtitles, err
}

// GetStatusChannel 获取状态通道
func (m *Manager) GetStatusChannel() <-chan *TranscodeTask {
	return m.statusChan
}

// === Legacy Manager 方法 ===

// Transcode 原有的转码方法
func (lm *LegacyManager) Transcode(taskID uint, inputPath string) (string, string, error) {
	// 检查文件是否存在
	if _, err := os.Stat(inputPath); os.IsNotExist(err) {
		return "", "", fmt.Errorf("输入文件不存在: %s", inputPath)
	}

	// 获取转码的这个文件的纯名字
	filenameOnly := filepath.Base(inputPath)
	if ext := filepath.Ext(filenameOnly); ext != "" {
		filenameOnly = filenameOnly[:len(filenameOnly)-len(ext)]
	}

	// 创建任务特定的输出目录
	taskDir := filepath.Join(lm.outputDir, filenameOnly)
	if err := os.MkdirAll(taskDir, 0755); err != nil {
		return "", "", fmt.Errorf("创建任务输出目录失败: %w", err)
	}

	// 标记任务为活跃
	lm.mu.Lock()
	lm.activeJobs[taskID] = true
	lm.mu.Unlock()

	// 清理函数
	defer func() {
		lm.mu.Lock()
		delete(lm.activeJobs, taskID)
		lm.mu.Unlock()
	}()

	log.Printf("开始处理任务 %d: %s -> %s", taskID, inputPath, taskDir)

	// 使用默认HLS配置
	config := DefaultHLSConfig()

	// 对MKV文件启用字幕提取
	ext := strings.ToLower(filepath.Ext(inputPath))
	if ext == ".mkv" {
		config.ExtractSubtitles = true
		log.Printf("检测到MKV文件，启用字幕提取功能")
	}

	// 进行HLS切片处理(不做转码)
	m3u8Path, err := ConvertToHLS(inputPath, taskDir, config)
	if err != nil {
		return "", "", fmt.Errorf("HLS转码失败: %w", err)
	}

	// 处理字幕文件
	subtitles, err := lm.ConvertSubtitle(taskDir, filepath.Dir(inputPath))
	if err != nil {
		log.Printf("字幕处理失败: %v", err)
	} else {
		log.Printf("处理了 %d 个字幕文件", len(subtitles))
	}

	log.Printf("处理完成: %s", m3u8Path)
	return m3u8Path, taskDir, nil
}

// ConvertSubtitle 原有的字幕转换方法（简化版）
func (lm *LegacyManager) ConvertSubtitle(taskDir string, downloadPath string) ([]string, error) {
	// 支持的字幕扩展名
	subtitleExts := map[string]bool{
		".srt": true,
		".vtt": true,
		".ass": true,
		".ssa": true,
		".sub": true,
		".txt": true,
	}

	targetSrts := make([]string, 0)

	// 遍历downloadPath下所有文件
	err := filepath.Walk(downloadPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		
		ext := filepath.Ext(info.Name())
		if !subtitleExts[ext] {
			return nil
		}

		// 目标srt文件名
		baseName := info.Name()[:len(info.Name())-len(ext)]
		targetSrt := filepath.Join(taskDir, baseName+".srt")

		// 复制字幕文件
		if err := copyFile(path, targetSrt); err != nil {
			log.Printf("复制字幕文件失败: %s -> %s, err: %v", path, targetSrt, err)
		} else {
			log.Printf("已复制字幕文件: %s -> %s", path, targetSrt)
			targetSrts = append(targetSrts, targetSrt)
		}

		return nil
	})

	return targetSrts, err
}

// copyFile 复制文件的辅助函数
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = dstFile.ReadFrom(srcFile)
	return err
}

// HLSConfig 配置HLS转换参数
type HLSConfig struct {
	SegmentDuration  int    // 片段时长（秒）
	PlaylistType     string // 播放列表类型（event或vod）
	ExtractSubtitles bool   // 是否提取字幕文件
}

// DefaultHLSConfig 返回默认的HLS配置
func DefaultHLSConfig() HLSConfig {
	return HLSConfig{
		SegmentDuration:  10,
		PlaylistType:     "vod",
		ExtractSubtitles: false,
	}
}

// ConvertToHLS 将视频文件转换为HLS格式，不进行转码，只做切片
func ConvertToHLS(inputPath string, outputDir string, config HLSConfig) (string, error) {
	// 检查输入文件是否存在
	if _, err := os.Stat(inputPath); os.IsNotExist(err) {
		return "", fmt.Errorf("输入文件不存在: %s", err)
	}

	// 构建输出文件路径
	outputName := "index.m3u8"
	outputPath := filepath.Join(outputDir, outputName)

	// 检查输出文件是否已存在
	if _, err := os.Stat(outputPath); err == nil {
		log.Println("输出文件已存在，返回输出文件路径: ", outputPath)
		return outputPath, nil
	}

	// 确保输出目录存在
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("创建输出目录失败: %s", err)
	}

	// 如果启用了字幕提取，先提取字幕
	if config.ExtractSubtitles {
		if err := extractSubtitles(inputPath, outputDir); err != nil {
			log.Printf("警告: 字幕提取失败: %s", err)
			// 继续处理，不因字幕提取失败而中断主流程
		}
	}

	// 构建基本的FFmpeg命令，使用-c copy只做切片不做转码
	args := []string{
		"-i", inputPath,
		"-c", "copy", // 只拷贝流，不做转码
		"-start_number", "0",
		"-hls_time", fmt.Sprintf("%d", config.SegmentDuration),
		"-hls_list_size", "0", // 所有片段保持在播放列表中
		"-hls_playlist_type", config.PlaylistType,
		"-f", "hls",
		outputPath,
	}

	// 执行FFmpeg命令
	cmd := exec.Command("ffmpeg", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	log.Printf("开始处理: %s -> %s", inputPath, outputPath)
	log.Printf("处理参数: %v", args)

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("FFmpeg处理失败: %s", err)
	}

	log.Printf("处理完成: %s", outputPath)
	return outputPath, nil
}

// 提取视频中的字幕
func extractSubtitles(inputPath string, outputDir string) error {
	// 首先检查视频中的字幕流
	subtitleStreams, err := getSubtitleStreams(inputPath)
	if err != nil {
		return fmt.Errorf("获取字幕流信息失败: %s", err)
	}

	if len(subtitleStreams) == 0 {
		log.Println("视频中没有发现字幕流")
		return nil
	}

	log.Printf("发现 %d 个字幕流，开始提取", len(subtitleStreams))

	// 为每个字幕流执行提取
	for _, stream := range subtitleStreams {
		outputFile := filepath.Join(outputDir, fmt.Sprintf("subtitle_%s.%s", stream.index, stream.format))

		// 构建提取字幕的ffmpeg命令
		args := []string{
			"-i", inputPath,
			"-map", fmt.Sprintf("0:%s", stream.index),
			"-c", "copy",
			outputFile,
		}

		cmd := exec.Command("ffmpeg", args...)
		if err := cmd.Run(); err != nil {
			log.Printf("警告: 提取字幕流 %s 失败: %s", stream.index, err)
			continue
		}

		log.Printf("已提取字幕流 %s 到 %s", stream.index, outputFile)
	}

	return nil
}

// 字幕流信息
type subtitleStream struct {
	index  string // 流索引
	format string // 字幕格式
	lang   string // 语言（如果有）
}

// 获取视频中的字幕流信息
func getSubtitleStreams(inputPath string) ([]subtitleStream, error) {
	var streams []subtitleStream

	// 使用ffprobe获取字幕流信息
	cmd := exec.Command("ffprobe",
		"-v", "quiet",
		"-select_streams", "s",
		"-show_entries", "stream=index,codec_name:stream_tags=language",
		"-of", "csv=p=0",
		inputPath,
	)

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe获取字幕信息失败: %s", err)
	}

	// 解析输出结果
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}

		parts := strings.Split(line, ",")
		if len(parts) < 2 {
			continue
		}

		stream := subtitleStream{
			index:  parts[0],
			format: parts[1],
		}

		if len(parts) > 2 {
			stream.lang = parts[2]
		}

		streams = append(streams, stream)
	}

	return streams, nil
}