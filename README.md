# MagnetM3U8 项目

MagnetM3U8 是一个微服务项目，能够通过磁力链接下载Torrent文件，并将其转换为M3U8格式，通过WebRTC P2P进行流媒体播放。

## 项目结构

- **服务A**（本代码库）：公网服务器，负责API、信令和前端
- **服务B**：内网服务器，负责Torrent下载和文件转码

## 目录结构

```
.
├── api/             # API控制器和路由
├── db/              # 数据库连接和操作
├── models/          # 数据模型
├── services/        # 业务逻辑服务
├── static/          # 前端静态文件
├── main.go          # 程序入口
└── README.md        # 项目说明
```

## 功能特点

- 支持提交磁力链接进行下载
- 下载完成后自动转码为HLS（M3U8）格式
- 通过WebRTC P2P直接从内网服务器播放视频
- 实时显示下载进度和状态

## 依赖

- Go 1.21+
- Gin Web框架
- GORM ORM框架
- SQLite数据库
- WebRTC
- Websocket

## 安装和运行

1. 克隆项目
```
git clone https://github.com/yourusername/magnetm3u8.git
cd magnetm3u8
```

2. 下载依赖
```
go mod tidy
```

3. 运行服务A
```
go run main.go
```

服务将运行在 `http://localhost:8080`

## 环境变量

- `PORT`: 服务端口号，默认为8080
- `GIN_MODE`: Gin运行模式，可设置为"release"用于生产环境

## API接口

### 磁力链接

- `POST /api/magnets`: 提交磁力链接
- `GET /api/tasks`: 获取任务列表
- `GET /api/tasks/:id`: 获取任务详情

### WebRTC信令

- `POST /api/webrtc/offer`: 提交WebRTC Offer
- `POST /api/webrtc/ice`: 提交ICE Candidate

### WebSocket端点

- `GET /ws/service-b`: 服务B连接的WebSocket端点
- `GET /ws/client`: 客户端连接的WebSocket端点

## 注意事项

- 服务A需要有公网IP或域名，以便客户端和服务B能够连接
- 默认情况下，API允许所有来源的跨域请求，生产环境中应修改CORS设置

## 许可

MIT 