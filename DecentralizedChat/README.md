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
