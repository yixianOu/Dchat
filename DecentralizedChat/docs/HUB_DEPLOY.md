# NATS Hub 部署指南

## 概述

本文档描述如何部署一个启用了 JetStream 的 NATS 服务器作为 DChat 的公网 Hub。

## 前提条件

- 公网服务器（Linux 推荐）
- 开放以下端口：
  - `4222`: NATS 客户端连接
  - `7422`: LeafNode 连接（建议使用此端口）
- 安装 NATS Server

## 快速开始

### 1. 下载 NATS Server

```bash
# 使用 Docker（推荐）
docker pull nats:latest

# 或下载二进制文件
# https://github.com/nats-io/nats-server/releases
```

### 2. 创建配置文件

创建 `hub.conf` 文件：

```conf
# NATS Hub 配置文件
# 用于 DChat 的公网 Hub

# 服务器名称（必须唯一，JetStream 需要）
server_name: "dchat-hub-1"

# 监听地址
host: "0.0.0.0"
port: 4222

# LeafNode 配置（接受 LeafNode 连接）
leafnode {
  # LeafNode 监听地址
  host: "0.0.0.0"
  port: 7422

  # 不要求 LeafNode 认证（根据需要调整）
  no_auth_user: "leafnode"
}

# JetStream 配置（用于离线消息暂存）
jetstream {
  # 启用 JetStream
  enabled: true

  # 存储目录（持久化数据）
  store_dir: "/var/lib/nats/jetstream"

  # 内存存储限制（256MB）
  max_memory_store: 268435456

  # 文件存储限制（10GB）
  max_file_store: 10737418240
}

# 日志配置
debug: false
trace: false
logtime: true

# 系统账户（可选）
# system_account: "SYS"
```

### 3. 创建数据目录

```bash
# Docker 方式不需要这步，会自动管理
# 如果使用二进制文件：
sudo mkdir -p /var/lib/nats/jetstream
sudo chown -R $(whoami) /var/lib/nats
```

### 4. 启动 Hub

#### 使用 Docker（推荐）

```bash
docker run -d \
  --name dchat-hub \
  -p 4222:4222 \
  -p 7422:7422 \
  -v $(pwd)/hub.conf:/etc/nats/hub.conf \
  -v dchat-jetstream:/var/lib/nats/jetstream \
  nats:latest \
  -c /etc/nats/hub.conf
```

#### 使用二进制文件

```bash
# 假设 nats-server 已在 PATH 中
nats-server -c hub.conf
```

### 5. 验证部署

```bash
# 检查容器状态（Docker）
docker ps | grep dchat-hub

# 查看日志
docker logs dchat-hub

# 或使用 nats CLI 工具（如果安装了）
nats pub test "hello" --server localhost:4222
```

## 配置说明

### 端口说明

| 端口 | 用途 |
|------|------|
| 4222 | NATS 客户端连接（用户设备连接到本地 LeafNode 不需要这个） |
| 7422 | LeafNode 连接（用户设备的 LeafNode 连接到 Hub 使用这个） |

### JetStream 存储

JetStream 用于暂存离线消息，配置说明：

- `store_dir`: JetStream 数据存储目录
- `max_memory_store`: 内存存储限制（字节）
- `max_file_store`: 文件存储限制（字节）

### 生产环境建议

1. **启用认证**: 生产环境应使用 NATS 账户系统认证
2. **启用 TLS**: 生产环境应启用 TLS 加密
3. **数据持久化**: 确保 `store_dir` 有足够的磁盘空间
4. **高可用**: 部署多个 Hub 并配置集群

## LeafNode 连接配置

用户设备的 LeafNode 配置应包含：

```json
{
  "leafnode": {
    "hub_urls": [
      "nats://your-hub-ip:7422"
    ]
  }
}
```

## 故障排查

### 检查端口是否开放

```bash
# 检查 4222 端口
netstat -tlnp | grep 4222

# 检查 7422 端口
netstat -tlnp | grep 7422
```

### 查看日志

```bash
# Docker
docker logs -f dchat-hub

# 二进制文件（日志输出到控制台或配置的日志文件）
```

### JetStream 状态

使用 NATS CLI 检查 JetStream 状态：

```bash
nats account info --server localhost:4222
nats stream ls --server localhost:4222
```
