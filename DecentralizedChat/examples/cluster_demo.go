package main

import (
	"fmt"
	"time"

	"DecentralizedChat/internal/config"
	"DecentralizedChat/internal/nats"
	"DecentralizedChat/internal/routes"
)

func main() {
	// 演示新的集群管理和客户端设计
	fmt.Println("=== DecentralizedChat 集群演示 ===")

	// 1. 加载配置
	cfg, err := config.LoadConfig()
	if err != nil {
		fmt.Printf("加载配置失败: %v\n", err)
		return
	}

	// 启用Routes集群
	cfg.EnableRoutes("127.0.0.1", 4222, []string{})

	// 2. 创建集群管理器配置
	clusterConfig := &routes.ClusterConfig{
		Host:              cfg.Routes.Host,
		ClusterPortOffset: cfg.Routes.ClusterPortOffset,
	}

	// 创建集群管理器
	cm := routes.NewClusterManager(cfg.Routes.ClusterName, clusterConfig) // 3. 创建并启动节点A（种子节点）
	nodeA := cm.CreateNode("NodeA", 4222, []string{})
	if nodeA == nil {
		fmt.Println("创建NodeA失败")
		return
	}
	if err := cm.StartNode(nodeA); err != nil {
		fmt.Printf("启动NodeA失败: %v\n", err)
		return
	}
	defer nodeA.Server.Shutdown()

	time.Sleep(500 * time.Millisecond)

	// 4. 创建并启动节点B
	clusterPortA := 4222 + cfg.Routes.ClusterPortOffset
	nodeB := cm.CreateNode("NodeB", 4223, []string{fmt.Sprintf("nats://%s:%d", cfg.Routes.Host, clusterPortA)})
	if nodeB == nil {
		fmt.Println("创建NodeB失败")
		return
	}
	if err := cm.StartNode(nodeB); err != nil {
		fmt.Printf("启动NodeB失败: %v\n", err)
		return
	}
	defer nodeB.Server.Shutdown()

	time.Sleep(2 * time.Second)

	// 5. 检查集群连通性
	fmt.Println("\n=== 集群连通性检查 ===")
	cm.CheckClusterConnectivity()

	// 6. 创建NATS客户端
	clientCfg := nats.ClientConfig{
		URL:  fmt.Sprintf("nats://%s:%d", cfg.Routes.Host, cfg.Routes.ClientPort),
		Name: "DemoClient",
	}

	client, err := nats.NewService(clientCfg)
	if err != nil {
		fmt.Printf("创建NATS客户端失败: %v\n", err)
		return
	}
	defer client.Close()

	fmt.Printf("\n✅ 客户端连接状态: %v\n", client.IsConnected())

	// 7. 测试JSON消息
	testData := map[string]interface{}{
		"message":   "Hello DecentralizedChat!",
		"timestamp": time.Now(),
		"from":      "DemoClient",
	}

	err = client.PublishJSON("demo.test", testData)
	if err != nil {
		fmt.Printf("发送JSON消息失败: %v\n", err)
	} else {
		fmt.Println("✅ JSON消息发送成功")
	}

	// 8. 显示统计信息
	stats := client.GetStats()
	fmt.Printf("\n=== 客户端统计 ===\n")
	for key, value := range stats {
		fmt.Printf("%s: %v\n", key, value)
	}

	fmt.Println("\n=== 演示完成 ===")
}
