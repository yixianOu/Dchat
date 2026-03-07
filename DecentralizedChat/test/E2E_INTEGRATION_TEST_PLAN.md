# DecentralizedChat E2E 集成测试计划

**日期**: 2026-03-04
**更新**: 2026-03-07 完成 LeafNode 全部功能测试 + P2P 公网 Hub 通信测试，仅剩 chat 消息加密解密模块未测试

---

## 概述
本计划描述了 DecentralizedChat 项目的 e2e 集成测试策略。

根据 CLAUDE.md 的要求：
- **只写 e2e 集成测试**，不需要单元测试
- 测试使用内嵌 NATS 服务器
- 如果 e2e 测试失败说明代码逻辑有问题，需要写 bug 说明文档

---

## 测试范围（5 个核心模块）

### 1. internal/leafnode - LeafNode 管理器
### 2. internal/nats - NATS 客户端
### 3. internal/nscsetup - NSC 简化设置
### 4. internal/storage - SQLite 存储
### 5. 测试chat消息加密解密

---

## 目录结构

```
test/
├── leafnode/           # internal/leafnode 测试
│   ├── leafnode_sqlite_e2e_test.go
│   └── leafnode_p2p_hub_test.go
├── nats/               # internal/nats 测试
│   └── nats_client_e2e_test.go
├── nscsetup/           # internal/nscsetup 测试
│   └── nscsetup_e2e_test.go
├── storage/            # internal/storage 测试
│   └── storage_e2e_test.go
├── bugs/               # e2e 测试发现的 bug 文档
└── E2E_INTEGRATION_TEST_PLAN.md  # 本文档
```

---

## 测试模块详情

### 1. internal/leafnode 测试 (`test/leafnode/`)

| 测试文件 | 测试内容 | 状态 |
|-----------|---------|------|
| `leafnode_sqlite_e2e_test.go` | LeafNode + SQLite 完整架构 | ✅ 已完成 |
| `leafnode_p2p_hub_test.go` | 两个 LeafNode 通过公网 Hub 通信 | ✅ 已完成 |
| - | LeafNode Manager 启动/停止 | ✅ 已完成 |
| - | 连接 Hub | ✅ 已完成 |
| - | 获取本地连接地址 | ✅ 已完成 |
| - | 状态检查 | ✅ 已完成 |

### 2. internal/nats 测试 (`test/nats/`)

| 测试文件 | 测试内容 | 状态 |
|-----------|---------|------|
| `nats_client_e2e_test.go` | 连接本地 NATS | ✅ 已完成 |
| - | 发布/订阅消息 | ✅ 已完成 |
| - | PublishJSON/SubscribeJSON | ✅ 已完成 |
| - | 连接状态检查 | ✅ 已完成 |
| - | 统计获取 | ✅ 已完成 |

### 3. internal/nscsetup 测试 (`test/nscsetup/`)

| 测试文件 | 测试内容 | 状态 |
|-----------|---------|------|
| `nscsetup_e2e_test.go` | 简化 NSC 设置 | ✅ 已完成 |
| - | 确保密钥生成 | ✅ 已完成 |
| - | 配置文件创建 | ✅ 已完成 |
| - | JWT 生成验证 | ✅ 已完成 |

### 4. internal/storage 测试 (`test/storage/`)

| 测试文件 | 测试内容 | 状态 |
|-----------|---------|------|
| `storage_e2e_test.go` | SQLite 基本 CRUD | ✅ 已完成 |
| - | 会话保存/查询 | ✅ 已完成 |
| - | 消息保存/查询 | ✅ 已完成 |
| - | 消息顺序验证 | ✅ 已完成 |
| - | 标记已读 | ✅ 已完成 |
| - | 搜索功能 | ✅ 已完成 |
| - | 数据持久化 | ✅ 已完成 |

---

## 测试工具和约定

### 测试辅助函数

所有测试使用以下约定：

```go
// 启动内嵌 NATS Hub
func startTestHub(t *testing.T) (*server.Server, string)

// 启动内嵌 NATS Spoke
func startTestSpoke(t *testing.T, hubURL string) (*server.Server, string)

// 等待连接就绪
func waitForConnection(t *testing.T, check func() error)
```

### 测试命名约定

- 测试函数名格式：`Test<Module>_<Scenario>_E2E`
- 每个测试独立，不依赖其他测试
- 使用 `t.TempDir()` 做临时存储
- 测试结束清理所有资源

---

## 测试实施优先级

### Phase 1: Storage ✅ 已完成
- ✅ storage_e2e_test.go - 4 个测试全部通过

### Phase 2: LeafNode ✅ 已完成
- ✅ leafnode_sqlite_e2e_test.go - 3 个测试全部通过

### Phase 3: NATS Client ✅ 已完成
- ✅ nats_client_e2e_test.go - 2 个测试全部通过
- ✅ 连接本地 NATS
- ✅ 发布/订阅消息
- ✅ PublishJSON/SubscribeJSON
- ✅ 连接状态检查
- ✅ 统计获取

### Phase 4: NSC Setup ✅ 已完成
- ✅ nscsetup_e2e_test.go - 3 个测试全部通过
- ✅ 简化 NSC 设置测试
- ✅ 密钥生成测试
- ✅ JWT 生成验证

---

## 现有测试

已有测试文件：
- `test/storage/storage_e2e_test.go` - Storage 模块完整测试 (✅ 4 个测试通过)
- `test/leafnode/leafnode_sqlite_e2e_test.go` - LeafNode + SQLite 集成测试 (✅ 7 个测试通过)
- `test/leafnode/leafnode_p2p_hub_test.go` - LeafNode P2P 公网 Hub 通信测试 (✅ 1 个测试通过)
- `test/nats/nats_client_e2e_test.go` - NATS 客户端测试 (✅ 2 个测试通过)
- `test/nscsetup/nscsetup_e2e_test.go` - NSC 简化设置测试 (✅ 3 个测试通过)

### 待完成测试
- **internal/chat - Chat 消息加密解密模块**：唯一剩余未测试的核心模块

---

## Bug 报告

如果 e2e 测试发现 bug，在 `test/bugs/` 目录下写 bug 文档，格式：

```
test/bugs/BUG_<BUG_ID>.md

内容：
- 问题描述
- 复现步骤
- 测试代码
- 预期行为
- 实际行为
```

