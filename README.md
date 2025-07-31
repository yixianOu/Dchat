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

### Routes配置示例

#### 节点配置
```conf
# nats-routes-node.conf
port: 4222
server_name: "node-#{node_id}"

# Routes集群配置
cluster: {
  name: "decentralized_chat"
  port: 6222
  routes: [
    "nats://seed-node.example.com:6222"  # 只需要一个种子节点
  ]
}

# 账户配置
include "accounts.conf"
```

#### FRP配置（Routes端口）
```ini
[common]
server_addr = frp.server.com
server_port = 7000
token = your_token

# Routes端口映射
[cluster]
type = tcp
local_port = 6222
# remote_port 不指定，使用随机分配
```

### Routes实现方案

#### 启动流程
1. **种子节点启动**：第一个节点启动作为种子
2. **节点加入**：新节点连接到种子节点（或任意现有节点）
3. **自动发现**：Routes协议自动建立全网状连接
4. **消息路由**：所有节点间消息自动路由

#### 动态扩展示例
```bash
# 启动种子节点
nats-server -c seed-node.conf

# 新节点加入（只需要种子节点地址）
nats-server -c new-node.conf  # routes: ["nats://seed:6222"]

# 网络自动形成全连通图：
# seed ←→ new-node ←→ another-node ←→ ...
```

### 技术优势

#### 1. 真正去中心化
- ❌ 无单点故障：任意节点故障网络仍可用
- ✅ 无固定服务器：所有节点地位平等  
- ✅ 无配置依赖：不需要固定的"超级节点"

#### 2. 动态自适应
- ✅ **链式发现**：A→B→C，A自动发现C
- ✅ **热插拔**：节点可随时加入/退出
- ✅ **自愈网络**：故障节点自动从网络中移除

#### 3. 配置简化
- ✅ **单一种子**：只需要一个初始连接点
- ✅ **零运维**：无需修改现有节点配置
- ✅ **容错启动**：种子节点故障后其他节点可作为种子

### FRP配置策略

#### Routes集群的FRP映射

**FRP配置示例：**
```ini
[common]
server_addr = frp.server.com
server_port = 7000
token = your_token

# NATS客户端端口映射
[nats-client]
type = tcp
local_port = 4222
# remote_port 不指定，使用随机分配

# Routes集群端口映射
[nats-routes]
type = tcp
local_port = 6222
# remote_port 不指定，使用随机分配
```

**优势：**
- ✅ 零配置，无端口冲突
- ✅ FRP自动管理，稳定可靠
- ✅ 支持大规模节点扩展
- ✅ 降低运维复杂度
- ✅ Routes自动发现，无需手动配置连接

## 实现优势

### 真正去中心化特性
- ✅ **无单点故障**：Routes全网状网络，任意节点故障不影响全局
- ✅ **无固定服务器**：所有节点地位平等，无"超级节点"概念
- ✅ **自动发现**：新节点连接任意现有节点即可加入网络
- ✅ **动态自愈**：故障节点自动从网络中移除

### 可扩展性
- ✅ **水平扩展**：Routes支持无限节点扩展
- ✅ **弹性伸缩**：节点可动态加入和退出网络
- ✅ **负载分散**：消息路由自动在所有节点间分布
- ✅ **配置简化**：新节点只需一个种子节点地址

### Routes架构优势
- ✅ **链式连接**：支持A→B→C的自动发现连接
- ✅ **全网状拓扑**：最终形成完全连通的网状网络  
- ✅ **零配置扩展**：新节点加入无需修改现有配置
- ✅ **灵活路由**：消息自动在全网络中路由

## 真正去中心化方案：NATS Routes集群

### 核心发现：Routes支持动态链式连接！

从NATS源码分析发现，**Routes机制支持链式连接和动态扩展**：

#### Routes vs LeafNode vs Gateway对比

| 特性 | Routes | LeafNode | Gateway |
|------|--------|----------|---------|
| **链式连接** | ✅ 支持链式连接 | ❌ 无法链式连接 | ✅ 支持但需双向配置 |
| **动态扩展** | ✅ 自动发现和连接 | ❌ 需要预配置服务器 | ❌ 需要双方修改配置 |
| **配置复杂度** | ✅ 简单，只需指定一个种子节点 | ✅ 简单 | ❌ 复杂，需要双向配置 |
| **去中心化** | ✅ 真正去中心化 | ❌ 需要固定服务器 | ❌ 需要固定服务器 |

#### Routes链式连接原理

```
节点A ──routes──► 节点B ──routes──► 节点C
   ▲                                  │
   └─────────── 自动发现连接 ──────────┘
```

**关键优势：**
- ✅ **链式连接**：A→B→C，A会自动发现并连接到C
- ✅ **动态发现**：新节点只需连接到任一现有节点
- ✅ **全网状拓扑**：最终形成完全连通的网状网络
- ✅ **零固定节点**：任意节点都可以作为种子节点

### 新架构设计：纯Routes集群

```
Region A            Region B            Region C
┌──────────┐       ┌──────────┐       ┌──────────┐
│ NATS     │◄──────┤ NATS     │──────►│ NATS     │
│ (Routes) │       │ (Routes) │       │ (Routes) │
└─────▲────┘       └─────▲────┘       └─────▲────┘
      │                  │                  │
      │                  │                  │
┌─────▼────┐       ┌─────▼────┐       ┌─────▼────┐
│用户设备   │       │用户设备   │       │用户设备   │
│(Client)  │       │(Client)  │       │(Client)  │
└──────────┘       └──────────┘       └──────────┘
```

**新方案特点：**
- 🎯 **真正去中心化**：无固定服务器，任意节点故障不影响网络
- 🎯 **动态扩展**：新节点连接到任一现有节点即可加入网络
- 🎯 **自动发现**：Routes会自动建立全网状连接
- 🎯 **配置简单**：每个节点只需要一个种子节点地址

## Option（已废弃）

leafNode cluster无法热更新,考虑mcp server
中心化的frp(暴露cluster端口),发现靠tls公钥(携带公网cluster端口)
问题1:重启后映射的端口会变,需要重新注册发现(可以尝试frp的api查询和通配符公钥)
去中心化,但是各个集群内部可以广播自己的位置(群聊只需加入一个节点并订阅subject即可)
所有节点都是同一个集群,初始连接为服务器节点,通过allow subject实现隔离
leafNode不支持链式连接,考虑Gateway

## TODO
1. ✅ 重新设计：纯LeafNode架构替代Gateway（已废弃）
2. ✅ 简化FRP策略：纯随机端口+DHT服务发现
3. 🎯 **重新设计：Routes集群实现真正去中心化**
4. 🔄 实现Routes集群Demo
5. ⏳ 实现FRP API客户端
6. ⏳ 开发DHT服务发现机制
7. ⏳ 实现客户端连接
8. ⏳ 开发跨区域消息路由
9. ⏳ 集成消息加密和用户认证
10. ⏳ 构建聊天室UI界面