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

	// 检测文件类型并选择合适的转码配置
	config := selectHLSConfig(inputPath)

	// 使用HLSConverter进行转码
	log.Printf("开始转码任务 %d: %s -> %s", taskID, inputPath, taskDir)
	outputPath, err := ConvertToHLS(inputPath, taskDir, config)
	if err != nil {
		return "", fmt.Errorf("转码失败: %w", err)
	}

	log.Printf("转码完成: %s", outputPath)
	return outputPath, nil
}

// selectHLSConfig 根据输入文件选择合适的HLS配置
func selectHLSConfig(inputPath string) HLSConfig {
	// 默认配置
	config := DefaultHLSConfig()

	// 获取文件扩展名
	ext := strings.ToLower(filepath.Ext(inputPath))

	// 根据文件类型调整配置
	switch ext {
	case ".mp4", ".mov", ".m4v":
		// 对于常见的MP4类文件，使用默认配置即可
	case ".mkv", ".avi", ".wmv":
		// 对于更复杂的容器，可能需要更高质量的编码
		config.VideoPreset = "slow"
		config.VideoCRF = 20
	case ".webm", ".vp9":
		// 对于WebM格式，使用更高效的编码
		config.VideoPreset = "faster"
		config.VideoCRF = 22
	case ".ts", ".m2ts":
		// 对于已经是HLS兼容的格式，可以使用轻量级转码
		config.VideoPreset = "ultrafast"
		config.VideoCRF = 21
	default:
		// 对于其他格式，使用保守的设置
		config.VideoPreset = "medium"
		config.VideoCRF = 23
	}

	// 获取文件大小以决定是否调整质量
	fileInfo, err := os.Stat(inputPath)
	if err == nil {
		// 对于大于1GB的文件，可以考虑使用高质量编码
		if fileInfo.Size() > 1024*1024*1024 {
			config.VideoCRF = 22 // 稍微提高质量
		}
	}

	return config
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
