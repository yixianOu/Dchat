package main

import (
	"fmt"
	"net/url"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

// NATSServer 包含服务器和相关配置
type NATSServer struct {
	name   string
	server *server.Server
	port   int
}

// NATSClient 包含客户端连接和相关信息
type NATSClient struct {
	name     string
	conn     *nats.Conn
	serverID string
}

func main() {
	const (
		address     = "localhost"
		mainPort    = 4222
		clusterPort = 7422
		leafPort    = 4223
		subjectName = "foo"
		timeout     = time.Second
	)

	// 创建并启动服务器
	mainServer := createMainServer(address, mainPort, clusterPort)
	leafServer := createLeafServer(address, leafPort, address, clusterPort)

	// 启动服务器
	startServers(mainServer, leafServer)
	defer stopServers(mainServer, leafServer)

	// 连接到服务器
	mainClient := connectClient("main", address, mainPort)
	leafClient := connectClient("leaf", address, leafPort)
	defer func() {
		mainClient.conn.Drain()
		leafClient.conn.Drain()
	}()

	// 测试场景1: 主服务器订阅，叶服务器请求
	runScenario(mainClient, leafClient, subjectName,
		"response from main server", "request from leaf server", timeout)

	// 测试场景2: 叶服务器也订阅，再次请求（两个响应者）
	sub := subscribe(leafClient, subjectName, "response from leaf server")
	defer sub.Drain()

	// 在叶节点发送请求，应该收到来自叶节点的响应（本地优先）
	sendRequest(leafClient, subjectName, "request from leaf server", timeout)

	// 测试场景3: 关闭主服务器后，叶节点仍能处理请求
	fmt.Println("关闭主服务器，测试叶节点自主处理能力...")
	mainServer.server.Shutdown()
	time.Sleep(100 * time.Millisecond)

	sendRequest(leafClient, subjectName, "request from leaf server after main shutdown", timeout)
}

// 创建主服务器配置
func createMainServer(address string, mainPort, clusterPort int) *NATSServer {
	mainLeafConf := server.LeafNodeOpts{
		Port: clusterPort,
	}
	mainConf := server.Options{
		Host:     address,
		Port:     mainPort,
		LeafNode: mainLeafConf,
	}

	srv, err := server.NewServer(&mainConf)
	if err != nil {
		panic(fmt.Sprintf("创建主服务器失败: %v", err))
	}
	return &NATSServer{name: "main", server: srv, port: mainPort}
}

// 创建叶服务器配置
func createLeafServer(address string, leafPort int, mainAddress string, mainClusterPort int) *NATSServer {
	leafNodeConf := server.LeafNodeOpts{
		Remotes: []*server.RemoteLeafOpts{
			{
				URLs: []*url.URL{
					{Scheme: "nats", Host: fmt.Sprintf("%s:%d", mainAddress, mainClusterPort)},
				},
			},
		},
	}
	leafConf := server.Options{
		Host:     address,
		Port:     leafPort,
		LeafNode: leafNodeConf,
	}

	srv, err := server.NewServer(&leafConf)
	if err != nil {
		panic(fmt.Sprintf("创建叶服务器失败: %v", err))
	}
	return &NATSServer{name: "leaf", server: srv, port: leafPort}
}

// 启动多个服务器
func startServers(servers ...*NATSServer) {
	for _, s := range servers {
		go s.server.Start()
		fmt.Printf("%s服务器已启动在端口 %d\n", s.name, s.port)
	}
	// 等待服务器完全启动
	time.Sleep(time.Second)
}

// 停止多个服务器
func stopServers(servers ...*NATSServer) {
	for _, s := range servers {
		if s.server.Running() {
			s.server.Shutdown()
			fmt.Printf("%s服务器已关闭\n", s.name)
		}
	}
}

// 连接到NATS服务器
func connectClient(name, address string, port int) *NATSClient {
	url := fmt.Sprintf("nats://%s:%d", address, port)
	conn, err := nats.Connect(url)
	if err != nil {
		panic(fmt.Sprintf("连接到%s服务器失败: %v", name, err))
	}
	fmt.Printf("客户端已连接到%s服务器\n", name)
	return &NATSClient{name: name, conn: conn, serverID: fmt.Sprintf("%s:%d", address, port)}
}

// 创建订阅
func subscribe(client *NATSClient, subject, response string) *nats.Subscription {
	sub, err := client.conn.Subscribe(subject, func(msg *nats.Msg) {
		msg.Respond([]byte(response))
	})
	if err != nil {
		panic(fmt.Sprintf("在%s客户端创建订阅失败: %v", client.name, err))
	}
	fmt.Printf("%s客户端已订阅主题 '%s'\n", client.name, subject)
	time.Sleep(100 * time.Millisecond) // 等待订阅生效
	return sub
}

// 发送请求并处理响应
func sendRequest(client *NATSClient, subject, requestData string, timeout time.Duration) {
	fmt.Printf("%s客户端发送请求: %s\n", client.name, requestData)
	resp, err := client.conn.Request(subject, []byte(requestData), timeout)
	if err != nil {
		fmt.Printf("请求失败: %v\n", err)
		return
	}
	fmt.Printf("收到响应: %s\n", string(resp.Data))
}

// 运行测试场景
func runScenario(responder, requester *NATSClient, subject, responseMsg, requestMsg string, timeout time.Duration) {
	// 在响应者上创建订阅
	sub := subscribe(responder, subject, responseMsg)
	defer sub.Drain()

	// 从请求者发送请求
	sendRequest(requester, subject, requestMsg, timeout)
}
