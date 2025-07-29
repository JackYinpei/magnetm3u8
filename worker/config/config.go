package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/google/uuid"
)

// Config 工作节点配置
type Config struct {
	Node     NodeConfig     `json:"node"`
	Gateway  GatewayConfig  `json:"gateway"`
	Storage  StorageConfig  `json:"storage"`
	Limits   LimitsConfig   `json:"limits"`
	Network  NetworkConfig  `json:"network"`
}

// NodeConfig 节点配置
type NodeConfig struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Address string `json:"address"`
}

// GatewayConfig 网关配置
type GatewayConfig struct {
	URL             string        `json:"url"`
	ReconnectDelay  time.Duration `json:"reconnect_delay"`
	HeartbeatPeriod time.Duration `json:"heartbeat_period"`
}

// StorageConfig 存储配置
type StorageConfig struct {
	DownloadPath string `json:"download_path"`
	M3U8Path     string `json:"m3u8_path"`
	MaxSizeGB    int    `json:"max_size_gb"`
}

// LimitsConfig 限制配置
type LimitsConfig struct {
	MaxDownloads   int `json:"max_downloads"`
	MaxTranscodes  int `json:"max_transcodes"`
	DiskSpaceGB    int `json:"disk_space_gb"`
	MaxConnections int `json:"max_connections"`
}

// NetworkConfig 网络配置
type NetworkConfig struct {
	ListenPort    int      `json:"listen_port"`
	STUNServers   []string `json:"stun_servers"`
	TURNServers   []string `json:"turn_servers"`
	MaxBandwidth  int      `json:"max_bandwidth_kbps"`
}

// Load 加载配置文件
func Load(configPath string) (*Config, error) {
	// 创建配置目录
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return nil, err
	}

	// 如果配置文件不存在，创建默认配置
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		defaultConfig := Default()
		if err := Save(configPath, defaultConfig); err != nil {
			return nil, err
		}
		return defaultConfig, nil
	}

	// 读取配置文件
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

// Save 保存配置文件
func Save(configPath string, config *Config) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0644)
}

// Default 返回默认配置
func Default() *Config {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "worker"
	}

	return &Config{
		Node: NodeConfig{
			ID:      generateNodeID(),
			Name:    hostname + "-worker",
			Address: "localhost",
		},
		Gateway: GatewayConfig{
			URL:             "ws://localhost:8080/ws/nodes",
			ReconnectDelay:  5 * time.Second,
			HeartbeatPeriod: 30 * time.Second,
		},
		Storage: StorageConfig{
			DownloadPath: "data/downloads",
			M3U8Path:     "data/m3u8",
			MaxSizeGB:    100,
		},
		Limits: LimitsConfig{
			MaxDownloads:   5,
			MaxTranscodes:  3,
			DiskSpaceGB:    50,
			MaxConnections: 10,
		},
		Network: NetworkConfig{
			ListenPort: 0, // 自动分配
			STUNServers: []string{
				"stun:stun.l.google.com:19302",
				"stun:stun1.l.google.com:19302",
			},
			TURNServers:   []string{},
			MaxBandwidth:  5000, // 5 Mbps
		},
	}
}

// generateNodeID 生成节点ID
func generateNodeID() string {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown"
	}
	
	return hostname + "-" + uuid.New().String()[:8]
}

// GetStoragePaths 获取存储路径（确保目录存在）
func (c *Config) GetStoragePaths() error {
	paths := []string{
		c.Storage.DownloadPath,
		c.Storage.M3U8Path,
		"data/config",
		"data/logs",
	}

	for _, path := range paths {
		if err := os.MkdirAll(path, 0755); err != nil {
			return err
		}
	}

	return nil
}

// GetSystemInfo 获取系统信息
func (c *Config) GetSystemInfo() map[string]interface{} {
	return map[string]interface{}{
		"os":           runtime.GOOS,
		"arch":         runtime.GOARCH,
		"cpu_count":    runtime.NumCPU(),
		"go_version":   runtime.Version(),
		"hostname":     c.Node.Name,
	}
}