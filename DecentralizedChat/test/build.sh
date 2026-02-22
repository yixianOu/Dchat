#!/bin/bash
# Docker 构建脚本

set -e

echo "=== 构建 P2P NAT 穿透测试镜像 ==="

# 构建信令服务器镜像
echo "[1/2] 构建信令服务器镜像..."
docker build -f Dockerfile.server -t p2p-signal-server .

# 构建 P2P 节点镜像
echo "[2/2] 构建 P2P 节点镜像..."
docker build -f Dockerfile.client -t p2p-node .

echo ""
echo "=== 构建完成 ==="
echo ""
echo "运行信令服务器:"
echo "  docker run -d -p 8080:8080 --name signal-server p2p-signal-server"
echo ""
echo "运行 P2P 节点 (交互式):"
echo "  docker run -it --rm --name p2p-node p2p-node"
echo "  # 然后在容器内执行:"
echo "  ./p2p_node -node-id Alice -signal-server http://<服务器IP>:8080"
echo ""
echo "或者直接带参数运行:"
echo "  docker run -it --rm p2p-node ./p2p_node -node-id Bob -signal-server http://<服务器IP>:8080"
