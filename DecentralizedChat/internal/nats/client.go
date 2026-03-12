// Package nats 实现了 NATS 客户端服务，封装了连接管理、消息发布订阅以及 JetStream KV 存储等功能
package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
)

type Service struct {
	conn *nats.Conn
	mu   sync.RWMutex

	// ========== 离线消息同步相关字段 ==========
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

type ClientConfig struct {
	URL           string        // e.g. nats://localhost:4222
	User          string        // Optional legacy username (not recommended)
	Password      string        // Optional legacy password (not recommended)
	Token         string        // Optional auth token
	CredsFile     string        // Path to NSC generated .creds file (preferred)
	Name          string        // Client name
	Timeout       time.Duration // Connect timeout
	MaxReconnect  int           // Max reconnect attempts (-1 infinite)
	ReconnectWait time.Duration // Wait between reconnect attempts
}

// NewService creates a NATS client service with auth support
func NewService(cfg ClientConfig) (*Service, error) {
	var opts []nats.Option

	// Auth priority: creds -> token -> user/pass
	if cfg.CredsFile != "" {
		opts = append(opts, nats.UserCredentials(cfg.CredsFile))
	} else if cfg.Token != "" {
		opts = append(opts, nats.Token(cfg.Token))
	} else if cfg.User != "" && cfg.Password != "" {
		opts = append(opts, nats.UserInfo(cfg.User, cfg.Password))
	}

	// Client name option
	if cfg.Name != "" {
		opts = append(opts, nats.Name(cfg.Name))
	}

	// Timeout option
	if cfg.Timeout > 0 {
		opts = append(opts, nats.Timeout(cfg.Timeout))
	} else {
		opts = append(opts, nats.Timeout(5*time.Second))
	}

	// Reconnect settings
	maxReconnect := cfg.MaxReconnect
	if maxReconnect == 0 {
		maxReconnect = -1 // default infinite
	}
	opts = append(opts, nats.MaxReconnects(maxReconnect))

	reconnectWait := cfg.ReconnectWait
	if reconnectWait == 0 {
		reconnectWait = time.Second
	}
	opts = append(opts, nats.ReconnectWait(reconnectWait))

	// Connection event handlers
	opts = append(opts,
		nats.DisconnectErrHandler(func(nc *nats.Conn, err error) {
			if err != nil {
				slog.Error("NATS disconnected", "error", err)
			}
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			slog.Info("NATS reconnected", "url", nc.ConnectedUrl())
		}),
	)

	nc, err := nats.Connect(cfg.URL, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	// 初始化同步上下文
	syncCtx, syncCancel := context.WithCancel(context.Background())

	return &Service{
		conn: nc,
		syncCtx: syncCtx,
		syncCancel: syncCancel,
	}, nil
}

func (s *Service) Subscribe(subject string, handler func(msg *nats.Msg)) error {
	_, err := s.conn.Subscribe(subject, handler)
	return err
}

func (s *Service) Publish(subject string, data []byte) error {
	return s.conn.Publish(subject, data)
}

func (s *Service) PublishJSON(subject string, data interface{}) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	err = s.conn.Publish(subject, jsonData)
	if err != nil {
		return fmt.Errorf("failed to publish JSON to %s: %w", subject, err)
	}
	return nil
}

// RequestJSON sends a JSON request and waits for a reply
func (s *Service) RequestJSON(subject string, data interface{}, timeout time.Duration) (*nats.Msg, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal JSON: %w", err)
	}

	msg, err := s.conn.Request(subject, jsonData, timeout)
	if err != nil {
		return nil, fmt.Errorf("failed to request %s: %w", subject, err)
	}
	return msg, nil
}

// SubscribeJSON subscribes and forwards raw JSON payload to handler
func (s *Service) SubscribeJSON(subject string, handler func(data []byte) error) error {
	_, err := s.conn.Subscribe(subject, func(msg *nats.Msg) {
		if err := handler(msg.Data); err != nil {
			slog.Error("failed to process JSON message", "error", err, "subject", subject)
		}
	})
	if err != nil {
		return fmt.Errorf("failed to subscribe to %s: %w", subject, err)
	}
	return nil
}

func (s *Service) Close() error {
	if s.conn != nil {
		s.conn.Close()
	}
	return nil
}

func (s *Service) IsConnected() bool {
	return s.conn != nil && s.conn.IsConnected()
}

func (s *Service) GetStats() map[string]interface{} {
	if s.conn == nil {
		return map[string]interface{}{
			"connected": false,
		}
	}
	stats := s.conn.Stats()
	return map[string]interface{}{
		"connected":    s.conn.IsConnected(),
		"reconnects":   stats.Reconnects,
		"bytes_in":     stats.InBytes,
		"bytes_out":    stats.OutBytes,
		"messages_in":  stats.InMsgs,
		"messages_out": stats.OutMsgs,
	}
}

