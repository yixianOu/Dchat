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

## Go 定时器机制

### time.NewTicker(time.Second) 的作用

`time.NewTicker(time.Second)` 创建一个定时器，每隔指定时间间隔（1秒）触发一次：

**基本功能：**
- **定期发送时间信号**：每秒向其 `C` 通道发送一个 `time.Time` 值
- **持续运行**：直到被明确停止
- **非阻塞**：不会阻塞创建它的 goroutine

**典型用法模式：**
```go
ticker := time.NewTicker(time.Second)
defer ticker.Stop() // 重要：确保资源被释放

for {
    select {
    case <-ticker.C:
        // 每秒执行一次的逻辑
        fmt.Println("定时任务执行")
    case <-ctx.Done():
        return
    }
}
```

**在 NATS Server 中的作用：**
- 健康检查：定期检查服务器状态
- 统计信息更新：每秒更新服务器统计数据
- 连接监控：定期检查客户端连接状态
- 清理任务：定期清理过期的资源
- 心跳机制：向其他节点发送心跳信号

## goroutine 管理设计分析

### startGoRoutine 设计问题

**当前设计的问题：**
```go
func (s *Server) startGoRoutine(f func(), tags ...pprofLabels) bool {
    s.grWG.Add(1)
    go func() {
        setGoRoutineLabels(tags...)
        f() // 如果f()内部panic，grWG.Done()永远不会被调用！
    }()
    return true
}
```

**问题分析：**
- **容易出错**：开发者可能忘记添加 `defer s.grWG.Done()`
- **不安全**：如果函数 panic，WaitGroup 会永远阻塞
- **重复代码**：每个地方都要手动添加

**建议的改进设计：**
```go
func (s *Server) startGoRoutine(f func()) bool {
    s.grMu.Lock()
    defer s.grMu.Unlock()
    if s.grRunning {
        s.grWG.Add(1)
        go func() {
            defer func() {
                if r := recover(); r != nil {
                    s.Errorf("Goroutine panic: %v", r)
                }
                s.grWG.Done() // 确保总是会调用
            }()
            f()
        }()
        return true
    }
    return false
}
```

## 集群连接建立机制

### 连接信息交换的时序控制

**问题背景：**
在 NATS 集群中，服务器间建立连接时需要交换 INFO 信息，这个过程有特定的时序要求。

**延迟发送 INFO 的场景：**
```
// If we are creating a pooled connection and this is the server soliciting
// the connection, we will delay sending the INFO after we have processed
// the incoming INFO from the remote. Also delay if configured for compression.
```

**含义解释：**
1. **pooled connection** - 池化连接：NATS 支持连接池机制，可以在两个服务器间建立多个并行连接
2. **server soliciting** - 服务器主动发起连接：区分主动发起方和被动接受方
3. **delay sending INFO** - 延迟发送信息：等待处理远程 INFO 后再发送自己的

**为什么需要延迟？**
- **避免竞态条件**：确保连接参数正确协商
- **提高兼容性**：不同版本服务器间更好的互操作
- **优化性能**：压缩参数能够正确协商
- **连接稳定性**：减少连接建立失败的可能性

**时序流程：**
```
Server A (soliciting) ----连接----> Server B (accepting)
                                    Server B: 立即发送 INFO
Server A: 接收并处理远程 INFO
Server A: 基于远程信息发送自己的 INFO
```

## 连接健康监控

### Stale Connection 检测机制

**Stale 的含义：**
"Stale" 表示 **"陈旧的、失效的、不活跃的"** 连接，特指那些看起来还连接着但实际上已经无法正常通信的连接。

**出现原因：**
1. **网络中断但 TCP 连接未断开**：网络设备故障、路由问题
2. **半开连接**：一端进程崩溃重启，另一端不知情
3. **长时间无活动**：中间网络设备可能清理了连接状态

**检测机制：**
```go
func (c *client) watchForStaleConnection(pingInterval time.Duration, pingMax int) {
    // 设置超时：pingInterval * (pingMax + 1)
    c.ping.tmr = time.AfterFunc(pingInterval*time.Duration(pingMax+1), func() {
        c.Debugf("Stale Client Connection - Closing")
        c.closeConnection(StaleConnection) // 关闭失效连接
    })
}
```

**Ping-Pong 检测流程：**
```
服务器 A ----PING----> 服务器 B
服务器 A <---PONG----- 服务器 B   ✓ 连接正常

服务器 A ----PING----> 服务器 B
服务器 A ----PING----> 服务器 B   ✗ 无响应
服务器 A ----PING----> 服务器 B   ✗ 无响应
...达到 pingMax 次数后认为连接 stale
```

**配置示例：**
```yaml
cluster:
  ping_interval: "2s"    # 每2秒发送一次 PING
  ping_max: 3           # 最多3次无响应后断开
# 总超时时间 = 2秒 × (3+1) = 8秒
```

**应用场景：**
- **集群健康监控**：及时发现节点故障
- **网络故障恢复**：触发重连机制
- **资源清理**：释放无效连接占用的内存

## 总结

以上内容涵盖了 NATS 服务器的关键设计模式：
1. **职责分离**：客户端和集群连接的分离处理
2. **定时机制**：基于 ticker 的周期性任务
3. **goroutine 管理**：安全的并发控制
4. **连接协商**：智能的信息交换时序
5. **健康监控**：主动的连接状态检测

这些机制共同保证了 NATS 集群的可靠性、性能和可扩展性。