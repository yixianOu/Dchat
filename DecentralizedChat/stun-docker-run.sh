#!/bin/bash
# STUN服务器Docker启动脚本

# 配置
STUN_PORT=3478
EXTERNAL_IP="121.199.173.116"

# 停止并删除旧容器
docker stop stun-server 2>/dev/null
docker rm stun-server 2>/dev/null

# 启动STUN服务器
echo "正在启动STUN服务器..."
echo "外部IP: $EXTERNAL_IP"
echo "端口: $STUN_PORT"

docker run -d \
    --name stun-server \
    --restart unless-stopped \
    --network host \
    coturn/coturn:latest \
    -n \
    --listening-port=$STUN_PORT \
    --listening-ip=0.0.0.0 \
    --external-ip=$EXTERNAL_IP \
    --realm=stun.dchat.local \
    --no-cli \
    --no-tls \
    --no-dtls \
    --log-file=stdout \
    --verbose

echo "STUN服务器已启动"
echo "测试命令:"
echo "  docker logs -f stun-server"
