# 去中心化聊天室需求与开发计划

## 设计方案

### 架构概述
采用 **Gateway集群 + LeafNode** 的混合模式实现去中心化聊天室：
- 每个地理区域部署一个Gateway集群作为"超级节点"
- 用户设备通过LeafNode连接到最近的Gateway集群
- Gateway集群间通过FRP实现公网互连

### NATS集群方式重新选择

#### LeafNode架构关键问题

**LeafNode能否链式连接？**
- ❌ **LeafNode不能作为其他LeafNode的服务器**
- ❌ LeafNode只能连接到NATS Server，不能互相连接
- ❌ 这意味着无法通过LeafNode实现多层级连接

#### 方案对比

**Gateway集群的问题：**
- ❌ 需要双方都修改配置才能建立连接
- ❌ 配置复杂，不利于动态扩展
- ❌ 新节点加入需要所有节点更新配置

**纯LeafNode的限制：**
- ❌ LeafNode无法链式连接，必须都连接到Server
- ❌ 需要所有区域都有固定的NATS Server
- ❌ 无法实现真正的点对点网络

**最终推荐：Server + LeafNode混合**
```
Region A                    Region B
┌─────────────┐            ┌─────────────┐
│NATS Server A│◄──────────►│NATS Server B│ (Gateway连接)
│             │            │             │
└─────────────┘            └─────────────┘
       ▲                            ▲
       │                            │
┌─────────────┐            ┌─────────────┐
│LeafNode     │            │LeafNode     │
│(User Device)│            │(User Device)│
└─────────────┘            └─────────────┘
```

**优势：**
- ✅ 用户设备零配置：LeafNode连接固定Server
- ✅ 服务器间用Gateway：自动消息路由
- ✅ 混合最佳特性：LeafNode的简单性 + Gateway的互联性

#### 2. 混合架构设计

**NATS服务器（区域固定节点）：**
- 每个区域部署固定的NATS服务器
- 服务器间通过Gateway互联
- 通过FRP暴露LeafNode端口到公网

**LeafNode客户端（用户设备）：**
- 用户设备作为LeafNode连接到最近的服务器
- 只需配置服务器地址，无需相互连接
- 消息通过Gateway自动在区域间路由

### 具体实现方案

#### Phase 1: 区域NATS服务器
1. **服务器部署**
   - 每个区域部署2-3个NATS服务器做高可用
   - 服务器间配置Gateway互联
   - 通过FRP暴露LeafNode端口

2. **Gateway配置**
   ```conf
   # 亚洲服务器gateway配置
   gateway: {
     name: "asia"
     port: 7222
     gateways: [
       {name: "europe", urls: ["nats://frp-eu.com:23456"]},
       {name: "america", urls: ["nats://frp-us.com:34567"]}
     ]
   }
   ```

#### Phase 2: LeafNode客户端
1. **用户设备连接**
   - 连接到最近的区域服务器
   - 支持多服务器故障转移
   - 消息自动路由到全球用户

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

### 服务器节点配置
```conf
# nats-server-asia.conf
port: 4222
server_name: "server-asia-1"

# Gateway配置 - 服务器间互联
gateway: {
  name: "asia"
  port: 7222
  gateways: [
    {name: "europe", urls: ["nats://frp-eu.com:23456"]},
    {name: "america", urls: ["nats://frp-us.com:34567"]}
  ]
}

# LeafNode配置 - 接受客户端连接
leafnodes: {
  port: 7422
}

# 账户配置
include "accounts.conf"
```

### LeafNode客户端配置
```conf
# 用户设备NATS配置
port: 4223
leafnode: {
  remotes: [
    {urls: ["nats://frp-asia.com:12345"]},    # 亚洲服务器
    {urls: ["nats://frp-eu.com:23456"]},      # 欧洲服务器(备用)
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

# LeafNode端口映射
[leafnode]
type = tcp
local_port = 7422
# remote_port 不指定，使用随机分配

# Gateway端口映射(如果需要)
[gateway]
type = tcp  
local_port = 7222
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
- 水平扩展：增加新的服务器节点支持更多用户
- 弹性伸缩：LeafNode可动态连接和断开
- 负载分散：用户分布连接到不同服务器节点

### 纯LeafNode架构优势
- 单向连接：只需LeafNode主动连接服务器
- 零配置扩展：新节点加入无需修改现有配置  
- 简化运维：服务器配置相对固定
- 灵活路由：通过账户和权限控制消息路由

## Option

leafNode cluster无法热更新,考虑mcp server
中心化的frp(暴露cluster端口),发现靠tls公钥(携带公网cluster端口)
问题1:重启后映射的端口会变,需要重新注册发现(可以尝试frp的api查询和通配符公钥)
去中心化,但是各个集群内部可以广播自己的位置(群聊只需加入一个节点并订阅subject即可)
所有节点都是同一个集群,初始连接为服务器节点,通过allow subject实现隔离
leafNode不支持链式连接,考虑Gateway

## TODO
1. ✅ 重新设计：纯LeafNode架构替代Gateway
2. ✅ 简化FRP策略：纯随机端口+DHT服务发现
3. 🔄 实现LeafNode服务器Demo
4. ⏳ 实现FRP API客户端
5. ⏳ 开发DHT服务发现机制
6. ⏳ 实现LeafNode客户端连接
7. ⏳ 开发跨区域网桥机制
8. ⏳ 集成消息加密和用户认证
9. ⏳ 构建聊天室UI界面