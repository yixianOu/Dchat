# Docker 部署指南

## 架构说明

```
公网服务器                    局域网1设备                  局域网2设备
+-------------+              +------------------+         +------------------+
| 信令服务器   |<------------>| P2P节点 (Alice)  |<=======>| P2P节点 (Bob)    |
| :8080       |   注册/查询   | :10001           |  UDP直连 | :10002           |
+-------------+              +------------------+         +------------------+
```

**注意**：同一台机器上的多个 Docker 容器**无法**模拟跨局域网环境，因为它们共享网络。真实测试需要两台不同物理设备。

## 构建镜像

```bash
cd test && make build
```

## 部署信令服务器（公网）

```bash
docker run -d \
  --name signal-server \
  -p 8080:8080 \
  --restart unless-stopped \
  p2p-signal-server
```

## 部署 P2P 节点（各局域网设备）

**重要：必须使用 `--network host` 模式**

由于 Docker 的端口映射会导致 STUN 获取的公网端口与容器监听端口不一致，必须使用 host 网络模式：

```
容器内端口 10001
    ↓ (host 模式，无 NAT)
宿主机端口 10001
    ↓ 路由器 NAT
公网端口 10001 (STUN 获取的一致)
```

### 设备 A（局域网 1）：

```bash
docker run -it --rm \
  --network host \
  --name p2p-node \
  p2p-node /p2p_node \
  -node-id Alice \
  -listen-port 10001 \
  -signal-server http://121.199.173.116:8080
```

### 设备 B（局域网 2）：

```bash
docker run -it --rm \
  --network host \
  --name p2p-node \
  p2p-node /p2p_node \
  -node-id Bob \
  -listen-port 10002 \
  -signal-server http://121.199.173.116:8080
```

### 验证启动成功

启动后会显示类似以下信息：

```
========================================
P2P NAT穿透测试节点
========================================
节点已创建: Alice
监听地址: 0.0.0.0:10001

正在通过STUN获取公网地址...
✅ 公网地址: 120.239.59.111:10001   <-- 端口应与监听端口一致
   NAT类型: Cone NAT

正在注册到信令服务器...
✅ 已注册到信令服务器
```

**关键检查点**：`公网地址` 的端口必须与 `-listen-port` 指定的端口一致（如都是 10001）。如果不一致，说明存在多层 NAT，打洞会失败。

### 交互式命令

```
> l              # 查看在线节点
> c Bob          # 连接到 Bob（执行 UDP 打洞）
> m Hello!       # 发送消息
> s              # 显示状态（查看 connection 是否为 true）
> h              # 再次打洞
> q              # 退出
```

## 常见问题

### 问题 1：公网端口与监听端口不一致

**症状**：
```
监听地址: 0.0.0.0:10001
公网地址: 120.239.59.111:2162  <-- 端口不一致！
```

**原因**：使用了 `-p 10001:10001/udp` 映射，导致 Docker 多了一层 NAT

**解决**：使用 `--network host` 模式

### 问题 2：连接状态始终为 false

**症状**：执行 `c Bob` 后，`s` 显示 `连接状态: false`

**排查步骤**：
1. 检查双方公网端口是否与监听端口一致
2. 检查路由器是否为 Symmetric NAT（`s` 命令查看 NAT 类型）
3. 检查防火墙是否阻止 UDP 入站
4. 在双方节点都执行 `c <对方ID>`（双向打洞）

### 问题 3：一方是 Symmetric NAT

**症状**：无法收到打洞包，连接始终失败

**原因**：Symmetric NAT 会为每个目标分配不同端口，无法预测

**解决**：需要部署 TURN 中继服务器（当前版本不支持）

## 不使用 host 模式的替代方案（不推荐）

如果无法使用 host 模式（如 Mac/Windows Docker），需要在路由器上配置**端口转发**：

```
路由器配置：
外部端口 10001 → 转发到 宿主机IP:10001
```

然后使用端口映射启动：
```bash
docker run -it --rm \
  -p 10001:10001/udp \
  --name p2p-node \
  p2p-node /p2p_node \
  -node-id Alice \
  -listen-port 10001 \
  -signal-server http://121.199.173.116:8080
```

**注意**：即使配置了端口转发，STUN 获取的端口仍可能与监听端口不一致，导致打洞失败。

## 常用命令

```bash
# 导出镜像（用于离线传输到其他设备）
make export
# 生成 dist/p2p-signal-server.tar 和 dist/p2p-node.tar

# 在其他设备加载镜像
docker load -i p2p-signal-server.tar
docker load -i p2p-node.tar

# 查看日志
docker logs -f signal-server
```

## 防火墙配置

### 公网服务器

开放 8080 端口：
```bash
sudo firewall-cmd --permanent --add-port=8080/tcp
sudo firewall-cmd --reload
```

### 局域网设备

确保 UDP 出站和入站不被阻止：
```bash
# Linux (iptables)
sudo iptables -I INPUT -p udp --dport 10001 -j ACCEPT

# 或使用 ufw
sudo ufw allow 10001/udp
```

### 路由器

部分路由器需要开启 UPnP 或手动配置端口转发。

## 测试验证清单

- [ ] 设备 A 启动，STUN 获取公网地址成功，端口与监听端口一致
- [ ] 设备 B 启动，STUN 获取公网地址成功，端口与监听端口一致
- [ ] 双方 `l` 命令都能看到对方在线
- [ ] 双方互相执行 `c <对方ID>` 进行双向打洞
- [ ] `s` 命令显示 `连接状态: true`
- [ ] `m <消息>` 双方都能收到对方消息

---

## 自建 STUN 服务器（Docker）

使用 coturn 镜像快速部署：

```bash
docker run -d \
  --name stun-server \
  --restart unless-stopped \
  --network host \
  coturn/coturn:latest \
  -n \
  --listening-port=3478 \
  --listening-ip=0.0.0.0 \
  --external-ip=121.199.173.116 \
  --realm=stun.dchat.local \
  --no-cli --no-tls --no-dtls \
  --log-file=stdout --verbose
```

防火墙开放 3478/udp 端口后，修改代码使用 `121.199.173.116:3478`。
