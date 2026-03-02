# LeafNode 局限性分析

**日期**: 2026-03-02
**基于**: 源码分析 + 文档研究

---

## 概述

LeafNode 解决了 NAT 穿透问题，但付出了中心化的代价。本文档详细记录其局限性。

---

## 局限性一览

| 编号 | 局限性 | 严重程度 | 说明 |
|------|--------|----------|------|
| 1 | 中心化架构瓶颈 | 🔴 高 | 所有消息经过 Hub |
| 2 | Hub 单点故障 | 🔴 高 | Hub 故障导致全网中断 |
| 3 | Spoke 间无法直连 | 🟡 中 | 必须经过 Hub 中转 |
| 4 | 延迟增加 | 🟡 中 | 2 跳 vs 1 跳 |
| 5 | 吞吐量瓶颈 | 🟡 中 | Hub 处理能力决定上限 |
| 6 | 订阅传播限制 | 🟢 低 | 防环路的必要设计 |

---

## 详细分析

### 1. 中心化架构瓶颈

**源码位置**: `leafnode.go:127-133`

```go
func (c *client) isSpokeLeafNode() bool {
    return c.kind == LEAF && c.leaf.isSpoke
}

func (c *client) isHubLeafNode() bool {
    return c.kind == LEAF && !c.leaf.isSpoke
}
```

**问题说明**:

```
┌─────────────────────────────────────────────────────────┐
│                    公网 Hub                              │
│                                                          │
│  所有消息都经过这里！                                    │
│  ┌───────────────────────────────────────────────────┐  │
│  │  吞吐量 = min(Hub 处理能力, 总需求)              │  │
│  └───────────────────────────────────────────────────┘  │
└──────┬──────────────────┬──────────────────┬───────────┘
       │                  │                  │
   Spoke C            Spoke D            Spoke E
```

**量化分析**:

假设 100 个 Spoke，每个每秒发 1000 条消息：

| 架构 | 入站流量 | 出站流量 | 瓶颈 |
|------|---------|---------|------|
| Routes 全网状 | 100,000 msg/s | 100,000 msg/s | 无 |
| LeafNode | 100,000 msg/s | ~10,000,000 msg/s | Hub |

---

### 2. Hub 单点故障

**源码位置**: `leafnode.go:144-198` - `solicitLeafNodeRemotes()`

```go
// Spoke 主动连接 Hub，Hub 不主动连接 Spoke
func (s *Server) solicitLeafNodeRemotes(remotes []*RemoteLeafOpts) {
    for _, r := range remotes {
        if !r.Disabled {
            // 关键：Spoke 主动发起连接
            s.startGoRoutine(func() {
                s.connectToRemoteLeafNode(remote, true)
            })
        }
    }
}
```

**故障场景**:

```
正常状态：
Spoke C ──┐
           ├──> Hub A <── Spoke D
Spoke E ──┘

所有 Spoke 都能通信


           ↓ Hub A 故障


故障状态：
Spoke C    Spoke D    Spoke E
   │          │          │
   └──────────┴──────────┘  全部断开！

没有 Spoke 能通信了！
```

**缓解方案**: Hub 集群

可以用 Routes 把多个 Hub 组成集群：

```
┌─────────────────────────────────────────────────────────┐
│               Hub 集群 (Routes 全网状)                   │
│                                                          │
│  ┌──────────┐      ┌──────────┐      ┌──────────┐     │
│  │ Hub A    │◄────►│ Hub B    │◄────►│ Hub C    │     │
│  └────┬─────┘      └────┬─────┘      └────┬─────┘     │
└───────┼──────────────────┼──────────────────┼───────────┘
        │                  │                  │
   Spoke C,D,E         Spoke F,G,H         Spoke I,J,K
```

优点：
- Hub A 故障，连到 Hub A 的 Spoke 可以重连到 Hub B 或 Hub C
- 提高可用性

缺点：
- 仍然是中心化架构（Hub 集群）
- 配置复杂度增加
- Spoke 需要配置多个 Hub 地址

---

### 3. Spoke 间无法直连

**源码位置**: `client.go:5012`

```go
if c != sub.client && (c.kind != ROUTER || sub.client.isHubLeafNode() || isServiceReply(c.pa.subject))
```

**消息路径对比**:

```
Routes 全网状（理想）:
Spoke C ────────────────────> Spoke D
           (1 跳，直连)

LeafNode:
Spoke C ──> Hub A ──> Spoke D
           (2 跳，中转)
```

---

### 4. 延迟增加

| 场景 | Routes | LeafNode | 增加 |
|------|--------|----------|------|
| 同局域网内 | 1 跳 | 2 跳（可能绕公网） | ~100ms |
| 跨局域网 | 1 跳 | 2 跳 | ~2x |

---

### 5. 订阅传播限制

**源码位置**: `leafnode.go:2522-2564` - `updateSmap()`

```go
func (c *client) updateSmap(sub *subscription, delta int32, isLDS bool) {
    // ...

    // 关键判断：如果是 Spoke，只传播本地客户端的订阅
    skind := sub.client.kind
    updateClient := skind == CLIENT || skind == SYSTEM || skind == JETSTREAM || skind == ACCOUNT

    // 如果是 Spoke，且不是本地客户端订阅 → 不传播
    if !isLDS && c.isSpokeLeafNode() && !(updateClient || (skind == LEAF && !sub.client.isSpokeLeafNode())) {
        return  // 直接返回，不传播！
    }

    // ...
}
```

**订阅传播规则**:

| 订阅来源 | Spoke → Hub | Hub → Spoke | 说明 |
|---------|------------|------------|------|
| 本地客户端 | ✅ 传播 | ✅ 传播 | 正常业务订阅 |
| 其他 LeafNode | ❌ 不传播 | ✅ 传播 | 防止环路 |
| Route | ⚠️ 条件传播 | ✅ 传播 | 需判断 |

**设计原因**: 防止消息环路

```
如果 Spoke 把从 Hub 收到的订阅再发回 Hub：

Spoke C ── LS+ "chat" ──> Hub
                           │
                           ├─> 传播给 Spoke D
                           │
                           └─> 传播回 Spoke C（环路！）
```

---

### 6. JetStream 集成的约束

虽然 JetStream 和 LeafNode 可以同时使用（见 `jetstream-leafnode-analysis.md`），但有以下约束：

**JetStream 要求**（来自 `jetstream_cluster.go:835-840`）:
1. 必须配置固定的 `cluster_name`
2. 必须配置至少一个 seed route

**部署建议**:
- JetStream RAFT 集群运行在 Hub 侧
- Spoke 侧可以启用 JetStream，但元数据仍需同步到 Hub 集群

---

## 源码证据索引

| 功能 | 文件位置 |
|------|---------|
| Spoke/Hub 角色判断 | `leafnode.go:127-133` |
| Spoke 主动连接 Hub | `leafnode.go:144-198` |
| 订阅传播过滤 | `leafnode.go:2522-2564` |
| LeafNode 消息处理 | `leafnode.go:3072-3144` |
| 消息发送判断 | `client.go:5012` |

---

## 方案对比

| 特性 | Routes 全网状 | LeafNode | FRP + Routes |
|------|--------------|----------|--------------|
| NAT 穿透 | ❌ 不支持 | ✅ 支持 | ✅ 支持 |
| 去中心化 | ✅ 完全 | ❌ Hub 中心化 | ✅ 完全 |
| 单点故障 | ❌ 无 | ✅ 有 | ❌ 无 |
| Spoke 间延迟 | 1 跳 | 2 跳 | 1 跳 |
| 吞吐量 | 无瓶颈 | Hub 是瓶颈 | 无瓶颈 |
| 配置复杂度 | 简单 | 简单 | 复杂 |
| 维护成本 | 低 | 低 | 高（需维护 FRP） |

---

## 适用场景建议

### 选择 LeafNode，如果：
- ✅ 你有混合网络环境（公网 + 多局域网）
- ✅ 你接受一定程度的中心化
- ✅ 你的消息量不大，Hub 不会成为瓶颈
- ✅ 你需要简单的配置和维护

### 选择 Routes 全网状，如果：
- ✅ 所有节点都在同一网络（全公网或全内网）
- ✅ 你需要完全去中心化
- ✅ 你需要最低延迟和最高吞吐量

### 选择 FRP + Routes，如果：
- ✅ 你有混合网络环境
- ✅ 你必须完全去中心化
- ✅ 你愿意维护 FRP 基础设施
- ✅ 你有能力处理复杂配置

---

## 总结

**LeafNode 不是银弹！**

它解决了 NAT 连通性问题，但付出了以下代价：

1. ❌ 失去了完全去中心化
2. ❌ Hub 成为单点故障
3. ❌ Hub 成为吞吐量瓶颈
4. ❌ 消息延迟增加（2 跳）
5. ❌ Spoke 之间无法直接通信

**这是一个经典的权衡：**

```
连通性 ✅  ←── 权衡 ──→  ❌ 去中心化
```

**如果你必须在混合网络中实现完全去中心化，LeafNode 不能满足你的需求！**

---

**文档状态**: ✅ 完成
**最后更新**: 2026-03-02
