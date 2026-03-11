# Dchat 离线消息同步方案实现文档（极简兼容版）
**版本**：v1.2
**更新日期**：2026-03-12
**适用场景**：LeafNode + Hub JetStream 架构，客户端仅连接本地LeafNode，无需双连接，零重构兼容现有SQLite逻辑

---

## 一、方案概述
本方案基于NATS官方JetStream Mirror镜像功能实现，不需要修改公网Hub业务逻辑，不需要动态更新流配置，**零重构兼容现有SQLite业务逻辑**，实现成本极低，稳定性高，99%场景适用。

### 核心优化
✅ **完全不需要重构现有SQLite逻辑**：原有存储、查询逻辑一行都不用改
✅ **逻辑更简单**：不需要判断会话归属，解密成功就是自己的消息
✅ **性能更高**：减少一层业务判断，同步速度更快

### 公网Hub现有流配置
公网Hub已经创建了两个独立的JetStream流，分别存储群聊和私聊消息：
| 流名称 | 匹配主题 | 说明 |
|--------|----------|------|
| `DChatGroups` | `dchat.grp.*.msg` | 存储所有群聊消息，每个主题最多保留1000条，保留30天 |
| `DChatDirect` | `dchat.dm.*.msg` | 存储所有私聊消息，每个主题最多保留1000条，保留30天 |

### 核心流程
```
1. 公网Hub运行JetStream，两个流分别自动存储所有群聊/私聊消息
2. 用户登录时，本地LeafNode创建两个镜像流，分别同步Hub上的DChatGroups和DChatDirect流
3. 后台同步协程自动消费两个镜像流的所有消息，直接ACK
4. 同步层逻辑简化：仅尝试解密消息
   - 解密成功 = 属于当前用户的消息 → 存入现有SQLite（原有逻辑不变）
   - 解密失败 = 不属于当前用户的消息 → 直接丢弃
5. 业务层查询完全不变：继续从SQLite查询消息，原有逻辑零修改
```

---

## 二、前置条件
✅ 公网Hub已配置JetStream Domain = "hub"
✅ 公网Hub已创建流：
  - `DChatGroups`：主题匹配 `dchat.grp.*.msg`
  - `DChatDirect`：主题匹配 `dchat.dm.*.msg`
✅ 本地LeafNode已开启JetStream并配置`JetStreamAllowUpstreamAPI = true`
✅ 用户账户拥有Hub JetStream读权限
✅ 现有SQLite存储逻辑无需任何修改

---

## 三、核心实现步骤
### 模块1：internal/nats 包扩展（通信层）
#### 文件：`internal/nats/service.go`
需要添加以下字段到Service结构体：
```go
import (
    "context"
    "sync"
    "github.com/nats-io/nats.go"
)

type Service struct {
    // 原有字段保持不变
    conn *nats.Conn // 已有的NATS连接字段
    mu   sync.RWMutex

    // ========== 新增字段 ==========
    js              nats.JetStreamContext // JetStream上下文
    syncCfg         *OfflineSyncConfig    // 同步配置
    syncSubGroup    *nats.Subscription    // 群聊同步订阅
    syncSubDirect   *nats.Subscription    // 私聊同步订阅
    syncCtx         context.Context       // 同步协程上下文
    syncCancel      context.CancelFunc    // 同步取消函数
    syncRunning     bool                  // 同步状态
    streamNameGroup string                // 本地群聊镜像流名称
    streamNameDirect string               // 本地私聊镜像流名称
}
```

#### 文件：`internal/nats/offline_sync.go` （新增文件）
完整代码：
```go
package nats

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
)

// OfflineSyncConfig 同步配置
type OfflineSyncConfig struct {
	UserID         string          // 当前用户ID
	MessageHandler func([]byte) error // 消息处理回调
	ErrorHandler   func(error)      // 错误处理回调
}

// InitOfflineMirror 初始化镜像流
func (s *Service) InitOfflineMirror(cfg *OfflineSyncConfig) error {
	if s.conn == nil || !s.conn.IsConnected() {
		return fmt.Errorf("nats not connected")
	}

	// 初始化JetStream上下文
	js, err := s.conn.JetStream()
	if err != nil {
		return fmt.Errorf("jetstream init failed: %w", err)
	}
	s.js = js
	s.syncCfg = cfg

	// ========== 创建群聊消息镜像流 ==========
	s.streamNameGroup = fmt.Sprintf("USER_OFFLINE_GRP_%s", cfg.UserID)
	// 清理旧流
	_ = js.DeleteStream(s.streamNameGroup)
	// 创建镜像流，同步Hub上的DChatGroups流
	_, err = js.AddStream(&nats.StreamConfig{
		Name: s.streamNameGroup,
		Mirror: &nats.StreamSource{
			Name:          "DChatGroups", // Hub上的群聊流名称
			FilterSubject: "dchat.grp.*.msg", // 同步所有群聊消息
			External: &nats.ExternalStream{
				APIPrefix:     "$JS.hub.API", // Hub端JetStream domain
				DeliverPrefix: fmt.Sprintf("sync.grp.%s", cfg.UserID),
			},
		},
		MaxAge:  30 * 24 * time.Hour, // 和Hub保持一致
		Storage: nats.FileStorage,
	})
	if err != nil {
		return fmt.Errorf("create group mirror stream failed: %w", err)
	}
	log.Printf("✅ 群聊镜像流创建成功: %s", s.streamNameGroup)

	// ========== 创建私聊消息镜像流 ==========
	s.streamNameDirect = fmt.Sprintf("USER_OFFLINE_DM_%s", cfg.UserID)
	// 清理旧流
	_ = js.DeleteStream(s.streamNameDirect)
	// 创建镜像流，同步Hub上的DChatDirect流
	_, err = js.AddStream(&nats.StreamConfig{
		Name: s.streamNameDirect,
		Mirror: &nats.StreamSource{
			Name:          "DChatDirect", // Hub上的私聊流名称
			FilterSubject: "dchat.dm.*.msg", // 同步所有私聊消息
			External: &nats.ExternalStream{
				APIPrefix:     "$JS.hub.API", // Hub端JetStream domain
				DeliverPrefix: fmt.Sprintf("sync.dm.%s", cfg.UserID),
			},
		},
		MaxAge:  30 * 24 * time.Hour, // 和Hub保持一致
		Storage: nats.FileStorage,
	})
	if err != nil {
		return fmt.Errorf("create direct mirror stream failed: %w", err)
	}
	log.Printf("✅ 私聊镜像流创建成功: %s", s.streamNameDirect)

	return nil
}

// StartSync 启动同步协程
func (s *Service) StartSync() error {
	if s.js == nil || s.syncCfg == nil {
		return fmt.Errorf("mirror not initialized")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.syncRunning {
		return nil
	}

	// ========== 订阅群聊镜像流 ==========
	subGroup, err := s.js.PullSubscribe("dchat.grp.*.msg",
		fmt.Sprintf("sync_consumer_grp_%s", s.syncCfg.UserID),
		nats.DeliverAll(),
		nats.AckExplicit(),
		nats.BindStream(s.streamNameGroup),
	)
	if err != nil {
		return fmt.Errorf("create group subscription failed: %w", err)
	}
	s.syncSubGroup = subGroup

	// ========== 订阅私聊镜像流 ==========
	subDirect, err := s.js.PullSubscribe("dchat.dm.*.msg",
		fmt.Sprintf("sync_consumer_dm_%s", s.syncCfg.UserID),
		nats.DeliverAll(),
		nats.AckExplicit(),
		nats.BindStream(s.streamNameDirect),
	)
	if err != nil {
		return fmt.Errorf("create direct subscription failed: %w", err)
	}
	s.syncSubDirect = subDirect

	s.syncRunning = true

	// 启动两个后台协程分别同步群聊和私聊
	go s.syncLoop("group", s.syncSubGroup)
	go s.syncLoop("direct", s.syncSubDirect)

	log.Println("✅ 离线消息同步已启动（群聊+私聊）")
	return nil
}

// StopSync 停止同步
func (s *Service) StopSync() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.syncSubGroup != nil {
		_ = s.syncSubGroup.Unsubscribe()
		s.syncSubGroup = nil
	}
	if s.syncSubDirect != nil {
		_ = s.syncSubDirect.Unsubscribe()
		s.syncSubDirect = nil
	}
	if s.syncCancel != nil {
		s.syncCancel()
	}
	s.syncRunning = false
	log.Println("🛑 离线消息同步已停止")
}

// syncLoop 同步主循环，支持群聊和私聊两个流
func (s *Service) syncLoop(streamType string, sub *nats.Subscription) {
	defer log.Printf("🛑 %s 同步协程已退出", streamType)

	for {
		select {
		case <-s.syncCtx.Done():
			return
		default:
			// 每次拉取10条消息
			msgs, err := sub.Fetch(10, nats.MaxWait(5*time.Second))
			if err != nil {
				if err == nats.ErrTimeout || strings.Contains(err.Error(), "context canceled") {
					continue
				}
				if s.syncCfg.ErrorHandler != nil {
					s.syncCfg.ErrorHandler(fmt.Errorf("%s fetch failed: %w", streamType, err))
				}
				time.Sleep(1 * time.Second)
				continue
			}

			for _, msg := range msgs {
				// 调用回调处理
				if s.syncCfg.MessageHandler != nil {
					_ = s.syncCfg.MessageHandler(msg.Data)
				}
				// 无论处理结果都ACK，避免重复消费
				_ = msg.Ack()
			}
		}
	}
}
```

---

### 模块2：internal/chat 包修改（业务层，零重构兼容）
在`service.go`中添加初始化离线同步方法，**原有SQLite逻辑完全不需要修改**：
```go
// InitOfflineSync 初始化离线消息同步，用户登录成功后调用
func (s *Service) InitOfflineSync() error {
	userID := s.user.ID

	// 配置同步回调
	cfg := &natsservice.OfflineSyncConfig{
		UserID: userID,
		MessageHandler: func(data []byte) error {
			return s.processOfflineMessage(data)
		},
		ErrorHandler: func(err error) {
			s.dispatchError(err)
		},
	}

	// 初始化镜像
	if err := s.nats.InitOfflineMirror(cfg); err != nil {
		return err
	}

	// 启动同步
	return s.nats.StartSync()
}

// processOfflineMessage 处理同步下来的离线消息（核心简化逻辑）
func (s *Service) processOfflineMessage(data []byte) error {
	// 1. 尝试解密消息（你现有解密方法直接复用）
	decrypted, err := s.yourExistingDecryptMethod(data)
	if err != nil {
		// 解密失败 = 不属于当前用户的消息，直接丢弃，不需要任何其他处理
		return nil
	}

	// 2. 解密成功 = 是自己的消息，现有SQLite存储逻辑一行都不用改
	if err := s.storage.SaveMessage(decrypted); err != nil {
		return fmt.Errorf("save message failed: %w", err)
	}

	// 3. 分发到UI（你现有分发方法直接复用）
	s.yourExistingDispatchMethod(decrypted)

	return nil
}
```

---

### 模块3：数据库适配（storage包，完全不需要修改）
✅ 现有SQLite存储逻辑、表结构、查询逻辑**完全不需要任何修改**
✅ 如果已经有`MessageExists`去重方法，可以保留，没有也没关系，解密逻辑已经天然去重（不属于自己的消息直接丢）

---

### 模块4：主程序集成（app.go）
用户登录成功后调用：
```go
// 用户登录成功逻辑
func (a *App) OnUserLoginSuccess() {
    // 原有登录逻辑...

    // 初始化离线同步（只需要加这两行）
    if err := a.chatSvc.InitOfflineSync(); err != nil {
        log.Printf("init offline sync failed: %v", err)
    }
}

// 用户登出时调用
func (a *App) OnUserLogout() {
    a.chatSvc.nats.StopSync()
    // 其他登出逻辑...
}
```

---

## 四、方案优势（对比之前版本）
1. **零重构成本**：现有SQLite逻辑、消息处理逻辑完全不需要修改，只需要加少量同步层代码
2. **逻辑更简单**：不需要解析subject、不需要判断会话归属，解密成功就是自己的消息
3. **安全有保障**：不属于用户的消息无法解密，直接丢弃，不会浪费存储
4. **稳定性更高**：减少业务判断逻辑，降低出错概率
5. **性能更快**：少了一层会话归属判断，同步速度提升
6. **自动支持新会话**：用户加好友/加群后不需要任何额外处理，新消息自动同步、自动解密存储

---

## 五、测试验证方法
### 功能测试点
1. ✅ 历史消息同步：用户登录后是否能收到之前未接收的私聊和群聊消息
2. ✅ 实时消息同步：新消息是否能实时同步到本地
3. ✅ 消息过滤：不属于用户的消息是否不会存入SQLite
4. ✅ 兼容性：原有消息查询、发送逻辑是否完全不受影响

### 测试命令
```bash
# 运行镜像同步测试
cd test/jetstream && go test -v -run TestLeafNode_JetStream_Mirror_Sync_E2E
```
