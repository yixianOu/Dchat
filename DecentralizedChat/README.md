# DecentralizedChat - 基于Wails的去中心化聊天室

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
    ├── network/          # 网络管理(Tailscale)
    │   └── tailscale.go
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
- **Tailscale网络**: 创建 `internal/network/tailscale.go`，管理Tailscale网络状态
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
- **Tailscale集成**: 自动网络发现，P2P连接管理
- **聊天服务**: 聊天室管理，消息历史，用户管理

### 3. 配置系统
- **用户配置**: 昵称、头像等个人信息
- **网络配置**: Tailscale设置，种子节点配置
- **NATS配置**: 端口设置，集群名称等

## 下一步开发计划

1. **依赖完善**: 添加缺少的Go模块依赖
2. **错误修复**: 修复编译错误和类型问题
3. **功能测试**: 验证NATS Routes和Tailscale集成
4. **UI完善**: 添加更多React组件和交互功能
5. **打包构建**: 配置Wails构建流程

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
- **后端**: Go + NATS + Tailscale
- **框架**: Wails v2
- **网络**: NATS Routes集群 + Tailscale VPN
- **构建**: Vite + Go build

## 开发记录

### 5. ClusterPortOffset移除重构 (2024-12-28 14:45)

#### 执行步骤：
```bash
# 1. 移除ClusterPortOffset概念
grep -r "ClusterPortOffset" DecentralizedChat/  # 检查所有引用

# 2. 更新方法签名和配置结构
go build ./...  # 验证编译无错误

# 3. 测试集群功能完整性
go run examples/cluster_demo.go  # 验证集群演示正常运行
```

#### 实现内容：
- **配置简化**：完全移除ClusterPortOffset字段和相关逻辑
- **方法重构**：CreateNode、DynamicJoin等方法改为显式传递端口参数
- **结构优化**：RouteNode增加ClusterPort字段，直接存储集群端口
- **API清理**：移除所有基于offset的端口计算逻辑

### 6. 错误处理完善重构 (2024-12-28 15:10)

#### 执行步骤：
```bash
# 1. 检查所有函数的错误处理
grep -r "err" internal/routes/routes.go  # 查找错误处理模式

# 2. 重构函数返回值以包含错误信息
go build ./...  # 验证编译无错误

# 3. 更新调用方处理新的错误返回值
go run examples/cluster_demo.go  # 测试错误处理功能
```

#### 实现内容：
- **CreateNode函数**：改为返回(*RouteNode, error)，当URL解析或服务器创建失败时返回具体错误
- **ConnectClient函数**：改为返回(*nats.Conn, error)，移除panic改为返回错误
- **TestMessageRouting函数**：改为返回error，所有内部错误都正确传播
- **DynamicJoin函数**：改为返回(*RouteNode, error)，节点创建和启动失败时返回详细错误信息

### 7. 架构重构：单节点管理设计 (2024-12-28 15:35)

#### 执行步骤：
```bash
# 1. 分析原有ClusterManager设计问题
grep -r "nodes.*map" internal/routes/  # 检查多节点管理代码

# 2. 重构为单节点管理器
go build ./...  # 验证编译无错误

# 3. 更新主应用和演示代码
go run examples/cluster_demo.go  # 测试新的单节点设计
```

#### 重构原因：
- **设计误区**：原ClusterManager假设一个应用管理多个节点，不符合去中心化场景
- **实际需求**：每个DChat应用只启动一个本地节点，通过Tailscale连接其他节点
- **概念澄清**：去中心化≠集中管理多节点，而是分布式单节点网络

#### 实现内容：
- **NodeManager替代ClusterManager**：管理单个本地NATS节点
- **移除nodes map**：不再维护多节点映射表，符合单体应用特性
- **简化API**：StartLocalNode、StopLocalNode、GetClusterInfo等单节点操作
- **集成app.go**：主应用直接使用NodeManager启动本地节点

### 8. 服务器端权限控制重构 (2024-12-28 16:10)

#### 执行步骤：
```bash
# 1. 重构权限控制架构
grep -r "PermissionChecker" internal/  # 检查客户端权限检查代码

# 2. 移除客户端权限检查，改为服务器端配置
go build ./...  # 验证编译无错误

# 3. 测试服务器端权限控制
go run examples/cluster_demo.go  # 验证权限在服务器端生效
```

#### 重构原因：
- **架构错误**：原设计在客户端检查权限，但权限应该在服务器端强制执行
- **安全漏洞**：客户端权限检查可以被绕过，不提供真正的安全保障  
- **NATS最佳实践**：权限应该在NATS服务器配置中定义，而非客户端

#### 实现内容：
- **服务器端权限**：在NATS服务器启动时配置用户账户和权限
- **UserPermissionConfig**：定义发布/订阅权限的服务器端配置结构
- **默认权限策略**：发布允许所有主题(\*)，订阅需明确授权
- **凭据认证**：客户端必须使用正确的用户名/密码连接

#### 技术特点：
- 真正安全：权限在服务器端强制执行，无法绕过
- 配置灵活：支持细粒度的主题权限控制
- 默认安全：采用白名单模式，订阅权限需明确授权
- 架构清晰：权限配置与业务逻辑分离，遵循NATS安全模型

### 9. 演示程序重构与测试 (2025-08-07 14:20)

#### 执行步骤：
```bash
# 1. 修正演示文件的包导入和main函数冲突
rm examples/permission_demo.go  # 移除冲突文件

# 2. 重新创建权限演示程序
go build -o demo/permission_demo demo/permission_demo.go

# 3. 测试权限控制效果
./demo/permission_demo

# 4. 验证集群演示程序
go build -o examples/cluster_demo examples/cluster_demo.go
./examples/cluster_demo
```

#### 问题解决：
- **main函数冲突**：将权限演示移到独立的demo/目录，避免与examples/cluster_demo.go冲突
- **包导入错误**：修正nats包导入，使用internal/nats服务而非官方nats包
- **接口匹配**：更新客户端配置调用，使用natsSvc.ClientConfig和natsSvc.NewService

#### 实现内容：
- **权限演示程序**：创建demo/permission_demo.go，展示不同权限配置的效果
- **演示文档**：创建demo/README.md，详细说明权限系统设计和使用方法
- **测试验证**：通过实际运行验证服务器端权限控制正常工作
- **代码清理**：移除重复文件，确保项目结构清晰

#### 演示效果：
- 测试1显示默认拒绝所有订阅权限
- 测试2展示chat.*通配符权限匹配
- 测试3验证精确主题权限控制
- 权限违规会在服务器端记录并拒绝操作

### 10. 去中心化架构优化：RoutePermissions重构 (2025-08-07 15:10)

#### 执行步骤：
```bash
# 1. 重构权限架构，从用户级改为节点级
# 使用RoutePermissions替代UserPermissions，更符合去中心化设计

# 2. 更新权限配置结构
go build -o demo/permission_demo demo/permission_demo.go
go build -o examples/cluster_demo examples/cluster_demo.go

# 3. 验证节点间路由权限控制
./demo/permission_demo     # 测试路由权限效果
./examples/cluster_demo    # 验证集群节点启动
```

#### 重构原因：
- **架构层次错误**：之前使用客户端级的server.Permissions，不适合去中心化节点间通信
- **权限级别混淆**：应该在节点级别控制消息路由，而非客户端级别控制订阅
- **去中心化本质**：节点间通过RoutePermissions控制Import/Export，实现真正的分布式权限管理

#### 实现内容：
- **RoutePermissions架构**：使用Import/Export权限控制节点间消息流向
- **NodePermissionConfig**：节点级权限配置，替代用户级权限
- **集群路由权限**：在server.ClusterOpts中配置RoutePermissions
- **权限语义转换**：订阅权限→导入权限，发布权限→导出权限

#### 技术特点：
- 节点自治：每个节点独立配置Import/Export权限
- 去中心化安全：权限在节点级别执行，无需中央权限服务器
- 路由控制：精确控制哪些主题可以在节点间传播
- 架构清晰：权限与网络拓扑分离，易于扩展和维护

#### 架构对比：
- **重构前**: 客户端→服务器端用户权限→单节点权限控制
- **重构后**: 节点→节点路由权限→去中心化权限网络

### 11. 身份认证升级：用户名/密码 -> JWT 公私钥机制 (2025-08-08 11:30)

#### 执行步骤：
```bash
# 1. 自动初始化(首次运行)生成 Operator + SYS 账户与 resolver.conf
go run DecentralizedChat/demo/cluster/cluster_demo.go

# 2. 查看生成的配置文件 (默认 ~/.dchat/config.json)
cat ~/.dchat/config.json | jq '.NSC'

# 3. 使用生成的 creds 连接客户端
go run DecentralizedChat/demo/cluster/cluster_demo.go
```

#### 实现内容：
- 集成 nsc 自动化: 首次启动检测无 Operator/SYS 时自动执行 nsc 命令生成相关目录与 JWT/creds
- 配置新增: NATS.CredsFile, Routes.ResolverConfigPath, NSCConfig(OperatorDir, AccountsDir, KeysDir, SysCreds, SysJwt, SysPubKey, ResolverConf)
- 服务端加载 resolver.conf 建立内嵌账户解析器
- 客户端优先级: creds > token > user/pass

#### 安全收益：
- 移除明文用户名/密码
- 采用短生命周期签权 (基于 nsc 生成的用户 JWT)
- 为后续多账户与签发策略扩展奠定基础

### 12. 动态订阅权限与可信公钥路径持久化 (2025-08-08 12:10)

#### 执行步骤：
```bash
# 添加订阅允许主题
go run cmd/tool/add_sub_allow.go chat.room.1

# 配置写入后再次运行节点演示
go run DecentralizedChat/demo/cluster/cluster_demo.go
```

#### 实现内容：
- 配置新增字段: Routes.SubscribeAllow 动态白名单 + 方法 Add/RemoveSubscribePermissionAndSave
- 可信公钥列表: NSC.TrustedPubKeyPaths + AddTrustedPubKeyPath (后续将映射到 server.Options.TrustedKeys)

### 13. 引导(bootstrap)公共节点可行性评估与初始实现 (2025-08-09 09:20)

#### 目标问题：
在完全去中心化 & NAT/Tailscale 混合网络中，首次节点如何发现其他对等节点？

#### 评估方案对比：
1. 纯 DHT: 需要额外协议与打洞，复杂度高，冷启动慢。
2. 静态手工列表: 维护成本高，变更需重新分发。
3. 公共引导节点(少量) + 运行期退场: 简单、延迟低、符合当前阶段最小可行实现。

#### 采纳策略（当前阶段）：
采取混合式：
- 启动阶段: 连接 1..N 个 BootstrapServers 获取活动路由视图/对等地址
- 达到阈值 (BootstrapMinPeers) 后: 可选主动断开公共引导 (DisconnectBootstrap=true)
- 后续发现: 依赖现有路由传播 + Tailscale/局域广播 (未来扩展空间)

#### 新增配置字段：
```jsonc
"Routes": {
    "BootstrapServers": ["nats://1.2.3.4:4222"],
    "BootstrapMinPeers": 3,
    "DisconnectBootstrap": true
}
```

#### 初始实现范围：
- 配置结构与持久化已添加
- 演示程序尝试连接首个引导节点并输出成功/失败
- 尚未实现: 自动统计当前集群路由数并在阈值后断开逻辑 (下一步)
- 尚未实现: 从引导节点拉取对等列表 (可通过系统账号订阅 _sys 命名空间或监控 API, 后续补充)

#### 后续迭代建议：
1. 实现 Route 连接建立后的对等统计与自动断连
2. 监听 $SYS.ACCOUNT.*.CONNECT 事件动态收集对等
3. 将 TrustedPubKeyPaths 解析为 TrustedKeys 注入 server.Options
4. 如引入多引导节点：并行连接 + 超时快速失败策略
5. 增加对 Tailscale IP 映射的过滤/优先级 (内网优先, 公网降级)

#### 执行步骤(当前演示)：
```bash
# 编辑配置(或首次运行自动生成基础结构后手工添加BootstrapServers)
$EDITOR ~/.dchat/config.json

# 运行演示 (会尝试连接第一个Bootstrap服务器)
go run DecentralizedChat/demo/cluster/cluster_demo.go
```

### 14. 变更记录 (本段文件追加操作日志)
- 自动化集成 nsc 首次初始化，新增JWT凭据支持
- 增加动态订阅允许主题与可信公钥路径列表持久化接口
- 新增引导节点(bootstrap)配置字段与演示初始连接逻辑
- 将 internal/ 下核心 Go 源码文件内中文注释统一翻译为英文（config.go, routes.go, nscsetup/setup.go, nats/service.go, app.go）
- 修正 nsc 环境解析：移除不存在的 -J 标志，改为解析 `nsc env` 文本输出提取 keys/store 目录
- 修正 nsc describe JSON 解析：使用 sub 字段为账户公钥并推导 account.jwt 路径，移除不存在的 jwt/public_key 字段访问

