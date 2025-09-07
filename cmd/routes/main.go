package main

import (
	"fmt"
	"net/url"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

// RouteNode 表示一个NATS Routes节点
type RouteNode struct {
	ID     string
	Server *server.Server
	Port   int
	Routes []string
}

func main() {
	// 演示Routes集群的链式连接特性
	fmt.Println("=== NATS Routes集群链式连接演示 ===")

	// 创建种子节点 (Node A)
	nodeA := createNode("NodeA", 4222, []string{})
	startNode(nodeA)
	defer nodeA.Server.Shutdown()

	// 等待种子节点启动
	time.Sleep(500 * time.Millisecond)

	// 创建Node B，连接到Node A
	nodeB := createNode("NodeB", 4223, []string{"nats://localhost:6222"})
	startNode(nodeB)
	defer nodeB.Server.Shutdown()

	// 等待Node B连接
	time.Sleep(500 * time.Millisecond)

	// 创建Node C，连接到Node B (不直接连接Node A)
	nodeC := createNode("NodeC", 4224, []string{"nats://localhost:6223"})
	startNode(nodeC)
	defer nodeC.Server.Shutdown()

	// 等待集群形成
	time.Sleep(2 * time.Second)

	// 验证链式连接：检查Node A是否自动发现了Node C
	fmt.Println("\n=== 验证链式连接效果 ===")
	checkClusterConnectivity(nodeA, nodeB, nodeC)

	// 测试消息路由
	fmt.Println("\n=== 测试消息路由 ===")
	testMessageRouting(nodeA, nodeB, nodeC)

	// 测试动态加入
	fmt.Println("\n=== 测试动态节点加入 ===")
	testDynamicJoin(nodeA, nodeB, nodeC)

	fmt.Println("\n=== 演示完成 ===")
	fmt.Println("Key Insights:")
	fmt.Println("✅ Routes支持链式连接：A→B→C，A自动发现C")
	fmt.Println("✅ 真正去中心化：无固定超级节点")
	fmt.Println("✅ 动态扩展：新节点只需连接任一现有节点")
	fmt.Println("✅ 自动路由：消息在全网络中自动路由")
}

// createNode 创建一个NATS节点
func createNode(name string, clientPort int, routes []string) *RouteNode {
	clusterPort := clientPort + 2000 // cluster端口 = client端口 + 2000

	opts := &server.Options{
		ServerName: name,
		Host:       "localhost",
		Port:       clientPort,
		Cluster: server.ClusterOpts{
			Name: "decentralized_chat",
			Host: "localhost",
			Port: clusterPort,
		},
	}

	// 设置Routes连接
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

// startNode 启动节点
func startNode(node *RouteNode) {
	go node.Server.Start()

	// 等待服务器启动
	if !node.Server.ReadyForConnections(5 * time.Second) {
		panic(fmt.Sprintf("Node %s failed to start", node.ID))
	}

	fmt.Printf("✅ 节点 %s 启动成功 (Client: %d, Cluster: %d)\n",
		node.ID, node.Port, node.Port+2000)

	if len(node.Routes) > 0 {
		fmt.Printf("   └─ 连接到: %v\n", node.Routes)
	} else {
		fmt.Printf("   └─ 种子节点\n")
	}
}

// checkClusterConnectivity 检查集群连通性
func checkClusterConnectivity(nodes ...*RouteNode) {
	for _, node := range nodes {
		routes := node.Server.NumRoutes()
		fmt.Printf("📊 %s: 连接数 = %d\n", node.ID, routes)

		// 期望的连接数 = 总节点数 - 1 (除自己外的所有节点)
		expectedRoutes := len(nodes) - 1
		if routes == expectedRoutes {
			fmt.Printf("   ✅ 连接正常 (期望: %d, 实际: %d)\n", expectedRoutes, routes)
		} else {
			fmt.Printf("   ⚠️  连接异常 (期望: %d, 实际: %d)\n", expectedRoutes, routes)
		}
	}
}

// testMessageRouting 测试消息路由
func testMessageRouting(nodeA, nodeB, nodeC *RouteNode) {
	// 连接到各个节点
	clientA := connectClient(nodeA, "ClientA")
	defer clientA.Close()

	clientB := connectClient(nodeB, "ClientB")
	defer clientB.Close()

	clientC := connectClient(nodeC, "ClientC")
	defer clientC.Close()

	// 在Node C订阅消息
	msgChan := make(chan string, 10)
	sub, err := clientC.Subscribe("test.routes", func(msg *nats.Msg) {
		msgChan <- fmt.Sprintf("NodeC收到: %s", string(msg.Data))
	})
	if err != nil {
		fmt.Printf("❌ 订阅失败: %v\n", err)
		return
	}
	defer sub.Unsubscribe()

	// 等待订阅传播
	time.Sleep(200 * time.Millisecond)

	// 从Node A发送消息
	testMsg := "Hello from NodeA!"
	err = clientA.Publish("test.routes", []byte(testMsg))
	if err != nil {
		fmt.Printf("❌ 发送失败: %v\n", err)
		return
	}

	// 检查是否收到消息
	select {
	case msg := <-msgChan:
		fmt.Printf("✅ 消息路由成功: %s\n", msg)
		fmt.Printf("   └─ 路径: NodeA → Routes网络 → NodeC\n")
	case <-time.After(2 * time.Second):
		fmt.Printf("❌ 消息路由失败: 超时未收到消息\n")
	}
}

// testDynamicJoin 测试动态节点加入
func testDynamicJoin(existingNodes ...*RouteNode) {
	fmt.Printf("🔄 动态加入新节点 NodeD...\n")

	// 创建Node D，连接到Node B (任意现有节点)
	nodeD := createNode("NodeD", 4225, []string{"nats://localhost:6223"})
	startNode(nodeD)
	defer nodeD.Server.Shutdown()

	// 等待连接建立
	time.Sleep(2 * time.Second)

	// 检查所有节点的连接状态
	allNodes := append(existingNodes, nodeD)
	fmt.Printf("📊 动态加入后的集群状态:\n")
	checkClusterConnectivity(allNodes...)
}

// connectClient 连接到NATS客户端
func connectClient(node *RouteNode, clientName string) *nats.Conn {
	url := fmt.Sprintf("nats://localhost:%d", node.Port)

	nc, err := nats.Connect(url, nats.Name(clientName))
	if err != nil {
		panic(fmt.Sprintf("Failed to connect client %s to %s: %v", clientName, node.ID, err))
	}

	return nc
}
