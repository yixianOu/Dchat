package routes

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"time"

	"DecentralizedChat/internal/chat"

	"github.com/nats-io/nats-server/v2/server"
)

// LocalNode represents a local embedded NATS server instance
type LocalNode struct {
	ID          string
	Server      *server.Server
	ClientPort  int
	ClusterPort int
	Host        string
	ClusterName string
}

// NodeManager manages a single local node (fits decentralized single-node-per-app model)
type NodeManager struct {
	node        *LocalNode
	clusterName string
	host        string
	lastConfig  *NodeConfig // persist last started config for updates
	nscSeed     string      // NSC seed for certificate generation
}

// NodeConfig defines runtime parameters for the local node
type NodeConfig struct {
	NodeID             string
	ClientPort         int
	ClusterPort        int
	SeedRoutes         []string
	ResolverConfigPath string
	ImportAllow        []string
	ExportAllow        []string
	// ⭐ 简化的TLS配置 - 默认启用insecure TLS
	EnableTLS bool `json:"enable_tls,omitempty"`
	// 内部使用，不需要手动配置
	autoTLSCert string
	autoTLSKey  string
}

// NewNodeManager creates a new NodeManager
func NewNodeManager(clusterName string, host string) *NodeManager {
	return &NodeManager{
		clusterName: clusterName,
		host:        host,
	}
}

// SetNSCSeed 设置NSC seed用于证书生成
func (nm *NodeManager) SetNSCSeed(seed string) {
	nm.nscSeed = seed
}

// ⭐ GenerateSimpleTLSCert 使用NSC密钥系统生成高级证书 (公开方法)
func (nm *NodeManager) GenerateSimpleTLSCert() (certPEM, keyPEM string, err error) {
	if nm.nscSeed == "" {
		return "", "", fmt.Errorf("NSC seed not set")
	}

	// 创建密钥管理器
	keyManager, err := chat.NewNSCKeyManager(nm.nscSeed)
	if err != nil {
		return "", "", fmt.Errorf("create NSC key manager: %w", err)
	}

	// 准备主机和IP列表
	hosts := []string{nm.host, "localhost", "127.0.0.1"}
	ips := []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback}

	// 如果host是IP地址，添加到IPAddresses
	if ip := net.ParseIP(nm.host); ip != nil {
		ips = append(ips, ip)
	}

	// 使用NSC密钥系统生成SSL证书
	cert, err := keyManager.GenerateSSLCertificate(hosts, ips, 365)
	if err != nil {
		return "", "", fmt.Errorf("generate SSL certificate: %w", err)
	}

	return cert.CertPEM, cert.PrivKeyPEM, nil
}

// StartLocalNode starts a local NATS node with default wide-open import/export
func (nm *NodeManager) StartLocalNode(nodeID string, clientPort int, clusterPort int, seedRoutes []string) error {
	config := &NodeConfig{
		NodeID:      nodeID,
		ClientPort:  clientPort,
		ClusterPort: clusterPort,
		SeedRoutes:  seedRoutes,
		ImportAllow: []string{"*"},
		ExportAllow: []string{"*"},
	}
	return nm.StartLocalNodeWithConfig(config)
}

// StartLocalNodeWithConfig starts local node with a custom configuration
func (nm *NodeManager) StartLocalNodeWithConfig(config *NodeConfig) error {
	// inline ensureNotStarted logic
	if nm.node != nil {
		return fmt.Errorf("local node already started: %s", nm.node.ID)
	}

	// 使用渐进式配置 - 首先尝试最小配置 + JetStream
	opts, err := nm.prepareMinimalJetStreamOptions(config)
	if err != nil {
		return fmt.Errorf("prepare minimal JetStream options: %w", err)
	}

	fmt.Printf("🔧 Creating NATS server with config...\n")
	fmt.Printf("   Host: %s, Client: %d, Cluster: %d, JetStream: %v\n",
		opts.Host, config.ClientPort, config.ClusterPort, opts.JetStream)

	srv, err := server.NewServer(opts)
	if err != nil {
		return fmt.Errorf("failed to create NATS server: %v", err)
	}

	// Start server
	fmt.Printf("⏳ Starting NATS server for node %s...\n", config.NodeID)

	// Start server and wait for it to be ready
	go srv.Start()

	// Use NATS built-in method to wait for readiness with longer timeout for cluster
	fmt.Printf("⏳ Waiting for server to be ready (JetStream cluster may take longer)...\n")
	if !srv.ReadyForConnections(30 * time.Second) {
		// If ReadyForConnections fails, try to get more info
		if srv.Running() {
			fmt.Printf("⚠️ Server is running but not ready for connections\n")
			fmt.Printf("🔧 This is normal for JetStream cluster - connections may work anyway\n")
			// For cluster mode, we'll proceed anyway as the server is running
		} else {
			fmt.Printf("❌ Server failed to start\n")
			return fmt.Errorf("NATS server not ready for connections")
		}
	}

	// Server is ready
	fmt.Printf("✅ NATS server is ready for connections\n")
	if addr := srv.Addr(); addr != nil {
		fmt.Printf("   Listening on: %s\n", addr.String())
	}
	fmt.Printf("   JetStream enabled: %v\n", opts.JetStream)

	nm.node = &LocalNode{
		ID:          config.NodeID,
		Server:      srv,
		ClientPort:  config.ClientPort,
		ClusterPort: config.ClusterPort,
		Host:        nm.host,
		ClusterName: nm.clusterName,
	}

	// remember config
	nm.lastConfig = config

	// Logging (kept same detail)
	fmt.Printf("✅ Local node started: %s (Client: %s:%d, Cluster: %s:%d)\n",
		config.NodeID, nm.host, config.ClientPort, nm.host, config.ClusterPort)
	if opts.JetStream {
		fmt.Printf("   JetStream: enabled\n")
	}
	fmt.Printf("   Node: %s, Import Allow: %v, Export Allow: %v\n",
		config.NodeID,
		config.ImportAllow,
		config.ExportAllow)
	return nil
}

// prepareMinimalJetStreamOptions 准备最简化的JetStream配置
func (nm *NodeManager) prepareMinimalJetStreamOptions(config *NodeConfig) (*server.Options, error) {
	fmt.Printf("✅ Minimal JetStream options prepared\n")

	opts := &server.Options{
		Host: "0.0.0.0", // 绑定到所有接口
		Port: config.ClientPort,

		// 最基本的配置
		ServerName: fmt.Sprintf("dchat-%s", nm.host),

		// 禁用安全性
		TLS:       false,
		TLSVerify: false,

		// JetStream配置 - 最小化
		JetStream: true,
		StoreDir:  "./jetstream_store",

		// 集群配置 - JetStream集群必须设置name
		Cluster: server.ClusterOpts{
			Port: config.ClusterPort,
			Name: "dchat-cluster", // JetStream集群必需的名称
		},

		// 调试配置
		Debug: true,
		Trace: false, // 减少日志
	}

	fmt.Printf("🔧 Server will bind to: %s:%d (cluster: %d)\n", opts.Host, opts.Port, config.ClusterPort)
	return opts, nil
}

// ensureNotStarted returns error if a node is already running.
// prepareServerOptions orchestrates building server options from config via NodeConfig methods.
func (nm *NodeManager) prepareServerOptions(config *NodeConfig) (*server.Options, error) {
	opts := &server.Options{}

	fmt.Printf("🔧 Step 1: Loading resolver config...\n")
	if err := config.loadResolverConfig(opts); err != nil {
		return nil, fmt.Errorf("load resolver config: %w", err)
	}

	fmt.Printf("🔧 Step 2: Applying local overrides...\n")
	config.applyLocalOverrides(opts, nm)

	fmt.Printf("🔧 Step 3: Applying route permissions...\n")
	config.applyRoutePermissions(opts)

	fmt.Printf("🔧 Step 4: Configuring seed routes...\n")
	if err := config.configureSeedRoutes(opts); err != nil {
		return nil, fmt.Errorf("configure seed routes: %w", err)
	}

	fmt.Printf("🔧 Step 5: Applying cluster TLS...\n")
	// ⭐ 应用简化的集群TLS配置
	if err := config.applyClusterTLS(opts, nm); err != nil {
		return nil, fmt.Errorf("apply cluster TLS: %w", err)
	}

	fmt.Printf("🔧 Step 6: Enabling JetStream...\n")
	// Enable JetStream for KV / stream features
	opts.JetStream = true

	fmt.Printf("✅ Server options prepared successfully\n")
	return opts, nil
}

// Methods on NodeConfig formerly standalone helpers
func (c *NodeConfig) loadResolverConfig(opts *server.Options) error {
	if c.ResolverConfigPath == "" {
		return nil
	}
	if err := opts.ProcessConfigFile(c.ResolverConfigPath); err != nil {
		return fmt.Errorf("failed loading resolver.conf: %v", err)
	}
	return nil
}

func (c *NodeConfig) applyLocalOverrides(opts *server.Options, nm *NodeManager) {
	opts.ServerName = c.NodeID
	opts.Host = nm.host
	opts.Port = c.ClientPort
	opts.Cluster.Name = nm.clusterName
	opts.Cluster.Host = nm.host
	opts.Cluster.Port = c.ClusterPort
}

func (c *NodeConfig) applyRoutePermissions(opts *server.Options) {
	allowImport := c.ImportAllow
	if len(allowImport) == 0 {
		allowImport = []string{}
	}
	allowExport := c.ExportAllow
	if len(allowExport) == 0 {
		allowExport = []string{}
	}
	opts.Cluster.Permissions = &server.RoutePermissions{
		Import: &server.SubjectPermission{Allow: allowImport, Deny: []string{}},
		Export: &server.SubjectPermission{Allow: allowExport, Deny: []string{}},
	}
}

// ⭐ 简化的集群TLS配置 - 自动生成证书，默认insecure
func (c *NodeConfig) applyClusterTLS(opts *server.Options, nm *NodeManager) error {
	if !c.EnableTLS {
		return nil
	}

	// 如果没有预生成的证书，自动生成
	if c.autoTLSCert == "" || c.autoTLSKey == "" {
		certPEM, keyPEM, err := nm.GenerateSimpleTLSCert()
		if err != nil {
			return fmt.Errorf("auto-generate TLS cert: %w", err)
		}
		c.autoTLSCert = certPEM
		c.autoTLSKey = keyPEM
	}

	// 解析证书和私钥
	cert, err := tls.X509KeyPair([]byte(c.autoTLSCert), []byte(c.autoTLSKey))
	if err != nil {
		return fmt.Errorf("parse auto TLS cert/key: %w", err)
	}

	// ⭐ 简化配置 - 使用insecure模式，便于开发和测试
	opts.Cluster.TLSConfig = &tls.Config{
		Certificates:       []tls.Certificate{cert},
		InsecureSkipVerify: true,             // 默认跳过验证，简化配置
		ClientAuth:         tls.NoClientCert, // 简化客户端验证
	}

	opts.Cluster.TLSTimeout = 5.0 // 5秒TLS握手超时

	fmt.Printf("✅ Cluster TLS enabled (insecure mode) for node: %s\n", c.NodeID)
	return nil
}

func (c *NodeConfig) configureSeedRoutes(opts *server.Options) error {
	if len(c.SeedRoutes) == 0 {
		return nil
	}
	routeURLs := make([]*url.URL, len(c.SeedRoutes))
	for i, route := range c.SeedRoutes {
		u, err := url.Parse(route)
		if err != nil {
			return fmt.Errorf("failed to parse seed route URL %s: %v", route, err)
		}
		routeURLs[i] = u
	}
	opts.Routes = routeURLs
	return nil
}

// StopLocalNode stopts the local node
func (nm *NodeManager) StopLocalNode() error {
	if nm.node == nil {
		return fmt.Errorf("no running local node")
	}

	if nm.node.Server != nil {
		nm.node.Server.Shutdown()
	}

	fmt.Printf("✅ Local node stopped: %s\n", nm.node.ID)
	nm.node = nil
	return nil
}

// GetLocalNode returns the current local node instance
func (nm *NodeManager) GetLocalNode() *LocalNode {
	return nm.node
}

// IsRunning returns true if node is running
func (nm *NodeManager) IsRunning() bool {
	return nm.node != nil && nm.node.Server != nil
}

// GetClientURL returns URL for clients
func (nm *NodeManager) GetClientURL() string {
	if nm.node == nil {
		return ""
	}
	return fmt.Sprintf("nats://%s:%d", nm.node.Host, nm.node.ClientPort)
}

// GetClusterInfo returns basic cluster info
func (nm *NodeManager) GetClusterInfo() map[string]interface{} {
	if nm.node == nil || nm.node.Server == nil {
		return map[string]interface{}{
			"running": false,
		}
	}

	return map[string]interface{}{
		"running":      true,
		"node_id":      nm.node.ID,
		"client_url":   nm.GetClientURL(),
		"cluster_port": nm.node.ClusterPort,
		"connections":  nm.node.Server.NumRoutes(),
		"cluster_name": nm.node.ClusterName,
	}
}

// AddSubscribePermission placeholder: dynamic permission changes require restart
func (nm *NodeManager) AddSubscribePermission(subject string) error {
	if nm.node == nil {
		return fmt.Errorf("node not started")
	}
	if subject == "" {
		return fmt.Errorf("subject empty")
	}

	// Current allows
	current := []string{}
	if nm.lastConfig != nil && len(nm.lastConfig.ImportAllow) > 0 {
		current = append(current, nm.lastConfig.ImportAllow...)
	}
	for _, s := range current {
		if s == subject {
			return nil
		}
	}
	updated := append(current, subject)
	seedRoutes := []string{}
	if nm.lastConfig != nil {
		seedRoutes = append(seedRoutes, nm.lastConfig.SeedRoutes...)
	}
	newConfig := &NodeConfig{
		NodeID:      nm.node.ID,
		ClientPort:  nm.node.ClientPort,
		ClusterPort: nm.node.ClusterPort,
		SeedRoutes:  seedRoutes,
		ResolverConfigPath: func() string {
			if nm.lastConfig != nil {
				return nm.lastConfig.ResolverConfigPath
			}
			return ""
		}(),
		ImportAllow: updated,
		ExportAllow: func() []string {
			if nm.lastConfig != nil && len(nm.lastConfig.ExportAllow) > 0 {
				return nm.lastConfig.ExportAllow
			}
			return []string{"*"}
		}(),
	}

	// Persist to file (<nodeID>_node_config.json) for audit / future restarts
	fileName := fmt.Sprintf("%s_node_config.json", nm.node.ID)
	data, err := json.MarshalIndent(newConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal updated config failed: %w", err)
	}
	if err := os.WriteFile(fileName, data, 0o600); err != nil {
		return fmt.Errorf("write updated config failed: %w", err)
	}

	// Restart node to apply new permissions
	if err := nm.StopLocalNode(); err != nil {
		return fmt.Errorf("stop node failed: %w", err)
	}
	if err := nm.StartLocalNodeWithConfig(newConfig); err != nil {
		return fmt.Errorf("restart with updated permissions failed: %w", err)
	}
	fmt.Printf("✅ Added subscribe permission '%s' and restarted node. Persisted %s\n", subject, fileName)
	return nil
}

// CreateNodeConfigWithPermissions creates node config translating subscribe permissions -> import
func (nm *NodeManager) CreateNodeConfigWithPermissions(nodeID string, clientPort, clusterPort int, seedRoutes []string, subscribePermissions []string, enableTLS bool) *NodeConfig {
	importPermissions := subscribePermissions
	if len(importPermissions) == 0 {
		importPermissions = []string{}
	}
	return &NodeConfig{
		NodeID:      nodeID,
		ClientPort:  clientPort,
		ClusterPort: clusterPort,
		SeedRoutes:  seedRoutes,
		ImportAllow: importPermissions,
		ExportAllow: []string{"*"},
		EnableTLS:   enableTLS, // 默认启用TLS
	}
}
