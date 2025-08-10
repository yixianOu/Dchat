package routes

import (
	"encoding/json"
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
	NodeConfig         *NodePermissionConfig // Node level permission spec
	ResolverConfigPath string                // Optional: path to resolver.conf enabling JWT account resolver
}

// SubjectPermission subject allow/deny definition
type SubjectPermission struct {
	Allow []string `json:"allow,omitempty"`
	Deny  []string `json:"deny,omitempty"`
}

// ResponsePermission response permission limits
type ResponsePermission struct {
	MaxMsgs int           `json:"max"`
	Expires time.Duration `json:"ttl"`
}

// RoutePermissions controls import/export between route-connected nodes
type RoutePermissions struct {
	Import *SubjectPermission `json:"import"`
	Export *SubjectPermission `json:"export"`
}

// NodePermissions encapsulates route and response permissions
type NodePermissions struct {
	Routes   *RoutePermissions   `json:"routes"`
	Response *ResponsePermission `json:"responses,omitempty"`
}

// NodePermissionConfig permission wrapper for a node
type NodePermissionConfig struct {
	NodeName    string           `json:"node_name"`
	Credentials string           `json:"credentials"`
	Permissions *NodePermissions `json:"permissions"`
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
		NodeConfig: &NodePermissionConfig{
			NodeName:    nodeID,
			Credentials: "dchat_node_credentials",
			Permissions: &NodePermissions{
				Routes: &RoutePermissions{
					Import: &SubjectPermission{
						Allow: []string{"*"}, // allow importing all subjects by default
						Deny:  []string{},
					},
					Export: &SubjectPermission{
						Allow: []string{"*"}, // allow exporting all subjects by default
						Deny:  []string{},
					},
				},
				Response: &ResponsePermission{
					MaxMsgs: 1000,        // allow response messages
					Expires: time.Minute, // response expiration
				},
			},
		},
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
		return fmt.Errorf("node %s start timeout", config.NodeID)
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
		config.NodeConfig.NodeName,
		config.NodeConfig.Permissions.Routes.Import.Allow,
		config.NodeConfig.Permissions.Routes.Export.Allow)
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
	npc := c.NodeConfig
	if npc == nil || npc.Permissions == nil || npc.Permissions.Routes == nil {
		opts.Cluster.Permissions = &server.RoutePermissions{
			Import: &server.SubjectPermission{Allow: []string{}, Deny: []string{"*"}},
			Export: &server.SubjectPermission{Allow: []string{}, Deny: []string{"*"}},
		}
		return
	}
	rp := npc.Permissions.Routes
	opts.Cluster.Permissions = &server.RoutePermissions{
		Import: &server.SubjectPermission{Allow: rp.Import.Allow, Deny: rp.Import.Deny},
		Export: &server.SubjectPermission{Allow: rp.Export.Allow, Deny: rp.Export.Deny},
	}
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

	// Use lastConfig to derive existing permissions
	var currentAllows []string
	if nm.lastConfig != nil && nm.lastConfig.NodeConfig != nil && nm.lastConfig.NodeConfig.Permissions != nil && nm.lastConfig.NodeConfig.Permissions.Routes != nil && nm.lastConfig.NodeConfig.Permissions.Routes.Import != nil {
		currentAllows = append(currentAllows, nm.lastConfig.NodeConfig.Permissions.Routes.Import.Allow...)
	}

	// Check if already present
	for _, s := range currentAllows {
		if s == subject {
			// Already allowed  nothing to do
			return nil
		}
	}
	updatedAllows := append(currentAllows, subject)

	// Reuse previous SeedRoutes if available
	seedRoutes := []string{}
	if nm.lastConfig != nil {
		seedRoutes = append(seedRoutes, nm.lastConfig.SeedRoutes...)
	}

	// Build updated node config mirroring defaults used at start
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
		NodeConfig: &NodePermissionConfig{
			NodeName:    nm.node.ID,
			Credentials: "dchat_node_credentials", // same default credential label
			Permissions: &NodePermissions{
				Routes: &RoutePermissions{
					Import: &SubjectPermission{Allow: updatedAllows, Deny: []string{}},
					Export: &SubjectPermission{Allow: []string{"*"}, Deny: []string{}},
				},
				Response: &ResponsePermission{MaxMsgs: 1000, Expires: time.Minute},
			},
		},
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
	// Translate user subscribe permissions to route import permissions
	importPermissions := subscribePermissions
	if len(importPermissions) == 0 {
		importPermissions = []string{} // empty slice => deny all imports
	}

	return &NodeConfig{
		NodeID:      nodeID,
		ClientPort:  clientPort,
		ClusterPort: clusterPort,
		SeedRoutes:  seedRoutes,
		NodeConfig: &NodePermissionConfig{
			NodeName:    nodeID,
			Credentials: "dchat_node_credentials",
			Permissions: &NodePermissions{
				Routes: &RoutePermissions{
					Import: &SubjectPermission{
						Allow: importPermissions, // allow limited imports based on user configuration
						Deny:  []string{},
					},
					Export: &SubjectPermission{
						Allow: []string{"*"}, // export all subjects
						Deny:  []string{},
					},
				},
				Response: &ResponsePermission{
					MaxMsgs: 1000,
					Expires: time.Minute,
				},
			},
		},
	}
}
