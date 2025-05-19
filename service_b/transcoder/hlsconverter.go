package transcoder

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// HLSConfig 配置HLS转换参数
type HLSConfig struct {
	SegmentDuration      int    // 片段时长（秒）
	PlaylistType         string // 播放列表类型（event或vod）
	VideoCodec           string // 视频编码器
	VideoPreset          string // 编码速度预设
	VideoCRF             int    // 视频质量系数（0-51，越小质量越好）
	AudioCodec           string // 音频编码器
	AudioBitrate         string // 音频比特率
	Width                int    // 视频宽度，0表示保持原始尺寸
	Height               int    // 视频高度，0表示保持原始尺寸
	MaxBitrate           string // 最大比特率
	EnableVariantStreams bool   // 是否启用多码率
}

// DefaultHLSConfig 返回默认的HLS配置
func DefaultHLSConfig() HLSConfig {
	return HLSConfig{
		SegmentDuration:      10,
		PlaylistType:         "vod",
		VideoCodec:           "libx264",
		VideoPreset:          "medium",
		VideoCRF:             23,
		AudioCodec:           "aac",
		AudioBitrate:         "128k",
		Width:                0,
		Height:               0,
		MaxBitrate:           "0",
		EnableVariantStreams: false,
	}
}

// HighQualityHLSConfig 返回高质量HLS配置
func HighQualityHLSConfig() HLSConfig {
	config := DefaultHLSConfig()
	config.VideoPreset = "slow"
	config.VideoCRF = 18
	config.AudioBitrate = "192k"
	return config
}

// MobileOptimizedHLSConfig 返回针对移动设备优化的HLS配置
func MobileOptimizedHLSConfig() HLSConfig {
	config := DefaultHLSConfig()
	config.VideoPreset = "faster"
	config.VideoCRF = 26
	config.AudioBitrate = "96k"
	config.Width = 720
	config.Height = 0 // 等比例缩放
	config.MaxBitrate = "1500k"
	return config
}

// ConvertToHLS 将视频文件转换为HLS格式
func ConvertToHLS(inputPath string, outputDir string, config HLSConfig) (string, error) {
	// 检查输入文件是否存在
	if _, err := os.Stat(inputPath); os.IsNotExist(err) {
		return "", fmt.Errorf("输入文件不存在: %s", err)
	}

	// 确保输出目录存在
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("创建输出目录失败: %s", err)
	}

	// 获取输入文件的格式信息
	fileInfo, err := getFileInfo(inputPath)
	if err != nil {
		return "", fmt.Errorf("获取文件信息失败: %s", err)
	}

	// 根据文件信息优化转码参数
	optimizedConfig := optimizeConfig(config, fileInfo)

	// 构建输出文件路径
	outputName := "index.m3u8"
	outputPath := filepath.Join(outputDir, outputName)

	// 构建基本的FFmpeg命令
	args := []string{
		"-i", inputPath,
		"-map", "0:v", // 选择所有视频流
		"-map", "0:a?", // 选择所有音频流（如果存在）
	}

	// 添加视频转码参数
	if fileInfo.HasVideo {
		args = append(args,
			"-c:v", optimizedConfig.VideoCodec,
			"-preset", optimizedConfig.VideoPreset,
			"-crf", fmt.Sprintf("%d", optimizedConfig.VideoCRF),
		)

		// 如果指定了尺寸，添加缩放参数
		if optimizedConfig.Width > 0 || optimizedConfig.Height > 0 {
			width := optimizedConfig.Width
			height := optimizedConfig.Height

			if width == 0 {
				args = append(args, "-vf", fmt.Sprintf("scale=-2:%d", height))
			} else if height == 0 {
				args = append(args, "-vf", fmt.Sprintf("scale=%d:-2", width))
			} else {
				args = append(args, "-vf", fmt.Sprintf("scale=%d:%d", width, height))
			}
		}

		// 如果指定了最大比特率
		if optimizedConfig.MaxBitrate != "0" {
			args = append(args,
				"-maxrate", optimizedConfig.MaxBitrate,
				"-bufsize", optimizedConfig.MaxBitrate,
			)
		}
	}

	// 添加音频转码参数
	if fileInfo.HasAudio {
		args = append(args,
			"-c:a", optimizedConfig.AudioCodec,
			"-b:a", optimizedConfig.AudioBitrate,
		)
	}

	// 添加HLS特定参数
	args = append(args,
		"-profile:v", "high", // 使用high profile
		"-level", "4.1", // 更高的兼容性级别
		"-start_number", "0",
		"-hls_time", fmt.Sprintf("%d", optimizedConfig.SegmentDuration),
		"-hls_list_size", "0", // 所有片段保持在播放列表中
		"-hls_playlist_type", optimizedConfig.PlaylistType,
		"-f", "hls",
		outputPath,
	)

	// 执行FFmpeg命令
	cmd := exec.Command("ffmpeg", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	log.Printf("开始转码: %s -> %s", inputPath, outputPath)
	log.Printf("转码参数: %v", args)

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("FFmpeg转码失败: %s", err)
	}

	log.Printf("转码完成: %s", outputPath)
	return outputPath, nil
}

// 文件信息结构
type fileInfo struct {
	Format     string
	HasVideo   bool
	VideoCodec string
	HasAudio   bool
	AudioCodec string
	Width      int
	Height     int
	Bitrate    string
	Duration   float64
}

// 获取文件信息
func getFileInfo(inputPath string) (fileInfo, error) {
	info := fileInfo{}

	// 使用ffprobe获取文件信息
	cmd := exec.Command("ffprobe",
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		inputPath,
	)

	output, err := cmd.Output()
	if err != nil {
		return info, fmt.Errorf("ffprobe获取文件信息失败: %s", err)
	}

	// 这里简化了，实际应该解析JSON
	// 简单检测视频和音频流是否存在
	outputStr := string(output)
	info.HasVideo = strings.Contains(outputStr, "\"codec_type\":\"video\"")
	info.HasAudio = strings.Contains(outputStr, "\"codec_type\":\"audio\"")

	// 提取格式信息
	if strings.Contains(outputStr, "\"format_name\":") {
		formatNameStart := strings.Index(outputStr, "\"format_name\":") + 14
		formatNameEnd := strings.Index(outputStr[formatNameStart:], "\"") + formatNameStart
		if formatNameEnd > formatNameStart {
			info.Format = outputStr[formatNameStart:formatNameEnd]
		}
	}

	return info, nil
}

// 根据文件信息优化转码配置
func optimizeConfig(config HLSConfig, info fileInfo) HLSConfig {
	optimized := config

	// 根据输入文件格式优化配置
	if strings.Contains(info.Format, "mp4") || strings.Contains(info.Format, "mov") {
		// 对于MP4可以使用更快的转码方式
		if config.VideoPreset == "medium" {
			optimized.VideoPreset = "faster"
		}
	} else if strings.Contains(info.Format, "mkv") || strings.Contains(info.Format, "avi") {
		// 对于MKV/AVI等，可能需要更保守的编码设置
		if config.VideoCRF < 20 {
			optimized.VideoCRF = 20
		}
	} else if strings.Contains(info.Format, "webm") {
		// 对于WebM，可能希望保持相似的质量
		optimized.VideoCRF = 22
	}

	return optimized
}
