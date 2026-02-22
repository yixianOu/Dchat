# P2P + NAT穿透最小验证测试

本测试验证了使用 P2P 和 NAT 穿透技术实现两个局域网设备互联的可行性。

## 测试架构

```
Device A (局域网1) <---> Internet <---> Device B (局域网2)
     |                                        |
 NAT Router A                          NAT Router B
     |                                        |
192.168.1.x                            192.168.2.x
```

## 核心流程

1. **STUN获取公网地址** - 通过STUN协议获取NAT后的公网IP和端口
2. **信令交换** - 通过文件模拟信令服务器交换地址信息
3. **UDP打洞** - 同时向对方发送UDP包，在NAT上创建映射
4. **双向通信验证** - 验证P2P直连是否成功

## 运行测试

```bash
cd test
go test -v -timeout 60s
```

## 测试用例

### TestSTUNClient - STUN客户端测试
- 创建STUN客户端
- 获取公网地址
- 检测NAT类型

### TestUDPHolePunching - UDP打洞测试
- 创建两个P2P节点
- 通过STUN获取公网地址
- 信令交换
- 执行UDP打洞
- 验证双向通信

### TestP2PCommunication - 完整P2P通信测试
- 创建两个节点(Alice和Bob)
- 双向发送多条消息
- 统计通信成功率

### TestNATTypeDetection - NAT类型检测测试
- 多次获取公网地址
- 判断NAT类型(Cone NAT / Symmetric NAT)

## 测试结果示例

```
=== RUN   TestSTUNClient
========================================
测试1: STUN客户端 - 获取公网地址
========================================
本地地址: 0.0.0.0:49210
公网地址: 120.239.59.111:2090
NAT类型: Cone NAT
✅ STUN测试通过

=== RUN   TestP2PCommunication
========================================
测试3: 完整P2P通信流程
========================================
Alice: 内网=0.0.0.0:10001, 公网=120.239.59.111:2098, NAT=Cone NAT
Bob:   内网=0.0.0.0:10002, 公网=120.239.59.111:2099, NAT=Cone NAT

双向通信测试...
[Bob] 收到: Message 1 from Alice
[Alice] 收到: Message 1 from Bob
...
通信统计:
  Alice 收到: 8 条消息
  Bob 收到: 8 条消息
✅ 双向P2P通信成功!
```

## 技术实现

### STUN协议 (RFC 5389)
- Binding Request / Response
- XOR-MAPPED-ADDRESS解析
- 支持多个STUN服务器

### UDP打洞 (Hole Punching)
- 同时向公网地址和内网地址发送打洞包
- 在NAT上创建临时映射表项
- 双向同时打洞提高成功率

### 信令交换
- 基于文件的信令交换(模拟)
- 支持offer/answer模式
- 可替换为真实的信令服务器或DHT

## 注意事项

- **Cone NAT** 环境下打洞成功率约70-80%
- **Symmetric NAT** 几乎无法直接穿透，需要TURN中继
- 实际部署需要真实的信令服务器或DHT网络
- 企业防火墙可能阻止UDP打洞

## 扩展方向

1. **TURN中继** - 为Symmetric NAT提供后备方案
2. **ICE框架** - 整合STUN/TURN，按优先级尝试连接
3. **DHT信令** - 使用DHT网络替代中心化信令服务器
4. **QUIC协议** - 使用QUIC替代UDP，提高穿透率
