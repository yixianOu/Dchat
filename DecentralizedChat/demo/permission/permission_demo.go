package main

import (
	"fmt"
	"time"

	"DecentralizedChat/internal/config"
	natsSvc "DecentralizedChat/internal/nats"
	"DecentralizedChat/internal/routes"

	"github.com/nats-io/nats.go"
)

const (
	DefaultClientPort  = 4222
	DefaultClusterPort = 6222
)

func main() {
	fmt.Println("=== NATS权限控制演示 ===")

	// 1. 加载配置
	cfg, err := config.LoadConfig()
	if err != nil {
		fmt.Printf("加载配置失败: %v\n", err)
		return
	}

	cfg.EnableRoutes(cfg.Network.LocalIP, DefaultClientPort, DefaultClusterPort, []string{})

	// 2. 创建节点管理器
	nodeManager := routes.NewNodeManager(cfg.Routes.ClusterName, cfg.Routes.Host)

	// 3. 测试不同权限配置
	fmt.Println("\n=== 测试1: 默认权限（订阅全部拒绝）===")
	testPermissions(nodeManager, "test1-node", []string{})

	fmt.Println("\n=== 测试2: 允许订阅chat.*主题 ===")
	testPermissions(nodeManager, "test2-node", []string{"chat.*"})

	fmt.Println("\n=== 测试3: 允许订阅特定聊天室 ===")
	testPermissions(nodeManager, "test3-node", []string{"chat.general", "chat.tech"})

	fmt.Println("\n=== 权限演示完成 ===")
}

func testPermissions(nodeManager *routes.NodeManager, nodeID string, subscribePermissions []string) {
	// 创建带权限的节点配置
	nodeConfig := nodeManager.CreateNodeConfigWithPermissions(
		nodeID, DefaultClientPort, DefaultClusterPort, []string{}, subscribePermissions)

	// 启动节点
	err := nodeManager.StartLocalNodeWithConfig(nodeConfig)
	if err != nil {
		fmt.Printf("启动节点失败: %v\n", err)
		return
	}
	defer func() {
		if err := nodeManager.StopLocalNode(); err != nil {
			fmt.Printf("停止节点失败: %v\n", err)
		}
	}()

	// 获取凭据
	fmt.Printf("节点: %s, 订阅权限: %v\n", nodeID, subscribePermissions)

	// 创建客户端
	clientCfg := natsSvc.ClientConfig{
		URL:  nodeManager.GetClientURL(),
		Name: "PermissionTestClient",
	}

	client, err := natsSvc.NewService(clientCfg)
	if err != nil {
		fmt.Printf("创建客户端失败: %v\n", err)
		return
	}
	defer client.Close()

	// 测试发布（应该总是成功）
	err = client.Publish("test.publish", []byte("test message"))
	if err != nil {
		fmt.Printf("❌ 发布失败: %v\n", err)
	} else {
		fmt.Printf("✅ 发布到test.publish成功\n")
	}

	// 测试不同主题的订阅
	testSubjects := []string{
		"chat.general",
		"chat.tech",
		"system.status",
		"admin.control",
	}

	for _, subject := range testSubjects {
		err := client.Subscribe(subject, func(msg *nats.Msg) {
			fmt.Printf("收到消息: %s -> %s\n", subject, string(msg.Data))
		})

		if err != nil {
			fmt.Printf("❌ 订阅%s失败: %v\n", subject, err)
		} else {
			fmt.Printf("✅ 订阅%s成功\n", subject)
		}
	}

	time.Sleep(100 * time.Millisecond)
}
