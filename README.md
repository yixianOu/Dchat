# 去中心化聊天室需求与开发计划

## 设计方案

### 架构概述
采用 **Gateway集群 + LeafNode** 的混合模式实现去中心化聊天室：
- 每个地理区域部署一个Gateway集群作为"超级节点"
- 用户设备通过LeafNode连接到最近的Gateway集群
- Gateway集群间通过FRP实现公网互连

### NATS集群方式选择

#### 1. Gateway集群 (推荐主架构)
```
Region A          Region B          Region C
┌─────────┐      ┌─────────┐      ┌─────────┐
│Gateway A│◄────►│Gateway B│◄────►│Gateway C│
│ Cluster │      │ Cluster │      │ Cluster │
└─────────┘      └─────────┘      └─────────┘
     ▲                ▲                ▲
     │                │                │
┌─────────┐      ┌─────────┐      ┌─────────┐
│LeafNode │      │LeafNode │      │LeafNode │
│(Users)  │      │(Users)  │      │(Users)  │
└─────────┘      └─────────┘      └─────────┘
```

**优势：**
- 消息在集群间自动路由，无需手动配置
- 支持主题订阅的全局传播
- 可以处理网络分区和节点故障
- LeafNode可以动态连接到任意Gateway集群

#### 2. FRP端口映射策略
```ini
# Gateway集群节点配置
[gateway-7222]
type = tcp
local_port = 7222
remote_port = 17222  # 固定端口，避免重启变化

[cluster-6222] 
type = tcp
local_port = 6222
remote_port = 16222  # Gateway集群内部通信端口
```

### 具体实现方案

#### Phase 1: 基础Gateway集群
1. **每个区域部署3节点Gateway集群**
   - 提供高可用性
   - 内部使用私网通信
   - 通过FRP暴露Gateway端口到公网

2. **服务发现机制**
   ```json
   {
     "regions": {
       "asia": ["frp.server.com:17222", "frp.server.com:17223"],
       "europe": ["frp.eu.com:17222", "frp.eu.com:17223"],
       "america": ["frp.us.com:17222", "frp.us.com:17223"]
     }
   }
   ```

#### Phase 2: 用户LeafNode连接
1. **客户端启动流程**
   - 获取服务发现配置
   - 测试延迟选择最近的Gateway集群
   - 建立LeafNode连接

2. **消息路由**
   - 聊天室消息自动在Gateway集群间传播
   - 用户只需连接任一可用的Gateway节点

#### Phase 3: 高级功能
1. **智能重连**
   - LeafNode支持多个Gateway endpoint
   - 自动故障转移到其他区域

2. **负载均衡**
   - 根据用户地理位置分配Gateway集群
   - 动态调整连接策略

操作记录：

- 2025-07-22：修改cmd/LeafJWT/main.go，依次执行4条nsc命令并打印输出。
- 2025-07-21：修改cmd/LeafJWT/main.go，捕获并打印nsc命令(nsc push -a APP)的stdout输出。
- 2025-07-06：添加项目需求与开发计划

## 技术细节

### Gateway集群配置示例
```conf
# nats-gateway-asia.conf
port: 4222
server_name: "gateway-asia-1"

# Gateway配置
gateway: {
  name: "asia"
  port: 7222
  gateways: [
    {name: "asia", urls: ["nats://localhost:7222"]},
    {name: "europe", urls: ["nats://frp.eu.com:17222"]},
    {name: "america", urls: ["nats://frp.us.com:17222"]}
  ]
}

# 集群配置(区域内)
cluster: {
  port: 6222
  routes: [
    "nats://192.168.1.10:6222",
    "nats://192.168.1.11:6222"
  ]
}
```

### LeafNode客户端配置
```conf
# 用户设备NATS配置
port: 4223
leafnode: {
  remotes: [
    {urls: ["nats://frp.server.com:17222"]},  # 亚洲Gateway
    {urls: ["nats://frp.eu.com:17222"]},      # 欧洲Gateway
    {urls: ["nats://frp.us.com:17222"]}       # 美洲Gateway
  ]
}
```

### FRP配置策略

#### 采用随机端口映射 + 实时服务发现

**FRP配置示例：**
```ini
[common]
server_addr = frp.server.com
server_port = 7000
token = your_token

# 随机端口映射
[gateway]
type = tcp
local_port = 7222
# remote_port 不指定，使用随机分配

[cluster]
type = tcp
local_port = 6222
# remote_port 不指定，使用随机分配
```

**优势：**
- ✅ 零配置，无端口冲突
- ✅ FRP自动管理，稳定可靠
- ✅ 支持大规模节点扩展
- ✅ 降低运维复杂度

## 实现优势

### 去中心化特性
- 无单点故障：任一Gateway集群故障不影响全局
- 就近连接：用户自动连接最近的Gateway集群
- 消息传播：聊天消息在所有Gateway集群间同步

### 可扩展性
- 水平扩展：增加新的Gateway集群支持更多区域
- 弹性伸缩：集群内节点可动态增减
- 负载分散：用户分布在不同Gateway集群

### FRP随机端口优势
- 零配置：新节点无需端口规划
- 高可用：FRP自动处理端口分配
- 简化运维：减少端口冲突管理

## Option

leafNode cluster无法热更新,考虑mcp server
中心化的frp(暴露cluster端口),发现靠tls公钥(携带公网cluster端口)
问题1:重启后映射的端口会变,需要重新注册发现(可以尝试frp的api查询和通配符公钥)
去中心化,但是各个集群内部可以广播自己的位置(群聊只需加入一个节点并订阅subject即可)
所有节点都是同一个集群,初始连接为服务器节点,通过allow subject实现隔离

## TODO
1. ✅ 设计Gateway+LeafNode混合架构
2. ✅ 简化FRP策略：纯随机端口+DHT服务发现
3. 🔄 实现Gateway集群Demo (cmd/superCluster)
4. ⏳ 实现FRP API客户端
5. ⏳ 开发DHT服务发现机制
6. ⏳ 实现LeafNode客户端连接
7. ⏳ 集成消息加密和用户认证
8. ⏳ 构建聊天室UI界面