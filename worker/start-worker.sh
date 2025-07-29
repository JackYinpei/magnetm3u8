#!/bin/bash

# MagnetM3U8 Worker 启动脚本
# 使用纯Go SQLite实现，避免CGO依赖和符号冲突

set -e  # 遇到错误立即退出

# 脚本配置
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
WORKER_BINARY="$SCRIPT_DIR/worker"
CONFIG_DIR="$SCRIPT_DIR/config"
DATA_DIR="$SCRIPT_DIR/data"
LOG_FILE="$SCRIPT_DIR/worker.log"

# 默认配置
DEFAULT_GATEWAY="ws://localhost:8080/ws/nodes"
DEFAULT_WORKER_ID=""
DEFAULT_WORKER_NAME="Worker-$(hostname)-$(date +%s)"

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 日志函数
log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_debug() {
    echo -e "${BLUE}[DEBUG]${NC} $1"
}

# 显示帮助信息
show_help() {
    cat << EOF
MagnetM3U8 Worker 启动脚本

用法: $0 [选项]

选项:
    -h, --help              显示此帮助信息
    -g, --gateway URL       网关WebSocket地址 (默认: $DEFAULT_GATEWAY)
    -i, --id ID             Worker节点ID (默认: 自动生成)
    -n, --name NAME         Worker节点名称 (默认: $DEFAULT_WORKER_NAME)
    -c, --config FILE       配置文件路径 (默认: config/worker.json)
    -d, --data DIR          数据目录路径 (默认: ./data)
    -l, --log FILE          日志文件路径 (默认: ./worker.log)
    --build                 重新构建Worker二进制文件
    --clean                 清理数据和日志文件
    --status                检查Worker状态
    --stop                  停止Worker进程

示例:
    $0                                          # 使用默认配置启动
    $0 -g ws://gateway.example.com:8080/ws/nodes  # 指定网关地址
    $0 --build                                  # 重新构建并启动
    $0 --clean                                  # 清理数据文件
    $0 --status                                 # 检查运行状态

数据库说明:
    本Worker使用纯Go SQLite实现 (modernc.org/sqlite)
    - ✅ 无需CGO，避免了与torrent包的符号冲突
    - ✅ 完整SQL支持，使用GORM ORM
    - ✅ 支持复杂查询和数据关系
    - ✅ 更好的开发和调试体验
EOF
}

# 检查Go环境
check_go() {
    if ! command -v go &> /dev/null; then
        log_error "Go未安装或不在PATH中"
        exit 1
    fi
    
    local go_version=$(go version | awk '{print $3}' | sed 's/go//')
    log_info "检测到Go版本: $go_version"
}

# 创建必要的目录
create_directories() {
    log_info "创建必要目录..."
    mkdir -p "$CONFIG_DIR"
    mkdir -p "$DATA_DIR"
    mkdir -p "$DATA_DIR/downloads"
    mkdir -p "$DATA_DIR/m3u8"
    mkdir -p "$DATA_DIR/temp"
    
    log_debug "目录结构:"
    log_debug "  配置目录: $CONFIG_DIR"
    log_debug "  数据目录: $DATA_DIR"
    log_debug "  下载目录: $DATA_DIR/downloads"
    log_debug "  视频目录: $DATA_DIR/m3u8"
    log_debug "  临时目录: $DATA_DIR/temp"
}

# 构建Worker二进制文件
build_worker() {
    log_info "构建Worker二进制文件（使用纯Go SQLite实现）..."
    
    # 清理旧的构建
    if [ -f "$WORKER_BINARY" ]; then
        rm -f "$WORKER_BINARY"
    fi
    
    # 更新依赖
    log_info "更新Go模块依赖..."
    go mod tidy
    
    # 构建 - 使用纯Go实现，禁用CGO避免符号冲突
    log_info "编译Worker..."
    CGO_ENABLED=0 go build -o "$WORKER_BINARY" .
    
    if [ -f "$WORKER_BINARY" ]; then
        chmod +x "$WORKER_BINARY"
        local size=$(ls -lh "$WORKER_BINARY" | awk '{print $5}')
        log_info "✅ Worker构建成功! 文件大小: $size"
        log_info "数据库: SQLite (纯Go实现，无CGO依赖)"
    else
        log_error "Worker构建失败"
        exit 1
    fi
}

# 检查二进制文件是否存在
check_binary() {
    if [ ! -f "$WORKER_BINARY" ]; then
        log_warn "Worker二进制文件不存在，开始构建..."
        build_worker
    fi
}

# 创建默认配置文件
create_default_config() {
    local config_file="$CONFIG_DIR/worker.json"
    
    if [ ! -f "$config_file" ]; then
        log_info "创建默认配置文件: $config_file"
        cat > "$config_file" << EOF
{
    "worker_id": "$DEFAULT_WORKER_ID",
    "worker_name": "$DEFAULT_WORKER_NAME",
    "gateway_url": "$DEFAULT_GATEWAY",
    "data_path": "$DATA_DIR",
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
EOF
        log_info "✅ 默认配置文件已创建"
    fi
}

# 检查Worker状态
check_worker_status() {
    local pid_file="$SCRIPT_DIR/worker.pid"
    
    if [ -f "$pid_file" ]; then
        local pid=$(cat "$pid_file")
        if ps -p "$pid" > /dev/null 2>&1; then
            log_info "✅ Worker正在运行 (PID: $pid)"
            return 0
        else
            log_warn "PID文件存在但进程不存在，清理PID文件"
            rm -f "$pid_file"
        fi
    fi
    
    # 检查是否有其他worker进程
    local running_pids=$(pgrep -f "$WORKER_BINARY" 2>/dev/null || true)
    if [ -n "$running_pids" ]; then
        log_warn "发现运行中的Worker进程: $running_pids"
        return 0
    fi
    
    log_info "Worker未运行"
    return 1
}

# 停止Worker
stop_worker() {
    local pid_file="$SCRIPT_DIR/worker.pid"
    
    if [ -f "$pid_file" ]; then
        local pid=$(cat "$pid_file")
        if ps -p "$pid" > /dev/null 2>&1; then
            log_info "停止Worker进程 (PID: $pid)..."
            kill -TERM "$pid"
            
            # 等待进程结束
            local count=0
            while ps -p "$pid" > /dev/null 2>&1 && [ $count -lt 10 ]; do
                sleep 1
                count=$((count + 1))
            done
            
            if ps -p "$pid" > /dev/null 2>&1; then
                log_warn "进程未正常结束，强制终止..."
                kill -KILL "$pid"
            fi
            
            rm -f "$pid_file"
            log_info "✅ Worker已停止"
        else
            log_warn "PID文件存在但进程不存在"
            rm -f "$pid_file"
        fi
    else
        # 尝试停止所有worker进程
        local running_pids=$(pgrep -f "$WORKER_BINARY" 2>/dev/null || true)
        if [ -n "$running_pids" ]; then
            log_info "停止所有Worker进程: $running_pids"
            echo "$running_pids" | xargs kill -TERM
            sleep 2
            # 检查是否还有残留进程
            running_pids=$(pgrep -f "$WORKER_BINARY" 2>/dev/null || true)
            if [ -n "$running_pids" ]; then
                echo "$running_pids" | xargs kill -KILL
            fi
            log_info "✅ 所有Worker进程已停止"
        else
            log_info "没有发现运行中的Worker进程"
        fi
    fi
}

# 清理数据和日志
clean_data() {
    log_warn "清理数据和日志文件..."
    
    # 先停止Worker
    if check_worker_status; then
        stop_worker
    fi
    
    # 清理文件
    [ -f "$LOG_FILE" ] && rm -f "$LOG_FILE" && log_info "✅ 已删除日志文件"
    [ -f "$DATA_DIR/worker.db" ] && rm -f "$DATA_DIR/worker.db" && log_info "✅ 已删除数据库文件"
    [ -d "$DATA_DIR/downloads" ] && rm -rf "$DATA_DIR/downloads"/* && log_info "✅ 已清理下载目录"
    [ -d "$DATA_DIR/m3u8" ] && rm -rf "$DATA_DIR/m3u8"/* && log_info "✅ 已清理视频目录"
    [ -d "$DATA_DIR/temp" ] && rm -rf "$DATA_DIR/temp"/* && log_info "✅ 已清理临时目录"
    
    log_info "✅ 数据清理完成"
}

# 启动Worker
start_worker() {
    local gateway_url="$1"
    local worker_id="$2"
    local worker_name="$3"
    local config_file="$4"
    
    # 检查是否已经在运行
    if check_worker_status; then
        log_error "Worker已在运行，请先停止现有进程"
        exit 1
    fi
    
    log_info "启动Worker节点..."
    log_info "  网关地址: $gateway_url"
    log_info "  节点ID: ${worker_id:-自动生成}"
    log_info "  节点名称: $worker_name"
    log_info "  配置文件: $config_file"
    log_info "  数据目录: $DATA_DIR"
    log_info "  日志文件: $LOG_FILE"
    
    # 构建启动参数
    local args=()
    [ -n "$gateway_url" ] && args+=("-gateway" "$gateway_url")
    [ -n "$worker_id" ] && args+=("-id" "$worker_id")
    [ -n "$worker_name" ] && args+=("-name" "$worker_name")
    [ -n "$config_file" ] && args+=("-config" "$config_file")
    
    # 启动Worker
    log_info "执行命令: $WORKER_BINARY ${args[*]}"
    
    # 后台启动并记录PID
    nohup "$WORKER_BINARY" "${args[@]}" > "$LOG_FILE" 2>&1 &
    local pid=$!
    echo "$pid" > "$SCRIPT_DIR/worker.pid"
    
    # 等待一下检查是否启动成功
    sleep 2
    if ps -p "$pid" > /dev/null 2>&1; then
        log_info "✅ Worker启动成功! (PID: $pid)"
        log_info "查看日志: tail -f $LOG_FILE"
    else
        log_error "Worker启动失败，请检查日志: $LOG_FILE"
        exit 1
    fi
}

# 主函数
main() {
    local gateway_url="$DEFAULT_GATEWAY"
    local worker_id="$DEFAULT_WORKER_ID"
    local worker_name="$DEFAULT_WORKER_NAME"
    local config_file="$CONFIG_DIR/worker.json"
    local should_build=false
    local should_clean=false
    local should_status=false
    local should_stop=false
    
    # 解析命令行参数
    while [[ $# -gt 0 ]]; do
        case $1 in
            -h|--help)
                show_help
                exit 0
                ;;
            -g|--gateway)
                gateway_url="$2"
                shift 2
                ;;
            -i|--id)
                worker_id="$2"
                shift 2
                ;;
            -n|--name)
                worker_name="$2"
                shift 2
                ;;
            -c|--config)
                config_file="$2"
                shift 2
                ;;
            -d|--data)
                DATA_DIR="$2"
                shift 2
                ;;
            -l|--log)
                LOG_FILE="$2"
                shift 2
                ;;
            --build)
                should_build=true
                shift
                ;;
            --clean)
                should_clean=true
                shift
                ;;
            --status)
                should_status=true
                shift
                ;;
            --stop)
                should_stop=true
                shift
                ;;
            *)
                log_error "未知参数: $1"
                show_help
                exit 1
                ;;
        esac
    done
    
    # 执行操作
    if [ "$should_clean" = true ]; then
        clean_data
        exit 0
    fi
    
    if [ "$should_status" = true ]; then
        check_worker_status
        exit $?
    fi
    
    if [ "$should_stop" = true ]; then
        stop_worker
        exit 0
    fi
    
    # 检查Go环境
    check_go
    
    # 创建目录
    create_directories
    
    # 构建或检查二进制文件
    if [ "$should_build" = true ]; then
        build_worker
    else
        check_binary
    fi
    
    # 创建默认配置
    create_default_config
    
    # 启动Worker
    start_worker "$gateway_url" "$worker_id" "$worker_name" "$config_file"
}

# 执行主函数
main "$@"