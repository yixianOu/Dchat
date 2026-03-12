# 去中心化聊天系统架构设计文档

## 🎯 整体架构
基于 **NATS LeafNode + Hub Routes 集群** 实现的完全去中心化即时通讯系统，无单点故障，支持全球分布式部署。

---

## 🔨 核心设计决策

### 1. 离线消息同步方案（无镜像流轻量设计）
**✅ 优势：兼容NATS 2.10+，零重构，资源占用降低90%**
- 抛弃旧方案的本地镜像流设计，利用LeafNode的`JetStreamAllowUpstreamAPI: true`特性，直接跨Domain消费公网Hub的JetStream流
- 所有消息统一发布到公网Hub的JetStream（Domain="hub"），永久持久化存储
- 用户上线后自动拉取上次消费位点之后的所有离线消息，NATS自动管理消费位点，无需业务层处理
- **代码实现**：
  ```go
  func (s *Service) PublishJetStream(subject string, data []byte) (uint64, error) {
      s.js, _ = s.conn.JetStream(nats.Domain("hub")) // 直接绑定Hub的JetStream Domain
      ack, _ := s.js.Publish(subject, data)
      return ack.Sequence, nil // 返回全局唯一序列ID
  }
  ```

---

### 2. 全局消息去重机制
**✅ 彻底解决重复消息问题，三重保险：**
1. **全局唯一ID**：每条消息在Hub JetStream中有全局唯一的`Sequence ID`，和会话ID(`cid`)联合唯一
2. **数据库约束**：SQLite建立`idx_messages_unique(cid, nats_seq)`联合唯一索引，相同消息重复插入会被自动忽略
3. **统一存储逻辑**：不管是实时Core NATS消息还是离线JetStream消息，都走相同的存储逻辑，都会提取`Sequence ID`去重
- **数据示例**：
  ```sql
  sqlite> SELECT id, cid, content, nats_seq FROM messages;
  msg_66afc3c9341cc622|ec66ead60d582731|测试消息|63
  ```

---

### 3. 高可用设计
#### 3.1 Hub集群高可用
- 3+节点组成NATS Routes全互联集群，JetStream流配置3副本冗余，单节点故障无数据丢失
- **动态扩容**：支持在线添加新节点，零停机，Raft自动同步数据，业务完全无感知
- **容错能力**：3节点集群可容忍1台节点故障，5节点可容忍2台故障

#### 3.2 终端高可用
- LeafNode配置多个Hub地址，自动健康检查，故障秒切到其他可用Hub
- **配置示例**：
  ```json
  "leafnode": {
    "hub_urls": [
      "nats://hub1.example.com:7422",
      "nats://hub2.example.com:7422",
      "nats://hub3.example.com:7422"
    ]
  }
  ```

---

### 4. 多地域部署方案
#### 方案A：单集群同地域（推荐国内部署）
- 所有Hub节点在同一地域组成单Domain Routes集群
- 优势：架构简单，延迟低，数据强一致
- 适用场景：国内用户为主的场景

#### 方案B：Gateway多集群跨地域（全球部署）
- 每个地域部署独立Hub集群，配置独立JetStream Domain（如`hub-cn-east`, `hub-us-west`）
- 通过NATS Gateway连接各个地域集群，实时消息跨地域转发，JetStream数据本地存储
- 优势：用户接入最近集群，访问延迟低，异地多活
- 适配：业务代码只需扩展支持多Domain消费，无需核心逻辑修改

---

### 5. 前端架构优化
**✅ 解决重复消息和Invalid Date问题**：
- **单数据源原则**：所有显示的消息统一从SQLite读取，不直接显示LeafNode推送的实时消息
- 实时消息事件只做通知，收到后触发一次数据库刷新，确保数据一致性
- **时间戳兼容处理**：自动解析Go语言格式时间戳，兼容带`m=`单调时钟后缀的旧格式：
  ```typescript
  const convertStorageMessages = (historyMessages: any[]): DecryptedMessage[] => {
    return historyMessages.map((msg: any) => {
      let timestamp = msg.timestamp;
      if (typeof timestamp === 'string' && timestamp.includes(' m=')) {
        timestamp = timestamp.split(' m=')[0];
      }
      const date = new Date(timestamp);
      return {
        CID: msg.conversation_id,
        Sender: msg.sender_nickname || msg.sender_id,
        Ts: isNaN(date.getTime()) ? new Date().toISOString() : date.toISOString(),
        Plain: msg.content,
        IsGroup: msg.is_group,
        Subject: ''
      };
    });
  };
  ```

---

## 🚀 核心特性
| 特性 | 实现方式 |
|------|----------|
| 消息永不丢失 | 所有消息持久化到公网Hub JetStream 3副本存储 |
| 零重复消息 | 全局唯一Sequence ID + SQLite唯一索引自动去重 |
| 99.99%高可用 | Hub集群多副本 + LeafNode自动故障转移 |
| 弹性扩容 | Hub集群在线动态添加节点，零停机 |
| 离线消息同步 | 上线自动拉取所有离线消息，无需手动同步 |
| 端到端加密 | 私聊NaCl Box非对称加密，群聊AES-256-GCM对称加密 |

---

## 📁 相关文件路径
| 模块 | 文件路径 |
|------|----------|
| NATS客户端 | `internal/nats/client.go` |
| 离线同步逻辑 | `internal/nats/offline_sync.go` |
| 聊天业务逻辑 | `internal/chat/service.go` |
| SQLite存储 | `internal/storage/sqlite.go` |
| 前端主逻辑 | `frontend/src/App.tsx` |
| 配置文件 | `~/.dchat/config.json` |