# JetStream Raft 集群形成条件分析

## 核心结论

**在 LeafNode 情况下，启动 JetStream 不会自动形成 Raft 集群！**

JetStream 集群（Raft）的形成**仅取决于是否配置了 `cluster { port: ... }`**，与 LeafNode 无关。

---

## 源码证据

### 1. 是否启用 JetStream 集群的判断

**文件位置：** `jetstream.go:3214-3217`

```go
// If not clustered no checks needed past here.
if !o.JetStream || o.Cluster.Port == 0 {
    return nil  // 关键：如果 Cluster.Port == 0，直接返回！
}
```

**关键点：**
- 如果 `o.Cluster.Port == 0`（即没有配置 `cluster { port: ... }`）
- **不进行任何集群检查**
- **直接返回** → JetStream 以单机模式运行

---

### 2. JetStream 集群启动条件

**文件位置：** `jetstream_cluster.go:817-842`

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

---

### 3. 是否是集群模式的判断

**文件位置：** `jetstream_cluster.go:846-850`

```go
func (js *jetStream) isClustered() bool {
    // This is only ever set, no need for lock here.
    return js.cluster != nil  // 只有 js.cluster != nil 才是集群模式
}
```

---

## 配置场景分析

### 场景 1：只有 LeafNode，没有 Cluster

**配置：**
```conf
jetstream {
    max_mem_store: 256MB
    max_file_store: 2GB
    store_dir: "/tmp/js"
}

leafnodes {
    remotes = [
        { url: "nats://hub:7422" }
    ]
}

# 注意：没有 cluster { ... } 配置！
```

**结果：**
- ✅ JetStream 以**单机模式**启动
- ❌ **不形成 Raft 集群**
- ✅ LeafNode 通信正常

---

### 场景 2：既有 LeafNode，也有 Cluster（但没有 Routes）

**配置：**
```conf
jetstream {
    max_mem_store: 256MB
    max_file_store: 2GB
    store_dir: "/tmp/js"
}

cluster {
    name: "my-cluster"
    listen: "0.0.0.0:6222"
}

leafnodes {
    remotes = [
        { url: "nats://hub:7422" }
    ]
}

# 注意：有 cluster 配置，但没有 routes！
```

**结果：**
- ❌ **启动失败！**
- 错误：`JetStream cluster requires configured routes or solicited leafnode for the system account`

---

### 场景 3：Cluster + Routes（形成 Raft 集群）

**配置：**
```conf
jetstream {
    max_mem_store: 256MB
    max_file_store: 2GB
    store_dir: "/tmp/js"
}

cluster {
    name: "my-cluster"
    listen: "0.0.0.0:6222"
    routes = [
        "nats-route://node2:6222"
    ]
}
```

**结果：**
- ✅ JetStream 以**集群模式**启动
- ✅ **形成 Raft 集群**（通过 Routes）
- Raft 用于：
  - Meta group（元数据管理）
  - Stream groups（流数据复制）

---

## 决策树

```
启动 JetStream
    │
    ├─ 是否配置了 cluster { port: ... }?
    │   ├─ 否 (Cluster.Port == 0)
    │   │   └─→ JetStream 单机模式
    │   │       ├─ 不形成 Raft 集群
    │   │       └─ LeafNode 通信正常 ✅
    │   │
    │   └─ 是 (Cluster.Port > 0)
    │       │
    │       ├─ 是否配置了 routes 或系统账户的 solicited leafnode?
    │       │   ├─ 否
    │       │   │   └─→ 启动失败 ❌
    │       │   │       (错误: 需要 routes 或 solicited leafnode)
    │       │   │
    │       │   └─ 是
    │       │       └─→ JetStream 集群模式 ✅
    │       │           ├─ 形成 Raft 集群
    │       │           └─ Raft 使用 Routes 通信
    │       │
```

---

## Raft 与 LeafNode 的关系

| 功能 | 通信方式 | 形成条件 |
|------|---------|---------|
| **JetStream Raft 集群** | Routes（全网状） | `cluster { port }` + `routes` |
| **LeafNode 通信** | Hub-Spoke | `leafnodes { remotes }` |

**关键点：**
- Raft 使用 **Routes** 通信，不是 LeafNode
- LeafNode 用于**普通消息传递**，不是 Raft
- 两者**完全独立**！

---

## 总结问答

| 问题 | 答案 |
|------|------|
| LeafNode 情况下启动 JetStream 会形成 Raft 集群吗？ | ❌ **不会** |
| 什么情况下才会形成 Raft 集群？ | 配置了 `cluster { port }` + `routes` |
| LeafNode 能用于 Raft 通信吗？ | ❌ **不能**，Raft 只用 Routes |
| JetStream 单机模式和 LeafNode 冲突吗？ | ❌ **不冲突**，完全兼容 |
| 没有 cluster 配置时 JetStream 能用吗？ | ✅ **能**，单机模式 |

---

## 最终结论

**在 LeafNode 情况下，启动 JetStream 不会形成 Raft 集群！**

- JetStream 集群模式（Raft）仅在配置了 `cluster { port: ... }` 和 `routes` 时才启动
- LeafNode 和 JetStream 是完全正交的功能
- 可以同时使用 LeafNode（用于跨网络通信）和 JetStream 单机模式（用于本地流存储）
