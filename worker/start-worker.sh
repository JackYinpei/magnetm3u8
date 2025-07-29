#!/bin/bash

# Worker Node 启动脚本

echo "Starting Worker Node..."

# 设置默认配置
GATEWAY_URL=${GATEWAY_URL:-"ws://localhost:8080/ws/nodes"}
NODE_ID=${NODE_ID:-""}
NODE_NAME=${NODE_NAME:-"$(hostname)-worker"}
CONFIG_FILE=${CONFIG_FILE:-"config/worker.json"}

# 创建必要的目录
mkdir -p data/downloads data/m3u8 data/config data/logs

# 启动worker节点
./worker \
  --gateway="${GATEWAY_URL}" \
  --id="${NODE_ID}" \
  --name="${NODE_NAME}" \
  --config="${CONFIG_FILE}"