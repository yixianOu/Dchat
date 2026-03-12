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
	UserID         string                // 当前用户ID
	MessageHandler func(*nats.Msg) error // 消息处理回调，支持获取NATS元数据
	ErrorHandler   func(error)           // 错误处理回调
}

// InitOfflineMirror 初始化离线同步（直接跨domain消费Hub上的流，无需本地镜像）
func (s *Service) InitOfflineMirror(cfg *OfflineSyncConfig) error {
	if s.conn == nil || !s.conn.IsConnected() {
		return fmt.Errorf("nats not connected")
	}

	// 初始化JetStream上下文，指定Hub的domain前缀
	js, err := s.conn.JetStream(nats.Domain("hub"))
	if err != nil {
		return fmt.Errorf("jetstream init failed: %w", err)
	}
	s.js = js
	s.syncCfg = cfg

	slog.Info("✅ 离线同步初始化成功，直接消费Hub domain流")
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

	// ========== 订阅Hub上的群聊流 ==========
	subGroup, err := s.js.PullSubscribe("dchat.grp.*.msg",
		fmt.Sprintf("sync_consumer_grp_%s", s.syncCfg.UserID),
		nats.DeliverAll(),
		nats.AckExplicit(),
		nats.BindStream("DChatGroups"),
	)
	if err != nil {
		return fmt.Errorf("create group subscription failed: %w", err)
	}
	s.syncSubGroup = subGroup

	// ========== 订阅Hub上的私聊流 ==========
	subDirect, err := s.js.PullSubscribe("dchat.dm.*.msg",
		fmt.Sprintf("sync_consumer_dm_%s", s.syncCfg.UserID),
		nats.DeliverAll(),
		nats.AckExplicit(),
		nats.BindStream("DChatDirect"),
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
				slog.Error("拉取消息失败", "type", streamType, "error", err)
				if s.syncCfg.ErrorHandler != nil {
					s.syncCfg.ErrorHandler(fmt.Errorf("%s fetch failed: %w", streamType, err))
				}
				time.Sleep(1 * time.Second)
				continue
			}

			if len(msgs) > 0 {
				slog.Info("拉取到消息", "type", streamType, "count", len(msgs))
			}

			for _, msg := range msgs {
				// 调用回调处理
				if s.syncCfg.MessageHandler != nil {
					err := s.syncCfg.MessageHandler(msg)
					if err != nil {
						slog.Error("处理离线消息失败", "error", err, "subject", msg.Subject)
					}
				}
				// 无论处理结果都ACK，避免重复消费
				_ = msg.Ack()
			}
		}
	}
}
