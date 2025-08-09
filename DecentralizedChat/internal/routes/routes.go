package routes

import (
	"fmt"
	"net/url"
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
	// Prevent duplicate start
	if nm.node != nil {
		return fmt.Errorf("local node already started: %s", nm.node.ID)
	}

	// Create server options; load resolver.conf first to avoid overwriting later fields
	opts := &server.Options{}
	if config.ResolverConfigPath != "" {
		if err := opts.ProcessConfigFile(config.ResolverConfigPath); err != nil {
			return fmt.Errorf("failed loading resolver.conf: %v", err)
		}
	}

	// Set local override options (these take precedence)
	opts.ServerName = config.NodeID
	opts.Host = nm.host
	opts.Port = config.ClientPort
	opts.Cluster.Name = nm.clusterName
	opts.Cluster.Host = nm.host
	opts.Cluster.Port = config.ClusterPort
	opts.Cluster.Permissions = &server.RoutePermissions{
		Import: &server.SubjectPermission{
			Allow: config.NodeConfig.Permissions.Routes.Import.Allow,
			Deny:  config.NodeConfig.Permissions.Routes.Import.Deny,
		},
		Export: &server.SubjectPermission{
			Allow: config.NodeConfig.Permissions.Routes.Export.Allow,
			Deny:  config.NodeConfig.Permissions.Routes.Export.Deny,
		},
	}

	// Load trusted public keys (future extension placeholder)
	trustedKeys := make([]string, 0)
	// If NodeConfig.Credentials == "use_local_trust" we could load keys here (not implemented)
	if config.NodeConfig != nil && config.NodeConfig.Credentials == "use_local_trust" {
		// no-op placeholder for future injection
	}
	// Apply if we actually loaded any and none set yet
	if len(trustedKeys) > 0 && len(opts.TrustedKeys) == 0 {
		opts.TrustedKeys = trustedKeys
	}

	// Configure seed routes
	if len(config.SeedRoutes) > 0 {
		routeURLs := make([]*url.URL, len(config.SeedRoutes))
		for i, route := range config.SeedRoutes {
			u, err := url.Parse(route)
			if err != nil {
				return fmt.Errorf("failed to parse seed route URL %s: %v", route, err)
			}
			routeURLs[i] = u
		}
		opts.Routes = routeURLs
	}

	srv, err := server.NewServer(opts)
	if err != nil {
		return fmt.Errorf("failed to create NATS server: %v", err)
	}

	// Start server
	go srv.Start()
	if !srv.ReadyForConnections(5 * time.Second) {
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

	fmt.Printf("✅ Local node started: %s (Client: %s:%d, Cluster: %s:%d)\n",
		config.NodeID, nm.host, config.ClientPort, nm.host, config.ClusterPort)
	fmt.Printf("   Node: %s, Import Allow: %v, Export Allow: %v\n",
		config.NodeConfig.NodeName, config.NodeConfig.Permissions.Routes.Import.Allow, config.NodeConfig.Permissions.Routes.Export.Allow)
	return nil
}

// StopLocalNode stops the local node
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
	// NATS server permissions can't be mutated live; requires restart
	return fmt.Errorf("dynamic permission change requires node restart")
}

// GetNodeCredentials returns client auth data (empty when using JWT/creds)
func (nm *NodeManager) GetNodeCredentials() (string, string) {
	// Not used with JWT/creds model
	return "", ""
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
