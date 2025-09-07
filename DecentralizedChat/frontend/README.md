# 去中心化聊天应用 - 前端代码

## 概述

这是基于 Wails 后端接口重新生成的前端代码，适配了去中心化聊天应用的核心功能。

## 主要文件结构

### 1. 类型定义 (`src/types/index.ts`)
- `User`: 用户信息
- `DecryptedMessage`: 解密后的消息结构
- `ChatSession`: 聊天会话信息
- `Friend`: 好友信息
- `Group`: 群组信息

### 2. API 服务 (`src/services/dchatAPI.ts`)
与后端 `app.go` 提供的接口对应：

#### 用户管理
- `setUserInfo(nickname: string)`: 设置用户昵称
- `getUser()`: 获取当前用户信息
- `setKeyPair(privB64: string, pubB64: string)`: 设置密钥对

#### 密钥管理
- `addFriendKey(uid: string, pubB64: string)`: 添加好友公钥
- `addGroupKey(gid: string, symB64: string)`: 添加群组对称密钥

#### 聊天功能
- `joinDirect(peerID: string)`: 加入私聊
- `joinGroup(gid: string)`: 加入群组
- `sendDirect(peerID: string, content: string)`: 发送私聊消息
- `sendGroup(gid: string, content: string)`: 发送群组消息

#### 事件监听
- `onDecrypted(callback)`: 监听解密消息
- `onError(callback)`: 监听错误

### 3. 主应用组件 (`src/App.tsx`)
主要功能：
- 用户信息管理
- 聊天会话列表
- 好友和群组管理
- 消息实时显示
- 设置界面

### 4. 聊天室组件 (`src/components/ChatRoom.tsx`)
功能：
- 显示聊天消息
- 发送消息
- 区分私聊/群聊

### 5. 密钥管理组件 (`src/components/KeyManager.tsx`)
功能：
- 生成密钥对
- 导入密钥对
- 密钥显示和复制

## 使用流程

1. **初始化**：启动应用后设置用户昵称
2. **密钥管理**：生成或导入密钥对
3. **添加好友**：输入好友ID和公钥
4. **开始私聊**：选择好友开始私聊
5. **加入群组**：输入群组ID和对称密钥
6. **发送消息**：在聊天界面发送加密消息

## 安全特性

- 端到端加密：私聊使用非对称加密，群聊使用对称加密
- 密钥本地管理：密钥仅存储在本地
- 去中心化：通过NATS网络进行消息传递
- 实时通信：自动接收和解密消息

## 开发说明

当后端接口更新时，需要：
1. 更新 `dchatAPI.ts` 中的函数签名
2. 更新 TypeScript 类型定义
3. 重新生成 Wails 绑定（如需要）

## 注意事项

- 私钥需要妥善保管，丢失无法恢复
- 好友公钥和群组密钥需要通过安全渠道分享
- 消息加密依赖于正确的密钥配置
