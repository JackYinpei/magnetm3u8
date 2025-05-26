package utils

import "strings"

// ExtractPath 从不同格式的路径中提取真正的文件路径
func ExtractPath(rawPath string) string {
	// 处理 URL 格式（包括可能不存在端口的情况）
	if strings.HasPrefix(rawPath, "http://") || strings.HasPrefix(rawPath, "https://") {
		// 找到域名后的路径部分
		parts := strings.Split(rawPath, "/")
		if len(parts) >= 4 {
			// 提取域名后的完整路径
			path := strings.Join(parts[3:], "/")

			// 如果路径以 video/ 开头，去掉这个前缀
			if strings.HasPrefix(path, "video/") {
				return strings.TrimPrefix(path, "video/")
			}

			return path
		}
		return ""
	}

	// 处理 /video/index_3/index.m3u8 格式
	if strings.HasPrefix(rawPath, "/video/") {
		// 去掉 /video/ 前缀，提取后面的部分
		return strings.TrimPrefix(rawPath, "/video/")
	}

	// 如果都不匹配，返回原始路径（去掉开头的/）
	return strings.TrimPrefix(rawPath, "/")
}
