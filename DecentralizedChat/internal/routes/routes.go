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
func (cm *ClusterManager) CreateNode(name string, clientPort int, clusterPort int, routes []string) *RouteNode {
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
				return nil
			}
			routeURLs[i] = u
		}
		opts.Routes = routeURLs
	}
	srv, err := server.NewServer(opts)
	if err != nil {
		return nil
	}

	node := &RouteNode{
		ID:          name,
		Server:      srv,
		Port:        clientPort,
		ClusterPort: clusterPort,
		Routes:      routes,
	}
	cm.nodes[name] = node
	return node
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
func (cm *ClusterManager) ConnectClient(node *RouteNode, clientName string) *nats.Conn {
	url := fmt.Sprintf("nats://%s:%d", cm.config.Host, node.Port)
	nc, err := nats.Connect(url, nats.Name(clientName))
	if err != nil {
		panic(fmt.Sprintf("Failed to connect client %s to %s: %v", clientName, node.ID, err))
	}
	return nc
}

// 测试消息路由
func (cm *ClusterManager) TestMessageRouting(nodeA, nodeB, nodeC *RouteNode) {
	clientA := cm.ConnectClient(nodeA, "ClientA")
	defer clientA.Close()
	clientB := cm.ConnectClient(nodeB, "ClientB")
	defer clientB.Close()
	clientC := cm.ConnectClient(nodeC, "ClientC")
	defer clientC.Close()

	msgChan := make(chan string, 10)
	sub, err := clientC.Subscribe("test.routes", func(msg *nats.Msg) {
		msgChan <- fmt.Sprintf("NodeC收到: %s", string(msg.Data))
	})
	if err != nil {
		fmt.Printf("订阅失败: %v\n", err)
		return
	}
	defer sub.Unsubscribe()
	time.Sleep(200 * time.Millisecond)
	testMsg := "Hello from NodeA!"
	err = clientA.Publish("test.routes", []byte(testMsg))
	if err != nil {
		fmt.Printf("发送失败: %v\n", err)
		return
	}
	select {
	case msg := <-msgChan:
		fmt.Printf("消息路由成功: %s\n", msg)
		fmt.Printf("   路径: NodeA → Routes网络 → NodeC\n")
	case <-time.After(2 * time.Second):
		fmt.Printf("消息路由失败: 超时未收到消息\n")
	}
}

// 动态加入节点
func (cm *ClusterManager) DynamicJoin(newNodeName string, newNodePort int, newNodeClusterPort int, existingNodeClusterPort int) *RouteNode {
	seedRoute := fmt.Sprintf("nats://%s:%d", cm.config.Host, existingNodeClusterPort)
	node := cm.CreateNode(newNodeName, newNodePort, newNodeClusterPort, []string{seedRoute})
	if node == nil {
		return nil
	}
	if err := cm.StartNode(node); err != nil {
		fmt.Printf("启动节点失败: %v\n", err)
		return nil
	}
	time.Sleep(2 * time.Second)
	return node
}
