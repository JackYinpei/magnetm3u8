package transcoder

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
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

func (m *Manager) ConvertSubtitle(taskDir string, downloadPath string) ([]string, error) {
	// 支持的字幕扩展名
	subtitleExts := map[string]bool{
		".srt": true,
		".vtt": true,
		".ass": true,
		".ssa": true,
		".sub": true,
		".txt": true,
	}

	if err := os.MkdirAll(taskDir, 0755); err != nil {
		return nil, fmt.Errorf("创建字幕输出目录失败: %w", err)
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
		ext := strings.ToLower(filepath.Ext(info.Name()))
		if !subtitleExts[ext] {
			return nil
		}

		// 目标srt文件名
		baseName := strings.TrimSuffix(info.Name(), ext)
		targetSrt := filepath.Join(taskDir, baseName+".srt")

		// 如果已经是srt，直接拷贝
		if ext == ".srt" {
			srcFile, err := os.Open(path)
			if err != nil {
				log.Printf("打开字幕文件失败: %s, err: %v", path, err)
				return nil
			}
			defer srcFile.Close()
			dstFile, err := os.Create(targetSrt)
			if err != nil {
				log.Printf("创建目标字幕文件失败: %s, err: %v", targetSrt, err)
				return nil
			}
			defer dstFile.Close()
			_, err = io.Copy(dstFile, srcFile)
			if err != nil {
				log.Printf("拷贝字幕文件失败: %s -> %s, err: %v", path, targetSrt, err)
			} else {
				log.Printf("已拷贝字幕文件: %s -> %s", path, targetSrt)
			}
			return nil
		}

		// 其他格式，使用ffmpeg转为srt
		cmd := fmt.Sprintf("ffmpeg -y -i %q %q", path, targetSrt)
		log.Printf("正在转换字幕: %s -> %s", path, targetSrt)
		out, err := executeShellCommand(cmd)
		if err != nil {
			log.Printf("字幕转换失败: %s, 输出: %s, 错误: %v", path, out, err)
		} else {
			log.Printf("字幕转换成功: %s -> %s", path, targetSrt)
		}
		targetSrts = append(targetSrts, targetSrt)
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("遍历字幕文件失败: %w", err)
	}
	return targetSrts, nil
}

// executeShellCommand 用于执行shell命令并返回输出
func executeShellCommand(cmd string) (string, error) {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return "", fmt.Errorf("命令为空")
	}
	name := parts[0]
	args := parts[1:]
	out, err := exec.Command(name, args...).CombinedOutput()
	return string(out), err
}

// Transcode 将视频转为HLS格式(不做转码，只切片)并提取字幕
// 返回文件的index.m3u8路径，保存路径，错误
func (m *Manager) Transcode(taskID uint, inputPath string) (string, string, error) {
	// 检查文件是否存在
	if _, err := os.Stat(inputPath); os.IsNotExist(err) {
		return "", "", fmt.Errorf("输入文件不存在: %s", inputPath)
	}

	// 获取转码的这个文件的纯名字
	filenameOnly := strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath))

	// 创建任务特定的输出目录
	taskDir := filepath.Join(m.outputDir, filenameOnly)
	if err := os.MkdirAll(taskDir, 0755); err != nil {
		return "", "", fmt.Errorf("创建任务输出目录失败: %w", err)
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
		return "", "", fmt.Errorf("处理失败: %w", err)
	}

	log.Printf("处理完成: %s", outputPath)
	return outputPath, taskDir, nil
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
