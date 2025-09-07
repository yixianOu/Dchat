# 去中心化聊天应用 - 前后端接口对接文档

## 项目概述

这个项目是一个基于 NATS 的去中心化聊天应用，使用 Wails 框架连接 Go 后端和 React 前端。

## 后端接口（Go）

### 核心服务类 `App`

位置：`DecentralizedChat/app.go`

#### 用户管理接口
```go
func (a *App) SetUserInfo(nickname string) error
func (a *App) GetUser() (chat.User, error)
func (a *App) SetKeyPair(privB64, pubB64 string) error
```

#### 密钥管理接口
```go
func (a *App) AddFriendKey(uid, pubB64 string) error
func (a *App) AddGroupKey(gid, symB64 string) error
```

#### 聊天功能接口
```go
func (a *App) JoinDirect(peerID string) error
func (a *App) JoinGroup(gid string) error
func (a *App) SendDirect(peerID, content string) error
func (a *App) SendGroup(gid, content string) error
```

#### 事件回调接口
```go
func (a *App) OnDecrypted(h func(*chat.DecryptedMessage)) error
func (a *App) OnError(h func(error)) error
```

## 前端实现（TypeScript/React）

### API 服务层
位置：`frontend/src/services/dchatAPI.ts`

对应后端接口的 TypeScript 包装函数，通过 Wails 运行时调用 Go 方法。

### 核心组件

#### 1. 主应用 (`App.tsx`)
- 管理用户状态和聊天会话
- 处理消息接收和显示
- 提供好友和群组管理界面

#### 2. 聊天室 (`ChatRoom.tsx`)
- 显示聊天消息
- 处理消息发送
- 支持私聊和群聊

#### 3. 密钥管理器 (`KeyManager.tsx`)
- 生成和导入密钥对
- 安全显示和复制密钥

### 类型定义
位置：`frontend/src/types/index.ts`

定义了与后端数据结构对应的 TypeScript 接口。

## 数据流

### 1. 消息发送流程
```
前端输入 → dchatAPI → Wails → Go后端 → NATS网络
```

### 2. 消息接收流程
```
NATS网络 → Go后端解密 → 事件回调 → 前端显示
```

### 3. 密钥管理流程
```
前端生成/导入 → 存储到Go后端 → 用于加密/解密
```

## 加密机制

### 私聊加密
- 使用非对称加密（基于密钥对）
- 每个用户有一对公私钥
- 消息用对方公钥加密，用自己私钥解密

### 群聊加密
- 使用对称加密
- 群组共享一个对称密钥
- 所有成员用相同密钥加密/解密

## 网络架构

### NATS 主题结构
- 私聊：`dchat.dm.{cid}.msg`
- 群聊：`dchat.grp.{gid}.msg`

### 消息格式
```json
{
  "cid": "会话ID",
  "sender": "发送者ID", 
  "ts": 1234567890,
  "nonce": "base64编码的随机数",
  "cipher": "base64编码的密文"
}
```

## 部署和构建

### 开发环境
1. 安装 Go 1.21+
2. 安装 Node.js 18+
3. 安装 Wails CLI
4. 运行 `wails dev`

### 生产构建
```bash
wails build
```

## 安全考虑

1. **密钥安全**：私钥仅存储在本地，不传输
2. **端到端加密**：所有消息在传输过程中都是加密的
3. **去中心化**：无中央服务器，减少单点故障风险
4. **身份验证**：通过密钥对验证消息来源

## 扩展性

### 新增功能建议
1. 文件传输支持
2. 消息历史持久化
3. 用户在线状态
4. 群组管理功能
5. 消息撤回功能

### 性能优化
1. 消息分页加载
2. 连接池管理
3. 内存使用优化
4. 网络重连机制

## 故障排除

### 常见问题
1. **消息无法发送**：检查密钥配置和网络连接
2. **解密失败**：验证密钥匹配性
3. **连接问题**：检查 NATS 服务器状态
4. **界面无响应**：检查事件监听器配置

### 调试方法
1. 查看浏览器控制台日志
2. 检查 Go 后端日志输出
3. 验证 NATS 消息流
4. 测试密钥生成和验证
