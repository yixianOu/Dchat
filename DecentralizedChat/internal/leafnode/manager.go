package leafnode

import (
	"fmt"
	"net/url"
	"sync"

	"github.com/nats-io/nats-server/v2/server"
)

// Manager LeafNode 管理器
type Manager struct {
	config *Config
	server *server.Server
	mu     sync.RWMutex

	// 连接状态
	connectedHubCount int
	lastError         error
}

// NewManager 创建管理器
func NewManager(cfg *Config) *Manager {
	return &Manager{
		config: cfg,
	}
}

// Start 启动 LeafNode
func (m *Manager) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.server != nil {
		return fmt.Errorf("leafnode already started")
	}

	// 1. 解析 Hub URLs
	remotes, err := m.parseHubURLs()
	if err != nil {
		return err
	}

	if len(remotes) == 0 {
		return fmt.Errorf("no valid hub URLs configured")
	}

	// 2. 配置 NATS Server
	opts := m.buildServerOptions(remotes)

	// 3. 创建并启动服务器
	srv, err := server.NewServer(opts)
	if err != nil {
		return err
	}

	go srv.Start()

	// 4. 等待就绪
	if !srv.ReadyForConnections(m.config.ConnectTimeout) {
		return fmt.Errorf("leafnode failed to start within timeout")
	}

	// 5. TODO: 验证至少连接了一个 Hub
	// 需要监控 outbound LeafNode 连接

	m.server = srv
	return nil
}

// Stop 停止 LeafNode
func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.server != nil {
		m.server.Shutdown()
		m.server = nil
	}
}

// IsRunning 是否正在运行
func (m *Manager) IsRunning() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.server != nil && m.server.Running()
}

// GetLocalNATSURL 获取本地 NATS 连接地址
func (m *Manager) GetLocalNATSURL() string {
	return fmt.Sprintf("nats://%s:%d", m.config.LocalHost, m.config.LocalPort)
}

// GetConnectedHubCount 获取已连接的 Hub 数量
func (m *Manager) GetConnectedHubCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.server == nil {
		return 0
	}

	// TODO: 需要获取 outbound LeafNode 连接数
	// NumLeafNodes() 返回的是 inbound 连接
	return m.server.NumLeafNodes()
}

// 内部方法

func (m *Manager) parseHubURLs() ([]*server.RemoteLeafOpts, error) {
	var remotes []*server.RemoteLeafOpts

	for _, hubURL := range m.config.HubURLs {
		u, err := url.Parse(hubURL)
		if err != nil {
			continue // 跳过无效 URL
		}
		remotes = append(remotes, &server.RemoteLeafOpts{
			URLs: []*url.URL{u},
		})
	}

	return remotes, nil
}

func (m *Manager) buildServerOptions(remotes []*server.RemoteLeafOpts) *server.Options {
	opts := &server.Options{
		Host: m.config.LocalHost,
		Port: m.config.LocalPort,
		LeafNode: server.LeafNodeOpts{
			Host:    m.config.LocalHost,
			Port:    -1, // 不接受 incoming LeafNode 连接
			Remotes: remotes,
		},
		NoLog:  true,
		NoSigs: true,
	}

	// 启用 JetStream
	if m.config.EnableJetStream {
		opts.JetStream = true
		opts.JetStreamMaxMemory = 256 * 1024 * 1024 // 256MB
		opts.JetStreamMaxStore = 1 * 1024 * 1024 * 1024 // 1GB
		if m.config.JetStreamStoreDir != "" {
			opts.StoreDir = m.config.JetStreamStoreDir
		}
	}

	return opts
}
