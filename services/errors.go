package services

import "errors"

// 定义服务错误
var (
	ErrNotConnected      = errors.New("未连接到服务B")
	ErrInvalidMagnetURL  = errors.New("无效的磁力链接")
	ErrTaskNotFound      = errors.New("任务未找到")
	ErrInvalidTaskStatus = errors.New("无效的任务状态")
	ErrWebRTCFailed      = errors.New("WebRTC连接失败")
)
