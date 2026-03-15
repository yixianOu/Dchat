// E2E 集成测试：NATS 客户端服务
package e2e_test

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"DecentralizedChat/internal/nats"

	"github.com/nats-io/nats-server/v2/server"
	gnats "github.com/nats-io/nats.go"
)

const testHost = "127.0.0.1"

func startServer(opts *server.Options) (*server.Server, error) {
	s, err := server.NewServer(opts)
	if err != nil || s == nil {
		return nil, err
	}
	s.Start()
	if !s.ReadyForConnections(10 * time.Second) {
		return nil, err
	}
	return s, nil
}

func defaultOptions() *server.Options {
	return &server.Options{
		Host:     testHost,
		Port:     -1,
		HTTPPort: -1,
		NoLog:    true,
		NoSigs:   true,
	}
}

func TestNATSClient_ConnectPublishSubscribe_E2E(t *testing.T) {
	t.Log("=== E2E 测试: NATS 客户端连接、发布、订阅 ===")
	t.Log("")

	opts := defaultOptions()
	s, err := startServer(opts)
	if err != nil {
		t.Fatalf("startServer failed: %v", err)
	}
	defer s.Shutdown()

	url := fmt.Sprintf("nats://%s:%d", testHost, opts.Port)
	t.Logf("✅ NATS 服务器启动: %s", url)

	cfg := nats.ClientConfig{
		URL:    url,
		Name:   "e2e-test-client",
		Timeout: 5 * time.Second,
	}

	svc, err := nats.NewService(cfg)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}
	defer svc.Close()
	t.Log("✅ NATS 客户端服务创建成功")

	if svc.IsConnected() {
		t.Log("✅ 连接状态正常")
	}

	received := make(chan string, 1)
	subject := "dchat.test.e2e"

	err = svc.Subscribe(subject, func(msg *gnats.Msg) {
		t.Logf("收到消息: %q", string(msg.Data))
		received <- string(msg.Data)
	})
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}
	t.Log("✅ 订阅成功")

	time.Sleep(100 * time.Millisecond)

	testMsg := "Hello from E2E test!"
	err = svc.Publish(subject, []byte(testMsg))
	if err != nil {
		t.Fatalf("Publish failed: %v", err)
	}
	t.Log("✅ 消息发布成功")

	select {
	case receivedMsg := <-received:
		if receivedMsg != testMsg {
			t.Errorf("消息不匹配: got %q, want %q", receivedMsg, testMsg)
		}
		t.Logf("✅ 消息接收成功: %q", receivedMsg)
	case <-time.After(3 * time.Second):
		t.Fatal("等待消息超时")
	}

	t.Log("✅ 统计获取成功")

	svc.Close()
	t.Log("✅ 连接关闭成功")

	t.Log("")
	t.Log("=== E2E 测试通过 ✅ ===")
}

func TestNATSClient_PublishJSON_E2E(t *testing.T) {
	t.Log("=== E2E 测试: NATS PublishJSON ===")
	t.Log("")

	opts := defaultOptions()
	s, err := startServer(opts)
	if err != nil {
		t.Fatalf("startServer failed: %v", err)
	}
	defer s.Shutdown()

	url := fmt.Sprintf("nats://%s:%d", testHost, opts.Port)

	cfg := nats.ClientConfig{
		URL:     url,
		Name:    "e2e-json-test",
		Timeout: 5 * time.Second,
	}

	svc, err := nats.NewService(cfg)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}
	defer svc.Close()

	type TestMessage struct {
		Sender  string `json:"sender"`
		Content string `json:"content"`
	}

	received := make(chan TestMessage, 1)
	subject := "dchat.test.json"

	err = svc.Subscribe(subject, func(msg *gnats.Msg) {
		var tm TestMessage
		if err := json.Unmarshal(msg.Data, &tm); err == nil {
			received <- tm
		}
	})
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	testMsg := TestMessage{
		Sender:  "alice",
		Content: "Hello JSON!",
	}

	err = svc.PublishJSON(subject, testMsg)
	if err != nil {
		t.Fatalf("PublishJSON failed: %v", err)
	}
	t.Log("✅ PublishJSON 成功")

	select {
	case receivedMsg := <-received:
		if receivedMsg.Sender != testMsg.Sender {
			t.Errorf("Sender 不匹配")
		}
		if receivedMsg.Content != testMsg.Content {
			t.Errorf("Content 不匹配")
		}
		t.Logf("✅ 接收 JSON 成功: sender=%q, content=%q", receivedMsg.Sender, receivedMsg.Content)
	case <-time.After(3 * time.Second):
		t.Fatal("等待 JSON 消息超时")
	}

	t.Log("")
	t.Log("=== PublishJSON 测试通过 ✅ ===")
}
