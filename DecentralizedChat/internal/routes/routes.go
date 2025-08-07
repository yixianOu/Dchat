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
	NodeID      string
	ClientPort  int
	ClusterPort int
	SeedRoutes  []string
	UserConfig  *UserPermissionConfig // 用户权限配置
}

// UserPermissionConfig 用户权限配置
type UserPermissionConfig struct {
	Username       string
	Password       string
	PublishAllow   []string
	PublishDeny    []string
	SubscribeAllow []string
	SubscribeDeny  []string
	AllowResponses bool
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
		UserConfig: &UserPermissionConfig{
			Username:       "dchat_user",
			Password:       "dchat_pass",
			PublishAllow:   []string{"*"}, // 默认允许发布所有主题
			PublishDeny:    []string{},    // 无发布限制
			SubscribeAllow: []string{},    // 默认无订阅权限
			SubscribeDeny:  []string{"*"}, // 拒绝订阅所有主题
			AllowResponses: true,          // 允许响应消息
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

	// 创建用户权限配置
	userPerms := &server.Permissions{
		Publish: &server.SubjectPermission{
			Allow: config.UserConfig.PublishAllow,
			Deny:  config.UserConfig.PublishDeny,
		},
		Subscribe: &server.SubjectPermission{
			Allow: config.UserConfig.SubscribeAllow,
			Deny:  config.UserConfig.SubscribeDeny,
		},
		Response: &server.ResponsePermission{
			MaxMsgs: server.DEFAULT_ALLOW_RESPONSE_MAX_MSGS,
			Expires: server.DEFAULT_ALLOW_RESPONSE_EXPIRATION,
		},
	}

	// 创建用户
	user := &server.User{
		Username:    config.UserConfig.Username,
		Password:    config.UserConfig.Password,
		Permissions: userPerms,
	}

	// 创建服务器选项
	opts := &server.Options{
		ServerName: config.NodeID,
		Host:       nm.host,
		Port:       config.ClientPort,
		Cluster: server.ClusterOpts{
			Name: nm.clusterName,
			Host: nm.host,
			Port: config.ClusterPort,
		},
		Users: []*server.User{user}, // 直接设置用户列表
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
	fmt.Printf("   用户: %s, 发布权限: %v, 订阅权限: %v\n",
		config.UserConfig.Username, config.UserConfig.PublishAllow, config.UserConfig.SubscribeAllow)
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

// GetNodeCredentials 获取节点连接凭据
func (nm *NodeManager) GetNodeCredentials() (string, string) {
	return "dchat_user", "dchat_pass"
}

// CreateNodeConfigWithPermissions 创建带权限的节点配置
func (nm *NodeManager) CreateNodeConfigWithPermissions(nodeID string, clientPort, clusterPort int, seedRoutes []string, subscribePermissions []string) *NodeConfig {
	return &NodeConfig{
		NodeID:      nodeID,
		ClientPort:  clientPort,
		ClusterPort: clusterPort,
		SeedRoutes:  seedRoutes,
		UserConfig: &UserPermissionConfig{
			Username:       "dchat_user",
			Password:       "dchat_pass",
			PublishAllow:   []string{"*"}, // 允许发布所有主题
			PublishDeny:    []string{},
			SubscribeAllow: subscribePermissions, // 用户指定的订阅权限
			SubscribeDeny:  []string{},           // 如果有allow，则不设置deny
			AllowResponses: true,
		},
	}
}
