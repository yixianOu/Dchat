// Package leafnode 实现了 LeafNode 管理器，负责启动和管理本地 NATS Server 作为 LeafNode，并连接到远程 Hub
package leafnode

import (
	"fmt"
	"net/url"
	"sync"

	"DecentralizedChat/internal/config"

	"github.com/nats-io/nats-server/v2/server"
)

// Manager LeafNode 管理器
type Manager struct {
	config        *config.LeafNodeConfig
	server        *server.Server
	mu            sync.RWMutex
	listenPort    int // 实际监听的端口

	// 连接状态
	connectedHubCount int
	lastError         error
}

// NewManager 创建管理器
func NewManager(cfg *config.LeafNodeConfig) *Manager {
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

	// 保存实际监听的端口
	m.listenPort = opts.Port
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
	m.mu.RLock()
	defer m.mu.RUnlock()

	host := m.config.LocalHost
	if host == "" {
		host = "127.0.0.1"
	}

	port := m.config.LocalPort

	// 如果保存了实际监听端口，使用它
	if m.listenPort != 0 {
		port = m.listenPort
	}

	return fmt.Sprintf("nats://%s:%d", host, port)
}

// GetConfig 获取配置（只读）
func (m *Manager) GetConfig() config.LeafNodeConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return *m.config
}

// 内部方法

func (m *Manager) parseHubURLs() ([]*server.RemoteLeafOpts, error) {
	var remotes []*server.RemoteLeafOpts

	for _, hubURL := range m.config.HubURLs {
		u, err := url.Parse(hubURL)
		if err != nil {
			continue // 跳过无效 URL
		}

		remoteOpts := &server.RemoteLeafOpts{
			URLs: []*url.URL{u},
		}

		// 如果配置了 CredsFile，添加到远程连接配置
		if m.config.CredsFile != "" {
			remoteOpts.Credentials = m.config.CredsFile
		}

		remotes = append(remotes, remoteOpts)
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

	// 如果配置了开启JetStream
	if m.config.EnableJetStream {
		opts.JetStream = true
		opts.JetStreamMaxMemory = 256 * 1024 * 1024 // 256MB内存存储限制
		opts.JetStreamMaxStore = 1 * 1024 * 1024 * 1024 // 1GB磁盘存储限制
		if m.config.JetStreamStoreDir != "" {
			opts.StoreDir = m.config.JetStreamStoreDir
		}
	}

	return opts
}
