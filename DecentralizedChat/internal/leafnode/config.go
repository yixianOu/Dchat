package leafnode

import "time"

// Config LeafNode 配置
type Config struct {
	// 本地监听地址
	LocalHost string
	LocalPort int

	// 公网 Hub 地址列表（支持多个）
	// 按优先级排序，先尝试第一个，失败则尝试第二个
	HubURLs []string

	// 认证
	CredsFile string

	// TLS
	EnableTLS bool

	// JetStream
	EnableJetStream   bool
	JetStreamStoreDir string

	// 连接超时
	ConnectTimeout time.Duration
}

// DefaultConfig 默认配置
func DefaultConfig() *Config {
	return &Config{
		LocalHost: "127.0.0.1",
		LocalPort: 4222,
		HubURLs: []string{
			"nats://hub1.dchat.example.com:7422",
			"nats://hub2.dchat.example.com:7422",
		},
		EnableTLS:         false,
		EnableJetStream:   true,
		JetStreamStoreDir: "",
		ConnectTimeout:    10 * time.Second,
	}
}
