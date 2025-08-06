package routes

import (
	"fmt"
	"net/url"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

type RouteNode struct {
	ID          string
	Server      *server.Server
	Port        int
	ClusterPort int
	Routes      []string
}

type ClusterManager struct {
	nodes       map[string]*RouteNode
	clusterName string
	config      ClusterConfig
}

type ClusterConfig struct {
	Host string // 主机地址
}

func NewClusterManager(clusterName string, config *ClusterConfig) *ClusterManager {
	// 设置默认配置
	if config == nil {
		return nil // 不提供默认配置，强制用户传入配置
	}

	return &ClusterManager{
		nodes:       make(map[string]*RouteNode),
		clusterName: clusterName,
		config:      *config,
	}
}

// 创建带默认配置的集群管理器（用于简化使用）
func NewDefaultClusterManager(clusterName string, host string) *ClusterManager {
	config := &ClusterConfig{
		Host: host,
	}
	return NewClusterManager(clusterName, config)
}

// 创建NATS节点
func (cm *ClusterManager) CreateNode(name string, clientPort int, clusterPort int, routes []string) (*RouteNode, error) {
	opts := &server.Options{
		ServerName: name,
		Host:       cm.config.Host,
		Port:       clientPort,
		Cluster: server.ClusterOpts{
			Name: cm.clusterName,
			Host: cm.config.Host,
			Port: clusterPort,
		},
	}
	if len(routes) > 0 {
		routeURLs := make([]*url.URL, len(routes))
		for i, route := range routes {
			u, err := url.Parse(route)
			if err != nil {
				return nil, fmt.Errorf("解析路由URL失败 %s: %v", route, err)
			}
			routeURLs[i] = u
		}
		opts.Routes = routeURLs
	}
	srv, err := server.NewServer(opts)
	if err != nil {
		return nil, fmt.Errorf("创建NATS服务器失败: %v", err)
	}

	node := &RouteNode{
		ID:          name,
		Server:      srv,
		Port:        clientPort,
		ClusterPort: clusterPort,
		Routes:      routes,
	}
	cm.nodes[name] = node
	return node, nil
}

// 启动节点
func (cm *ClusterManager) StartNode(node *RouteNode) error {
	go node.Server.Start()
	if !node.Server.ReadyForConnections(5 * time.Second) {
		return fmt.Errorf("node %s failed to start", node.ID)
	}
	fmt.Printf("✅ 节点 %s 启动成功 (Client: %s:%d, Cluster: %s:%d)\n",
		node.ID, cm.config.Host, node.Port, cm.config.Host, node.ClusterPort)
	return nil
}

// 停止节点
func (cm *ClusterManager) StopNode(nodeID string) {
	if node, exists := cm.nodes[nodeID]; exists {
		if node.Server != nil {
			node.Server.Shutdown()
		}
		delete(cm.nodes, nodeID)
	}
}

// 获取所有节点
func (cm *ClusterManager) GetNodes() map[string]*RouteNode {
	return cm.nodes
}

// 检查集群连通性
func (cm *ClusterManager) CheckClusterConnectivity() {
	for _, node := range cm.nodes {
		routes := node.Server.NumRoutes()
		fmt.Printf("%s: 连接数 = %d\n", node.ID, routes)
		expectedRoutes := len(cm.nodes) - 1
		if routes == expectedRoutes {
			fmt.Printf("   ✅ 连接正常 (期望: %d, 实际: %d)\n", expectedRoutes, routes)
		} else {
			fmt.Printf("   ⚠️  连接异常 (期望: %d, 实际: %d)\n", expectedRoutes, routes)
		}
	}
}

// 连接NATS客户端
func (cm *ClusterManager) ConnectClient(node *RouteNode, clientName string) (*nats.Conn, error) {
	url := fmt.Sprintf("nats://%s:%d", cm.config.Host, node.Port)
	nc, err := nats.Connect(url, nats.Name(clientName))
	if err != nil {
		return nil, fmt.Errorf("连接NATS客户端失败 %s 到 %s: %v", clientName, node.ID, err)
	}
	return nc, nil
}

// 测试消息路由
func (cm *ClusterManager) TestMessageRouting(nodeA, nodeB, nodeC *RouteNode) error {
	clientA, err := cm.ConnectClient(nodeA, "ClientA")
	if err != nil {
		return fmt.Errorf("连接ClientA失败: %v", err)
	}
	defer clientA.Close()

	clientB, err := cm.ConnectClient(nodeB, "ClientB")
	if err != nil {
		return fmt.Errorf("连接ClientB失败: %v", err)
	}
	defer clientB.Close()

	clientC, err := cm.ConnectClient(nodeC, "ClientC")
	if err != nil {
		return fmt.Errorf("连接ClientC失败: %v", err)
	}
	defer clientC.Close()

	msgChan := make(chan string, 10)
	sub, err := clientC.Subscribe("test.routes", func(msg *nats.Msg) {
		msgChan <- fmt.Sprintf("NodeC收到: %s", string(msg.Data))
	})
	if err != nil {
		return fmt.Errorf("订阅失败: %v", err)
	}
	defer sub.Unsubscribe()
	time.Sleep(200 * time.Millisecond)
	testMsg := "Hello from NodeA!"
	err = clientA.Publish("test.routes", []byte(testMsg))
	if err != nil {
		return fmt.Errorf("发送失败: %v", err)
	}
	select {
	case msg := <-msgChan:
		fmt.Printf("消息路由成功: %s\n", msg)
		fmt.Printf("   路径: NodeA → Routes网络 → NodeC\n")
		return nil
	case <-time.After(2 * time.Second):
		return fmt.Errorf("消息路由失败: 超时未收到消息")
	}
}

// 动态加入节点
func (cm *ClusterManager) DynamicJoin(newNodeName string, newNodePort int, newNodeClusterPort int, existingNodeClusterPort int) (*RouteNode, error) {
	seedRoute := fmt.Sprintf("nats://%s:%d", cm.config.Host, existingNodeClusterPort)
	node, err := cm.CreateNode(newNodeName, newNodePort, newNodeClusterPort, []string{seedRoute})
	if err != nil {
		return nil, fmt.Errorf("创建节点失败: %v", err)
	}
	if err := cm.StartNode(node); err != nil {
		return nil, fmt.Errorf("启动节点失败: %v", err)
	}
	time.Sleep(2 * time.Second)
	return node, nil
}
