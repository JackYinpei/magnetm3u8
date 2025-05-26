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

// 检查文件是否存在
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}
