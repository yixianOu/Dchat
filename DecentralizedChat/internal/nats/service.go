package nats

import (
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
)

type Service struct {
	conn *nats.Conn
}

type ClientConfig struct {
	URL      string // 例如 nats://127.0.0.1:4222
	User     string // 可选
	Password string // 可选
	Token    string // 可选
	Name     string // 客户端名称
}

// 新建NATS客户端服务，支持鉴权
func NewService(cfg ClientConfig) (*Service, error) {
	var opts []nats.Option
	if cfg.User != "" && cfg.Password != "" {
		opts = append(opts, nats.UserInfo(cfg.User, cfg.Password))
	}
	if cfg.Token != "" {
		opts = append(opts, nats.Token(cfg.Token))
	}
	if cfg.Name != "" {
		opts = append(opts, nats.Name(cfg.Name))
	}
	opts = append(opts,
		nats.ReconnectWait(time.Second),
		nats.MaxReconnects(-1),
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
	// TODO: 实现JSON序列化
	return fmt.Errorf("PublishJSON未实现")
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
