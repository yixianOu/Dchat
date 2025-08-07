# NATS权限控制演示

## 概述

DecentralizedChat使用服务器端权限控制来管理NATS节点的消息发布和订阅权限。

## 演示程序

### 1. 集群演示 (`examples/cluster_demo.go`)

演示基本的单节点启动和客户端连接：

```bash
cd DecentralizedChat
go build -o examples/cluster_demo examples/cluster_demo.go
./examples/cluster_demo
```

功能：
- 启动本地NATS节点
- 创建认证客户端连接
- 测试JSON消息发布
- 显示节点统计信息

### 2. 权限控制演示 (`demo/permission_demo.go`)

演示不同权限配置的效果：

```bash
cd DecentralizedChat
go build -o demo/permission_demo demo/permission_demo.go
./demo/permission_demo
```

功能：
- 测试1: 默认权限（订阅全部拒绝）
- 测试2: 允许订阅chat.*主题
- 测试3: 允许订阅特定聊天室

## 权限系统设计

### 服务器端权限控制

- **发布权限**: 默认允许发布所有主题 (`["*"]`)
- **订阅权限**: 默认拒绝订阅所有主题 (`[]`)
- **用户认证**: 使用固定凭据 (`dchat_user` / `dchat_pass`)

### 权限配置

```go
UserPermissionConfig{
    Username:       "dchat_user",
    Password:       "dchat_pass", 
    PublishAllow:   []string{"*"},
    PublishDeny:    []string{},
    SubscribeAllow: []string{}, // 用户自定义
    SubscribeDeny:  []string{},
    AllowResponses: true,
}
```

### 动态权限管理

```go
// 创建带权限的节点配置
nodeConfig := nodeManager.CreateNodeConfigWithPermissions(
    nodeID, clientPort, clusterPort, seedRoutes, 
    []string{"chat.*", "system.info"}, // 订阅权限
)
```

## 安全模型

1. **服务器端强制**: 所有权限检查在NATS服务器端执行
2. **认证连接**: 客户端必须提供正确的用户名密码
3. **主题级控制**: 支持通配符和精确主题匹配
4. **权限违规记录**: 服务器记录所有权限违规尝试

## 运行结果说明

权限演示输出示例：
- ✅ 表示操作成功
- ❌ 表示操作失败
- `permissions violation` 消息表示服务器端权限检查生效
