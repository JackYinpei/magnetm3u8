# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

MagnetM3U8 is a distributed torrent downloading and video streaming system with P2P video playback using WebRTC. The system consists of three main components:

### 1. Gateway Server (公网服务器)
**Location**: `gateway/`

**Responsibilities:**
- Web interface serving (HTML/CSS/JS frontend)
- WebRTC signaling coordination between clients and worker nodes
- Worker node registration and health monitoring
- Task routing and load balancing
- Stateless message relay (no data storage)

### 2. Worker Node (内网工作节点) 
**Location**: `worker/`

**Responsibilities:**
- Torrent downloading and file storage
- Video transcoding to HLS format (M3U8/TS segments)
- WebRTC P2P connections for video file serving
- Task and metadata management with SQLite database
- File system storage management

### 3. Web Client (浏览器客户端)
**Location**: `gateway/static/`

**Responsibilities:**
- User interface for magnet link submission
- Worker node selection and task management
- WebRTC P2P connection establishment
- Video playback with HLS streaming via data channels
- XHR request interception for seamless video loading

## Architecture

### Gateway Server
- **Framework**: Gin (Go web framework)
- **Database**: None (stateless design)
- **Default Port**: 8080
- **Key Files**:
  - `main.go`: HTTP server and startup
  - `manager.go`: Worker node registry and session management
  - `routes.go`: API endpoints and WebSocket handlers
  - `static/`: Frontend HTML/CSS/JS files

### Worker Node
- **Framework**: Go with WebSocket client
- **Database**: SQLite with GORM ORM
- **Storage Structure**:
  - `data/config/worker.db`: SQLite database
  - `data/downloads/`: Torrent download storage
  - `data/m3u8/`: HLS transcoded videos (M3U8/TS files)
  - `data/logs/`: Application logs
- **Key Components**:
  - `main.go`: Service coordination and message handling
  - `client/gateway.go`: WebSocket connection to gateway
  - `downloader/manager.go`: BitTorrent download management
  - `transcoder/manager.go`: FFmpeg HLS transcoding
  - `webrtc/manager.go`: P2P streaming and file serving
  - `models/models.go`: Database models

### Web Client
- **Technology**: HTML5, JavaScript, WebRTC, Video.js
- **Key Files**:
  - `static/index.html`: Main interface for task management
  - `static/player.html`: Video player with P2P streaming
- **Features**:
  - WebRTC peer connection management
  - XHR interception for transparent video file loading
  - Real-time connection status monitoring

## Communication Protocols

### 1. Gateway ↔ Worker Node (WebSocket)

#### Connection Endpoint
```
ws://gateway:8080/ws/nodes
```

#### Message Format
```json
{
  "type": "message_type",
  "payload": {
    // message-specific data
  }
}
```

#### Worker → Gateway Messages

**Node Registration**
```json
{
  "id": "worker-node-001",
  "name": "Main Worker Node",
  "address": "192.168.1.100",
  "status": "online",
  "capabilities": ["torrent", "transcode", "webrtc"],
  "resources": {
    "max_downloads": 5,
    "max_transcodes": 2,
    "disk_space_gb": 500
  },
  "metadata": {
    "version": "1.0.0",
    "arch": "amd64"
  }
}
```

**Heartbeat**
```json
{
  "type": "heartbeat",
  "payload": {
    "timestamp": 1640995200,
    "node_id": "worker-node-001"
  }
}
```

**Task Status Update**
```json
{
  "type": "task_status",
  "payload": {
    "task_id": "task_1640995200123",
    "status": "downloading", // downloading, transcoding, ready, error
    "progress": 45,
    "timestamp": 1640995200
  }
}
```

**Tasks List Response**
```json
{
  "type": "tasks_response",
  "payload": {
    "request_id": "req_1640995200_123",
    "tasks": [
      {
        "id": "task_1640995200123",
        "magnet_url": "magnet:?xt=urn:btih:...",
        "status": "ready",
        "progress": 100,
        "speed": 0,
        "size": 1073741824,
        "downloaded": 1073741824,
        "files": ["movie.mp4", "subtitle.srt"],
        "torrent_name": "Sample Movie",
        "m3u8_path": "data/m3u8/task_1640995200123/index.m3u8",
        "srts": ["subtitle.srt"],
        "created_at": "2021-12-31T12:00:00Z",
        "updated_at": "2021-12-31T12:30:00Z",
        "worker_id": "worker-node-001"
      }
    ]
  }
}
```

**WebRTC Answer**
```json
{
  "type": "webrtc_answer",
  "payload": {
    "session_id": "client-1640995200-abc123",
    "sdp": "v=0\r\no=- 123456789 123456789 IN IP4 0.0.0.0\r\n..."
  }
}
```

**ICE Candidate**
```json
{
  "type": "ice_candidate",
  "payload": {
    "session_id": "client-1640995200-abc123",
    "candidate": "candidate:1 1 UDP 2013266431 192.168.1.100 54400 typ host"
  }
}
```

#### Gateway → Worker Messages

**Registration Confirmed**
```json
{
  "type": "registration_confirmed",
  "payload": {
    "node_id": "worker-node-001",
    "status": "registered"
  }
}
```

**Task Submit**
```json
{
  "type": "task_submit",
  "payload": {
    "magnet_url": "magnet:?xt=urn:btih:...",
    "timestamp": 1640995200
  }
}
```

**Get Tasks Request**
```json
{
  "type": "get_tasks",
  "payload": {
    "request_id": "req_1640995200_123",
    "timestamp": 1640995200
  }
}
```

**Get Task Detail**
```json
{
  "type": "get_task_detail",
  "payload": {
    "task_id": "task_1640995200123",
    "timestamp": 1640995200
  }
}
```

**WebRTC Offer**
```json
{
  "type": "webrtc_offer",
  "payload": {
    "session_id": "client-1640995200-abc123",
    "client_id": "client-1640995200-abc123",
    "sdp": "v=0\r\no=- 123456789 123456789 IN IP4 0.0.0.0\r\n..."
  }
}
```

### 2. Gateway ↔ Web Client (WebSocket)

#### Connection Endpoint
```
ws://gateway:8080/ws/clients?client_id=client-1640995200-abc123
```

#### Client → Gateway Messages

**WebRTC Offer**
```json
{
  "type": "webrtc_offer",
  "payload": {
    "worker_id": "worker-node-001",
    "session_id": "client-1640995200-abc123",
    "client_id": "client-1640995200-abc123",
    "sdp": "v=0\r\no=- 123456789 123456789 IN IP4 0.0.0.0\r\n..."
  }
}
```

**ICE Candidate**
```json
{
  "type": "ice_candidate",
  "payload": {
    "session_id": "client-1640995200-abc123",
    "candidate": "candidate:1 1 UDP 2013266431 10.0.0.1 54400 typ host"
  }
}
```

#### Gateway → Client Messages

**WebRTC Answer**
```json
{
  "type": "webrtc_answer",
  "payload": {
    "session_id": "client-1640995200-abc123",
    "sdp": "v=0\r\no=- 123456789 123456789 IN IP4 0.0.0.0\r\n..."
  }
}
```

**ICE Candidate**
```json
{
  "type": "ice_candidate",
  "payload": {
    "session_id": "client-1640995200-abc123",
    "candidate": "candidate:1 1 UDP 2013266431 192.168.1.100 54400 typ host"
  }
}
```

### 3. Gateway HTTP API

#### Node Management

**GET /api/nodes**
- **Description**: Get list of online worker nodes
- **Response**:
```json
{
  "success": true,
  "data": [
    {
      "id": "worker-node-001",
      "name": "Main Worker Node",
      "address": "192.168.1.100",
      "status": "online",
      "last_seen": "2021-12-31T12:00:00Z",
      "capabilities": ["torrent", "transcode", "webrtc"],
      "resources": {
        "max_downloads": 5,
        "max_transcodes": 2,
        "disk_space_gb": 500
      }
    }
  ]
}
```

**GET /api/nodes/:id**
- **Description**: Get specific worker node details
- **Response**: Same as single node object above

#### Task Management

**POST /api/tasks/submit**
- **Description**: Submit task to specific worker node
- **Request**:
```json
{
  "worker_id": "worker-node-001",
  "magnet_url": "magnet:?xt=urn:btih:..."
}
```
- **Response**:
```json
{
  "success": true,
  "message": "Task submitted successfully"
}
```

**GET /api/tasks**
- **Description**: Get all tasks from all worker nodes
- **Response**:
```json
{
  "success": true,
  "data": {
    "tasks": [
      // Task objects from all workers
    ]
  }
}
```

**GET /api/tasks/:id**
- **Description**: Get specific task details
- **Response**:
```json
{
  "success": false,
  "error": "Task not found"
}
```

#### WebRTC Signaling

**POST /api/webrtc/offer**
- **Description**: Submit WebRTC offer
- **Request**:
```json
{
  "worker_id": "worker-node-001",
  "client_id": "client-1640995200-abc123",
  "session_id": "client-1640995200-abc123",
  "sdp": "v=0\r\no=- 123456789..."
}
```
- **Response**:
```json
{
  "success": true,
  "session_id": "client-1640995200-abc123"
}
```

**POST /api/webrtc/ice**
- **Description**: Submit ICE candidate
- **Request**:
```json
{
  "session_id": "client-1640995200-abc123",
  "candidate": "candidate:1 1 UDP 2013266431...",
  "is_client": true
}
```
- **Response**:
```json
{
  "success": true
}
```

#### System Status

**GET /api/status**
- **Description**: Get system status
- **Response**:
```json
{
  "success": true,
  "data": {
    "online_nodes": 2,
    "total_nodes": 3,
    "active_sessions": 5
  }
}
```

### 4. Worker ↔ Client (WebRTC Data Channel)

#### File Request Protocol

**Client → Worker (File Request)**
```json
{
  "type": "hijackReq",
  "ts": "/video/task_1640995200123/index.m3u8",
  "id": "req_1640995200_456"
}
```

**Worker → Client (Text File Response)**
```json
{
  "type": "hijackRespText",
  "id": "req_1640995200_456",
  "sliceNum": 0,
  "totalSliceNum": 1,
  "totalLength": 2048,
  "payload": "I0VYVE0zVQojRVhULVgtVkVSU0lPTjozCiNFWFQtWC1UQVJHRVREVVJBV..."
}
```

**Worker → Client (Binary File Response)**
```json
{
  "type": "hijackRespData",
  "id": "req_1640995200_456",
  "sliceNum": 0,
  "totalSliceNum": 3,
  "totalLength": 49152,
  "payload": "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8gISIjJCUmJygpKissLS4vMDEyMzQ1Njc4OTo7PD0+P0BBQkNERUZHSElKS0xNTk9QUVJTVFVWV1hZWltcXV5fYGFiY2RlZmdoaWprbG1ub3BxcnN0dXZ3eHl6e3x9fn+AgYKDhIWGh4iJiouMjY6PkJGSk5SVlpeYmZqbnJ2en6ChoqOkpaanqKmqq6ytrq+wsbKztLW2t7i5uru8vb6/wMHCw8TFxsfIycrLzM3Oz9DR0tPU1dbX2Nna29zd3t/g4eLj5OXm5+jp6uvs7e7v8PHy8/T19vf4+fr7/P3+/w=="
}
```

**Worker → Client (Error Response)**
```json
{
  "type": "hijackError",
  "id": "req_1640995200_456",
  "error": "File not found"
}
```

## Database Schema

### Task Model (Worker SQLite Database)
```sql
CREATE TABLE tasks (
    task_id TEXT PRIMARY KEY,
    magnet_url TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    progress REAL DEFAULT 0,
    speed INTEGER DEFAULT 0,
    size INTEGER DEFAULT 0,
    downloaded INTEGER DEFAULT 0,
    torrent_name TEXT,
    torrent_files TEXT, -- JSON array
    m3u8_file_path TEXT,
    srts TEXT, -- JSON array
    segments TEXT, -- JSON array of TS file paths
    metadata TEXT, -- JSON object
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

## Development Commands

### Start Services

**Gateway Server**
```bash
cd gateway
go run main.go --port=8080
# Or with binary:
go build -o gateway . && ./gateway --port=8080
```

**Worker Node**
```bash
cd worker
go run main.go --gateway=ws://localhost:8080/ws/nodes --id=worker-001 --name="Main Worker"
# Or with binary:
go build -o worker . && ./worker --gateway=ws://localhost:8080/ws/nodes
```

### Build All Services
```bash
# Build Gateway
cd gateway && go build -o gateway .

# Build Worker
cd worker && go build -o worker .
```

### Dependencies
```bash
# Gateway dependencies
cd gateway && go mod tidy

# Worker dependencies  
cd worker && go mod tidy
```

## Configuration

### Gateway Server Environment Variables
- `GATEWAY_PORT`: Server port (default: 8080)
- `GIN_MODE`: Set to "release" for production

### Worker Node Configuration
**File**: `worker/config/worker.json`
```json
{
  "gateway": {
    "url": "ws://localhost:8080/ws/nodes"
  },
  "node": {
    "id": "worker-001",
    "name": "Main Worker Node",
    "address": "192.168.1.100"
  },
  "storage": {
    "download_path": "data/downloads",
    "m3u8_path": "data/m3u8",
    "database_path": "data/config"
  },
  "limits": {
    "max_downloads": 5,
    "max_transcodes": 2,
    "disk_space_gb": 500
  },
  "transcoding": {
    "segment_duration": 10,
    "video_codec": "libx264",
    "audio_codec": "aac"
  }
}
```

## Video Processing Pipeline

1. **Task Submission**: User submits magnet URL via web interface
2. **Task Routing**: Gateway routes task to selected worker node
3. **Download**: Worker downloads torrent files using BitTorrent protocol
4. **Auto-Transcoding**: Completed downloads automatically trigger HLS transcoding
5. **Storage**: M3U8 playlists and TS segments stored in `worker/data/m3u8/`
6. **Database Update**: Task status and file metadata saved to SQLite
7. **P2P Streaming**: Web client establishes WebRTC connection for video playback
8. **File Serving**: Worker serves M3U8/TS files via WebRTC data channels

## File Structure

### Gateway Server (`gateway/`)
```
gateway/
├── main.go              # HTTP server and startup
├── manager.go           # Worker node registry and session management  
├── routes.go            # API endpoints and WebSocket handlers
├── go.mod               # Go module dependencies
├── static/              # Frontend files
│   ├── index.html       # Main task management interface
│   └── player.html      # Video player with P2P streaming
└── gateway              # Compiled binary
```

### Worker Node (`worker/`)
```
worker/
├── main.go              # Service coordination and message handling
├── client/
│   └── gateway.go       # WebSocket connection to gateway
├── config/
│   └── config.go        # Configuration management
├── database/
│   └── database.go      # SQLite database operations
├── downloader/  
│   └── manager.go       # BitTorrent download management
├── transcoder/
│   └── manager.go       # FFmpeg HLS transcoding
├── webrtc/
│   └── manager.go       # P2P streaming and file serving
├── models/
│   └── models.go        # Database models
├── data/                # Runtime data directory
│   ├── config/          # SQLite database
│   ├── downloads/       # Torrent downloads
│   ├── m3u8/           # HLS transcoded videos
│   └── logs/           # Application logs
├── go.mod               # Go module dependencies
└── worker               # Compiled binary
```

## Testing

### Complete System Test
1. **Start Gateway**: `cd gateway && go run main.go`
2. **Start Worker**: `cd worker && go run main.go`
3. **Open Browser**: Navigate to `http://localhost:8080`
4. **Submit Task**: Enter magnet URL and select worker node
5. **Monitor Progress**: Watch download and transcoding in worker logs
6. **Test Playback**: Click play button to test P2P video streaming

### WebRTC P2P Streaming Test
1. **Submit Task**: Add a magnet URL with video content
2. **Wait for Ready**: Task status should become "ready" after transcoding
3. **Open Player**: Navigate to `/player?taskId=<task_id>`
4. **Establish P2P**: Click "建立P2P连接" to connect via WebRTC
5. **Test Playback**: Click "测试播放" to stream video via data channels
6. **Monitor Network**: Check browser dev tools for XHR interception logs

## Troubleshooting

### Common Issues

**WebRTC Connection Failed**
- Check STUN server accessibility
- Verify network NAT configuration
- Test with different network environments (avoid mobile hotspots)

**Video Files Not Found**
- Verify M3U8 files exist in `worker/data/m3u8/taskId/`
- Check transcoding completion in worker logs
- Ensure file paths match database records

**Worker Node Offline**
- Check WebSocket connection to gateway
- Verify gateway server accessibility
- Review worker logs for connection errors

**Database Errors**
- Ensure `worker/data/config/` directory exists
- Check SQLite file permissions
- Review database initialization logs

## Legacy References

The following directories contain legacy code for reference:
- `service_b/`: Previous worker implementation  
- `api/`, `services/`, `models/`: Legacy API server
- `main.go`: Legacy monolithic server

These files are kept for reference during development but are not part of the active distributed system.