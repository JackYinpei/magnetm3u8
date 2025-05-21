package transcoder

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Manager 管理视频转码
type Manager struct {
	inputDir   string
	outputDir  string
	activeJobs map[uint]bool
	mu         sync.RWMutex
}

// NewManager 创建新的转码管理器
func NewManager(inputDir, outputDir string) *Manager {
	// 确保输出目录存在
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		log.Fatalf("创建输出目录失败: %v", err)
	}

	return &Manager{
		inputDir:   inputDir,
		outputDir:  outputDir,
		activeJobs: make(map[uint]bool),
	}
}

// Transcode 将视频转为HLS格式(不做转码，只切片)并提取字幕
func (m *Manager) Transcode(taskID uint, inputPath string) (string, error) {
	// 检查文件是否存在
	if _, err := os.Stat(inputPath); os.IsNotExist(err) {
		return "", fmt.Errorf("输入文件不存在: %s", inputPath)
	}

	// 创建任务特定的输出目录
	taskDir := filepath.Join(m.outputDir, fmt.Sprintf("task_%d", taskID))
	if err := os.MkdirAll(taskDir, 0755); err != nil {
		return "", fmt.Errorf("创建任务输出目录失败: %w", err)
	}

	// 标记任务为活跃
	m.mu.Lock()
	m.activeJobs[taskID] = true
	m.mu.Unlock()

	// 清理函数
	defer func() {
		m.mu.Lock()
		delete(m.activeJobs, taskID)
		m.mu.Unlock()
	}()

	// 使用默认HLS配置
	config := DefaultHLSConfig()

	// 对MKV文件启用字幕提取
	ext := strings.ToLower(filepath.Ext(inputPath))
	if ext == ".mkv" {
		config.ExtractSubtitles = true
		log.Printf("检测到MKV文件，启用字幕提取功能")
	}

	// 进行HLS切片处理(不做转码)
	log.Printf("开始处理任务 %d: %s -> %s", taskID, inputPath, taskDir)
	outputPath, err := ConvertToHLS(inputPath, taskDir, config)
	if err != nil {
		return "", fmt.Errorf("处理失败: %w", err)
	}

	log.Printf("处理完成: %s", outputPath)
	return outputPath, nil
}

// IsTranscoding 检查任务是否正在处理中
func (m *Manager) IsTranscoding(taskID uint) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.activeJobs[taskID]
}

// GetM3U8Path 获取生成的M3U8文件路径
func (m *Manager) GetM3U8Path(taskID uint) string {
	taskDir := filepath.Join(m.outputDir, fmt.Sprintf("task_%d", taskID))
	return filepath.Join(taskDir, "index.m3u8")
}

// GetSubtitlePaths 获取提取的字幕文件路径
func (m *Manager) GetSubtitlePaths(taskID uint) ([]string, error) {
	taskDir := filepath.Join(m.outputDir, fmt.Sprintf("task_%d", taskID))

	// 确保目录存在
	if _, err := os.Stat(taskDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("任务目录不存在: %s", taskDir)
	}

	// 查找所有subtitle_*.* 文件
	pattern := filepath.Join(taskDir, "subtitle_*.*")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("查找字幕文件失败: %w", err)
	}

	return matches, nil
}

// Close 关闭管理器
func (m *Manager) Close() {
	// 清理资源，如有需要
}
