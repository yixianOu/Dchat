# NATS Hub 部署指南

## 概述

本文档描述如何部署一个启用了 JetStream 的 NATS 服务器作为 DChat 的公网 Hub。

## 前提条件

- 公网服务器（Linux 推荐）
- 开放以下端口：
  - `4222`: NATS 客户端连接
  - `7422`: LeafNode 连接
  - `8222`: HTTP 监控端口（可选）

## 快速开始

### 1. 下载 NATS Server

从 GitHub Releases 下载最新的 NATS Server 二进制文件：

```bash
# 访问 https://github.com/nats-io/nats-server/releases
# 下载适合你系统的版本（例如 nats-server-v2.11.7-linux-amd64.tar.gz）

# 解压并安装
tar -xzf nats-server-v2.11.7-linux-amd64.tar.gz
sudo mv nats-server-v2.11.7-linux-amd64/nats-server /usr/local/bin/
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

# HTTP 监控端口（可选）
http_port: 8222

# LeafNode 配置（接受 LeafNode 连接）
# 注意：键名是 "leaf" 不是 "leafnode"！
leaf {
  # LeafNode 监听地址
  host: "0.0.0.0"
  port: 7422
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
```

### 3. 创建数据目录

```bash
sudo mkdir -p /var/lib/nats/jetstream
mkdir -p /home/orician/workspace/nats/jetstream
sudo chown -R $(whoami) /var/lib/nats
```

### 4. 启动 Hub

```bash
# 前台启动（测试用）
nats-server -c hub.conf

```

### 5. 验证部署

```bash
# 使用 nats CLI 工具（如果安装了）
nats pub test "hello" --server localhost:4222

# 或使用 telnet 检查端口
telnet localhost 4222
telnet localhost 7422
```

## 配置说明

### 端口说明

| 端口 | 用途 |
|------|------|
| 4222 | NATS 客户端连接 |
| 7422 | LeafNode 连接（用户设备的 LeafNode 连接到 Hub 使用这个） |
| 8222 | HTTP 监控 API（可选） |

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
5. **Systemd 服务**: 使用 systemd 管理 NATS Server 进程

## Systemd 服务配置（推荐）

创建 `/etc/systemd/system/dchat-hub.service`：

```ini
[Unit]
Description=DChat NATS Hub
After=network.target

[Service]
Type=simple
User=orician
Group=orician
WorkingDirectory=/home/orician/workspace/nats
ExecStart=/home/orician/workspace/nats/nats-server -c /home/orician/workspace/nats/hub.conf
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

启动服务：

```bash
# 启动服务
sudo systemctl daemon-reload
sudo systemctl enable dchat-hub
sudo systemctl start dchat-hub

# 查看状态
sudo systemctl status dchat-hub
sudo journalctl -u dchat-hub -f
```

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

# 或使用 ss
ss -tlnp | grep -E '4222|7422'
```

### 查看日志

```bash
# 如果使用 systemd
sudo journalctl -u dchat-hub -f
```

### JetStream 状态

使用 NATS CLI 检查 JetStream 状态：

```bash
nats account info --server localhost:4222
nats stream ls --server localhost:4222
```
