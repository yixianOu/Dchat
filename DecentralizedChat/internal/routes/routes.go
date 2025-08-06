package routes

import (
	"fmt"
	"net/url"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

type RouteNode struct {
	ID     string
	Server *server.Server
	Port   int
	Routes []string
}

type ClusterManager struct {
	nodes   map[string]*RouteNode
	network string
}

func NewClusterManager(network string) *ClusterManager {
	return &ClusterManager{
		nodes:   make(map[string]*RouteNode),
		network: network,
	}
}

// 创建NATS节点
func (cm *ClusterManager) CreateNode(name string, clientPort int, routes []string) *RouteNode {
	clusterPort := clientPort + 2000
	opts := &server.Options{
		ServerName: name,
		Host:       "127.0.0.1",
		Port:       clientPort,
		Cluster: server.ClusterOpts{
			Name: cm.network,
			Host: "127.0.0.1",
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
		ID:     name,
		Server: srv,
		Port:   clientPort,
		Routes: routes,
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
	fmt.Printf("✅ 节点 %s 启动成功 (Client: %d, Cluster: %d)\n",
		node.ID, node.Port, node.Port+2000)
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

// 创建NATS节点 (兼容旧版API)
func CreateNode(name string, clientPort int, routes []string) *RouteNode {
	clusterPort := clientPort + 2000
	opts := &server.Options{
		ServerName: name,
		Host:       "127.0.0.1",
		Port:       clientPort,
		Cluster: server.ClusterOpts{
			Name: "decentralized_chat",
			Host: "127.0.0.1",
			Port: clusterPort,
		},
	}
	if len(routes) > 0 {
		routeURLs := make([]*url.URL, len(routes))
		for i, route := range routes {
			u, err := url.Parse(route)
			if err != nil {
				panic(fmt.Sprintf("Invalid route URL %s: %v", route, err))
			}
			routeURLs[i] = u

		}
		opts.Routes = routeURLs
	}
	srv, err := server.NewServer(opts)
	if err != nil {
		panic(fmt.Sprintf("Failed to create server %s: %v", name, err))
	}
	return &RouteNode{
		ID:     name,
		Server: srv,
		Port:   clientPort,
		Routes: routes,
	}
}

// 启动节点
func StartNode(node *RouteNode) {
	go node.Server.Start()
	if !node.Server.ReadyForConnections(5 * time.Second) {
		panic(fmt.Sprintf("Node %s failed to start", node.ID))
	}
}

// 检查集群连通性
func CheckClusterConnectivity(nodes ...*RouteNode) {
	for _, node := range nodes {
		routes := node.Server.NumRoutes()
		fmt.Printf("%s: 连接数 = %d\n", node.ID, routes)
		expectedRoutes := len(nodes) - 1
		if routes == expectedRoutes {
			fmt.Printf("   连接正常 (期望: %d, 实际: %d)\n", expectedRoutes, routes)
		} else {
			fmt.Printf("   连接异常 (期望: %d, 实际: %d)\n", expectedRoutes, routes)
		}
	}
}

// 动态加入节点
func DynamicJoin(existingNodes ...*RouteNode) *RouteNode {
	nodeD := CreateNode("NodeD", 4225, []string{"nats://127.0.0.1:6223"})
	StartNode(nodeD)
	time.Sleep(2 * time.Second)
	return nodeD
}

// 连接NATS客户端
func ConnectClient(node *RouteNode, clientName string) *nats.Conn {
	url := fmt.Sprintf("nats://127.0.0.1:%d", node.Port)
	nc, err := nats.Connect(url, nats.Name(clientName))
	if err != nil {
		panic(fmt.Sprintf("Failed to connect client %s to %s: %v", clientName, node.ID, err))
	}
	return nc
}

// 测试消息路由
func TestMessageRouting(nodeA, nodeB, nodeC *RouteNode) {
	clientA := ConnectClient(nodeA, "ClientA")
	defer clientA.Close()
	clientB := ConnectClient(nodeB, "ClientB")
	defer clientB.Close()
	clientC := ConnectClient(nodeC, "ClientC")
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
