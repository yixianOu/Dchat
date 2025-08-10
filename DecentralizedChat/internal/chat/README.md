任务接收：给出去中心化（多节点 NATS Routes 集群）下基于 subject 的好友私聊与群聊设计。下面提供可直接实施的命名规范、权限策略、加密与扩展点。

## 总体原则
- 扁平、前缀明确：统一根前缀 dchat.
- 区分数据面 (messages) 。
- 订阅粒度最小化：客户端只订阅与自己相关的匹配模式，减少无关流量。
- 私聊与群聊都可映射到 JetStream（历史 / 重放）。
- 对称密钥 / 公钥分发继续放 KV（已存在 dchat_friends / dchat_groups）。
- 全部消息主体加密（应用层），Subject 仅携带最少路由语义，不泄露内容。

## 标识与ID
- 用户 ID：uid（现有 user.ID）。
- 群 ID：gid（随机 128bit / base32 / 短 hash）。
- 私聊对话 ID（cid）：cid = hash( sort(uidA, uidB) )，避免双向重复建会话。
  - 推荐：cid = hex(SHA256(uidLow + \":\" + uidHigh))[0:16]

## Subject 命名约定

### 1. 私聊 (Direct Message) — 精简
仅保留单一消息 subject：`dchat.dm.{cid}.msg`。

| 功能 | Subject 模板 | 说明 |
|------|--------------|------|
| 加密消息 | dchat.dm.{cid}.msg | 所有私聊内容（文本/文件元数据）统一承载 |

简化点：移除 ack / typing / presence / rekey 额外 subject，最小化订阅与实现复杂度。

加密（极简）：
1. 发送方用对方公钥 + 自己私钥（X25519 / libsodium box）派生共享密钥。
2. 随机 12B nonce + AES-256-GCM（或 ChaCha20-Poly1305）加密明文。
3. 附带 sender 公钥与签名（Ed25519）。
4. 接收方用自身私钥 + sender 公钥派生同一共享密钥解密。

cid 计算：`cid = hex( SHA256( min(uidA,uidB) + ":" + max(uidA,uidB) ) )[0:16]`。

订阅：首次建立会话时订阅 `dchat.dm.{cid}.msg`（幂等）。

消息示例（简化 encWire）：
```json
{
  "ver": 1,
  "cid": "a1b2c3d4e5f6a7b8",
  "sender": "user_A",
  "ts": 1670000000,
  "nonce": "base64-12B",
  "cipher": "base64",
  "alg": "x25519-box"
}
```

公钥轮换：直接在后续消息使用新的 sender_pub；无需单独 rekey subject。

订阅模式：针对每个会话单独精确订阅，避免广域 dchat.dm.*.msg 过滤压力。

### 2. 群聊 (Group) — 精简设计
去中心化环境下不引入中心化管理/成员列表/踢人等复杂逻辑，任何获知群 ID 与密钥者即可参与。

最小 Subject 集合：

| 功能 | Subject 模板 | 说明 |
|------|--------------|------|
| 群消息 | dchat.grp.{gid}.msg | 群内所有加密载荷（文本/文件元数据等统一封装）|

删除的高级特性（后续可选扩展）：成员进出广播、踢人、已读回执、群密钥轮换、typing、presence、meta.patch、history.req/rep。

订阅策略：客户端知晓 gid 后订阅 dchat.grp.{gid}.msg（当前不考虑轮换）。

消息体示例（群同样使用 encWire，cid 复用为 gid）：
```json
{
  "ver": 1,
  "cid": "{gid}",
  "sender": "user_A",
  "ts": 1670000000,
  "nonce": "base64-12B",
  "cipher": "base64",
  "alg": "aes256-gcm"
}
```

### 4. JetStream 建议（后续实现）
| 流 | 绑定 Subjects |
|----|---------------|
| DCHAT_DM | dchat.dm.*.msg |
| DCHAT_GRP | dchat.grp.*.msg |
| DCHAT_META (可选) | dchat.grp.*.meta.patch |
| DCHAT_CTRL (可选) | dchat.grp.*.ctrl.* / dchat.dm.*.ctrl.* |

### 5. 加密与 KV 配合
- 私聊：从 KV dchat_friends 获取对方 pub（FriendPubKeyRecord）。使用对方公钥加密发送的消息,使用自己的私钥解密接收的消息.
- 群聊：KV dchat_groups[groupID] 仅存储 {sym}（32 字节对称密钥 base64），暂不支持 rekey；若需要剔除成员可手动生成新 gid 建新群。
- 消息体结构（示例）：
  {
    \"ver\":1,
    \"sender\":\"uidA\",
    \"cid\":\"<cid or gid>\",
    \"ts\":1670000000,
    \"nonce\":\"base64-12B\",
    \"cipher\":\"base64\",
    \"alg\":\"aes256-gcm\",
    \"sig\":\"base64-ed25519\"
  }

### 6. 权限（Import/Export 或 Subscribe/Publish 控制）
最小可行 Allow：
- 订阅：dchat.dm.{cid}.msg（参与的每个会话） + dchat.grp.{gid}.msg（已加入的群）
- 发布：all-allow
实现方式：
- 启动时根据已加入会话/群动态生成 ImportAllow（订阅）+ ExportAllow（发布）列表并重启（当前模型）。
- 后续可引入服务端签发基于 JWT 的动态权限（减少重启）。

### 7. 私聊建立流程
1. 用户选择好友 B（有其 pubKey；否则提示添加）。
2. 计算 cid（排序 + hash）。
3. 检查本地是否已有会话；若新建：订阅 dchat.dm.{cid}.msg。
4. 对方若未订阅：收到消息后本地自动补订阅（惰性建立）。

### 8. 群聊创建 / 加入（精简流程）
1. 创建者：生成 gid + 随机 32 字节密钥 -> KV dchat_groups[gid] = {sym}
2. 分发：将 gid 及密钥安全发送给欲加入成员（点对点加密或线下）。
3. 成员：从 KV 获取 {sym}，订阅 dchat.grp.{gid}.msg。

### 9. 历史消息（延后）
初期不做：需要时将 dchat.grp.*.msg 绑定 JetStream 流以支持回放；或实现简单本地缓存分享。

## 快速对照（实现优先级）
1. 私聊：dchat.dm.{cid}.msg + KV 公钥
2. 群聊：dchat.grp.{gid}.msg + KV 对称密钥（无轮换）
3. （可选）JetStream 历史

## 最小落地列表（立即可做）
- deriveCID(uidA, uidB)
- newGID()
- JoinDirect / SendDirect
- CreateGroup / JoinGroup / SendGroup

需要更多实现示例可再提出。若要，我可以下一步直接补充工具函数与接口签名。请告知是否继续。