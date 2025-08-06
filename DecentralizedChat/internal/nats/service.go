package nats

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
)

type Service struct {
	conn *nats.Conn
}

type ClientConfig struct {
	URL           string        // 例如 nats://127.0.0.1:4222
	User          string        // 可选
	Password      string        // 可选
	Token         string        // 可选
	Name          string        // 客户端名称
	Timeout       time.Duration // 连接超时
	MaxReconnect  int           // 最大重连次数，-1为无限重连
	ReconnectWait time.Duration // 重连等待时间
}

// 新建NATS客户端服务，支持鉴权
func NewService(cfg ClientConfig) (*Service, error) {
	var opts []nats.Option

	// 鉴权选项
	if cfg.User != "" && cfg.Password != "" {
		opts = append(opts, nats.UserInfo(cfg.User, cfg.Password))
	}
	if cfg.Token != "" {
		opts = append(opts, nats.Token(cfg.Token))
	}

	// 客户端名称
	if cfg.Name != "" {
		opts = append(opts, nats.Name(cfg.Name))
	}

	// 连接超时
	if cfg.Timeout > 0 {
		opts = append(opts, nats.Timeout(cfg.Timeout))
	} else {
		opts = append(opts, nats.Timeout(5*time.Second))
	}

	// 重连配置
	maxReconnect := cfg.MaxReconnect
	if maxReconnect == 0 {
		maxReconnect = -1 // 默认无限重连
	}
	opts = append(opts, nats.MaxReconnects(maxReconnect))

	reconnectWait := cfg.ReconnectWait
	if reconnectWait == 0 {
		reconnectWait = time.Second
	}
	opts = append(opts, nats.ReconnectWait(reconnectWait))

	// 连接事件处理
	opts = append(opts,
		nats.DisconnectErrHandler(func(nc *nats.Conn, err error) {
			if err != nil {
				fmt.Printf("NATS连接断开: %v\n", err)
			}
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			fmt.Printf("NATS重新连接到: %s\n", nc.ConnectedUrl())
		}),
	)

	nc, err := nats.Connect(cfg.URL, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	return &Service{conn: nc}, nil
}

func (s *Service) Subscribe(subject string, handler func(msg *nats.Msg)) error {
	_, err := s.conn.Subscribe(subject, handler)
	if err != nil {
		return fmt.Errorf("failed to subscribe to %s: %w", subject, err)
	}
	return nil
}

func (s *Service) Publish(subject string, data []byte) error {
	err := s.conn.Publish(subject, data)
	if err != nil {
		return fmt.Errorf("failed to publish to %s: %w", subject, err)
	}
	return nil
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

// RequestJSON 发送JSON请求并等待响应
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

// SubscribeJSON 订阅JSON消息
func (s *Service) SubscribeJSON(subject string, handler func(data []byte) error) error {
	_, err := s.conn.Subscribe(subject, func(msg *nats.Msg) {
		if err := handler(msg.Data); err != nil {
			fmt.Printf("处理JSON消息失败: %v\n", err)
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
