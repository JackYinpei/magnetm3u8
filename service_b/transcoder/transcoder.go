package transcoder

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
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
		log.Fatalf("创建M3U8输出目录失败: %v", err)
	}

	return &Manager{
		inputDir:   inputDir,
		outputDir:  outputDir,
		activeJobs: make(map[uint]bool),
	}
}

// Transcode 将视频转码为M3U8格式
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

	// 生成输出文件路径
	outputName := "index.m3u8"
	outputPath := filepath.Join(taskDir, outputName)

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

	// 使用FFmpeg进行转码
	cmd := exec.Command(
		"ffmpeg",
		"-i", inputPath,
		"-profile:v", "baseline", // 基本配置文件兼容性更好
		"-level", "3.0",
		"-start_number", "0",
		"-hls_time", "10", // 每个片段10秒
		"-hls_list_size", "0", // 所有片段保持在播放列表中
		"-f", "hls",
		outputPath,
	)

	// 获取FFmpeg输出
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	log.Printf("开始转码: %s -> %s", inputPath, outputPath)
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("FFmpeg转码失败: %w", err)
	}

	log.Printf("转码完成: %s", outputPath)
	return outputPath, nil
}

// IsTranscoding 检查任务是否正在转码中
func (m *Manager) IsTranscoding(taskID uint) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.activeJobs[taskID]
}

// GetM3U8Path 获取转码后的M3U8文件路径
func (m *Manager) GetM3U8Path(taskID uint) string {
	taskDir := filepath.Join(m.outputDir, fmt.Sprintf("task_%d", taskID))
	return filepath.Join(taskDir, "index.m3u8")
}

// Close 关闭转码管理器
func (m *Manager) Close() {
	// 清理资源，如有需要
}
