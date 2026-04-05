# AGENTS.md - AI Coding Agent Guide

This file provides guidance for AI coding agents working with the DecentralizedChat (DChat) codebase.

---

## 项目概述

**DecentralizedChat (DChat)** 是一个基于 **NATS LeafNode + JetStream + Wails** 构建的完全去中心化加密聊天应用。它提供安全的点对点通信，支持端到端加密、离线消息和跨平台桌面应用（Windows、macOS、Linux）。

### 核心特性

- ⚡ **LeafNode 架构** - 无需 NAT 穿透，用户只需连接公网 NATS Hub
- 🔒 **端到端加密** - 私聊使用 NaCl Box (X25519 + XSalsa20-Poly1305)，群聊使用 AES-256-GCM
- 🏗️ **混合去中心化** - Hub 形成去中心化的 Routes 集群，用户作为 LeafNode 连接
- 💬 **完整聊天功能** - 私聊、群聊、消息历史、搜索、已读标记
- 📱 **跨平台** - 通过 Wails 构建单二进制桌面应用 (Go + React)
- 📡 **离线消息支持** - Hub 上的 JetStream 在接收方离线时存储消息
- 💾 **本地历史** - SQLite 存储本地消息历史，支持完整查询能力

---

## 技术栈

| 层级 | 技术 | 版本 |
|------|------|------|
| 前端 | React + TypeScript + Vite | React 19, Vite 7 |
| 后端 | Go | 1.24.4 |
| 框架 | Wails | v2.10.2 |
| 网络 | NATS Server + NATS.go | v2.11.7, v1.44.0 |
| 本地存储 | SQLite (modernc.org/sqlite) | v1.46.1 |
| 安全 | NATS JWT, NKeys, Ed25519, NaCl Box, AES-256-GCM | - |

---

## 项目结构

```
DecentralizedChat/
├── main.go                      # Wails 应用入口
├── app.go                       # 主应用编排器
├── wails.json                   # Wails 配置
├── go.mod / go.sum              # Go 模块依赖
├── frontend/                    # React + TypeScript 前端
│   ├── src/
│   │   ├── App.tsx              # 主应用组件
│   │   ├── components/          # ChatRoom, KeyManager 等
│   │   ├── services/            # Wails API 绑定
│   │   └── types/               # TypeScript 类型定义
│   └── package.json
├── internal/                    # Go 后端包
│   ├── chat/                    # 聊天服务（加密、消息、密钥管理）
│   │   ├── service.go           # 核心聊天服务
│   │   ├── crypto.go            # 加密/解密实现
│   │   └── nsc_crypto.go        # NSC 密钥派生
│   ├── config/                  # 配置管理
│   │   └── config.go            # 配置结构和加载/保存
│   ├── nats/                    # NATS 客户端服务
│   │   ├── client.go            # NATS 连接管理
│   │   └── offline_sync.go      # 离线消息同步
│   ├── leafnode/                # 本地 LeafNode 服务器管理
│   │   └── manager.go           # LeafNode 管理器
│   ├── nscsetup/                # NSC (NATS Security Center) 自动设置
│   │   └── simple_setup.go      # 简化版 NSC 设置
│   └── storage/                 # SQLite 本地消息历史存储
│       ├── sqlite.go            # SQLite 实现
│       ├── schema.go            # 数据库 schema
│       └── types.go             # 存储类型定义
├── test/                        # E2E 集成测试
│   ├── chat/                    # 聊天加密 E2E 测试
│   ├── leafnode/                # LeafNode E2E 测试
│   ├── jetstream/               # JetStream 离线消息测试
│   ├── nats/                    # NATS 客户端测试
│   ├── storage/                 # 存储测试
│   └── nscsetup/                # NSC 设置测试
├── docker/                      # NATS Hub 集群部署配置
├── docs/                        # 架构和设计文档
│   ├── refactor.md              # LeafNode 架构重构计划
│   ├── architecture-design.md   # 详细架构设计
│   ├── offline.md               # 离线消息设计
│   └── leaf-node/               # LeafNode 技术分析
└── INTERFACE_DOCS.md            # 完整的前后端 API 文档
```

---

## 构建和开发命令

### 开发模式

```bash
# 启动 Wails 开发服务器（热重载）
wails dev

# 安装前端依赖
cd frontend && pnpm install
```

### 构建

```bash
# 为当前平台构建
wails build

# 为特定平台构建
wails build -platform windows/amd64
wails build -platform darwin/universal
wails build -platform linux/amd64
```

### Go 依赖

```bash
# 整理 Go 模块
go mod tidy
```

### 测试

```bash
# 运行 E2E 集成测试（从 test 目录）
cd test && go test -v -timeout 60s ./...

# 运行特定测试
cd test/chat && go test -v -run TestChat_DirectMessage_Encryption_E2E
```

### 部署 NATS Hub 集群 (Docker)

```bash
# 启动本地 Hub 集群用于测试
cd docker && docker-compose up -d
```

---

## 架构概述

### 高层架构

```
┌─────────────┐      ┌─────────────┐
│  User Device │      │  User Device │
│  (LeafNode)  │      │  (LeafNode)  │
└──────┬──────┘      └──────┬──────┘
       │                    │
       ├────────────────────┘
       ↓
┌───────────────────────────────────┐
│       Public Hub Cluster          │
│  (NATS Routes + JetStream)        │
│  Hubs are fully meshed with Routes│
└───────────────────────────────────┘
```

**为什么选择 LeafNode 而不是全网格 Routes？**
- 无需 NAT 穿透 - LeafNode 只发起出站连接
- 无需端口转发 - 可在任何 NAT/防火墙后工作
- 配置更简单 - 用户只需 Hub URL
- 更好的资源利用 - Hub 处理集群，用户只需连接

### 消息加密

| 消息类型 | 加密算法 | 密钥交换 |
|----------|----------|----------|
| 私聊 | NaCl Box (X25519 + XSalsa20-Poly1305) | ECDH with Ed25519 密钥 |
| 群聊 | AES-256-GCM | 共享对称密钥分发给所有成员 |

### NATS 主题结构

- 私聊消息: `dchat.dm.<conversation-id>.msg`
- 群聊消息: `dchat.grp.<group-id>.msg`

### 配置

配置文件位置: `~/.dchat/config.json`

```json
{
  "user": {
    "nickname": "User Name"
  },
  "leafnode": {
    "local_host": "127.0.0.1",
    "local_port": 4222,
    "hub_urls": ["nats://hub1.example.com:7422"],
    "enable_tls": false,
    "enable_jetstream": true
  },
  "keys": {
    "user_creds_path": "~/.dchat/nsc/.../user.creds",
    "user_seed_path": "~/.dchat/nsc/.../user.seed",
    "user_pub_key": "U..."
  },
  "sqlite_path": "~/.dchat/chat.db",
  "log_level": "info"
}
```

---

## 代码风格指南

### Go 代码风格

1. **包注释**: 每个包文件开头应有中文注释说明包的功能
2. **导出函数**: 使用大写字母开头，添加中文注释说明
3. **错误处理**: 使用 `fmt.Errorf("...: %w", err)` 包装错误
4. **日志**: 使用 `log/slog` 包，支持结构化日志
5. **并发安全**: 共享状态使用 `sync.RWMutex` 保护

### 命名约定

- **Go**: 使用驼峰命名法 (CamelCase)
- **文件**: 使用小写下划线命名 (snake_case)
- **包名**: 使用小写单数名词
- **常量**: 使用驼峰命名法

### 前端代码风格

1. **TypeScript**: 使用严格类型检查
2. **组件**: 使用函数组件和 React Hooks
3. **文件命名**: 使用 PascalCase (组件), camelCase (工具函数)

---

## 测试策略

### E2E 集成测试要求

**重要**: 每次修改代码都要写 E2E 集成测试（不需要单元测试）。

1. **使用嵌入式 NATS 服务器**: 测试应直接启动 Go 内嵌 NATS 服务器
2. **测试驱动**: 不要照着代码写测试，要按照需求写测试
3. **失败处理**: 如果 E2E 集成测试失败，说明代码逻辑有问题，要写 bug 说明文档
4. **需求检查**: 检查测试是否满足 `docs/refactor.md` 的要求

### 测试文件组织

```
test/
├── chat/              # 聊天服务 E2E 测试
├── leafnode/          # LeafNode E2E 测试
├── jetstream/         # JetStream 离线消息测试
├── nats/              # NATS 客户端测试
├── storage/           # 存储测试
└── nscsetup/          # NSC 设置测试
```

### 测试示例模式

```go
// E2E 集成测试：XXX 功能
package e2e_test

import (
    "testing"
    "time"
    
    "DecentralizedChat/internal/xxx"
    "github.com/nats-io/nats-server/v2/server"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

// 启动测试 NATS 服务器
func startTestNATSServer(t *testing.T) (*server.Server, string) {
    t.Helper()
    opts := &server.Options{
        Host:      "127.0.0.1",
        Port:      -1,  // 随机端口
        JetStream: true,
        StoreDir:  t.TempDir(),
        NoLog:     true,
        NoSigs:    true,
    }
    s, err := server.NewServer(opts)
    require.NoError(t, err)
    go s.Start()
    require.True(t, s.ReadyForConnections(10*time.Second))
    return s, fmt.Sprintf("nats://%s:%d", opts.Host, opts.Port)
}

func TestFeature_E2E(t *testing.T) {
    // 1. 启动测试服务器
    // 2. 创建客户端/服务
    // 3. 执行操作
    // 4. 验证结果
}
```

---

## 安全考虑

### 密钥管理

1. **NSC 密钥**: 使用 NATS NSC 生成 Ed25519 密钥对
2. **聊天密钥派生**: 从 NSC 密钥派生 X25519 密钥用于加密
3. **本地存储**: 密钥存储在 `~/.dchat/nsc/` 目录
4. **SQLite 加密**: 好友公钥和群组密钥存储在本地 SQLite

### 消息加密

1. **私聊**: 使用 NaCl Box 端到端加密
2. **群聊**: 使用 AES-256-GCM 对称加密
3. **密钥交换**: 通过带外方式安全交换公钥

### 网络安全

1. **认证**: 使用 NATS JWT + NKeys 认证
2. **连接**: LeafNode 只发起出站连接，不接收入站连接
3. **TLS**: 支持 TLS 加密连接（可选）

---

## 开发工作流程

### 添加新功能

1. **设计**: 先阅读相关文档 (`docs/`, `INTERFACE_DOCS.md`)
2. **实现**: 在相应包中实现功能
3. **测试**: 编写 E2E 集成测试
4. **验证**: 确保测试通过
5. **文档**: 更新相关文档

### 调试技巧

1. **日志**: 查看 `log.txt` 文件获取详细日志
2. **日志级别**: 在配置中设置 `"log_level": "debug"`
3. **NATS 监控**: 使用 NATS 内置监控端点
4. **SQLite**: 使用 SQLite 工具直接查看数据库

### 常见问题

1. **消息无法发送**: 检查密钥配置和网络连接
2. **解密失败**: 验证密钥匹配和格式
3. **连接问题**: 检查本地 LeafNode 状态和 Hub 可达性
4. **数据库锁定**: SQLite 使用 WAL 模式，通常自动解决

---

## 重要文档索引

| 文档 | 内容 |
|------|------|
| `README.md` | 项目概述、架构、快速开始 |
| `CLAUDE.md` | Claude Code 特定指南 |
| `INTERFACE_DOCS.md` | 完整的前后端 API 文档 |
| `docs/refactor.md` | LeafNode 架构重构计划 |
| `docs/architecture-design.md` | 详细架构设计 |
| `docs/offline.md` | 离线消息设计 |
| `docs/HUB_DEPLOY.md` | Hub 部署指南 |

---

## 端口要求

- **LeafNode 客户端端口**: 4222（仅本地，127.0.0.1）
- **Hub 客户端端口**: 7422（公网 NATS Hub 端口）

用户设备**不需要任何开放的入站端口** - 只需出站连接到 Hub。

---

## 贡献指南

1. 遵循现有代码风格和模式
2. 所有更改都需要 E2E 测试
3. 保持配置向后兼容
4. 更新相关文档
5. 使用中文注释（与现有代码保持一致）
