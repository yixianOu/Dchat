package routes

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

// CreateNode 创建一个NATS节点
func CreateNode(name string, clientPort int, routes []string) *RouteNode {
	clusterPort := clientPort + 2000 // cluster端口 = client端口 + 2000

	opts := &server.Options{
		ServerName: name,
		Host:       "127.0.0.1",
		Port:       clientPort,
		Cluster: server.ClusterOpts{
			Name: "dchat_network", // 改为dchat网络名称
			Host: "127.0.0.1",
			Port: clusterPort,
		},
	}

	// 设置Routes连接
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

	return &RouteNode{
		ID:     name,
		Server: srv,
		Port:   clientPort,
		Routes: routes,
	}
}

// StartNode 启动节点
func (node *RouteNode) Start() error {
	go node.Server.Start()

	// 等待服务器启动
	if !node.Server.ReadyForConnections(5 * time.Second) {
		return fmt.Errorf("node %s failed to start", node.ID)
	}

	return nil
}

// Stop 停止节点
func (node *RouteNode) Stop() {
	if node.Server != nil {
		node.Server.Shutdown()
	}
}

// IsRunning 检查节点是否运行中
func (node *RouteNode) IsRunning() bool {
	return node.Server != nil && node.Server.Running()
}

// GetRouteCount 获取路由连接数
func (node *RouteNode) GetRouteCount() int {
	if node.Server == nil {
		return 0
	}
	return node.Server.NumRoutes()
}

// GetClientURL 获取客户端连接URL
func (node *RouteNode) GetClientURL() string {
	return fmt.Sprintf("nats://127.0.0.1:%d", node.Port)
}

// ConnectClient 连接客户端
func (node *RouteNode) ConnectClient(clientName string) (*nats.Conn, error) {
	url := node.GetClientURL()

	nc, err := nats.Connect(url,
		nats.Name(clientName),
		nats.ReconnectWait(time.Second),
		nats.MaxReconnects(-1),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect client %s to %s: %w", clientName, node.ID, err)
	}

	return nc, nil
}

// TestMessageRouting 测试节点间消息路由
func TestMessageRouting(sender, receiver *RouteNode) error {
	// 连接发送端和接收端
	senderClient, err := sender.ConnectClient("Sender")
	if err != nil {
		return err
	}
	defer senderClient.Close()

	receiverClient, err := receiver.ConnectClient("Receiver")
	if err != nil {
		return err
	}
	defer receiverClient.Close()

	// 设置接收端订阅
	msgChan := make(chan string, 1)
	sub, err := receiverClient.Subscribe("test.routing", func(msg *nats.Msg) {
		msgChan <- string(msg.Data)
	})
	if err != nil {
		return err
	}
	defer sub.Unsubscribe()

	// 等待订阅传播
	time.Sleep(200 * time.Millisecond)

	// 发送测试消息
	testMsg := "Hello Routes!"
	err = senderClient.Publish("test.routing", []byte(testMsg))
	if err != nil {
		return err
	}

	// 检查是否收到消息
	select {
	case received := <-msgChan:
		if received == testMsg {
			return nil // 成功
		}
		return fmt.Errorf("message mismatch: expected %s, got %s", testMsg, received)
	case <-time.After(2 * time.Second):
		return fmt.Errorf("timeout: message not received")
	}
}
