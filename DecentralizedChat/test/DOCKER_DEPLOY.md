# Docker 部署指南

## 架构说明

```
公网服务器                    设备A（局域网1）              设备B（局域网2）
+-------------+              +------------------+         +------------------+
| 信令服务器   |<------------>| P2P节点 (Alice)  |<=======>| P2P节点 (Bob)    |
| :8080       |   注册/查询   | :10001           |  UDP直连 | :10002           |
+-------------+              +------------------+         +------------------+
```

- **信令服务器**：部署在公网，用于节点发现和地址交换
- **P2P节点**：运行在各自局域网内，通过信令服务器建立直连

## 快速开始

### 1. 构建镜像

```bash
cd test

# 构建信令服务器
docker build -f Dockerfile.server -t p2p-signal-server .

# 构建P2P节点
docker build -f Dockerfile.client -t p2p-node .
```

### 2. 部署信令服务器（公网）

```bash
# 运行
docker run -d \
  --name signal-server \
  -p 8080:8080 \
  --restart unless-stopped \
  p2p-signal-server

# 查看日志
docker logs -f signal-server

# 测试
curl http://localhost:8080/status
```

### 3. 运行P2P节点（各局域网设备）

```bash
# 进入交互式容器
docker run -it --rm --name p2p-node p2p-node

# 在容器内启动节点（替换<公网IP>为实际IP）
./p2p_node -node-id Alice -signal-server http://<公网IP>:8080
```

## 完整测试流程

### 第一步：公网服务器启动信令服务

```bash
# 服务器IP: 121.199.173.116
docker run -d -p 8080:8080 --name signal-server p2p-signal-server
```

### 第二步：设备A（家里）运行

```bash
docker run -it --rm p2p-node

# 容器内执行
./p2p_node -node-id Alice -signal-server http://121.199.173.116:8080
```

### 第三步：设备B（公司）运行

```bash
docker run -it --rm p2p-node

# 容器内执行
./p2p_node -node-id Bob -signal-server http://121.199.173.116:8080

# 然后连接Alice
> c Alice

# 发送消息
> m Hello Alice!
```

## 常用命令

```bash
# 查看运行中的容器
docker ps

# 停止/删除信令服务器
docker stop signal-server
docker rm signal-server

# 查看日志
docker logs -f signal-server

# 保存/加载镜像（用于离线传输）
docker save p2p-signal-server > signal-server.tar
docker load < signal-server.tar
```

## 防火墙配置

公网服务器开放8080端口：

```bash
# firewalld
sudo firewall-cmd --permanent --add-port=8080/tcp
sudo firewall-cmd --reload

# 或 ufw
sudo ufw allow 8080/tcp
```

## 推送镜像到仓库（可选）

```bash
# 登录Docker Hub
docker login

# 打标签
docker tag p2p-signal-server yourname/p2p-signal-server:latest
docker tag p2p-node yourname/p2p-node:latest

# 推送
docker push yourname/p2p-signal-server:latest
docker push yourname/p2p-node:latest

# 其他机器直接拉取运行
docker run -d -p 8080:8080 yourname/p2p-signal-server
```
