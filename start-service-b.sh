#!/bin/bash

# 默认参数
SERVER_A_URL="ws://localhost:8080/ws/service-b"
DOWNLOAD_DIR="./downloads"
M3U8_DIR="./m3u8"
RECONNECT_INTERVAL=5

# 显示帮助信息
show_help() {
    echo "使用方法: $0 [选项]"
    echo "选项:"
    echo "  -s, --server URL      服务A的WebSocket地址 (默认: $SERVER_A_URL)"
    echo "  -d, --download DIR    下载目录 (默认: $DOWNLOAD_DIR)"
    echo "  -m, --m3u8 DIR        M3U8文件存储目录 (默认: $M3U8_DIR)"
    echo "  -r, --reconnect N     重连间隔秒数 (默认: $RECONNECT_INTERVAL)"
    echo "  -h, --help            显示帮助信息"
}

# 解析命令行参数
while [[ $# -gt 0 ]]; do
    case "$1" in
        -s|--server)
            SERVER_A_URL="$2"
            shift 2
            ;;
        -d|--download)
            DOWNLOAD_DIR="$2"
            shift 2
            ;;
        -m|--m3u8)
            M3U8_DIR="$2"
            shift 2
            ;;
        -r|--reconnect)
            RECONNECT_INTERVAL="$2"
            shift 2
            ;;
        -h|--help)
            show_help
            exit 0
            ;;
        *)
            echo "未知选项: $1"
            show_help
            exit 1
            ;;
    esac
done

# 创建必要的目录
mkdir -p "$DOWNLOAD_DIR" "$M3U8_DIR"

echo "启动服务B..."
echo "服务A WebSocket: $SERVER_A_URL"
echo "下载目录: $DOWNLOAD_DIR"
echo "M3U8目录: $M3U8_DIR"
echo "重连间隔: $RECONNECT_INTERVAL 秒"

# 运行服务B
cd "$(dirname "$0")"
go run service_b/main.go service_b/websocket.go \
    --server="$SERVER_A_URL" \
    --download="$DOWNLOAD_DIR" \
    --m3u8="$M3U8_DIR" \
    --reconnect="$RECONNECT_INTERVAL" 