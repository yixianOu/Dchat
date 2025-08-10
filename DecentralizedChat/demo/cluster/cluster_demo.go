package main

import (
	"fmt"
	"time"

	"DecentralizedChat/internal/config"
	"DecentralizedChat/internal/nats"
	"DecentralizedChat/internal/nscsetup"
	"DecentralizedChat/internal/routes"
)

const (
	DefaultClientPort  = 4222
	DefaultClusterPort = 6222
)

func main() {
	// 演示新的单节点管理设计
	fmt.Println("=== DecentralizedChat 节点演示 ===")

	// 1. 加载配置
	cfg, err := config.LoadConfig()
	if err != nil {
		fmt.Printf("加载配置失败: %v\n", err)
		return
	}

	// 启用Routes集群，使用自动检测的本地IP
	cfg.EnableRoutes(cfg.Network.LocalIP, DefaultClientPort, DefaultClusterPort, []string{})

	// 首次运行：通过 nsc 初始化 SYS 账户与 resolver.conf，并将路径写入配置
	if err := nscsetup.EnsureSysAccountSetup(cfg); err != nil {
		fmt.Printf("初始化 NSC/SYS 失败: %v\n", err)
		return
	}

	// 2. 创建节点管理器 (使用 ServerOptionsLite)
	nodeManager := routes.NewNodeManager(cfg.Server.ClusterName, cfg.Server.Host)

	// 3. 启动本地节点
	nodeID := fmt.Sprintf("demo-node-%s", cfg.Server.Host)
	// 使用 Server.ImportAllow 作为订阅权限
	startCfg := nodeManager.CreateNodeConfigWithPermissions(nodeID, DefaultClientPort, DefaultClusterPort, []string{}, cfg.Server.ImportAllow)
	startCfg.ResolverConfigPath = cfg.Server.ResolverConf
	err = nodeManager.StartLocalNodeWithConfig(startCfg)
	if err != nil {
		fmt.Printf("启动本地节点失败: %v\n", err)
		return
	}
	defer func() {
		if err := nodeManager.StopLocalNode(); err != nil {
			fmt.Printf("停止节点失败: %v\n", err)
		}
	}()

	time.Sleep(1 * time.Second)

	// 4. 显示节点信息
	fmt.Println("\n=== 节点信息 ===")
	nodeInfo := nodeManager.GetClusterInfo()
	for key, value := range nodeInfo {
		fmt.Printf("%s: %v\n", key, value)
	}

	// 5. 创建NATS客户端（使用 NSC 生成的 creds 文件）
	clientCfg := nats.ClientConfig{URL: nodeManager.GetClientURL(), CredsFile: cfg.Server.CredsFile, Name: "DemoClient"}

	client, err := nats.NewService(clientCfg)
	if err != nil {
		fmt.Printf("创建NATS客户端失败: %v\n", err)
		return
	}
	defer client.Close()

	fmt.Printf("\n✅ 客户端连接状态: %v\n", client.IsConnected())
	fmt.Printf("✅ 连接地址: %s\n", nodeManager.GetClientURL())
	fmt.Printf("✅ 本地IP: %s\n", cfg.Network.LocalIP)

	// 6. 测试JSON消息

	// 引导节点逻辑已移除（RoutesConfig 已删除）
	testData := map[string]interface{}{
		"message":   "Hello DecentralizedChat!",
		"timestamp": time.Now(),
		"from":      nodeID,
	}

	err = client.PublishJSON("demo.test", testData)
	if err != nil {
		fmt.Printf("发送JSON消息失败: %v\n", err)
	} else {
		fmt.Println("✅ JSON消息发送成功")
	}

	// 7. 显示最终统计信息
	stats := client.GetStats()
	fmt.Printf("\n=== 客户端统计 ===\n")
	for key, value := range stats {
		fmt.Printf("%s: %v\n", key, value)
	}

	fmt.Println("\n=== 单节点演示完成 ===")
	fmt.Println("说明: 在实际使用中，每个DChat应用只启动一个本地节点")
	fmt.Println("通过Tailscale网络发现和连接其他用户的节点形成去中心化集群")
}
