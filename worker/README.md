# MagnetM3U8 Worker 节点

这是MagnetM3U8分布式系统的Worker节点，负责种子下载、视频转码和WebRTC流媒体服务。

## 特性

- 🚀 使用纯Go SQLite实现，无需CGO，避免了与torrent包的符号冲突
- 🎥 支持种子下载和视频转码为HLS格式
- 🌐 WebRTC P2P视频流媒体支持
- 📊 完整的SQL数据库支持，使用GORM ORM
- 🔧 简单的Shell脚本管理

## 快速开始

### 基本使用

```bash
# 查看帮助
./start-worker.sh --help

# 启动Worker（使用默认配置）
./start-worker.sh

# 指定网关地址启动
./start-worker.sh -g ws://your-gateway.com:8080/ws/nodes

# 检查运行状态
./start-worker.sh --status

# 停止Worker
./start-worker.sh --stop
```

### 构建和维护

```bash
# 重新构建并启动
./start-worker.sh --build

# 清理数据和日志
./start-worker.sh --clean
```

## 配置说明

Worker会自动创建默认配置文件 `config/worker.json`：

```json
{
    "worker_id": "",
    "worker_name": "Worker-hostname-timestamp",
    "gateway_url": "ws://localhost:8080/ws/nodes",
    "data_path": "./data",
    "max_concurrent_downloads": 3,
    "download_speed_limit": 0,
    "upload_speed_limit": 0,
    "log_level": "info",
    "webrtc": {
        "ice_servers": [
            {"urls": ["stun:stun.l.google.com:19302"]},
            {"urls": ["stun:stun1.l.google.com:19302"]}
        ]
    }
}
```

## 目录结构

```
worker/
├── start-worker.sh          # 启动脚本
├── worker                   # 二进制文件（自动构建）
├── worker.log              # 运行日志
├── config/
│   └── worker.json         # 配置文件
└── data/
    ├── worker.db           # SQLite数据库
    ├── downloads/          # 种子下载目录
    ├── m3u8/              # 转码后的视频文件
    └── temp/              # 临时文件
```

## 数据库

本Worker使用**纯Go SQLite实现** (`modernc.org/sqlite`)：

- ✅ 无需CGO，避免了与torrent包的符号冲突
- ✅ 完整SQL支持，使用GORM ORM
- ✅ 支持复杂查询和数据关系
- ✅ 更好的开发和调试体验
- ✅ 跨平台兼容性好

## 脚本参数

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `-h, --help` | 显示帮助信息 | - |
| `-g, --gateway URL` | 网关WebSocket地址 | `ws://localhost:8080/ws/nodes` |
| `-i, --id ID` | Worker节点ID | 自动生成 |
| `-n, --name NAME` | Worker节点名称 | `Worker-hostname-timestamp` |
| `-c, --config FILE` | 配置文件路径 | `config/worker.json` |
| `-d, --data DIR` | 数据目录路径 | `./data` |
| `-l, --log FILE` | 日志文件路径 | `./worker.log` |
| `--build` | 重新构建Worker二进制文件 | - |
| `--clean` | 清理数据和日志文件 | - |
| `--status` | 检查Worker状态 | - |
| `--stop` | 停止Worker进程 | - |

## 开发说明

### 构建要求

- Go 1.21+
- 纯Go环境，无需CGO

### 手动构建

```bash
# 自动下载依赖
go mod tidy

# 构建（使用纯Go SQLite实现）
CGO_ENABLED=0 go build -o worker .
```

### 日志查看

```bash
# 实时查看日志
tail -f worker.log

# 查看最近日志
./start-worker.sh --status && tail -20 worker.log
```

## 故障排除

1. **构建失败**：确保Go版本1.21+，运行 `go mod tidy` 更新依赖
2. **启动失败**：检查日志文件，通常是连接网关失败
3. **数据库问题**：使用 `./start-worker.sh --clean` 清理数据重新开始
4. **进程卡死**：使用 `./start-worker.sh --stop` 强制停止

## 系统架构

Worker节点是MagnetM3U8分布式系统的核心组件：

- **Gateway服务器**：负责节点注册、任务分发、WebRTC信令
- **Worker节点**：处理种子下载、视频转码、P2P流媒体
- **Web客户端**：通过WebRTC直接从Worker节点获取视频流