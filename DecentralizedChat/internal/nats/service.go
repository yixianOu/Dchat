package nats

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
)

type Service struct {
	conn *nats.Conn
	// lazy-initialized JetStream + KV buckets
	js nats.JetStreamContext
}

type ClientConfig struct {
	URL           string        // e.g. nats://127.0.0.1:4222
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
				fmt.Printf("NATS disconnected: %v\n", err)
			}
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			fmt.Printf("NATS reconnected to: %s\n", nc.ConnectedUrl())
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
			fmt.Printf("failed to process JSON message: %v\n", err)
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

// --- JetStream KV for keys ---

const (
	kvBucketFriends = "dchat_friends" // 存储好友公钥  key: user_pub_key  val: JSON{"pub":"..."}
	kvBucketGroups  = "dchat_groups"  // 存储群对称密钥  key: group_id val: JSON{"sym":"base64"}
)

// ensureJS 获取 JetStream 上下文
func (s *Service) ensureJS() (nats.JetStreamContext, error) {
	if s.js != nil {
		return s.js, nil
	}
	js, err := s.conn.JetStream()
	if err != nil {
		return nil, err
	}
	s.js = js
	return js, nil
}

// ensureBucket 确保 KV 桶存在
func (s *Service) ensureBucket(name string) (nats.KeyValue, error) {
	js, err := s.ensureJS()
	if err != nil {
		return nil, err
	}
	kv, err := js.KeyValue(name)
	if err == nil {
		return kv, nil
	}
	// 尝试创建
	return js.CreateKeyValue(&nats.KeyValueConfig{Bucket: name})
}

// PutFriendPubKey 存储好友公钥（幂等）
func (s *Service) PutFriendPubKey(pubKey, raw string) error {
	if pubKey == "" || raw == "" {
		return fmt.Errorf("empty pubKey/raw")
	}
	kv, err := s.ensureBucket(kvBucketFriends)
	if err != nil {
		return err
	}
	// 直接覆盖即可
	val, _ := json.Marshal(map[string]string{"pub": raw})
	_, err = kv.Put(pubKey, val)
	return err
}

// GetFriendPubKey 读取好友公钥
func (s *Service) GetFriendPubKey(pubKey string) (string, error) {
	if pubKey == "" {
		return "", fmt.Errorf("empty key")
	}
	kv, err := s.ensureBucket(kvBucketFriends)
	if err != nil {
		return "", err
	}
	e, err := kv.Get(pubKey)
	if err != nil {
		return "", err
	}
	var obj map[string]string
	if json.Unmarshal(e.Value(), &obj) == nil {
		if v, ok := obj["pub"]; ok {
			return v, nil
		}
	}
	return "", fmt.Errorf("pub key not found")
}

// PutGroupSymKey 存储群对称密钥（覆盖）
func (s *Service) PutGroupSymKey(groupID, keyB64 string) error {
	if groupID == "" || keyB64 == "" {
		return fmt.Errorf("empty group/key")
	}
	kv, err := s.ensureBucket(kvBucketGroups)
	if err != nil {
		return err
	}
	val, _ := json.Marshal(map[string]string{"sym": keyB64})
	_, err = kv.Put(groupID, val)
	return err
}

// GetGroupSymKey 读取群对称密钥
func (s *Service) GetGroupSymKey(groupID string) (string, error) {
	if groupID == "" {
		return "", fmt.Errorf("empty group id")
	}
	kv, err := s.ensureBucket(kvBucketGroups)
	if err != nil {
		return "", err
	}
	e, err := kv.Get(groupID)
	if err != nil {
		return "", err
	}
	var obj map[string]string
	if json.Unmarshal(e.Value(), &obj) == nil {
		if v, ok := obj["sym"]; ok {
			return v, nil
		}
	}
	return "", fmt.Errorf("group key not found")
}
