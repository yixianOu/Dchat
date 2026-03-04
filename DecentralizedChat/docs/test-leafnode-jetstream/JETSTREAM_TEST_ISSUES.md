# JetStream 测试问题记录

## 问题描述

在测试 Hub 层用 JetStream 暂存离线消息时，遇到以下问题：

### 1. "no responders available for request"

**错误信息**:
```
AddStream: nats: no responders available for request
```

**触发场景**:
- 配置了 `JetStreamMaxMemory` 和 `JetStreamMaxStore`
- `nc.JetStream()` 调用成功返回
- 但调用 `js.AddStream()` 时报错

**原因分析**:
- `nc.JetStream()` 只是创建了上下文对象，不验证 JetStream 是否真的启用
- JetStream 实际上还没完全启动或未正确启用

---

## JetStream 启动机制

### 需要的配置

根据 NATS Server 源码和文档分析，JetStream 启用需要：

1. **设置存储限制**:
   ```go
   opts.JetStreamMaxMemory = 256 * 1024 * 1024
   opts.JetStreamMaxStore = 1 * 1024 * 1024 * 1024
   opts.StoreDir = "/path/to/store"
   ```

2. **单机模式 vs 集群模式**:
   - 单机模式: `Cluster.Port == 0`
   - 集群模式: 需要配置 `cluster.name` 和 `routes`

3. **JetStream 实际上有单独的启用字段?**
   - 可能需要显式设置 `opts.JetStream = true` (不同版本可能不同)

---

## 测试建议

### 方案 A: 分开测试
1. 先测试纯 JetStream 功能（不涉及 LeafNode）
2. 再测试 LeafNode 消息转发
3. 最后整合两者

### 方案 B: 架构概念验证
由于测试环境配置复杂，可以先做：
- 架构设计文档
- 代码框架实现
- 实际部署时在真实环境验证 JetStream

---

## 当前项目的最佳实践

根据 refactor.md 和项目需求，最终架构建议：

| 层级 | 技术 | 说明 |
|------|------|------|
| **本地历史消息** | SQLite/bbolt | 查询灵活、简单 |
| **Hub 离线消息** | JetStream Stream | 纯消息暂存、TTL 清理 |
| **实时消息** | NATS Core (LeafNode) | 低延迟转发 |

---

## 更新记录

- 2026-03-02: 初始记录，测试遇到 "no responders available" 问题
