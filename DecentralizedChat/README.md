# DecentralizedChat - 基于Wails的去中心化聊天室

## 项目概述

基于 **NATS Routes集群 + Wails** 构建的真正去中心化聊天室应用。

### 核心特性
- ⚡ **自动发现**：节点自动形成全网状网络，无需手动配置
- 零硬编码：所有网络配置都通过参数传入
- 自动配置：自动检测本地 IP，自动生成 NATS URL
- 强类型：配置验证确保运行时安全
- 简洁API：移除冗余的向后兼容接口

## 项目结构

```
DecentralizedChat/
├── app.go                 # Wails应用主逻辑
├── main.go                # 程序入口
├── go.mod                 # Go模块依赖
├── wails.json             # Wails配置
├── build/                 # 构建输出
├── frontend/              # React前端代码
│   ├── dist/             # 构建输出
│   ├── src/
│   │   ├── App.jsx       # 主应用组件
│   │   ├── App.css       # 主样式文件
│   │   ├── main.jsx      # React入口
│   │   └── components/   # React组件
│   │       └── ChatRoom.jsx  # 聊天室组件
│   ├── index.html        # HTML模板
│   ├── package.json      # 前端依赖
│   ├── vite.config.js    # Vite配置
│   └── wailsjs/          # Wails生成的JS绑定
└── internal/             # Go后端代码
    ├── nats/             # NATS消息服务
    │   └── service.go
    ├── chat/             # 聊天服务
    │   └── service.go
    ├── config/           # 配置管理
    │   └── config.go
    └── routes/           # Routes集群工具
        └── routes.go
```

## 代码移动完成

### ✅ 前端代码移动
- **ChatRoom组件**: 从README示例移动到 `frontend/src/components/ChatRoom.jsx`
- **主App组件**: 更新 `frontend/src/App.jsx`，添加完整的聊天应用界面
- **样式文件**: 更新 `frontend/src/App.css`，添加现代化聊天界面样式

### ✅ 后端代码移动
- **NATS服务**: 创建 `internal/nats/service.go`，封装NATS连接和消息处理
- **聊天服务**: 创建 `internal/chat/service.go`，处理聊天室和消息逻辑
- **配置管理**: 创建 `internal/config/config.go`，管理应用配置
- **Routes工具**: 创建 `internal/routes/routes.go`，从cmd/routes移植核心功能

### ✅ 主应用集成
- **app.go**: 更新主应用逻辑，集成所有内部模块
- **main.go**: 修复Wails启动方法调用
- **go.mod**: 添加NATS依赖

## 主要功能模块

### 1. 前端 (React + Wails)
- **ChatRoom组件**: 实时聊天界面，支持消息发送/接收
- **侧边栏**: 聊天室列表，网络状态显示
- **响应式设计**: 现代化Discord风格界面

### 2. 后端服务
- **NATS服务**: 处理消息发布/订阅，Routes集群管理
- **聊天服务**: 聊天室管理，消息历史，用户管理

### 3. 配置系统
- **用户配置**: 昵称、头像等个人信息
- **网络配置**: 种子节点配置
- **NATS配置**: 端口设置，集群名称等

## 运行演示

```bash
cd DecentralizedChat
go run examples/cluster_demo.go
```

## 开发命令

```bash
# 开发模式
wails dev

# 构建生产版本
wails build

# 安装前端依赖
cd frontend && pnpm install

# 整理Go依赖
go mod tidy
```

## 技术栈

- **前端**: React.js + Vite + CSS3
- **后端**: Go + NATS
- **框架**: Wails v2
- **网络**: NATS Routes集群
- **构建**: Vite + Go build

## 技术架构深度分析

### 整体架构设计

```
用户设备A                用户设备B                用户设备C
│  (Routes)    │        │  (Routes)    │        │  (Routes)    │
│   Network    │        │   Network    │        │   Network    │
└──────────────┘        └──────────────┘        └──────────────┘
       │                        │                        │
       └────────────────────────┼────────────────────────┘
                                │
                     ┌──────────────┐
                     │   NATS Mesh   │
                     │   Network     │
                     └──────────────┘
```

### NATS Routes集群

- **用途**：实现真正去中心化的消息路由
- **优势**：
  - ✅ 支持链式连接（A→B→C自动发现）
  - ✅ 动态网络拓扑，无单点故障
  - ✅ 配置简单，只需种子节点地址
  - ✅ 自动形成全网状网络

### Wails框架

- **用途**：构建现代化桌面应用
- **优势**：
  - ✅ Go后端 + React前端
  - ✅ 原生性能
  - ✅ 跨平台打包
  - ✅ 热重载开发
  - ✅ 系统集成能力

### 去中心化网络拓扑

基于NATS Routes的去中心化设计：

```
初始状态：NodeA (种子节点)
┌─────────┐
│ Node A  │
└─────────┘

添加NodeB：A←→B
┌─────────┐    ┌─────────┐
│ Node A  │◄──►│ Node B  │
└─────────┘    └─────────┘

添加NodeC：A←→B←→C，A自动发现C
┌─────────┐    ┌─────────┐    ┌─────────┐
│ Node A  │◄──►│ Node B  │◄──►│ Node C  │
└─────────┘    └─────────┘    └─────────┘
      ▲                              │
      └──────────────────────────────┘
              自动建立连接

最终形成全网状网络：每个节点都与其他节点连接
```

**关键特性：**
- 🎯 **链式连接**：新节点只需连接任一现有节点
- 🎯 **自动发现**：Routes协议自动建立全连通网络
- 🎯 **动态自愈**：节点故障时自动从网络移除
- 🎯 **无中心节点**：所有节点地位平等

## 网络架构设计与讨论记录

### 问题背景

NATS Routes 支持链式连接：节点只需连接集群中任一节点即可加入。但实际工作机制是：
- 新节点会自动获取集群中所有节点的地址
- 然后尝试与所有节点建立直接连接
- 最终形成全连通网络拓扑

**核心问题**：这要求所有节点网络互通。但实际场景中大部分节点在 NAT 后的局域网内，无法被外部直接访问。

### NATS Routes 消息传播机制分析

**关键发现**：NATS Routes 采用**发布者直接广播**模式，而非转发扩散。

```
发布者节点 → 直接发送到所有订阅者节点
（不经过中间节点转发）
```

**影响**：
- 发布者必须能够直接访问订阅者
- 即使两个局域网节点都连接到同一个公网节点，它们之间也无法通信
- 因为发布者会尝试直接连接订阅者的局域网地址（失败）

### 方案探索

#### 方案一：引导节点 + cluster-advertise ❌

**问题**：引导节点成为单点依赖，违背去中心化原则。

#### 方案二：多公网节点对等架构 ❌

**问题**：仍然无法解决不同局域网节点之间的通信问题。

#### 方案三：Leaf Node 模式 ❌

**致命缺陷**：
- Leaf 节点可以接收主节点转发的消息
- 但 Leaf 节点发布的消息不会转发给其他 Leaf 节点
- 适合单向数据采集，不适合对等聊天场景

### 跨 NAT 通信解决方案 ✅

#### 推荐方案：公网 Routes 集群 + 局域网客户端连接

**核心思路**：区分节点角色，仅公网节点运行 NATS Server Routes 集群，局域网节点作为 NATS Client。

**架构设计**：
```
公网 NATS Routes 集群（去中心化）
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│ Public-1    │◄───►│ Public-2    │◄───►│ Public-3    │
│ NATS Server │     │ NATS Server │     │ NATS Server │
│ 1.2.3.4:4222│     │ 5.6.7.8:4222│     │ 9.10.11.12  │
└──────┬──────┘     └──────┬──────┘     └──────┬──────┘
       │                   │                    │
       │    NATS Client 连接（TCP 长连接）      │
       │                   │                    │
   ┌───┴───┐           ┌───┴───┐          ┌────┴────┐
   │ LA-1  │           │ LA-2  │          │  LB-1   │
   │Client │           │Client │          │ Client  │
   └───────┘           └───────┘          └─────────┘
  局域网 A              局域网 A            局域网 B
```

**工作机制**：
1. **公网节点**：运行 NATS Server + Routes 集群，互相全连通
2. **局域网节点**：仅运行 NATS Client，连接任意公网节点
3. **消息流转**：
   - LA-1 发布消息 → Public-1 收到 → 通过 Routes 广播到 Public-2/3
   - Public-2 将消息推送给其客户端 LA-2
   - Public-3 将消息推送给其客户端 LB-1

**关键特性**：
- ✅ 解决跨 NAT 问题（局域网主动连接公网，无需被动接受连接）
- ✅ 公网节点保持去中心化（Routes 全连通）
- ✅ 高可用（公网节点故障，客户端自动重连其他节点）

**优势对比**：

| 特性 | 同子网部署 | 应用层中转 | 公网集群+客户端 |
|------|-----------|-----------|----------------|
| 跨 NAT 通信 | ❌ | ✅ | ✅ |
| 去中心化程度 | ⚠️ 子网内 | ❌ 依赖中转 | ✅ 公网去中心化 |
| 实现复杂度 | 低 | 中 | 低 |
| 消息延迟 | 极低 | 中 | 低 |
| 部署成本 | 无 | 需运行中转 | 需 2-3 个公网节点 |

**角色定义**：
- **公网节点**：服务器角色，需要固定 IP，运行 NATS Server
- **局域网节点**：客户端角色，无需公网 IP，仅运行应用 + NATS Client
- **普通用户**：连接默认公网节点，默认权限
- **高级用户**：可自建公网节点，加入 Routes 集群后连接自己的公网节点，支持鉴权后修改公网节点的options配置

**去中心化保障**：
- ✅ 无单点故障（3+ 公网节点 Routes 全连通）
- ✅ 任一公网节点失效不影响整体
- ✅ 用户可自建公网节点参与网络
- ⚠️ 局域网节点依赖至少 1 个公网节点可达（但有多个选择）

**结论**：
- **推荐方案**：公网 Routes 集群 + 局域网客户端模式
- **实现难度**：低（无需修改现有代码，仅调整配置）
- **适用范围**：覆盖 99% 场景（公网用户 + 局域网用户混合）
- **下一步**：修改 `internal/routes` 和 `internal/nats` 支持客户端模式

### 跨公网/局域网混合拓扑配置指引

公共节点（有公网 IP，暴露 cluster 端口）示例：
```bash
./chatpeer --client-port 4222 --cluster-port 6222 \
  --cluster-advertise 1.2.3.4:6222
```

局域网节点（通过公共节点的 advertise 地址作为种子加入）：
```bash
./chatpeer --client-port 4322 --cluster-port 6322 \
  --seed-route nats://1.2.3.4:6222
```

随后按双向加密私聊流程互换 userID/PubKey，并以 --identity 保持稳定身份重复使用。

## 设计思路

1. nats routes支持一个节点链式连接到集群，就能共享到集群的消息传播，然后查询源码得知，这个功能的实现方式是：每个节点会搜集集群各个节点的地址，一旦有新节点连接了集群中的一个节点，那么新节点会拿到其他节点的位置并且自动连接这些节点。但是这带来一个问题：nats集群中的节点的网络需要互通。但是实际场景是：可能只有几个节点在公网，而大部分节点在局域网，通过nat网关,wifi等方式与公网交互。

2. 问题： 如果局域网a的nats节点用routes与公网nats集群连接了，局域网b的nats节点用routes与公网节点连接了，那么客户端a和客户端b的发送到局域网a/b的nats节点的消息会被公网nats集群转发吗？
    回答：不行

## 下一步开发计划

1. **依赖完善**: 添加缺少的Go模块依赖
2. **错误修复**: 修复编译错误和类型问题
3. **功能测试**: 验证NATS Routes集成
4. **UI完善**: 添加更多React组件和交互功能
5. **打包构建**: 配置Wails构建流程
6. 实现 AddSubscribePermission: 支持新增订阅权限，写入 <nodeID>_node_config.json 并自动重启节点应用新权限
7. 重构 config.go：新增扁平 server 配置(ServerOptionsLite)，与原 NATS/Routes 字段同步，提供 BuildServerOptions() 直接生成 server.Options，减少嵌套层级。
8. 二次重构 config：移除 RoutesConfig 与大部分嵌套字段，保留 Server(服务端) + NATS(向后兼容少量字段)；后续将逐步淘汰 NATS/旧权限字段，统一使用 Server + Auth 扁平结构。
9. 精简 routes.NodeConfig：删除多层 Permissions/Routes/SubjectPermission 结构，保留 ImportAllow/ExportAllow 扁平字段，AddSubscribePermission 逻辑同步简化。

## 技术优势总结

### 🎯 完全去中心化
- **无单点故障**：任意节点离线不影响网络
- **无固定服务器**：所有节点地位平等
- **自动网络发现**：新节点自动加入现有网络
- **动态自愈能力**：故障节点自动从网络移除

### 🔒 企业级安全
- **消息签名**：Ed25519数字签名验证身份
- **零信任架构**：不依赖中心化身份认证

### ⚡ 极简配置
- **一键启动**：Wails一键启动所有服务
- **自动发现**：NATS Routes自动建立连接
- **热插拔**：节点可随时加入/离开

### 🚀 现代化体验
- **原生性能**：Wails提供接近原生的性能
- **跨平台支持**：Windows/macOS/Linux统一体验
- **现代UI**：基于Web技术的灵活界面
- **实时通信**：NATS提供毫秒级消息延迟

## 参考资料

### 官方文档
- [NATS Routes官方文档](https://docs.nats.io/running-a-nats-service/configuration/clustering)
- [Wails框架文档](https://wails.io/docs/introduction)

### 技术研究
- [NATS Routes集群深度分析](./cmd/routes/routes.md)
- [TestChainedSolicitWorks源码分析](https://github.com/nats-io/nats-server/blob/main/test/route_discovery_test.go)

### 相关项目
- [nats-io/nats-server](https://github.com/nats-io/nats-server)
- [wailsapp/wails](https://github.com/wailsapp/wails)

---

**项目愿景**：构建一个真正去中心化、安全、易用的现代聊天平台，让每个人都能拥有自己的通信网络。

**开始时间**：2025年8月3日  
**技术栈**：NATS Routes + Wails + Go + React.js  
**核心特性**：去中心化、链式连接、零配置、企业级安全
