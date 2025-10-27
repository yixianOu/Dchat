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

## 下一步开发计划

1. **依赖完善**: 添加缺少的Go模块依赖
2. **错误修复**: 修复编译错误和类型问题
3. **功能测试**: 验证NATS Routes集成
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
- **后端**: Go + NATS
- **框架**: Wails v2
- **网络**: NATS Routes集群
- **构建**: Vite + Go build
