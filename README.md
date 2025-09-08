### 跨公网/局域网混合拓扑指引

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
```

## 运行演示
```bash
cd DecentralizedChat
go run examples/cluster_demo.go
```

## 新 API 特点
- 零硬编码：所有网络配置都通过参数传入
- 自动配置：自动检测本地 IP，自动生成 NATS URL
- 强类型：配置验证确保运行时安全
- 简洁API：移除冗余的向后兼容接口
# 去中心化聊天室 - DChat

## 项目概述

基于 **NATS Routes集群 + Wails** 构建的真正去中心化聊天室应用。

### 核心特性
- ⚡ **自动发现**：节点自动形成全网状网络，无需手动配置

## 技术架构

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

### 技术栈选择

#### 1. NATS Routes集群
- **用途**：实现真正去中心化的消息路由
- **优势**：
  - ✅ 支持链式连接（A→B→C自动发现）
  - ✅ 动态网络拓扑，无单点故障
  - ✅ 配置简单，只需种子节点地址
  - ✅ 自动形成全网状网络

#### 2. Wails框架
- **用途**：构建现代化桌面应用
- **优势**：
  - ✅ Go后端 + React前端
  - ✅ 原生性能
  - ✅ 跨平台打包
  - ✅ 热重载开发
  - ✅ 系统集成能力

## 核心特性详解

### 1. 去中心化网络拓扑

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

### 3. Wails应用架构

现代化桌面应用设计：

```
┌─────────────────────────────────────┐
│             前端 (React)            │
│         React.js + JSX             │
├─────────────────────────────────────┤
│             Wails Bridge            │
├─────────────────────────────────────┤
│              后端 (Go)              │
│  ├─ NATS客户端                      │
│  ├─ 消息加密/解密                    │
│  ├─ 用户管理                        │
│  └─ 系统集成                        │
└─────────────────────────────────────┘
```

## 实现方案

### 配置示例

#### 1. NATS Routes配置

**基础节点配置：**
```conf
# nats-node.conf
# 客户端连接端口
port: 4222
server_name: "dchat-node-{user_id}"

# Routes集群配置
cluster: {
  name: "dchat_network"
  # 集群端口
  port: 6222
  # 连接到种子节点
  routes: [
    "nats://seed-node-ip:6222"  # 种子节点的IP地址
  ]
}

# 账户和权限配置
include "accounts.conf"
```

**启动脚本：**
```bash
#!/bin/bash
# start-dchat-node.sh

# 启动NATS服务器
nats-server \
  -p 4222 \
  -cluster "nats://local-ip:6222" \
  -routes "nats://seed-node-ip:6222" \
  -server_name "dchat-${USER}-$(hostname)"
```

#### 2. Wails应用结构

**项目结构：**
```
dchat/
├── app.go                 # Wails应用入口
├── build/                 # 构建输出
├── frontend/              # 前端代码
│   ├── dist/
│   ├── index.html
│   ├── src/
│   │   ├── main.jsx       # React入口文件
│   │   ├── App.jsx        # 主应用组件
│   │   ├── components/    # React组件
│   │   │   ├── ChatRoom.jsx
│   │   │   ├── Sidebar.jsx
│   │   │   └── UserList.jsx
│   │   └── styles/        # CSS样式
│   │       ├── App.css
│   │       └── components/
│   ├── package.json       # Node.js依赖
│   └── vite.config.js     # Vite配置
├── wailsjs/               # Wails生成的JS绑定
│   ├── go/
│   └── runtime/
├── internal/              # 内部包
│   ├── nats/             # NATS客户端
│   ├── crypto/           # 消息加密
│   ├── chat/             # 聊天逻辑
│   └── config/           # 配置管理
├── wails.json            # Wails配置
└── main.go               # 程序入口
```

**主应用代码：**

### 启动流程

#### 1. 应用启动序列

```mermaid
sequenceDiagram
    participant User
    participant WailsApp
    participant NATS
    participant Network

    User->>WailsApp: 启动DChat
    WailsApp->>NATS: 启动NATS节点
    NATS->>Network: 连接到种子节点
    Network-->>NATS: 建立Routes连接
    NATS-->>WailsApp: 节点就绪
    WailsApp-->>User: 应用启动完成
```

#### 2. 节点发现流程

```bash
# 第一个用户启动（种子节点）
User A: 启动DChat → 成为种子节点（local-ip:6222）

# 第二个用户加入
User B: 启动DChat → 连接到种子节点 → 形成A←→B网络

# 第三个用户加入
User C: 启动DChat → 连接到B节点 → Routes自动发现A
结果：形成A←→B←→C全连通网络

# 后续用户加入
User D: 连接到任意现有节点 → 自动加入全网状网络
```

#### 3. 消息路由示例

```go
// 用户A发送消息到聊天室"general"
UserA.SendMessage("general", "Hello everyone!")

// NATS Routes自动路由到所有节点
// 所有订阅"chat.general"主题的用户都会收到消息
```

## 高级功能

### 1. 消息加密

### 2. 用户身份管理

### 3. 聊天室管理

### 4. 前端界面设计

**React.js聊天界面：**

## 部署和使用

### 1. 环境准备
 
## 操作日志追加
- 实现 AddSubscribePermission: 支持新增订阅权限，写入 <nodeID>_node_config.json 并自动重启节点应用新权限
- 重构 config.go：新增扁平 server 配置(ServerOptionsLite)，与原 NATS/Routes 字段同步，提供 BuildServerOptions() 直接生成 server.Options，减少嵌套层级。
- 二次重构 config：移除 RoutesConfig 与大部分嵌套字段，保留 Server(服务端) + NATS(向后兼容少量字段)；后续将逐步淘汰 NATS/旧权限字段，统一使用 Server + Auth 扁平结构。
- 精简 routes.NodeConfig：删除多层 Permissions/Routes/SubjectPermission 结构，保留 ImportAllow/ExportAllow 扁平字段，AddSubscribePermission 逻辑同步简化。

**安装依赖：**
```bash
# 安装NATS Server
go install github.com/nats-io/nats-server/v2@latest

# 安装Wails
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

### 2. 构建应用

```bash
# 克隆项目
git clone https://github.com/your-org/dchat.git
cd dchat

# 安装前端依赖
cd frontend
pnpm install
cd ..

# 构建开发版本（支持热重载）
wails dev

# 构建生产版本
wails build
```

### 3. 首次使用

```bash

# 2. 启动DChat应用
./build/bin/dchat

```

### 4. 网络拓扑示例

**小型团队（3-5人）：**
```
Alice (种子) ←→ Bob ←→ Charlie
     ↑                    ↓
     └──────── Diana ←────┘
```

**大型社区（10+人）：**
```
     Alice ←→ Bob ←→ Charlie
       ↑        ↑        ↓
    Diana ←→ Eve ←→ Frank ←→ Grace
       ↑        ↑        ↓
     Henry ←→ Ivan ←→ Jack
```

**全连通网络**：每个节点都能直接通信，消息延迟最低。

## 开发路线图

### Phase 1: 核心功能 (已完成)
- ✅ NATS Routes集群研究和验证
- ✅ 链式连接原理验证
- ✅ 基础Demo实现

### Phase 3: Wails应用开发 (计划中)
- ⏳ 项目结构搭建
- ⏳ Go后端服务架构
- ⏳ React.js前端界面
- ⏳ NATS客户端集成

### Phase 4: 聊天功能 (计划中)
- ⏳ 消息加密/解密
- ⏳ 用户身份管理
- ⏳ 聊天室管理
- ⏳ 文件传输支持

### Phase 5: 高级特性 (计划中)
- ⏳ 离线消息同步
- ⏳ 消息历史搜索
- ⏳ 群组权限管理
- ⏳ 插件系统

### Phase 6: 优化和发布 (计划中)
- ⏳ 性能优化
- ⏳ 跨平台测试
- ⏳ 打包和分发
- ⏳ 文档完善

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

TODO:
1. 因为每个nats是消息队列,每个节点通过subject与集群通信,每个节点默认publish-subject: all-allow, subscribe subject: all-deny. ok
2.  客户端连接使用公私钥而不是帐号密码,使用nsc生成jwt token ok
3.  用户可以自行添加allow subscribe subject,会被写入到本地的config.json持久化, 本地配置文件存储信任的公钥路径列表 ok
4. nsc自动生成凭证用于本地连接 ok
5.  研究creds,jwt,nkey的关系和作用 ok
6.  通过nats kv(https://docs.nats.io/nats-concepts/jetstream/key-value-store/kv_walkthrough)持久化私聊好友的公钥和群聊对称密钥 ok
7.  好友公钥和群聊对称公钥需要通过nats KV存储在本地,并且每次发送信息和接受信息是都需要加密解密.
8.  cluster节点的import配置能否热重启 不能
9.  通过nsc支持配置导出和导入(等)
10. 支持ip自签名,insecure tls
11. wails集成前端,检查


### 新增操作日志（2025-09-07 18:55）
- **完成 NSC 安全配置对接**：
  - 在 app.go 中集成 nscsetup.EnsureSysAccountSetup() 函数
  - 首次运行时自动初始化 NSC 操作员和 SYS 账户
  - 生成 resolver.conf 配置文件并写入 ~/.dchat/ 目录
  - 自动生成用户凭据文件 (.creds) 用于 JWT 认证
  - NATS 客户端使用生成的凭据文件进行安全连接
  - 集成配置持久化，确保重启后配置保持一致
  - 验证编译成功，所有模块完整对接
- 修复 TypeScript 配置弃用警告：
  - 更新 frontend/tsconfig.json：`moduleResolution: "Node"` → `"Bundler"`
  - 更新 frontend/tsconfig.node.json：同样修复 moduleResolution 配置
  - 解决 VS Code 提示："选项'moduleResolution=node10'已弃用"
  - 使用现代的 "Bundler" 模块解析策略，适配 Vite 构建环境
  - 验证配置正确性：`npx tsc --noEmit` 无错误输出

### 新增操作日志（2025-09-07 18:27）
- 成功安装 Deskflow（键盘鼠标共享工具）：
  - 下载 GitHub Release: deskflow-1.23.0-ubuntu-plucky-x86_64.deb
  - 解决系统依赖冲突（libxtst6、libqt6gui6、libqt6widgets6）
  - 最终使用 flatpak 安装：`flatpak install flathub org.deskflow.deskflow`
  - 验证安装成功：`flatpak run org.deskflow.deskflow --help`
  - 启动命令：`flatpak run org.deskflow.deskflow`

### 新增操作日志（2025-09-07 18:19）
- **成功解决 Wails 桌面应用编译问题**：
  - 解决依赖冲突：降级 libpng16-16t64、libxtst6 到仓库兼容版本
  - 安装完整 GTK3 开发环境：libgtk-3-dev 及所有依赖（67个包）
  - 安装 WebKit2GTK 开发包：libwebkit2gtk-4.1-dev 及依赖
  - 修复 pkg-config 配置：添加 javascriptcoregtk-4.1 库链接
  - **桌面应用启动成功**：显示 Gtk 主题警告但功能正常
  - Web 版本同时可用：http://localhost:34115（桌面版）、http://localhost:5173（前端）

### 新增操作日志（2025-09-07 14:30）
- 完善前端与后端 Wails 绑定对接：
  - 分析 Go 后端 app.go 接口，生成对应的 TypeScript 前端代码
  - 重构前端类型定义，使用 Wails 自动生成的绑定 (wailsjs/go/main/App.d.ts)
  - 更新 frontend/src/services/dchatAPI.ts，直接使用 Wails 生成的函数而非手动包装
  - 创建完整的去中心化聊天前端界面，包含用户管理、会话列表、密钥管理等功能
  - **重要发现**：Wails CLI 会自动生成 TypeScript 绑定文件：
    - `wailsjs/go/main/App.d.ts` - TypeScript 类型定义
    - `wailsjs/go/main/App.js` - JavaScript 实现
    - `wailsjs/go/models.ts` - Go 结构体对应的 TypeScript 类型
  - **绑定生成规则**：
    - 当运行 `wails dev` 或 `wails build` 时自动生成
    - 基于 Go 结构体方法的导出函数自动创建对应的 TypeScript 接口
    - 支持参数类型推断和 Promise 返回类型
    - 使用 `--skipbindings` 可跳过绑定生成（调试用）

### 新增操作日志（2025-09-01 15:30）
- 整理 cmd/routes/routes.md 文档，消除重复内容，为代码块标注文件路径和行数：
  - 统一整合研究背景、对比分析和核心发现
  - 为所有 NATS 源码示例添加准确的文件路径注释（如 nats-server/server/route.go:50-55）
  - 重新组织文档结构，突出链式连接实现原理和配置示例
  - 完善性能优化、监控调试和架构设计章节
- 文档现已成为 NATS Routes 集群机制的完整技术手册，包含源码分析、实现原理和实践指南

### 新增操作日志（2025-08-13 10:00）
- 修改 DecentralizedChat/cmd/chatpeer/main.go：
  - 增强 --cluster-advertise 支持，自动剥离 nats://、tls:// 前缀，按 host:port 注入 server.Options.Cluster.Advertise。
  - --seed-route 支持逗号/空格/分号分隔的多路由输入，便于一次性指定多个引导节点。
  - 启动信息新增 ClusterAdvertise 打印，便于核对公告地址；示例 SeedRoute 一并输出。
- 构建与使用步骤（混合公网/局域网）：
```bash
cd DecentralizedChat && go build ./cmd/chatpeer

# 公共节点（需公网IP/端口映射，对外公告6222）：
./chatpeer --client-port 4222 \
          --cluster-port 6222 \
          --cluster-advertise "<public_ip_or_dns>:6222" \
          --identity ~/.dchat/identity_pub.txt \
          --nick Public

# 私网/其它节点（经公共节点种子加入并发送一条测试消息）：
./chatpeer --client-port 4223 \
          --cluster-port 6223 \
          --seed-route "nats://<public_ip_or_dns>:6222" \
          --identity ~/.dchat/identity_lan.txt \
          --peer-id <public_user_id> \
          --peer-pub <public_user_pubkey_b64> \
          --send "hello over hybrid"

# 多引导路由（可选，支持逗号/空格/分号分隔）：
./chatpeer --seed-route "nats://a:6222, nats://b:6222 nats://c:6222" --identity ~/.dchat/identity_x.txt
```
# 2025-08-06 重大重构
- 完善 internal/routes/routes.go，支持链式集群、动态节点加入、集群连通性检查、消息路由测试等功能，参考cmd/routes/main.go。
- 重构 internal/nats/service.go，仅保留NATS客户端功能，支持鉴权连接，去除服务端嵌入式启动。
- 新增 ClusterManager 类型，提供集群管理功能，支持节点创建、启动、停止、连通性检查。
- 完善 NATS 客户端，新增 JSON 序列化/反序列化、请求-响应模式、增强连接配置。
- 重构 config.go，分离 NATS 客户端配置和 Routes 集群配置，新增配置辅助方法。
- 创建 examples/cluster_demo.go 演示新设计的使用方法。
- **优化设计**：重命名 ClusterManager.network → clusterName，移除硬编码，新增 ClusterConfig 结构体支持可配置的主机地址和端口偏移量。
- **增强配置**：Routes 配置新增 Host 和 ClusterPortOffset 字段，支持更灵活的部署环境。
- **🔥 彻底清理硬编码**：
  - 移除所有硬编码的 IP 地址和端口
  - 移除向后兼容的旧 API，只保留最新设计
  - 新增 `GetLocalIP()` 自动检测本地 IP 地址
  - 新增 `ValidateAndSetDefaults()` 自动验证和设置配置默认值
  - 强制用户提供配置，避免隐式默认值
- **API 简化**：ClusterManager 现在要求明确的配置参数，增强了代码的可预测性和可维护性。

## 2025-08-08 记录：引入 NSC/JWT 凭据与首次初始化
- 客户端优先使用 NSC 生成的 .creds（JWT/公私钥）进行鉴权（internal/nats/service.go）。
- 配置新增字段：
  - nats.creds_file；routes.resolver_config；nsc 子配置（operator/store_dir/keys_dir/sys_jwt_path/sys_pub_path/sys_seed_path）。
- 新增 internal/nscsetup/setup.go：首次运行时通过 nsc 创建/初始化 operator(SYS)、生成 resolver.conf，写入 ~/.dchat；并把路径持久化到 ~/.dchat/config.json。
- 内置节点（internal/routes/routes.go）支持加载 resolver.conf，去除用户名/密码。
- demo/cluster 改为使用 creds 连接，并在启动前调用首启初始化。

实际执行步骤（zsh）：
```bash
# 构建（可选）
cd /home/orician/workspace/learn/nats/Dchat
go build ./...

# 运行 demo（首次会自动执行 nsc 初始化并生成 ~/.dchat/resolver.conf）
go run DecentralizedChat/demo/cluster/cluster_demo.go
```
备注：nsc 调用包含如下动作（由程序自动执行）：
- nsc add operator --generate-signing-key --sys --name local
- nsc edit operator --require-signing-keys --account-jwt-server-url nats://<host>:<port>
- nsc edit account SYS --sk generate
- nsc generate config --nats-resolver --sys-account SYS > ~/.dchat/resolver.conf

- 简化 SYS JWT 路径解析：移除多次回退 (JSON/文本) 解析逻辑，改为单次通过目录结构推导 `stores/<operator>/accounts/SYS/SYS.jwt`。
- 种子获取方式变更：不再遍历 keys 目录匹配公钥，改用 `nsc export keys --accounts --account SYS` 导出种子并写入本地配置目录。
- 清理: 移除未使用的 firstMatch 助手与 regexp 依赖（JWT 路径解析已无需正则）。
- 配置调整：NSC 配置改为存储用户级 (SYS/sys) 的 JWT/creds/seed（user_jwt_path/user_creds_path/user_seed_path, 增加 account/user 字段），不再持久化账户级 JWT。

## 2025-08-09 调整：停止记录 JWT 路径，仅保留 nkey (seed) 与 creds
- 移除 NSCConfig 中 user_jwt_path 与 account_jwt_path 字段及默认值。
- 删除 setup 初始化中对用户与账户 JWT 路径的收集与持久化逻辑，仅保留：
  - 用户级：user_creds_path, user_seed_path
  - 账户级：account_creds_path, account_seed_path
- 移除 findUserJWTPath / findAccountJWTPath 方法，避免不必要的磁盘路径依赖。
- 目的：运行期只需 creds（含 JWT + 签名身份）与必要的私钥 seed；JWT 原始文件路径不再需要持久化。

操作日志：
- 修改 internal/config/config.go 移除字段 user_jwt_path/account_jwt_path
- 修改 internal/nscsetup/setup.go 移除相关赋值与查找函数
- 更新 README 增加本节说明
 - 重构 internal/nscsetup/setup.go：引入 execCommand 统一 run 与 runOut 的公共逻辑，消除重复代码（DRY）。
 - 合并 seed 导出与 creds 查找：exportUserSeed/exportAccountSeed 合并为 exportSeed；findUserCredsFile/findAccountCredsFile 合并为 findCredsFile，减少重复。
 - 移除 run / runOut 包装函数，直接使用 execCommand，进一步简化命令执行路径。
  - 去除 setup 中硬编码的 operator/local、SYS、sys、resolver.conf：改为可配置 (operator/account/user 可由配置覆盖，resolver 文件名基于账户动态生成 <account>_resolver.conf)。
  - 精简 setup：移除 collectUserArtifacts/collectAccountArtifacts 未使用参数 (storeDir/keysDir/cfg)，消除 gopls unusedparams 警告。
  - 重构 routes：内联 ensureNotStarted，辅助函数 (loadResolverConfig/applyLocalOverrides/applyRoutePermissions/configureSeedRoutes/loadTrustedKeysIfRequested) 改为 NodeConfig 方法。
  - 配置精简：移除 account_seed_path 及账户种子导出逻辑，仅保留用户 user_seed_path 与 user_creds_path。
    - 移除 StoreDir 持久化：不再跟踪 NSC store_dir（JWT 存储目录）路径，仅保留 keys_dir + 用户 creds/seed，进一步最小化配置。
    - 修改 internal/config/config.go 删除 nsc.store_dir 字段；修改 internal/nscsetup/setup.go 去除赋值；README 追加该操作记录。
    - 二次清理：删除 residual StoresDir 相关函数与解析逻辑（readEnvPaths 去掉 storeDir 返回，移除 defaultStoresDir，实现最小依赖）。
  - 新增 nsc.user_pub_key 字段：在初始化时解析 user JWT 的 sub 保存用户公钥，避免二次调用 nsc 解析。

  ## 2025-08-10 重构：移除 NATSConfig 结构，直接使用 server.Options 扁平字段
  - 删除 internal/config/config.go 中 NATSConfig / Permissions / PermissionRules 结构体。
  - 所有客户端连接参数改由 ServerOptionsLite + 运行时拼接 URL 提供（Host/ClientPort/CredsFile）。
  - 订阅/发布权限：移除嵌套 permissions，统一使用 Server.ImportAllow / ExportAllow。
  - 删除 ensurePermissionsDefaults / syncServerFlat 中与旧结构相关逻辑。
  - 简化 CanPublish/CanSubscribe：发布默认放行；订阅基于 ImportAllow 简单匹配。
  - 更新 internal/nscsetup/setup.go 生成 NATS URL 逻辑，移除 cfg.NATS 引用。
  - 更新 demo/cluster/cluster_demo.go 适配新结构，去除已删除的 Routes/NATS 字段引用。
  - 构建验证通过。

  操作日志：
  - 修改 internal/config/config.go 移除 NATSConfig 及权限结构
  - 修改 internal/nscsetup/setup.go 替换 cfg.NATS.URL 访问
  - 修改 demo/cluster/cluster_demo.go 使用 cfg.Server.* 字段
  - 更新 README.md 追加本节说明
  - 精简订阅权限 API：删除 AddSubscribePermission / RemoveSubscribePermission 非持久化方法，只保留 AddSubscribePermissionAndSave / RemoveSubscribePermissionAndSave，确保权限修改即刻落盘。
  - 移除配置中 TrustedPubKeyPaths；新增 NATS KV (dchat_friends / dchat_groups) 存储好友公钥与群聊对称密钥。
  - KV 存储格式改为结构体：FriendPubKeyRecord{pub} / GroupSymKeyRecord{sym}，替换原 map，实现类型安全与易扩展。
  - 启用内置 JetStream：在 NodeManager.prepareServerOptions 中设置 opts.JetStream = true 以支持 KV。
    - 精简 internal/chat 结构体：User 去除 Avatar；Message 去除 Username/Type；Room 去除 Name/Description/Members，仅保留最小字段（ID/Messages/CreatedAt）。同步更新 service.go 相关引用与 SetUser 签名（改为仅接受 nickname）。
    - 统一加密消息载荷结构 encWire(ver,cid,sender,ts,nonce,cipher,alg,sig)；私聊与群聊复用，移除 mid/from/to/gid 等冗余字段。
    - 再次裁剪 encWire：去除 ver/alg/sig 字段，最终格式 {cid,sender,ts,nonce,cipher}，算法由 subject 推断；更新 service.go 与 internal/chat/README.md 示例。
    - 重写 internal/chat/service.go：移除房间/历史存储 API，仅保留私聊/群聊加密发送接收 (JoinDirect/JoinGroup/SendDirect/SendGroup)，新增解密回调；调整 app.go 删除房间相关方法并新增 Direct/Group 封装。
    - 优化 service.go 回调分发代码风格：显式局部变量 + 保护性 defer 注释，提升可读性。
    - 更新 app.go SetUserInfo 签名以适配 SetUser 仅接收 nickname。
    - 更新 internal/chat/README.md 移除 mid/from/to/gid 示例字段，采用统一 encWire(ver,cid,sender,ts,nonce,cipher,alg)。
  - 再次优化 internal/chat/service.go 代码风格：拆分长行（Subscribe 回调、结构体字面量、fmt.Sprintf、多参数函数调用），提高可读性与 diff 友好性。
  - 引入错误事件回调：新增 ErrorEvent/ErrorHandler，handleEncrypted 拆分为解析、解密、成功与错误分发，提高内聚与可观察性；避免静默失败。
  - 进一步简化错误回调：移除 ErrorEvent 结构，仅保留 func(error) 形式，减少耦合与调用复杂度；fmt.Sprintf 短行恢复单行表达。
  - 调整 service.go 代码风格：一行一逻辑（GetUser/handleEncrypted/dispatch* 等拆分），去除多语句单行，提升可读性与审查效率。
  - 更新 app.go：新增 SetKeyPair / OnDecrypted / OnError / GetUser 封装，提供与 service.go 对应外部调用入口。
  - 精简 app.go：移除房间/历史/统计/权限热重启等非最小聊天能力，仅保留 Direct/Group 相关 API 与启动初始化。
  - 新增跨节点加密往返测试：internal/chat/dual_node_encrypt_test.go，单机模拟双节点（不同端口 + Routes seed）验证私聊加密 A<->B 往返成功。
  - 新增 cmd/genkey & cmd/chatpeer：支持两台电脑快速生成密钥、启动本地嵌入式节点并进行私聊加密往返测试。
  - chatpeer 增强：
    - 支持 --identity 持久化 (ID/PRIV/PUB) 与 --id 覆盖，避免重启后身份变化导致无法预填对端参数。
    - 支持 --cluster-advertise 用于"公共节点对外暴露集群端口"的方案。

新增操作日志：
- 修改 internal/nscsetup/setup.go：移除单一 deriveAccountJWTPath 假设，新增 findAccountJWTPath 支持多种 nsc 存储结构并回退浅层遍历匹配 SYS.jwt。
- 重构 internal/nscsetup/setup.go：拆分 EnsureSysAccountSetup 为多个小函数（配置目录解析、NATS URL 生成、NSC 初始化、resolver.conf 生成、SYS 账户工件收集）。
- 简化 internal/nscsetup/setup.go 的 findAccountJWTPath，只保留单路径判断。
- 重写 internal/nscsetup/setup.go 的 findSeedByPublicKey，去除利用 errors.New("found") 作为控制流的反模式。
- 调整 internal/nscsetup/setup.go：collectSysAccountArtifacts 仅记录 findAccountCredsFile 返回的 SYS creds 路径，去除 sys.pub 写入。
1.  修正 SYS 账户 JWT 路径推断：支持当前 nsc 目录结构 (stores/<op>/accounts/SYS/SYS.jwt) 及旧布局 (account.jwt 与 hash 子目录)，新增 findAccountJWTPath 逻辑。
2.  重构 EnsureSysAccountSetup：抽取 resolveConfigDir/ensureNATSURL/initNSCOperatorAndSys/generateResolverConfig/collectSysAccountArtifacts，提升内聚与可读性。
3.  简化 findAccountJWTPath：仅保留当前实际结构 stores/<op>/accounts/SYS/SYS.jwt 解析逻辑，移除多余候选与遍历。
4.  重写 findSeedByPublicKey：移除通过返回 error 终止遍历的做法，改为正常遍历并在匹配后跳过后续处理逻辑，增强语义清晰度。
5.  SYS 公钥路径改为优先记录 creds 文件路径 (keys/creds/<operator>/SYS/*.creds)，找不到再回退写 sys.pub。
6.  移除 sys.pub 回退逻辑：仅记录已有 creds 文件路径，不再生成 sys.pub。
7.  重构 internal/chat/service.go：引入并发安全（RWMutex）、房间订阅幂等、OnMessage 回调机制、LeaveRoom、GetUser、历史快照复制、随机ID生成，新增 Close 释放订阅。
8.  精简 internal/chat/README.md 群聊部分：仅保留 dchat.grp.<gid>.msg 与可选 ctrl.rekey，删除成员/ack/typing/presence/meta/history 等扩展，定位最小去中心化实现，并在文档中解释软权限通过密钥轮换实现。
9.  精简 internal/chat/README.md 私聊设计：移除 ack/typing/presence/rekey 多余 subject，统一为 dchat.dm.{cid}.msg，说明直接使用对方公钥 + 自己私钥派生共享密钥加密消息。
10. 新增 internal/chat/crypto.go：实现 encryptDirect (NaCl box) 与 encryptGroup (AES-256-GCM)；扩展 chat.Service 提供 SetKeyPair/SendDirect/JoinDirect/SendGroup，消息发送前加密，接收后待后续解密集成。
11. 精简 chat/README.md 密钥策略：群聊去除 rekey/version，KV 仅存储 {sym}；私聊仅使用己私钥+对方公钥派生共享密钥，不做 ratchet 与轮换描述。
12. 集成 NSC 安全设置：修改 app.go 在 OnStartup 中调用 nscsetup.EnsureSysAccountSetup()，自动配置 SYS 账户、生成 JWT、建立 resolver.conf。
13. 移除 Wails build tags：删除 main.go 和 app.go 中的 "//go:build desktop" 约束，使用 wails build 官方构建命令完成桌面应用编译。
14. 修复并验证 Wails 应用：使用 wails build 成功构建 build/bin/DecentralizedChat，应用启动正常，GTK/WebKit 引擎加载完成，NSC 初始化运行（存在密钥解析问题待解决）。
15. 修复 SSH 连接问题：配置 ~/.ssh/config 使用 ssh.github.com:443 端口绕过网络限制，恢复 git pull 正常工作。