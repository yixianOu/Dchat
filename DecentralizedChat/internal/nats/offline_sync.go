package nats

import (
	"fmt"
	"log/slog"
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
	slog.Info("✅ 群聊镜像流创建成功", "stream", s.streamNameGroup)

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
	slog.Info("✅ 私聊镜像流创建成功", "stream", s.streamNameDirect)

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

	slog.Info("✅ 离线消息同步已启动（群聊+私聊）")
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
	slog.Info("🛑 离线消息同步已停止")
}

// syncLoop 同步主循环，支持群聊和私聊两个流
func (s *Service) syncLoop(streamType string, sub *nats.Subscription) {
	defer slog.Info("🛑 同步协程已退出", "type", streamType)

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
