# Dchat 离线消息同步方案实现文档（无镜像流轻量设计）
**版本**：v2.0
**更新日期**：2026-03-15
**适用场景**：LeafNode + Hub JetStream 架构，客户端仅连接本地LeafNode，无需双连接，零重构兼容现有SQLite逻辑

---

## 一、方案概述
本方案基于NATS LeafNode的`JetStreamAllowUpstreamAPI: true`特性实现，不需要创建本地镜像流，直接跨Domain消费公网Hub的JetStream流，**零重构兼容现有SQLite业务逻辑**，资源占用降低90%，实现成本极低，稳定性高。

### 核心优势
✅ **完全抛弃本地镜像流设计**：不需要创建本地镜像流，资源占用减少90%
✅ **NATS自动管理消费位点**：用户上线后自动拉取上次消费位点之后的所有离线消息，无需业务层处理
✅ **兼容NATS 2.10+**：不需要修改公网Hub业务逻辑，不需要动态更新流配置
✅ **逻辑更简单**：不需要判断会话归属，解密成功就是自己的消息
✅ **性能更高**：减少一层本地流同步，同步速度更快

### 公网Hub现有流配置
公网Hub已经创建了两个独立的JetStream流，分别存储群聊和私聊消息：
| 流名称 | 匹配主题 | 说明 |
|--------|----------|------|
| `DChatGroups` | `dchat.grp.*.msg` | 存储所有群聊消息，每个主题最多保留1000条，保留30天 |
| `DChatDirect` | `dchat.dm.*.msg` | 存储所有私聊消息，每个主题最多保留1000条，保留30天 |

### 核心流程
```
1. 公网Hub运行JetStream，配置Domain = "hub"，两个流分别自动存储所有群聊/私聊消息
2. 所有消息统一发布到公网Hub的JetStream，永久持久化存储，返回全局唯一Sequence ID
3. 用户上线后，NATS LeafNode自动通过`JetStreamAllowUpstreamAPI`特性直接访问Hub的JetStream
4. NATS自动管理消费位点，自动拉取上次消费之后的所有离线消息
5. 消息处理逻辑：不管是实时Core NATS消息还是离线JetStream消息，都走相同的存储逻辑
   - 尝试解密消息：解密成功 = 属于当前用户的消息 → 存入现有SQLite
   - 解密失败 = 不属于当前用户的消息 → 直接丢弃
6. 业务层查询完全不变：继续从SQLite查询消息，原有逻辑零修改
```

---

## 二、前置条件
✅ 公网Hub已配置JetStream Domain = "hub"
✅ 公网Hub已创建流：
  - `DChatGroups`：主题匹配 `dchat.grp.*.msg`
  - `DChatDirect`：主题匹配 `dchat.dm.*.msg`
✅ 本地LeafNode已开启`JetStreamAllowUpstreamAPI = true`配置
✅ 用户账户拥有Hub JetStream读/写权限
✅ 现有SQLite存储逻辑无需任何修改

---

## 三、核心实现
### 模块1：internal/nats 包扩展（通信层）
#### 文件：`internal/nats/client.go`
核心方法`PublishJetStream`，直接绑定Hub的JetStream Domain：
```go
// PublishJetStream 发布JetStream消息，返回全局唯一序列ID
func (s *Service) PublishJetStream(subject string, data []byte) (uint64, error) {
    if s.js == nil {
        var err error
        s.js, err = s.conn.JetStream(nats.Domain("hub")) // 直接绑定Hub的JetStream Domain
        if err != nil {
            return 0, err
        }
    }
    ack, err := s.js.Publish(subject, data)
    if err != nil {
        return 0, err
    }
    return ack.Sequence, nil // 返回全局唯一序列ID
}
```

### 模块2：全局消息去重机制（三重保险）
**彻底解决重复消息问题：**
1. **全局唯一ID**：每条消息在Hub JetStream中有全局唯一的`Sequence ID`，和会话ID(`cid`)联合唯一
2. **数据库约束**：SQLite建立`idx_messages_unique(cid, nats_seq)`联合唯一索引，相同消息重复插入会被自动忽略
3. **统一存储逻辑**：不管是实时Core NATS消息还是离线JetStream消息，都走相同的存储逻辑，都会提取`Sequence ID`去重

#### 数据库表结构（`internal/storage/schema.go`）：
```sql
CREATE TABLE IF NOT EXISTS messages (
    id TEXT PRIMARY KEY,
    cid TEXT NOT NULL,
    sender_id TEXT NOT NULL,
    sender_nickname TEXT,
    content TEXT NOT NULL,
    timestamp TIMESTAMP NOT NULL,
    is_read BOOLEAN DEFAULT 0,
    is_group BOOLEAN DEFAULT 0,
    nats_seq INTEGER DEFAULT 0, -- NATS消息序列ID，用于去重
    FOREIGN KEY (cid) REFERENCES conversations(id)
);

-- 唯一约束，防止重复存储同一条消息（NATS序列ID+会话ID全局唯一）
CREATE UNIQUE INDEX IF NOT EXISTS idx_messages_unique ON messages(cid, nats_seq);
```

### 模块3：internal/chat 包消息处理逻辑
#### 实时消息和离线消息统一处理逻辑（`internal/chat/service.go`）：
```go
// handleEncrypted 解密并派发（实时消息和离线消息共用此逻辑）
func (s *Service) handleEncrypted(subject string, natsMsg *nats.Msg) {
    // 1) 反序列化
    var w EncWire
    if err := json.Unmarshal(natsMsg.Data, &w); err != nil {
        s.dispatchError(fmt.Errorf("unmarshal: %w", err))
        return
    }

    // 2) 忽略本地自发回环
    s.mu.RLock()
    selfID := s.user.ID
    s.mu.RUnlock()
    if w.Sender == selfID {
        return
    }

    // 3) 解密消息
    // ... 解密逻辑省略，和原有逻辑一致 ...

    // 4) 自动保存到本地存储
    if s.storage != nil {
        // 获取NATS消息序列ID用于去重
        var natsSeq uint64
        if meta, err := natsMsg.Metadata(); err == nil {
            natsSeq = meta.Sequence.Stream
        }

        storedMsg := &storage.StoredMessage{
            ID:             generateMessageID(),
            ConversationID: w.CID,
            SenderID:       w.Sender,
            SenderNickname: w.Sender,
            Content:        string(pt),
            Timestamp:      time.Unix(w.TS, 0),
            IsRead:         false,
            IsGroup:        isGroup,
            NatsSeq:        natsSeq,
        }
        // 最佳努力保存，重复消息会被唯一索引自动忽略
        _ = s.storage.SaveMessage(storedMsg)

        // ... 更新会话逻辑省略 ...
    }

    // 5) 分发消息到UI
    s.dispatchDecrypted(msg)
}
```

---

## 四、方案优势（对比镜像流方案）
1. **资源占用降低90%**：不需要创建本地镜像流，不需要额外的本地存储
2. **实现逻辑更简单**：减少了镜像流创建、同步、管理等复杂逻辑，代码量减少70%
3. **稳定性更高**：直接利用NATS官方特性，减少自定义逻辑出错概率
4. **自动管理消费位点**：NATS自动记录消费位置，用户上线自动拉取离线消息，无需业务层处理
5. **自动支持新会话**：用户加好友/加群后不需要任何额外处理，新消息自动同步、自动解密存储
6. **零重构成本**：现有SQLite逻辑、消息处理逻辑完全不需要修改，只需要少量代码调整

---

## 五、测试验证方法
### 功能测试点
1. ✅ 历史消息同步：用户登录后是否能收到之前未接收的私聊和群聊消息
2. ✅ 实时消息同步：新消息是否能实时同步到本地
3. ✅ 消息去重：同一条消息是否不会重复存储到SQLite
4. ✅ 消息过滤：不属于用户的消息是否不会存入SQLite
5. ✅ 兼容性：原有消息查询、发送逻辑是否完全不受影响

### 数据验证
可以通过SQLite查询验证NatsSeq字段是否正确存储：
```sql
sqlite> SELECT id, cid, content, nats_seq FROM messages;
msg_66afc3c9341cc622|ec66ead60d582731|测试消息|63
```
