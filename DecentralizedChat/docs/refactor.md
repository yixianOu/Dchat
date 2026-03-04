# LeafNode 架构重构计划

## 重要更新：持久化方案决策（2026-03-04）

经过实际测试验证，我们对持久化方案做出以下调整：

| 层级 | 技术方案 | 说明 |
|------|----------|------|
| **本地历史消息** | **SQLite** | 不使用 JetStream，原因见下文 |
| **Hub 离线消息** | **JetStream** | 继续使用 JetStream |
| **实时消息** | NATS Core + LeafNode | 不变 |

### 为什么本地不用 JetStream？

**JetStream 的局限性：**
- ❌ 查询能力有限 - 只能按 subject/sequence 消费
- ❌ 无法实现聊天应用常用功能：
  - 按会话查询历史消息
  - 按时间范围筛选
  - 全文搜索消息内容
  - 分页显示
  - 标记已读/未读状态

**SQLite 的优势：**
- ✅ 强大的 SQL 查询能力
- ✅ 支持索引，查询性能好
- ✅ 成熟稳定，工具丰富
- ✅ 可以轻松实现聊天应用的所有需求

### 更新后的架构

```
┌─────────────────────────────────────────────────────────┐
│                    LeafNode (用户设备)                   │
│                                                          │
│  ┌──────────────────┐      ┌───────────────────────┐   │
│  │  NATS Core       │      │  SQLite (本地历史)    │   │
│  │  (实时消息)      │─────▶│  - 会话表            │   │
│  │                 │      │  - 消息表            │   │
│  └──────────────────┘      │  - 索引: 时间/发送者 │   │
│              │             └───────────────────────┘   │
└──────────────┼──────────────────────────────────────────┘
               │
         LeafNode 连接
               │
┌──────────────▼──────────────────────────────────────────┐
│                    Hub (公网服务器)                      │
│                                                          │
│  ┌──────────────────────────────────────────────────┐  │
│  │  JetStream (离线消息暂存)                         │  │
│  │  - Stream: OFFLINE_MSGS                          │  │
│  │  - TTL: 7天                                       │  │
│  └──────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────┘
```

---

## 一、为什么用 LeafNode 代替 Routes？

### 1.1 Routes 架构的问题

**当前 Routes 架构：**
```
                    公网
              ┌──────────┐
              │  Node-A  │ (公网IP)
              └────┬─────┘
                   │
         ┌─────────┼─────────┐
         │         │         │
    ┌────▼────┐ ┌▼────┐ ┌▼────┐
    │  Node-B │ │Node-C│ │Node-D│
    │(局域网) │ │(局域网)│ │(局域网)│
    └─────────┘ └─────┘ └─────┘
```

**核心问题：**

1. **NAT 穿透困难**
   - Routes 要求所有节点网络互通
   - 局域网节点无法被外部直接访问
   - 家庭/办公室网络通常有防火墙/NAT

2. **配置复杂**
   - 每个节点需要配置 cluster_port
   - 需要配置防火墙开放端口
   - 需要配置 cluster_advertise（公网IP）

---

### 1.2 LeafNode 架构的优势

**LeafNode Hub-Spoke 架构：**
```
                    公网 Hub 集群
              (JetStream 启用)
        ┌──────────────┐
        │  Hub-1       │
        │  (JetStream) │◄──┐
        └──────────────┘   │
                              │
        ┌──────────────┐   │
        │  Hub-2       │◄──┤
        │  (JetStream) │   │
        └──────────────┘   │
                              │
        ┌──────────────┐   │
        │  Hub-3       │◄──┘
        │  (JetStream) │
        └──────────────┘
              │
              │ LeafNode 连接
              │ (支持多个 Remotes)
              │
    ┌─────────┴─────────┐
    │                   │
用户设备 A        用户设备 B
(LeafNode 模式)    (LeafNode 模式)
┌─────────────┐  ┌─────────────┐
│ 本地 NATS   │  │ 本地 NATS   │
│ LeafNode    │  │ LeafNode    │
│ (多 Hub)    │  │ (多 Hub)    │
│ JetStream    │  │ JetStream    │
└─────────────┘  └─────────────┘
       │                  │
  本地客户端          本地客户端
(NATS Client)      (NATS Client)
```

**核心优势：**

1. **无需 NAT 穿透**
   - LeafNode（用户设备）主动连接 Hub
   - 不需要公网 IP
   - 不需要开放端口
   - 适合局域网/移动网络

2. **配置简单**
   - 用户只需配置 Hub 地址列表
   - 无需集群配置
   - 无需防火墙设置

---

## 二、Routes vs LeafNode 详细对比

### 2.1 连接方向对比

| 特性 | Routes | LeafNode |
|------|--------|----------|
| 连接方向 | 双向连接（所有节点互通） | 单向连接（Spoke→Hub） |
| 需要公网 IP | 是 | 否（仅 Hub 需要） |
| 需要开放端口 | 是 | 否（仅 Hub 需要） |
| NAT 穿透 | 需要 | 不需要 |

### 2.2 消息流对比

**Routes 消息流（1跳）：**
```
用户A 发布消息
    │
    ├─> 直接发送给所有订阅者节点
    │
    └─> 用户B 直接收到消息

延迟：1跳 (用户A → 用户B)
```

**LeafNode 消息流（2跳）：**
```
用户A 发布消息
    │
    ├─> 本地 LeafNode → Hub
    │
    ├─> Hub 转发给其他 LeafNode
    │
    └─> 用户B 的 LeafNode 收到消息

延迟：2跳 (用户A → Hub → 用户B)
```

### 2.3 去中心化程度对比

| 特性 | Routes | LeafNode |
|------|--------|----------|
| 用户层 | 完全去中心化 | 中心化（依赖 Hub） |
| Hub 层 | 无 | 可去中心化（Hub 间用 Routes） |
| 单点故障 | 无 | Hub 故障影响用户 |
| 推荐方案 | 全公网环境 | 混合网络环境 |

---

## 三、目标架构详细设计

### 3.1 整体架构

#### 架构分层

```
┌─────────────────────────────────────────────────────────────┐
│ Layer 4: 应用层（用户界面）                        │
│ React 前端 + Wails                          │
└─────────────────────────────────────────────────────────────┘
                          ↕
┌─────────────────────────────────────────────────────────────┐
│ Layer 3: 业务层（聊天服务）                      │
│ internal/chat.Service                          │
│ - 加密/解密                              │
│ - 密钥管理                              │
│ - 消息处理                              │
└─────────────────────────────────────────────────────────────┘
                          ↕
┌─────────────────────────────────────────────────────────────┐
│ Layer 2: 本地 NATS（LeafNode 模式）            │
│ 本地 NATS Server + JetStream                  │
│ - 连接公网 Hub                          │
│ - 本地消息持久化                          │
│ - KV 存储（密钥）                        │
└─────────────────────────────────────────────────────────────┘
                          ↕ LeafNode 连接
┌─────────────────────────────────────────────────────────────┐
│ Layer 1: 公网 Hub 层                          │
│ 公网 NATS Server + JetStream + Routes          │
│ - 离线消息存储                          │
│ - 消息转发                              │
│ - Hub 间集群（可选）                        │
└─────────────────────────────────────────────────────────────┘
```

### 3.2 本地启动详细流程

```
用户启动应用：
    │
    ├─> Step 1: 加载配置
    │     ├─> 从 ~/.dchat/config.json 读取
    │     ├─> 获取 HubURLs 列表
    │     └─> 获取本地监听地址/端口
    │
    ├─> Step 2: 初始化 NSC（安全设置）
    │     ├─> 检查 Operator/Account/User
    │     └─> 生成 .creds 凭据文件
    │
    ├─> Step 3: 启动本地 LeafNode
    │     │
    │     ├─> 3.1 配置 NATS Server
    │     │     ├─> 本地监听: 127.0.0.1:4222
    │     │     ├─> 不接受外部连接（安全）
    │     │     ├─> 启用 JetStream（本地持久化）
    │     │     └─> 配置 Remotes（多个 Hub）
    │     │
    │     ├─> 3.2 解析 Hub URLs
    │     │     ├─> Hub-1: nats://hub1.example.com:7422
    │     │     ├─> Hub-2: nats://hub2.example.com:7422
    │     │     └─> 验证 URL 格式
    │     │
    │     ├─> 3.3 启动 NATS Server
    │     │     ├─> 初始化 JetStream
    │     │     ├─> 创建 Stream/KV Bucket
    │     │     └─> 开始连接 Hub
    │     │
    │     └─> 3.4 等待连接成功
    │           ├─> 等待至少连接 1 个 Hub
    │           ├─> 超时: 10 秒
    │           └─> 失败: 报错退出
    │
    ├─> Step 4: 创建本地 NATS Client
    │     ├─> 连接本地: nats://127.0.0.1:4222
    │     ├─> 使用 .creds 认证
    │     └─> 连接成功
    │
    ├─> Step 5: 初始化聊天服务
    │     ├─> 加载 NSC 密钥
    │     ├─> 设置消息回调
    │     └─> 订阅消息主题
    │
    └─> Step 6: 应用启动完成
          ├─> 前端加载
          ├─> 显示聊天界面
          └─> 开始接收消息
```

### 3.3 LeafNode 配置详细说明

**配置文件位置**: `~/.dchat/config.json`

```json
{
  "user": {
    "id": "user_abc123xyz",
    "nickname": "Alice"
  },
  "leafnode": {
    "local_host": "127.0.0.1",
    "local_port": 4222,
    "hub_urls": [
      "nats://hub1.dchat.example.com:7422"
    ],
    "enable_tls": false,
    "enable_jetstream": true,
    "jetstream_store_dir": ""
  },
  "keys": {
    "operator": "dchat",
    "keys_dir": "~/.dchat/nsc",
    "user_creds_path": "~/.dchat/nsc/dchat/USERS/default.creds",
    "user_seed_path": "~/.dchat/nsc/dchat/USERS/default.seed",
    "account": "USERS",
    "user": "default"
  }
}
```

**配置说明**:

| 字段 | 说明 | 默认值 |
|------|------|--------|
| `local_host` | 本地监听地址 | 127.0.0.1 |
| `local_port` | 本地监听端口 | 4222 |
| `hub_urls` | Hub 地址列表（支持多个） | 项目默认 Hub |
| `enable_tls` | 是否启用 TLS | false |
| `enable_jetstream` | 是否启用 JetStream | true |
| `jetstream_store_dir` | JetStream 存储目录 | 空（默认位置） |

### 3.4 消息流详细设计

#### 用户发送消息流程

```
用户在前端输入消息并点击发送
    │
    ↓ (React → Wails)
app.SendDirect(peerID, content)
    │
    ↓
chat.Service.SendDirect()
    ├─> 1. 派生会话 ID (deriveCID)
    ├─> 2. 获取好友公钥
    ├─> 3. NaCl Box 加密
    ├─> 4. 构造 encWire {cid, sender, ts, nonce, cipher}
    └─> 5. 序列化为 JSON
    │
    ↓
nats.Service.Publish("dchat.dm.{cid}.msg", data)
    │
    ↓ (本地 NATS Client → 本地 LeafNode)
本地 NATS Server (LeafNode)
    │
    ↓ (LeafNode 连接)
公网 Hub
    │
    ↓ (Hub 转发)
其他用户的 LeafNode
    │
    ↓ (本地 NATS Server)
其他用户的 nats.Client 收到消息
    │
    ↓
chat.Service.handleEncrypted()
    ├─> 1. 反序列化 encWire
    ├─> 2. 获取解密密钥
    ├─> 3. NaCl Box 解密
    └─> 4. 构造 DecryptedMessage
    │
    ↓
触发 OnDecrypted 回调
    │
    ↓ (Wails → React)
前端实时显示消息
```

#### 消息持久化流程

**本地历史消息（SQLite）：**
```
收到解密后的消息
    │
    ├─> Step 1: 实时显示在 UI
    │
    └─> Step 2: 异步写入 SQLite
          ├─> 表: messages
          ├─> 字段: id, cid, sender_id, sender_nickname,
          │       content, timestamp, is_read, is_group
          └─> 索引: (cid, timestamp DESC), (sender_id)
```

**查询历史消息（SQLite）：**
```
前端请求历史消息
    │
    ↓
app.GetMessageHistory(cid, limit, before_ts)
    │
    ↓
chat.Service.GetHistory(cid, limit, before_ts)
    ├─> Step 1: SQL 查询
    │     └─> SELECT * FROM messages
    │            WHERE cid = ? AND timestamp < ?
    │            ORDER BY timestamp DESC LIMIT ?
    ├─> Step 2: 扫描行到 Message 结构体
    └─> Step 3: 反转顺序（从旧到新显示）
    │
    ↓
返回给前端显示
```

**SQLite Schema 设计：**
```sql
-- 会话表
CREATE TABLE conversations (
    id TEXT PRIMARY KEY,
    type TEXT NOT NULL, -- 'dm' or 'group'
    last_message_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 消息表
CREATE TABLE messages (
    id TEXT PRIMARY KEY,
    cid TEXT NOT NULL,
    sender_id TEXT NOT NULL,
    sender_nickname TEXT,
    content TEXT NOT NULL,
    timestamp TIMESTAMP NOT NULL,
    is_read BOOLEAN DEFAULT 0,
    is_group BOOLEAN DEFAULT 0,
    FOREIGN KEY (cid) REFERENCES conversations(id)
);

-- 索引
CREATE INDEX idx_messages_cid_time ON messages(cid, timestamp DESC);
CREATE INDEX idx_messages_sender ON messages(sender_id);
```

---

## 四、重构分阶段详细实施计划

### Phase 1: 清理旧代码（0.5天）

#### 1.1 删除 routes 相关代码

**删除的文件/目录：**
- [ ] 删除 `internal/routes/` 完整目录
- [ ] 删除 `docker/routes/` 相关配置（如有）
- [ ] 删除 `docs/routes/` 相关文档（如有）

**修改的文件：**
- [ ] 修改 `app.go`
  - 删除 `nodeManager` 字段
  - 删除 `startNATSNode()` 方法
  - 删除 `GenerateSSLCertificate()` 方法
  - 删除 `GetAllDerivedKeys()` 方法

- [ ] 修改 `go.mod`
  - 移除 routes 相关导入

- [ ] 修改 `config.go`
  - 删除 `ServerOptionsLite` 结构体
  - 删除 `BuildServerOptions()` 方法
  - 删除 `AddSubscribePermissionAndSave()` 方法
  - 删除 `RemoveSubscribePermissionAndSave()` 方法

#### 1.2 清理后的配置结构

**简化后的 Config 结构：**
```go
type Config struct {
    User     UserConfig     `json:"user"`
    LeafNode LeafNodeConfig `json:"leafnode"`  // 新增
    Keys     KeysConfig     `json:"keys"`
    // 删除 Server 字段
}
```

---

### Phase 2: LeafNode 配置模块（1天）

#### 2.1 配置定义（已合并到 config 包）

**说明**: LeafNode 配置已合并到 `internal/config/config.go` 中，不再有单独的 `leafnode/config.go` 文件。

**目录结构：**
```
internal/leafnode/
└── manager.go   # 管理器
```

**LeafNodeConfig 定义在 `internal/config/config.go`**:
```go
// LeafNodeConfig LeafNode 配置
type LeafNodeConfig struct {
    // 本地监听地址
    LocalHost string
    LocalPort int

    // 公网 Hub 地址列表（支持多个）
    // 按优先级排序，先尝试第一个，失败则尝试第二个
    HubURLs []string

    // 认证（用于连接 Hub）
    CredsFile string

    // TLS
    EnableTLS bool

    // 连接超时
    ConnectTimeout time.Duration
}

// DefaultLeafNodeConfig 返回默认的 LeafNode 配置
func DefaultLeafNodeConfig() *LeafNodeConfig {
    return &LeafNodeConfig{
        LocalHost: "127.0.0.1",
        LocalPort: 4222,
        HubURLs: []string{
            "nats://hub1.dchat.example.com:7422",
            "nats://hub2.dchat.example.com:7422",
        },
        CredsFile:      "",
        EnableTLS:      false,
        ConnectTimeout: 10 * time.Second,
    }
}
```

**注意**: `SQLitePath` 是 `Config` 结构体的顶层字段，不属于 `LeafNodeConfig`，因为 LeafNode manager 不使用它。

---

### Phase 3: LeafNode 管理模块（2天）

#### 3.1 Manager 结构体

**文件**: `internal/leafnode/manager.go`

```go
package leafnode

import (
    "fmt"
    "net/url"
    "sync"
    "time"

    "github.com/nats-io/nats-server/v2/server"
)

// Manager LeafNode 管理器
type Manager struct {
    config *Config
    server *server.Server
    mu     sync.RWMutex

    // 连接状态
    connectedHubCount int
    lastError         error
}

// NewManager 创建管理器
func NewManager(cfg *Config) *Manager {
    return &Manager{
        config: cfg,
    }
}

// Start 启动 LeafNode
func (m *Manager) Start() error {
    m.mu.Lock()
    defer m.mu.Unlock()

    if m.server != nil {
        return fmt.Errorf("leafnode already started")
    }

    // 1. 解析 Hub URLs
    remotes, err := m.parseHubURLs()
    if err != nil {
        return err
    }

    if len(remotes) == 0 {
        return fmt.Errorf("no valid hub URLs configured")
    }

    // 2. 配置 NATS Server
    opts := m.buildServerOptions(remotes)

    // 3. 创建并启动服务器
    srv, err := server.NewServer(opts)
    if err != nil {
        return err
    }

    go srv.Start()

    // 4. 等待就绪
    if !srv.ReadyForConnections(m.config.ConnectTimeout) {
        return fmt.Errorf("leafnode failed to start within timeout")
    }

    // 5. TODO: 验证至少连接了一个 Hub
    // 需要监控 outbound LeafNode 连接

    m.server = srv
    return nil
}

// Stop 停止 LeafNode
func (m *Manager) Stop() {
    m.mu.Lock()
    defer m.mu.Unlock()

    if m.server != nil {
        m.server.Shutdown()
        m.server = nil
    }
}

// IsRunning 是否正在运行
func (m *Manager) IsRunning() bool {
    m.mu.RLock()
    defer m.mu.RUnlock()
    return m.server != nil && m.server.Running()
}

// GetLocalNATSURL 获取本地 NATS 连接地址
func (m *Manager) GetLocalNATSURL() string {
    return fmt.Sprintf("nats://%s:%d", m.config.LocalHost, m.config.LocalPort)
}

// GetConnectedHubCount 获取已连接的 Hub 数量
func (m *Manager) GetConnectedHubCount() int {
    m.mu.RLock()
    defer m.mu.RUnlock()

    if m.server == nil {
        return 0
    }

    // TODO: 需要获取 outbound LeafNode 连接数
    // NumLeafNodes() 返回的是 inbound 连接
    return m.server.NumLeafNodes()
}

// 内部方法

func (m *Manager) parseHubURLs() ([]*server.RemoteLeafOpts, error) {
    var remotes []*server.RemoteLeafOpts

    for _, hubURL := range m.config.HubURLs {
        u, err := url.Parse(hubURL)
        if err != nil {
            continue // 跳过无效 URL
        }
        remotes = append(remotes, &server.RemoteLeafOpts{
            URLs: []*url.URL{u},
        })
    }

    return remotes, nil
}

func (m *Manager) buildServerOptions(remotes []*server.RemoteLeafOpts) *server.Options {
    opts := &server.Options{
        Host: m.config.LocalHost,
        Port: m.config.LocalPort,
        LeafNode: server.LeafNodeOpts{
            Host:    m.config.LocalHost,
            Port:    -1, // 不接受 incoming LeafNode 连接
            Remotes: remotes,
        },
        NoLog:  true,
        NoSigs: true,
    }

    // LeafNode 不需要本地 JetStream（用 SQLite 代替）
    return opts
}
```

---

### Phase 4: 重构配置系统（1天）

#### 4.1 更新 Config 结构

**文件**: `internal/config/config.go`

**主要变更：**
- 删除 `ServerOptionsLite` 结构体
- 删除 `Server` 字段
- 新增 `LeafNode` 字段
- 新增 `SQLitePath` 顶层字段（不属于 LeafNodeConfig，因为 LeafNode manager 不使用它）

**新增的 LeafNodeConfig：**
```go
// LeafNodeConfig LeafNode 配置
type LeafNodeConfig struct {
    LocalHost      string        `json:"local_host"`
    LocalPort      int           `json:"local_port"`
    HubURLs        []string      `json:"hub_urls"`
    CredsFile      string        `json:"creds_file"`
    EnableTLS      bool          `json:"enable_tls"`
    ConnectTimeout time.Duration `json:"connect_timeout"`
}
```

**更新默认配置：**
```go
var defaultConfig = Config{
    User: UserConfig{
        ID:       "",
        Nickname: "Anonymous",
    },
    LeafNode: LeafNodeConfig{
        LocalHost: "127.0.0.1",
        LocalPort: 4222,
        HubURLs: []string{
            "nats://hub1.dchat.example.com:7422",
            "nats://hub2.dchat.example.com:7422",
        },
        CredsFile:      "",
        EnableTLS:      false,
        ConnectTimeout: 10 * time.Second,
    },
    SQLitePath: "", // 默认 ~/.dchat/chat.db，顶层字段
    Keys: KeysConfig{
        // ... 保持不变
    },
}
```

---

### Phase 5: 重构 app.go（2天）

#### 5.1 更新导入和字段

**移除的导入：**
- `DecentralizedChat/internal/routes`

**新增的导入：**
- `DecentralizedChat/internal/leafnode`

**更新 App 结构体：**
```go
type App struct {
    ctx            context.Context
    chatSvc        *chat.Service
    natsSvc        *nats.Service
    leafnodeMgr    *leafnode.Manager  // 替换 nodeManager
    storage        *storage.Storage    // 新增：SQLite 本地存储
    config         *config.Config
    mu             sync.RWMutex
}
```

#### 5.2 更新 OnStartup 流程

**主要变更：**
1. 移除 NodeManager 初始化
2. 移除 startNATSNode() 调用
3. 新增 LeafNode 初始化和启动
4. 更新 NATS 客户端连接地址（本地 LeafNode）

**代码参考：** 见前文"3.2 本地启动详细流程"

#### 5.3 更新 OnShutdown

```go
func (a *App) OnShutdown(ctx context.Context) {
    if a.leafnodeMgr != nil {
        a.leafnodeMgr.Stop()
    }
    if a.natsSvc != nil {
        a.natsSvc.Close()
    }
}
```

#### 5.4 更新 GetNetworkStatus

```go
func (a *App) GetNetworkStatus() (map[string]interface{}, error) {
    if a.natsSvc == nil || a.leafnodeMgr == nil {
        return nil, fmt.Errorf("services not initialized")
    }

    result := make(map[string]interface{})
    result["nats"] = a.natsSvc.GetStats()
    result["leafnode"] = map[string]interface{}{
        "connected":         a.leafnodeMgr.IsRunning(),
        "hub_urls":          a.config.LeafNode.HubURLs,
        "connected_hubs":   a.leafnodeMgr.GetConnectedHubCount(),
        "local_url":         a.leafnodeMgr.GetLocalNATSURL(),
        "jetstream_enabled": a.config.LeafNode.EnableJetStream,
    }
    result["config"] = map[string]interface{}{
        "hub_urls": a.config.LeafNode.HubURLs,
    }

    return result, nil
}
```

---

### Phase 6: 创建本地存储模块（2天）

#### 6.1 创建 SQLite 本地存储包

**目录结构：**
```
internal/
└── storage/
    ├── sqlite.go       # SQLite 实现
    ├── schema.go       # 数据库 schema 和迁移
    └── types.go        # 数据类型定义
```

#### 6.2 Schema 和迁移

**文件**: `internal/storage/schema.go`

```go
package storage

const schema = `
CREATE TABLE IF NOT EXISTS conversations (
    id TEXT PRIMARY KEY,
    type TEXT NOT NULL,
    last_message_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS messages (
    id TEXT PRIMARY KEY,
    cid TEXT NOT NULL,
    sender_id TEXT NOT NULL,
    sender_nickname TEXT,
    content TEXT NOT NULL,
    timestamp TIMESTAMP NOT NULL,
    is_read BOOLEAN DEFAULT 0,
    is_group BOOLEAN DEFAULT 0,
    FOREIGN KEY (cid) REFERENCES conversations(id)
);

CREATE INDEX IF NOT EXISTS idx_messages_cid_time ON messages(cid, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_messages_sender ON messages(sender_id);
`
```

#### 6.3 Storage 接口

**文件**: `internal/storage/sqlite.go`

```go
package storage

import (
    "database/sql"
    "time"

    _ "modernc.org/sqlite"
)

type Storage struct {
    db *sql.DB
}

func NewSQLiteStorage(path string) (*Storage, error) {
    db, err := sql.Open("sqlite", path)
    if err != nil {
        return nil, err
    }
    // 执行 schema 初始化
    _, err = db.Exec(schema)
    if err != nil {
        return nil, err
    }
    return &Storage{db: db}, nil
}

func (s *Storage) SaveMessage(msg *StoredMessage) error {
    // INSERT OR REPLACE INTO messages ...
}

func (s *Storage) GetMessages(cid string, limit int, before *time.Time) ([]*StoredMessage, error) {
    // SELECT * FROM messages WHERE cid = ? AND timestamp < ? ORDER BY ...
}

func (s *Storage) MarkAsRead(cid string, before time.Time) error {
    // UPDATE messages SET is_read = 1 WHERE cid = ? AND timestamp < ?
}
```

**详细实现参考：** 见前文"3.4 消息持久化流程"

**关键方法：**
- `SaveMessage(msg)` - 保存消息到 SQLite
- `GetMessages(cid, limit, before)` - 查询历史消息
- `MarkAsRead(cid, before)` - 标记已读
- `SearchMessages(query)` - 全文搜索

---

### Phase 7: 测试与验证（2天）

#### 7.1 单元测试

**测试清单：**

- [ ] `internal/leafnode/config_test.go`
  - 测试默认配置
  - 测试配置验证

- [ ] `internal/leafnode/manager_test.go`
  - 测试启动/停止
  - 测试连接状态
  - 测试多 Hub 配置

- [ ] `internal/storage/sqlite_test.go`
  - 测试数据库初始化
  - 测试消息保存/查询
  - 测试分页查询
  - 测试标记已读
  - 测试全文搜索

- [ ] `internal/config/config_test.go`
  - 测试配置加载/保存
  - 测试默认值填充

#### 7.2 集成测试

**测试场景：**

参考 `docs/leaf-node/cmd/test1/leafnode_test.go`：

- [ ] 场景 1: 单 Hub + 单 LeafNode
  - 启动测试 Hub（内存模式）
  - 启动 LeafNode 连接 Hub
  - 验证连接成功
  - 验证消息收发

- [ ] 场景 2: 多 Hub + 单 LeafNode
  - 启动 2 个测试 Hub
  - 配置 LeafNode 连接两个 Hub
  - 验证连接成功
  - 断开一个 Hub，验证自动切换

- [ ] 场景 3: 双 LeafNode 通信
  - 启动 2 个 LeafNode 连接同一个 Hub
  - LeafNode1 发送消息
  - LeafNode2 接收消息
  - 验证消息正确

- [ ] 场景 4: 历史消息存储
  - 发送多条消息
  - 重启应用
  - 查询历史消息
  - 验证消息完整性

- [ ] 场景 5: 离线消息（Hub 端 JetStream）
  - LeafNode1 发送消息（LeafNode2 离线）
  - LeafNode2 上线
  - 验证收到离线消息

---

### Phase 8: 文档更新（1天）

**更新的文档：**
- [ ] `README.md` - 更新架构说明
- [ ] `INTERFACE_DOCS.md` - 更新 API 文档
- [ ] `docs/TODO.md` - 更新任务清单

**新增的文档：**
- [ ] `docs/HUB_SETUP.md` - Hub 部署指南
  - Hub 配置说明
  - JetStream 配置
  - Docker 部署
  - 系统服务配置

---

## 五、关键目录结构变更

### 变更前：

```
DecentralizedChat/
├── app.go
├── main.go
├── internal/
│   ├── chat/
│   ├── config/
│   ├── nats/
│   ├── nscsetup/
│   └── routes/        <-- 删除
│       └── routes.go
└── ...
```

### 变更后：

```
DecentralizedChat/
├── app.go
├── main.go
├── internal/
│   ├── chat/         <-- 不变
│   ├── config/       <-- 更新（删除 Server，新增 LeafNode）
│   ├── nats/         <-- 更新（新增 JetStream Stream）
│   ├── nscsetup/     <-- 不变
│   └── leafnode/     <-- 新增
│       ├── config.go
│       └── manager.go
└── ...
```

---

## 六、时间估算

| 阶段 | 工作量 | 说明 |
|------|--------|------|
| Phase 1: 清理旧代码 | 0.5 天 | 删除 routes 相关代码 |
| Phase 2: LeafNode 配置 | 1 天 | 配置结构体和默认值 |
| Phase 3: LeafNode 管理 | 2 天 | Manager 实现和测试 |
| Phase 4: 配置系统 | 1 天 | 更新 config.go |
| Phase 5: 重构 app.go | 2 天 | 更新启动流程 |
| Phase 6: SQLite 本地存储 | 2 天 | storage 包实现和测试 |
| Phase 7: 测试验证 | 2 天 | 单元测试 + 集成测试 |
| Phase 8: 文档更新 | 1 天 | 更新和新增文档 |
| **总计** | **11.5 天** | 约 2 周 |

---

## 七、风险评估与缓解

| 风险 | 影响 | 概率 | 缓解措施 |
|------|------|------|---------|
| LeafNode outbound 连接监控 | 中 | 高 | 需要深入研究 NATS Server API，找到监控 outbound 连接的方法 |
| 消息延迟增加 | 低 | 高 | 这是架构权衡，可通过自建就近 Hub 缓解 |
| Hub 单点故障 | 高 | 中 | 配置多个 Hub，支持自动切换 |
| JetStream Stream 查询性能 | 中 | 中 | 合理设置 retention 策略，建立索引（如需要） |
| 迁移过程中的 bug | 高 | 中 | 充分测试，保持旧代码分支作为备份 |

---

## 八、功能特性总结

| 特性 | 说明 |
|------|------|
| 多 Hub 支持 | LeafNode 配置多个 Remote Hub，高可用 |
| Hub 自动切换 | 一个 Hub 故障自动切换到其他 Hub |
| JetStream (Hub) | 存储暂未被接收的消息（离线消息） |
| SQLite (本地) | 本地消息历史存储，支持灵活查询 |
| 历史消息查询 | 按会话、时间范围、发送者查询（SQL） |
| 全文搜索 | 支持搜索消息内容（SQL LIKE） |
| 已读状态 | 支持标记消息已读/未读 |
| 无需 NAT 穿透 | LeafNode 主动连接，无需公网 IP |
| 配置简单 | 用户只需配置 Hub 地址列表 |
| 移动端友好 | 手机可以正常使用 |

---

## 九、测试验证结果总结

根据 `.vscode/leaf-node/cmd/test-leafnode-jetstream/` 中的实际测试：

### 测试结论

| 测试 | 结果 |
|------|------|
| 纯 JetStream 单机 | ✅ 通过 |
| LeafNode 本地 JetStream | ✅ 通过（功能正常，但查询能力有限） |
| Hub 端 JetStream | ✅ 通过（适合离线消息暂存） |
| 完整架构集成 | ✅ 通过 |

### JetStream 启用关键配置

```go
opts := &server.Options{
    ServerName: "my-server",   // 必须设置！
    JetStream: true,             // 必须显式设置为 true！
    JetStreamMaxMemory: 256 * 1024 * 1024,
    JetStreamMaxStore: 1 * 1024 * 1024 * 1024,
    StoreDir: "/path/to/store",
}
```

注意：不要设置 Cluster 配置（单机模式），否则 JetStream 可能无法启动。


