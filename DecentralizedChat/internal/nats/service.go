package nats

import (
	"fmt"
	"log"
	"time"

	"github.com/nats-io/nats.go"
)

type Service struct {
	conn        *nats.Conn
	localIP     string
	clusterPort int
	clientPort  int
}

type ConnectionConfig struct {
	LocalIP     string
	ClientPort  int
	ClusterPort int
	Routes      []string
}

func NewService(config ConnectionConfig) (*Service, error) {
	svc := &Service{
		localIP:     config.LocalIP,
		clusterPort: config.ClusterPort,
		clientPort:  config.ClientPort,
	}

	// 启动内嵌的NATS服务器
	if err := svc.startNATSServer(config); err != nil {
		return nil, fmt.Errorf("failed to start NATS server: %w", err)
	}

	// 连接到本地NATS服务器
	if err := svc.connect(); err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	return svc, nil
}

func (s *Service) startNATSServer(config ConnectionConfig) error {
	// TODO: 使用NATS server嵌入式启动
	// 这里需要集成NATS server的嵌入式模式
	log.Printf("Starting NATS server on %s:%d (cluster: %d)", config.LocalIP, config.ClientPort, config.ClusterPort)

	// 临时实现：假设外部NATS服务器已启动
	time.Sleep(1 * time.Second)
	return nil
}

func (s *Service) connect() error {
	url := fmt.Sprintf("nats://%s:%d", s.localIP, s.clientPort)

	var err error
	s.conn, err = nats.Connect(url,
		nats.ReconnectWait(time.Second),
		nats.MaxReconnects(-1),
		nats.DisconnectErrHandler(func(nc *nats.Conn, err error) {
			log.Printf("NATS disconnected: %v", err)
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			log.Printf("NATS reconnected to %s", nc.ConnectedUrl())
		}),
	)

	if err != nil {
		return fmt.Errorf("failed to connect to NATS: %w", err)
	}

	log.Printf("Connected to NATS server at %s", url)
	return nil
}

func (s *Service) Subscribe(subject string, handler func(msg *nats.Msg)) error {
	_, err := s.conn.Subscribe(subject, handler)
	if err != nil {
		return fmt.Errorf("failed to subscribe to %s: %w", subject, err)
	}
	log.Printf("Subscribed to subject: %s", subject)
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
	err := s.conn.Publish(subject, nil) // TODO: 实现JSON序列化
	if err != nil {
		return fmt.Errorf("failed to publish JSON to %s: %w", subject, err)
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
