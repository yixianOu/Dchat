# JetStream 在 LeafNode 模式下的限制

**日期**: 2026-03-02
**基于**: 源码分析 + 文档研究

---

## 两个关键问题的回答

### Q1: Spoke 侧可以启用 JetStream，但元数据仍需同步到 Hub 集群？
**A: 不需要！** Spoke 侧启用 JetStream 时，默认是**单机模式**，元数据**不会**自动同步到 Hub。

### Q2: JetStream 在 LeafNode 到底有什么限制？
**A: 请看下面的详细分析。**

---

## 核心结论

**JetStream 和 LeafNode 是完全正交的功能！**

| 功能 | 用途 | 通信方式 | 配置依据 |
|------|------|---------|---------|
| **JetStream** | 流存储、持久化 | 单机 / Routes (Raft) | `cluster { port }` |
| **LeafNode** | 跨网络消息传递 | Hub-Spoke | `leafnodes { remotes }` |

---

## JetStream 启动模式判断

### 源码证据 1: 是否单机模式

**文件位置**: `server.go:1567-1570`

```go
// Determines if this server is in standalone mode, meaning no routes or gateways.
func (s *Server) standAloneMode() bool {
    opts := s.getOpts()
    return opts.Cluster.Port == 0 && opts.Gateway.Port == 0
}
```

**关键点**:
- `Cluster.Port == 0` → 没有配置 `cluster { listen: ... }`
- `standAloneMode() == true` → **JetStream 单机模式**

---

### 源码证据 2: 是否启用集群模式

**文件位置**: `jetstream_cluster.go:817-861`

```go
func (s *Server) enableJetStreamClustering() error {
    if !s.isRunning() {
        return nil
    }
    js := s.getJetStream()
    if js == nil {
        return NewJSNotEnabledForAccountError()
    }
    // Already set.
    if js.cluster != nil {
        return nil
    }

    s.Noticef("Starting JetStream cluster")  // 关键：只有这里才会启动集群！

    // 检查条件
    hasLeafNodeSystemShare := s.canExtendOtherDomain()
    if s.isClusterNameDynamic() && !hasLeafNodeSystemShare {
        return errors.New("JetStream cluster requires cluster name")
    }
    if s.configuredRoutes() == 0 && !hasLeafNodeSystemShare {
        return errors.New("JetStream cluster requires configured routes or solicited leafnode for the system account")
    }

    return js.setupMetaGroup()  // 这里才设置 Raft meta group
}
```

**关键点**:
- 只有调用 `enableJetStreamClustering()` 才会形成 Raft 集群
- 需要满足两个条件之一：
  1. 配置了 `routes`
  2. **或者** 配置了共享系统账户的 solicited leafnode（见下文）

---

### 源码证据 3: 启动决策树

**文件位置**: `jetstream.go:501-523`

```go
standAlone, canExtend := s.standAloneMode(), s.canExtendOtherDomain()
if standAlone && canExtend && s.getOpts().JetStreamExtHint != jsWillExtend {
    canExtend = false
    s.Noticef("Standalone server started in clustered mode do not support extending domains")
    s.Noticef(`Manually disable standalone mode by setting the JetStream Option "extension_hint: %s"`, jsWillExtend)
}

// Indicate if we will be standalone for checking resource reservations, etc.
js.setJetStreamStandAlone(standAlone && !canExtend)

// Enable accounts and restore state before starting clustering.
if err := s.enableJetStreamAccounts(); err != nil {
    return err
}

// If we are in clustered mode go ahead and start the meta controller.
if !standAlone || canExtend {
    if err := s.enableJetStreamClustering(); err != nil {
        return err
    }
    // Set our atomic bool to clustered.
    s.jsClustered.Store(true)
}
```

---

## 配置场景详解

### 场景 1: Spoke 只配置 LeafNode，不配置 Cluster

**配置**:
```conf
# Spoke 配置
jetstream {
    max_mem_store: 256MB
    max_file_store: 2GB
    store_dir: "/tmp/js-spoke"
}

leafnodes {
    remotes = [
        { url: "nats://hub:7422", local_account: "$G" }
    ]
}

# 注意：没有 cluster { ... } 配置！
```

**结果**:

| 项目 | 结果 |
|------|------|
| `Cluster.Port` | 0 |
| `standAloneMode()` | `true` |
| `canExtendOtherDomain()` | 可能 `true` (如果 local_account 是 $SYS) |
| JetStream 模式 | **单机模式** |
| Raft 集群 | ❌ 不形成 |
| 元数据同步 | ❌ 不同步到 Hub |
| 本地流存储 | ✅ 正常工作 |
| LeafNode 通信 | ✅ 正常工作 |

**数据隔离**:
- Spoke 的 JetStream 数据只存在于 Spoke 本地
- Hub 看不到 Spoke 的流
- Spoke 也看不到 Hub 的流

---

### 场景 2: Spoke 配置 Cluster + Routes（在 Hub 侧）

**配置** (Hub 侧):
```conf
# Hub 配置
jetstream {
    max_mem_store: 1GB
    max_file_store: 10GB
    store_dir: "/tmp/js-hub"
}

cluster {
    name: "hub-cluster"
    listen: "0.0.0.0:6222"
    routes = [
        "nats-route://hub2:6222"
    ]
}

leafnodes {
    listen: "0.0.0.0:7422"
}
```

**结果** (Hub 侧):

| 项目 | 结果 |
|------|------|
| `Cluster.Port` | 6222 |
| `standAloneMode()` | `false` |
| JetStream 模式 | **集群模式** |
| Raft 集群 | ✅ 形成（通过 Routes） |
| 元数据同步 | ✅ 在 Hub 节点间同步 |

**结果** (Spoke 侧):
- 如果 Spoke 不配置 `cluster` → 仍是单机模式
- 如果 Spoke 也配置 `cluster` + `routes` → Spoke 自己也形成 Raft 集群（与 Hub 无关）

---

### 场景 3: 特殊模式 - 共享系统账户的 LeafNode

**源码证据**: `jetstream.go:536-550`

```go
// This will check if we have a solicited leafnode that shares the system account
// and extension is not manually disabled
func (s *Server) canExtendOtherDomain() bool {
    opts := s.getOpts()
    sysAcc := s.SystemAccount().GetName()
    for _, r := range opts.LeafNode.Remotes {
        if r.LocalAccount == sysAcc {  // 关键：local_account 是系统账户
            for _, denySub := range r.DenyImports {
                if subjectIsSubsetMatch(denySub, raftAllSubj) {
                    return false
                }
            }
            return true
        }
    }
    return false
}
```

**配置示例**:
```conf
# Spoke 配置
jetstream {
    store_dir: "/tmp/js-spoke"
    extension_hint: "will_extend"  # 可选
}

leafnodes {
    remotes = [
        {
            url: "nats://hub:7422",
            local_account: "$SYS"  # 关键：共享系统账户
        }
    ]
}
```

**结果**:
- `canExtendOtherDomain()` 返回 `true`
- 即使 `standAloneMode() == true`，也会调用 `enableJetStreamClustering()`
- **这是 JetStream "扩展" 模式**，用于多域部署
- **这不是 Spoke 同步元数据到 Hub 的模式！**

---

## JetStream 与 LeafNode 的关系总结

| 特性 | 说明 |
|------|------|
| **正交性** | JetStream 和 LeafNode 完全独立 |
| **通信方式** | JetStream Raft 使用 Routes，不使用 LeafNode |
| **数据隔离** | Spoke 的 JetStream 数据默认与 Hub 隔离 |
| **元数据同步** | 不会自动跨 LeafNode 同步 |
| **统一视图** | 需要特殊配置（共享系统账户）才能实现多域 |

---

## 常见误解澄清

### 误解 1: "Spoke 启用 JetStream 后，数据会自动同步到 Hub"
❌ **错误**

- Spoke 的 JetStream 默认是单机模式
- 数据只存在于 Spoke 本地
- 不会自动同步到 Hub

---

### 误解 2: "LeafNode 可以用于 JetStream Raft 通信"
❌ **错误**

- Raft 只使用 **Routes** 通信
- LeafNode 用于普通消息传递
- 两者是独立的通信通道

---

### 误解 3: "如果 Spoke 和 Hub 都启用 JetStream，它们会自动组成集群"
❌ **错误**

- Spoke 和 Hub 是独立的 JetStream 实例
- 需要分别配置各自的 `cluster { ... }` 和 `routes`
- 它们不会自动组成一个 Raft 集群

---

## 实际部署建议

### 方案 A: 分布式 JetStream（每个 Spoke 独立）

```
每个 Spoke 自己管理 JetStream：

Spoke C (局域网)
├─ 本地 JetStream (单机模式)
└─ LeafNode 连 Hub (只传普通消息)

Spoke D (局域网)
├─ 本地 JetStream (单机模式)
└─ LeafNode 连 Hub (只传普通消息)

Hub (公网)
└─ 可选：自己的 JetStream (集群模式)
```

**适用场景**:
- 各局域网数据独立
- 不需要跨局域网的流复制
- 简单、隔离性好

---

### 方案 B: 集中式 JetStream（只在 Hub 侧）

```
只有 Hub 有 JetStream：

Spoke C (局域网)
├─ 不启用 JetStream
└─ LeafNode 连 Hub (消息存到 Hub 的 JetStream)

Spoke D (局域网)
├─ 不启用 JetStream
└─ LeafNode 连 Hub (消息存到 Hub 的 JetStream)

Hub (公网)
└─ JetStream (集群模式)
```

**适用场景**:
- 数据需要集中存储
- 需要跨局域网的流消费
- 但 Spoke 依赖 Hub 的可用性

---

### 方案 C: 混合模式（Hub + Spoke 都有 JetStream）

```
Hub 和 Spoke 都有 JetStream，但独立运行：

Spoke C (局域网)
├─ JetStream (存本地重要数据)
└─ LeafNode 连 Hub (部分消息同步)

Spoke D (局域网)
├─ JetStream (存本地重要数据)
└─ LeafNode 连 Hub (部分消息同步)

Hub (公网)
└─ JetStream (存全局数据)
```

**适用场景**:
- 分层数据存储
- 本地数据本地处理
- 全局数据汇聚到 Hub

---

## 源码索引

| 功能 | 文件位置 |
|------|---------|
| 单机模式判断 | `server.go:1567-1570` |
| JetStream 启动决策 | `jetstream.go:501-523` |
| 集群模式启动 | `jetstream_cluster.go:817-861` |
| 可扩展域检查 | `jetstream.go:536-550` |

---

## 总结问答

| 问题 | 答案 |
|------|------|
| Spoke 可以启用 JetStream 吗？ | ✅ **可以**，单机模式 |
| 元数据会自动同步到 Hub 吗？ | ❌ **不会**，默认隔离 |
| LeafNode 用于 Raft 通信吗？ | ❌ **不**，Raft 只用 Routes |
| Spoke 和 Hub 能组成一个 Raft 集群吗？ | ⚠️ 可以，但需要 Routes，不是 LeafNode |
| 能跨 LeafNode 共享 JetStream 吗？ | ⚠️ 可以，但需要特殊配置（共享系统账户） |

---

## 最终结论

**JetStream 在 LeafNode 模式下的核心限制：**

1. **数据隔离**: Spoke 的 JetStream 默认是单机模式，数据不与 Hub 共享
2. **Raft 不使用 LeafNode**: JetStream 集群（Raft）只使用 Routes 通信
3. **无自动同步**: 不会自动跨 LeafNode 同步元数据或流数据
4. **需要特殊配置**: 如需跨域 JetStream，需要配置共享系统账户的 LeafNode

**JetStream 和 LeafNode 是完全正交的功能，可以组合使用，但各自独立运行！**

---

**文档状态**: ✅ 完成
**最后更新**: 2026-03-02
