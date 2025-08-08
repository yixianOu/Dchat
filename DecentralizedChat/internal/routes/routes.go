package routes

import (
	"fmt"
	"net/url"
	"time"

	"github.com/nats-io/nats-server/v2/server"
)

// LocalNode 本地NATS节点
type LocalNode struct {
	ID          string
	Server      *server.Server
	ClientPort  int
	ClusterPort int
	Host        string
	ClusterName string
}

// NodeManager 单节点管理器（适用于去中心化应用）
type NodeManager struct {
	node        *LocalNode
	clusterName string
	host        string
}

// NodeConfig NATS节点配置
type NodeConfig struct {
	NodeID             string
	ClientPort         int
	ClusterPort        int
	SeedRoutes         []string
	NodeConfig         *NodePermissionConfig // 节点权限配置
	ResolverConfigPath string                // 可选：resolver.conf 路径，启用基于 JWT 的账户解析
}

// SubjectPermission 主题权限配置
type SubjectPermission struct {
	Allow []string `json:"allow,omitempty"`
	Deny  []string `json:"deny,omitempty"`
}

// ResponsePermission 响应权限配置
type ResponsePermission struct {
	MaxMsgs int           `json:"max"`
	Expires time.Duration `json:"ttl"`
}

// RoutePermissions 路由权限配置（用于去中心化节点间通信）
type RoutePermissions struct {
	Import *SubjectPermission `json:"import"`
	Export *SubjectPermission `json:"export"`
}

// NodePermissions 节点权限配置（替代用户权限）
type NodePermissions struct {
	Routes   *RoutePermissions   `json:"routes"`
	Response *ResponsePermission `json:"responses,omitempty"`
}

// NodePermissionConfig 节点权限配置
type NodePermissionConfig struct {
	NodeName    string           `json:"node_name"`
	Credentials string           `json:"credentials"`
	Permissions *NodePermissions `json:"permissions"`
}

// NewNodeManager 创建节点管理器
func NewNodeManager(clusterName string, host string) *NodeManager {
	return &NodeManager{
		clusterName: clusterName,
		host:        host,
	}
}

// StartLocalNode 启动本地NATS节点
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
						Allow: []string{"*"}, // 默认允许导入所有主题
						Deny:  []string{},
					},
					Export: &SubjectPermission{
						Allow: []string{"*"}, // 默认允许导出所有主题
						Deny:  []string{},
					},
				},
				Response: &ResponsePermission{
					MaxMsgs: 1000,        // 允许响应消息
					Expires: time.Minute, // 响应过期时间
				},
			},
		},
	}
	return nm.StartLocalNodeWithConfig(config)
}

// StartLocalNodeWithConfig 使用配置启动本地NATS节点
func (nm *NodeManager) StartLocalNodeWithConfig(config *NodeConfig) error {
	// 检查是否已有节点运行
	if nm.node != nil {
		return fmt.Errorf("本地节点已启动: %s", nm.node.ID)
	}

	// 创建服务器选项；为避免覆盖，先加载 resolver.conf，再设置字段
	opts := &server.Options{}
	if config.ResolverConfigPath != "" {
		if err := opts.ProcessConfigFile(config.ResolverConfigPath); err != nil {
			return fmt.Errorf("加载 resolver.conf 失败: %v", err)
		}
	}

	// 之后设置本地覆盖项（不会被配置文件覆盖）
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

	// 配置种子路由（连接到其他节点）
	if len(config.SeedRoutes) > 0 {
		routeURLs := make([]*url.URL, len(config.SeedRoutes))
		for i, route := range config.SeedRoutes {
			u, err := url.Parse(route)
			if err != nil {
				return fmt.Errorf("解析种子路由URL失败 %s: %v", route, err)
			}
			routeURLs[i] = u
		}
		opts.Routes = routeURLs
	}

	srv, err := server.NewServer(opts)
	if err != nil {
		return fmt.Errorf("创建NATS服务器失败: %v", err)
	}

	// 启动服务器
	go srv.Start()
	if !srv.ReadyForConnections(5 * time.Second) {
		return fmt.Errorf("节点 %s 启动超时", config.NodeID)
	}

	nm.node = &LocalNode{
		ID:          config.NodeID,
		Server:      srv,
		ClientPort:  config.ClientPort,
		ClusterPort: config.ClusterPort,
		Host:        nm.host,
		ClusterName: nm.clusterName,
	}

	fmt.Printf("✅ 本地节点启动成功: %s (Client: %s:%d, Cluster: %s:%d)\n",
		config.NodeID, nm.host, config.ClientPort, nm.host, config.ClusterPort)
	fmt.Printf("   节点: %s, 导入权限: %v, 导出权限: %v\n",
		config.NodeConfig.NodeName, config.NodeConfig.Permissions.Routes.Import.Allow, config.NodeConfig.Permissions.Routes.Export.Allow)
	return nil
}

// StopLocalNode 停止本地节点
func (nm *NodeManager) StopLocalNode() error {
	if nm.node == nil {
		return fmt.Errorf("没有运行中的本地节点")
	}

	if nm.node.Server != nil {
		nm.node.Server.Shutdown()
	}

	fmt.Printf("✅ 本地节点已停止: %s\n", nm.node.ID)
	nm.node = nil
	return nil
}

// GetLocalNode 获取本地节点信息
func (nm *NodeManager) GetLocalNode() *LocalNode {
	return nm.node
}

// IsRunning 检查本地节点是否运行中
func (nm *NodeManager) IsRunning() bool {
	return nm.node != nil && nm.node.Server != nil
}

// GetClientURL 获取客户端连接URL
func (nm *NodeManager) GetClientURL() string {
	if nm.node == nil {
		return ""
	}
	return fmt.Sprintf("nats://%s:%d", nm.node.Host, nm.node.ClientPort)
}

// GetClusterInfo 获取集群连接信息
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

// AddSubscribePermission 动态添加订阅权限（需要重启节点生效）
func (nm *NodeManager) AddSubscribePermission(subject string) error {
	if nm.node == nil {
		return fmt.Errorf("节点未启动")
	}
	// 注意：NATS服务器运行时无法动态修改权限，需要重启节点
	return fmt.Errorf("动态权限修改需要重启节点才能生效")
}

// GetNodeCredentials 获取节点连接凭据（客户端用）
func (nm *NodeManager) GetNodeCredentials() (string, string) {
	// 使用 JWT/creds 时，客户端侧不再提供用户名/密码
	return "", ""
}

// CreateNodeConfigWithPermissions 创建带权限的节点配置
func (nm *NodeManager) CreateNodeConfigWithPermissions(nodeID string, clientPort, clusterPort int, seedRoutes []string, subscribePermissions []string) *NodeConfig {
	// 将订阅权限转换为导入权限（去中心化节点接收其他节点的消息）
	importPermissions := subscribePermissions
	if len(importPermissions) == 0 {
		importPermissions = []string{} // 空数组表示拒绝所有导入
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
						Allow: importPermissions, // 根据用户配置允许导入特定主题
						Deny:  []string{},
					},
					Export: &SubjectPermission{
						Allow: []string{"*"}, // 允许导出所有主题到其他节点
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
