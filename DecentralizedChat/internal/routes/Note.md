# Nats
## AcceptLoop vs startRouteAcceptLoop 的功能对比

### 1. `AcceptLoop(clr chan struct{})` - 客户端连接接受循环

**主要功能：**
- **接受客户端连接**：为普通的 NATS 客户端（应用程序）提供连接服务
- **监听客户端端口**：默认是 4222 端口
- **处理客户端协议**：处理 NATS 客户端协议的连接

**具体职责：**
```go
// 创建客户端监听器
hp := net.JoinHostPort(opts.Host, strconv.Itoa(opts.Port))  // 默认 4222
l, e := s.getServerListener(hp)

// 处理客户端连接
go s.acceptConnections(l, "Client", func(conn net.Conn) { 
    s.createClient(conn)  // 创建客户端连接
}, ...)
```

**服务对象：**
- 应用程序客户端
- 发布/订阅消息的客户端
- 使用 NATS 协议的普通连接

### 2. `startRouteAcceptLoop()` - 路由连接接受循环

**主要功能：**
- **接受服务器间连接**：为其他 NATS 服务器提供集群路由连接
- **监听集群端口**：通常是 6222 端口（`opts.Cluster.Port`）
- **处理集群协议**：处理服务器间的路由协议

**具体职责：**
```go
// 创建集群监听器
hp := net.JoinHostPort(opts.Cluster.Host, strconv.Itoa(port))  // 默认 6222
l, e := natsListen("tcp", hp)

// 处理路由连接
go s.acceptConnections(l, "Route", func(conn net.Conn) { 
    s.createRoute(conn, nil, Implicit, gossipDefault, _EMPTY_)  // 创建路由连接
}, nil)
```

**服务对象：**
- 其他 NATS 服务器节点
- 集群中的路由连接
- 服务器间消息转发

## 关键区别总结

| 方面 | AcceptLoop | startRouteAcceptLoop |
|------|------------|---------------------|
| **端口** | `opts.Port` (4222) | `opts.Cluster.Port` (6222) |
| **连接类型** | 客户端连接 | 服务器间路由连接 |
| **协议** | NATS 客户端协议 | NATS 集群路由协议 |
| **创建函数** | `s.createClient(conn)` | `s.createRoute(conn, ...)` |
| **用途** | 接受应用程序连接 | 接受集群节点连接 |
| **信息交换** | 客户端信息 | 集群拓扑信息 |

## 实际场景

**AcceptLoop** 处理的连接：
```go
// 应用程序连接到 NATS
nc, _ := nats.Connect("nats://localhost:4222")
nc.Publish("subject", []byte("message"))
```

**startRouteAcceptLoop** 处理的连接：
```go
// 另一个 NATS 服务器连接到这个服务器
// 在 server.conf 中配置：
// cluster {
//   port: 6222
//   routes: ["nats://other-server:6222"]
// }
```

这种设计实现了 NATS 的**分离职责原则**：客户端流量和集群内部流量使用不同的端口和处理逻辑，确保了系统的可扩展性和安全性。