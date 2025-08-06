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

// NewNodeManager 创建节点管理器
func NewNodeManager(clusterName string, host string) *NodeManager {
	return &NodeManager{
		clusterName: clusterName,
		host:        host,
	}
}

// StartLocalNode 启动本地NATS节点
func (nm *NodeManager) StartLocalNode(nodeID string, clientPort int, clusterPort int, seedRoutes []string) error {
	// 检查是否已有节点运行
	if nm.node != nil {
		return fmt.Errorf("本地节点已启动: %s", nm.node.ID)
	}

	opts := &server.Options{
		ServerName: nodeID,
		Host:       nm.host,
		Port:       clientPort,
		Cluster: server.ClusterOpts{
			Name: nm.clusterName,
			Host: nm.host,
			Port: clusterPort,
		},
	}

	// 配置种子路由（连接到其他节点）
	if len(seedRoutes) > 0 {
		routeURLs := make([]*url.URL, len(seedRoutes))
		for i, route := range seedRoutes {
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
		return fmt.Errorf("节点 %s 启动超时", nodeID)
	}

	nm.node = &LocalNode{
		ID:          nodeID,
		Server:      srv,
		ClientPort:  clientPort,
		ClusterPort: clusterPort,
		Host:        nm.host,
		ClusterName: nm.clusterName,
	}

	fmt.Printf("✅ 本地节点启动成功: %s (Client: %s:%d, Cluster: %s:%d)\n",
		nodeID, nm.host, clientPort, nm.host, clusterPort)
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
