package routes

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/url"
	"os"
	"time"

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
	// ⭐ TLS configuration for cluster
	EnableClusterTLS bool   `json:"enable_cluster_tls,omitempty"`
	ClusterCertPEM   string `json:"cluster_cert_pem,omitempty"`
	ClusterKeyPEM    string `json:"cluster_key_pem,omitempty"`
	ClusterInsecure  bool   `json:"cluster_insecure,omitempty"`
	// Future: credentials path if needed
}

// NewNodeManager creates a new NodeManager
func NewNodeManager(clusterName string, host string) *NodeManager {
	return &NodeManager{
		clusterName: clusterName,
		host:        host,
	}
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

	opts, err := nm.prepareServerOptions(config)
	if err != nil {
		return err
	}

	srv, err := server.NewServer(opts)
	if err != nil {
		return fmt.Errorf("failed to create NATS server: %v", err)
	}

	// Start server
	const startTimeout = 5 * time.Second
	go srv.Start()
	if !srv.ReadyForConnections(startTimeout) {
		return fmt.Errorf("node %s start timeout (possible port conflict client:%d cluster:%d)", config.NodeID, config.ClientPort, config.ClusterPort)
	}

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
	fmt.Printf("   Node: %s, Import Allow: %v, Export Allow: %v\n",
		config.NodeID,
		config.ImportAllow,
		config.ExportAllow)
	return nil
}

// ensureNotStarted returns error if a node is already running.
// prepareServerOptions orchestrates building server options from config via NodeConfig methods.
func (nm *NodeManager) prepareServerOptions(config *NodeConfig) (*server.Options, error) {
	opts := &server.Options{}
	if err := config.loadResolverConfig(opts); err != nil {
		return nil, err
	}
	config.applyLocalOverrides(opts, nm)
	config.applyRoutePermissions(opts)
	if err := config.configureSeedRoutes(opts); err != nil {
		return nil, err
	}
	// ⭐ 应用集群TLS配置
	if err := config.applyClusterTLS(opts); err != nil {
		return nil, fmt.Errorf("apply cluster TLS: %w", err)
	}
	// Enable JetStream for KV / stream features
	opts.JetStream = true
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

// ⭐ 新增：配置集群TLS
func (c *NodeConfig) applyClusterTLS(opts *server.Options) error {
	if !c.EnableClusterTLS {
		return nil
	}

	if c.ClusterCertPEM == "" || c.ClusterKeyPEM == "" {
		return fmt.Errorf("cluster TLS enabled but cert/key PEM not provided")
	}

	// 解析证书和私钥
	cert, err := tls.X509KeyPair([]byte(c.ClusterCertPEM), []byte(c.ClusterKeyPEM))
	if err != nil {
		return fmt.Errorf("parse cluster TLS cert/key: %w", err)
	}

	// 解析证书以获取CA
	certBlock, _ := pem.Decode([]byte(c.ClusterCertPEM))
	if certBlock == nil {
		return fmt.Errorf("failed to decode cluster certificate PEM")
	}

	parsedCert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return fmt.Errorf("parse cluster certificate: %w", err)
	}

	// 创建CA池（自签名证书，使用自己作为CA）
	caCertPool := x509.NewCertPool()
	caCertPool.AddCert(parsedCert)

	// 配置集群TLS
	opts.Cluster.TLSConfig = &tls.Config{
		Certificates:       []tls.Certificate{cert},
		ClientAuth:         tls.RequireAndVerifyClientCert,
		ClientCAs:          caCertPool,
		RootCAs:            caCertPool,
		InsecureSkipVerify: c.ClusterInsecure,
		ServerName:         parsedCert.Subject.CommonName,
	}

	opts.Cluster.TLSTimeout = 5.0 // 5秒TLS握手超时

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
func (nm *NodeManager) CreateNodeConfigWithPermissions(nodeID string, clientPort, clusterPort int, seedRoutes []string, subscribePermissions []string) *NodeConfig {
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
	}
}

// ⭐ CreateNodeConfigWithTLS 创建启用集群TLS的节点配置
func (nm *NodeManager) CreateNodeConfigWithTLS(nodeID string, clientPort, clusterPort int, seedRoutes []string, subscribePermissions []string, certPEM, keyPEM string, insecure bool) *NodeConfig {
	config := nm.CreateNodeConfigWithPermissions(nodeID, clientPort, clusterPort, seedRoutes, subscribePermissions)
	config.EnableClusterTLS = true
	config.ClusterCertPEM = certPEM
	config.ClusterKeyPEM = keyPEM
	config.ClusterInsecure = insecure
	return config
}
