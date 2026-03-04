# NAT穿透技术指南

## 1. NAT类型分类

### 1.1 NAT类型及穿透难度

```
                    容易穿透
                       ▲
                      / \
            Full Cone   Address-Restricted
               NAT           Cone NAT
                  \         /
                   \       /
            Port-Restricted Cone NAT
                      |
                      ▼
               Symmetric NAT
                  最难穿透
```

### 1.2 各类型特征

**Full Cone NAT (完全圆锥型)**
```
内网 192.168.1.2:12345 → NAT → 1.2.3.4:50000
任何外部主机都可以访问 1.2.3.4:50000
```
- 穿透难度: ⭐
- 出现场景: 早期路由器

**Address-Restricted Cone NAT (地址限制)**
```
内网必须先向目标IP发送数据，该IP才能访问NAT映射端口
```
- 穿透难度: ⭐⭐
- 出现场景: 大多数家用路由器

**Port-Restricted Cone NAT (端口限制)**
```
内网必须先向目标IP:Port发送数据，该精确地址才能访问映射端口
```
- 穿透难度: ⭐⭐⭐
- 出现场景: 较新的家用路由器

**Symmetric NAT (对称型)**
```
发送到不同目的地使用不同映射端口，无法预测
```
- 穿透难度: ⭐⭐⭐⭐⭐
- 出现场景: 企业防火墙，运营商级NAT

---

## 2. 穿透技术

### 2.1 STUN协议

获取公网地址并判断NAT类型：

```
客户端 ──Binding Request──→ STUN服务器
客户端 ←─Response(公网IP:Port)─ STUN服务器
```

### 2.2 UDP打洞

```
准备阶段:
Alice(A:10000) → NAT_A(A':50000)
Bob(B:20000)   → NAT_B(B':60000)
       ↓              ↓
    信令服务器交换公网地址

打洞阶段:
A':50000 ←─────UDP─────→ B':60000
         (同时发送)
         
成功率: Cone NAT ~80%, Symmetric NAT ~0%
```

### 2.3 TURN中继

穿透失败时的后备方案：

```
Alice → TURN服务器 → Bob
```

缺点：服务器带宽成本高，延迟增加

### 2.4 ICE框架

综合使用STUN/TURN，按优先级尝试连接：

```
优先级(高→低):
1. Host Candidate (本地地址) - 同局域网直连
2. Server Reflexive (STUN获取) - NAT公网映射
3. Peer Reflexive (打洞发现) - 动态发现
4. Relayed (TURN中继) - 最后手段
```

---

## 3. 去中心化场景挑战

### 3.1 信令问题

传统WebRTC需要信令服务器，去中心化替代方案：

**方案1: DHT作为信令**
```
PeerA → DHT网络存储SDP → PeerB从DHT获取
```
- 延迟高，但完全去中心化

**方案2: 二维码/短码交换**
- 适用于面对面场景
- 扫描交换连接信息

**方案3: 区块链信令**
- 成本高但永久可用

### 3.2 移动网络挑战

1. **频繁切换网络** (WiFi ↔ 4G ↔ 5G)
   - IP地址变化需要ICE Restart

2. **运营商级NAT (CGNAT)**
   - 多层NAT嵌套
   - 几乎无法穿透，必须依赖中继

3. **电池优化**
   - 后台连接被杀死
   - 需要推送通知唤醒

---

## 4. 技术方案对比

### 4.1 方案分类

| 方案 | 直连率 | 中继依赖 | 延迟 | 复杂度 |
|------|--------|----------|------|--------|
| STUN only | ~60% | 无 | 低 | 低 |
| ICE (STUN+TURN) | ~95% | 可选 | 中 | 中 |
| UDP打洞 | ~70% | 无 | 低 | 中 |
| QUIC | ~80% | 可选 | 低 | 中 |

### 4.2 主流去中心化方案

**A. DHT + STUN/ICE (最流行)**
```
代表: Tox, libp2p

NodeA ←→ DHT网络 ←→ NodeB
  ↓                    ↓
 ICE Agent      ←→   ICE Agent
  ↓                    ↓
TURN中继(可选)

优点: 真正去中心化，可扩展
缺点: 实现复杂，连接建立较慢
```

**B. 中继网络**
```
代表: Nostr, Matrix, Session

ClientA ←→ Relay服务器 ←→ ClientB

优点: 实现简单，离线消息支持
缺点: 中继可见元数据，需要基础设施
```

**C. 混合网络 (洋葱路由)**
```
代表: Session (LokiNet)

Alice → 入口节点 → 中间节点 → 出口节点 → Bob
        (多层加密)

优点: 元数据保护强，抗审查
缺点: 延迟高，复杂度高
```

**D. 局域网优先**
```
代表: Briar

优先: WiFi/蓝牙直连
后备: 互联网中继

优点: 完全离线可用
缺点: 范围受限，同步延迟大
```

---

## 5. 实现建议

### 5.1 小型私密群组 (< 50人)

推荐方案：libp2p + WebRTC + 可选TURN

```javascript
// 核心配置
const node = await createLibp2p({
  transports: [
    tcp(),              // 直接连接
    webSockets(),       // 浏览器支持
    circuitRelayTransport()  // 中继后备
  ],
  
  peerDiscovery: [
    bootstrap({ list: ['引导节点列表'] }),
    kadDHT({ clientMode: false })
  ],
  
  services: {
    identify: identify(),
    autoNAT: autoNAT(),
    upnpNAT: upnpNAT()  // 尝试UPnP端口映射
  },
  
  nat: {
    enabled: true
  }
})
```

### 5.2 连接建立流程

```
1. 启动节点，连接引导节点
2. 通过DHT发现目标节点
3. 收集ICE候选者 (Host/STUN/TURN)
4. 按优先级尝试连接
5. 成功建立P2P连接或使用中继
```

### 5.3 性能优化

1. **连接预热** - 启动时预建立连接池
2. **智能路由** - 优先使用低延迟连接
3. **批量处理** - 合并小消息减少往返
4. **自适应质量** - 根据网络状况调整
5. **合理保活** - 电池感知的心跳间隔

---

## 6. 调试工具

**网络调试**
- Wireshark (抓包分析)
- chrome://webrtc-internals (Chrome浏览器)
- about:webrtc (Firefox浏览器)

**libp2p调试**
```bash
DEBUG=libp2p:* node app.js
```

**STUN/TURN测试**
- trickle-ice (在线WebRTC测试)
- stunclient (命令行工具)

---

## 7. 常见问题

**Q: 为什么Tailscale类方案不流行？**

A: Tailscale依赖中心控制服务器协调，违反去中心化原则。但其WireGuard协议、NAT穿透算法值得借鉴。

**Q: Symmetric NAT如何穿透？**

A: 几乎无法直接穿透，必须使用TURN中继或生日攻击(Birthday Attack)等高级技术。

**Q: 移动端如何处理网络切换？**

A: 实现ICE Restart机制，在网络变化时重新建立连接。使用推送通知在后台唤醒应用。

**Q: 如何降低TURN服务器成本？**

A: 
1. 仅在直连失败时使用
2. 限制中继带宽和时长
3. 鼓励用户贡献节点形成中继网络
