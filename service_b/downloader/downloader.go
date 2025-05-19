package downloader

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/anacrolix/torrent"
)

// TorrentInfo 表示Torrent的元数据信息
type TorrentInfo struct {
	Name     string     `json:"name"`
	Size     int64      `json:"size"`
	Files    []FileInfo `json:"files"`
	InfoHash string     `json:"info_hash"`
	Trackers []string   `json:"trackers"`
}

// FileInfo 表示Torrent中的文件信息
type FileInfo struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

// Manager 管理Torrent下载
type Manager struct {
	downloadDir string
	client      *torrent.Client
	torrents    map[uint]*torrent.Torrent
	mu          sync.RWMutex
}

// NewManager 创建新的下载管理器
func NewManager(downloadDir string) *Manager {
	// 确保下载目录存在
	if err := os.MkdirAll(downloadDir, 0755); err != nil {
		log.Fatalf("创建下载目录失败: %v", err)
	}

	// 创建Torrent客户端
	cfg := torrent.NewDefaultClientConfig()
	cfg.DataDir = downloadDir
	cfg.DisableIPv6 = true
	cfg.NoUpload = false
	cfg.Seed = true

	client, err := torrent.NewClient(cfg)
	if err != nil {
		log.Fatalf("创建Torrent客户端失败: %v", err)
	}

	return &Manager{
		downloadDir: downloadDir,
		client:      client,
		torrents:    make(map[uint]*torrent.Torrent),
	}
}

// GetTorrentInfo 从磁力链接获取Torrent信息
func (m *Manager) GetTorrentInfo(magnetURL string) (*TorrentInfo, error) {
	// 添加Torrent
	t, err := m.client.AddMagnet(magnetURL)
	if err != nil {
		return nil, fmt.Errorf("添加磁力链接失败: %w", err)
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

	// 等待元数据
	log.Println("等待获取Torrent元数据...")
	select {
	case <-t.GotInfo():
		info := t.Info()
		if info == nil {
			return nil, errors.New("获取Torrent信息失败")
		}

		// 构建TorrentInfo
		torrentInfo := &TorrentInfo{
			Name:     info.Name,
			Size:     info.TotalLength(),
			InfoHash: t.InfoHash().String(),
			Trackers: []string{},
		}

		// 添加文件信息
		for _, file := range t.Files() {
			torrentInfo.Files = append(torrentInfo.Files, FileInfo{
				Path: file.DisplayPath(),
				Size: file.Length(),
			})
		}

		return torrentInfo, nil
	case <-time.After(2 * time.Minute):
		return nil, errors.New("获取Torrent元数据超时")
	}
}

// Download 开始下载Torrent
func (m *Manager) Download(taskID uint, magnetURL string, progressCallback func(percentage float64, speed int64)) error {
	t, err := m.client.AddMagnet(magnetURL)
	if err != nil {
		return fmt.Errorf("添加磁力链接失败: %w", err)
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

	// 等待元数据
	log.Println("等待获取Torrent元数据...")
	select {
	case <-t.GotInfo():
		// 保存Torrent实例
		m.mu.Lock()
		m.torrents[taskID] = t
		m.mu.Unlock()

		// 开始下载所有文件
		t.DownloadAll()

		// 监控下载进度
		go m.monitorDownload(taskID, t, progressCallback)

		return nil
	case <-time.After(30 * time.Second):
		return errors.New("获取Torrent元数据超时")
	}
}

// monitorDownload 监控下载进度
func (m *Manager) monitorDownload(taskID uint, t *torrent.Torrent, progressCallback func(percentage float64, speed int64)) {
	var lastBytes int64
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			complete := t.BytesCompleted()
			total := t.Length()

			// 计算下载速度 (bytes/s)
			speed := complete - lastBytes
			lastBytes = complete

			// 计算完成百分比
			var percentage float64
			if total > 0 {
				percentage = float64(complete) / float64(total) * 100
				if percentage > 99.9 {
					percentage = 100.0
				}
			}

			// 调用进度回调
			if progressCallback != nil {
				progressCallback(percentage, speed)
			}

			// 下载完成，当完成字节数等于或接近总字节数时
			if total > 0 && complete >= total {
				log.Printf("下载完成: %s", t.Name())
				// 确保报告100%完成
				if progressCallback != nil && percentage < 100.0 {
					progressCallback(100.0, 0)
				}
				return
			}
		}
	}
}

// GetDownloadedFilePath 获取下载文件的路径
func (m *Manager) GetDownloadedFilePath(taskID uint) string {
	m.mu.RLock()
	t, exists := m.torrents[taskID]
	m.mu.RUnlock()

	if !exists {
		log.Printf("任务 %d 的Torrent实例不存在", taskID)
		return ""
	}

	// 确保文件已完成下载
	complete := t.BytesCompleted()
	total := t.Length()

	// 允许1MB的误差范围，或者99.9%就认为完成了
	downloadCompleted := complete >= total || total-complete <= 1024*1024 || float64(complete)/float64(total) >= 0.999

	if !downloadCompleted {
		log.Printf("任务 %d 的下载尚未完成: %d/%d 字节", taskID, complete, total)
	} else {
		log.Printf("任务 %d 的下载已完成: %d/%d 字节", taskID, complete, total)
	}

	// 如果是单文件Torrent
	if len(t.Files()) == 1 {
		path := filepath.Join(m.downloadDir, t.Files()[0].Path())
		log.Printf("单文件Torrent，文件路径: %s", path)
		// 检查文件是否存在
		if _, err := os.Stat(path); os.IsNotExist(err) {
			log.Printf("文件不存在: %s", path)
		}
		return path
	}

	// 如果是多文件Torrent，优先返回视频文件
	log.Printf("多文件Torrent，共 %d 个文件", len(t.Files()))
	for _, file := range t.Files() {
		path := file.Path()
		ext := filepath.Ext(path)
		fullPath := filepath.Join(m.downloadDir, path)

		log.Printf("检查文件: %s (大小: %d 字节)", path, file.Length())
		if ext == ".mp4" || ext == ".mkv" || ext == ".avi" || ext == ".mov" || ext == ".wmv" {
			// 检查文件是否存在
			if _, err := os.Stat(fullPath); os.IsNotExist(err) {
				log.Printf("视频文件不存在: %s", fullPath)
				continue
			}
			log.Printf("找到视频文件: %s", fullPath)
			return fullPath
		}
	}

	// 找不到视频文件，返回第一个文件
	if len(t.Files()) > 0 {
		path := filepath.Join(m.downloadDir, t.Files()[0].Path())
		log.Printf("使用第一个文件: %s", path)
		// 检查文件是否存在
		if _, err := os.Stat(path); os.IsNotExist(err) {
			log.Printf("文件不存在: %s", path)
		}
		return path
	}

	log.Printf("未找到任何文件")
	return ""
}

// Close 关闭下载管理器
func (m *Manager) Close() {
	if m.client != nil {
		m.client.Close()
	}
}
