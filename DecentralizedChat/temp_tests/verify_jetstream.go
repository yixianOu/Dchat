// 验证 LeafNode 模式下 JetStream 是否真的能用
package main

import (
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats-server/v2/server"
)

const testHost = "127.0.0.1"

func startServer(opts *server.Options) (*server.Server, error) {
	s, err := server.NewServer(opts)
	if err != nil || s == nil {
		return nil, fmt.Errorf("no NATS Server object returned: %v", err)
	}
	s.Start()
	if !s.ReadyForConnections(10 * time.Second) {
		return nil, fmt.Errorf("server not ready for connections")
	}
	return s, nil
}

func defaultOptions() *server.Options {
	return &server.Options{
		Host:     testHost,
		Port:     -1,
		HTTPPort: -1,
		Cluster:  server.ClusterOpts{Port: -1, Name: "test-cluster"},
		NoLog:    false,
		NoSigs:   true,
		Debug:    false,
		Trace:    false,
	}
}

func checkFor(t *testing.T, timeout, wait time.Duration, check func() error) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if err := check(); err == nil {
			return
		}
		time.Sleep(wait)
	}
	if err := check(); err != nil {
		t.Fatalf("Timed out waiting for condition: %v", err)
	}
}

// 测试1: 独立 NATS Server + JetStream (非 LeafNode) - 应该能工作
func Test_StandaloneJetStream_Works(t *testing.T) {
	t.Log("=== 测试1: 独立 NATS Server + JetStream ===")

	opts := defaultOptions()
	opts.JetStream = true  // 关键：显式启用
	opts.JetStreamMaxMemory = 256 * 1024 * 1024
	opts.JetStreamMaxStore = 1 * 1024 * 1024 * 1024
	opts.StoreDir = t.TempDir()

	s, err := startServer(opts)
	if err != nil {
		t.Fatalf("start server: %v", err)
	}
	defer s.Shutdown()

	nc, err := nats.Connect(fmt.Sprintf("nats://%s:%d", testHost, opts.Port))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer nc.Close()

	js, err := nc.JetStream()
	if err != nil {
		t.Fatalf("JetStream: %v", err)
	}
	t.Log("✅ JetStream context created")

	// 创建 Stream
	_, err = js.AddStream(&nats.StreamConfig{
		Name:     "TEST_STREAM",
		Subjects: []string{"test.>"},
		Storage:  nats.FileStorage,
	})
	if err != nil {
		t.Fatalf("AddStream: %v", err)
	}
	t.Log("✅ Stream created")

	// 发布消息
	pubAck, err := js.Publish("test.msg", []byte("hello jetstream"))
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	t.Logf("✅ Message published, seq=%d", pubAck.Sequence)

	// 读取消息
	sub, err := js.PullSubscribe("test.msg", "consumer",
		nats.Bind("TEST_STREAM", "consumer"),
		nats.DeliverAll())
	if err != nil {
		t.Fatalf("PullSubscribe: %v", err)
	}
	defer sub.Unsubscribe()

	msgs, err := sub.Fetch(1, nats.MaxWait(2*time.Second))
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	for _, m := range msgs {
		t.Logf("✅ Received: %q", string(m.Data))
		m.Ack()
	}

	t.Log("=== 测试1通过: 独立 NATS Server + JetStream 完全正常 ===")
}

// 测试2: LeafNode (Spoke) 本地启用 JetStream
func Test_LeafNodeLocalJetStream(t *testing.T) {
	t.Log("=== 测试2: LeafNode (Spoke) 本地 JetStream ===")

	// 启动 Hub (不需要 JetStream)
	hubOpts := defaultOptions()
	hubOpts.LeafNode.Host = testHost
	hubOpts.LeafNode.Port = -1
	hubOpts.ServerName = "hub-server"

	hub, err := startServer(hubOpts)
	if err != nil {
		t.Fatalf("start hub: %v", err)
	}
	defer hub.Shutdown()
	t.Logf("Hub started - client: %d, leafnode: %d", hubOpts.Port, hubOpts.LeafNode.Port)

	// 启动 Spoke (LeafNode)，启用 JetStream
	spokeURL, _ := url.Parse(fmt.Sprintf("nats://%s:%d", testHost, hubOpts.LeafNode.Port))
	spokeOpts := defaultOptions()
	spokeOpts.Cluster.Host = testHost
	spokeOpts.Cluster.Port = -1  // 不配置 cluster port (单机模式)
	spokeOpts.Cluster.Name = "spoke-cluster"
	spokeOpts.LeafNode.Host = testHost
	spokeOpts.LeafNode.Remotes = []*server.RemoteLeafOpts{{URLs: []*url.URL{spokeURL}}}
	spokeOpts.ServerName = "spoke-1"

	// 关键：JetStream 配置
	spokeOpts.JetStream = true  // 显式启用！
	spokeOpts.JetStreamMaxMemory = 256 * 1024 * 1024
	spokeOpts.JetStreamMaxStore = 1 * 1024 * 1024 * 1024
	spokeOpts.StoreDir = t.TempDir()

	spoke, err := startServer(spokeOpts)
	if err != nil {
		t.Fatalf("start spoke: %v", err)
	}
	defer spoke.Shutdown()

	// 等待 LeafNode 连接
	checkFor(t, 10*time.Second, 100*time.Millisecond, func() error {
		if n := hub.NumLeafNodes(); n != 1 {
			return fmt.Errorf("hub has %d leafnodes, want 1", n)
		}
		return nil
	})
	t.Log("✅ Spoke connected to Hub")

	// 连接到 Spoke 本地
	nc, err := nats.Connect(fmt.Sprintf("nats://%s:%d", testHost, spokeOpts.Port))
	if err != nil {
		t.Fatalf("connect to spoke: %v", err)
	}
	defer nc.Close()

	// 测试 JetStream
	js, err := nc.JetStream()
	if err != nil {
		t.Fatalf("JetStream: %v", err)
	}
	t.Log("✅ JetStream context created")

	// 创建 Stream
	_, err = js.AddStream(&nats.StreamConfig{
		Name:     "SPOKE_STREAM",
		Subjects: []string{"spoke.>"},
		Storage:  nats.FileStorage,
	})
	if err != nil {
		t.Fatalf("AddStream: %v", err)
	}
	t.Log("✅ Stream created on LeafNode!")

	// 发布消息
	pubAck, err := js.Publish("spoke.msg", []byte("hello from leafnode jetstream"))
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	t.Logf("✅ Message published, seq=%d", pubAck.Sequence)

	// 读取消息
	sub, err := js.PullSubscribe("spoke.msg", "consumer",
		nats.Bind("SPOKE_STREAM", "consumer"),
		nats.DeliverAll())
	if err != nil {
		t.Fatalf("PullSubscribe: %v", err)
	}
	defer sub.Unsubscribe()

	msgs, err := sub.Fetch(1, nats.MaxWait(2*time.Second))
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	for _, m := range msgs {
		t.Logf("✅ Received from LeafNode JetStream: %q", string(m.Data))
		m.Ack()
	}

	t.Log("=== 测试2通过: LeafNode 本地 JetStream 完全正常！ ===")
	t.Log("")
	t.Log("关键点:")
	t.Log("  1. 必须设置 opts.JetStream = true")
	t.Log("  2. 数据只存在于 LeafNode 本地，不同步到 Hub")
	t.Log("  3. LeafNode 连接 Hub 用于普通消息通信")
	t.Log("  4. JetStream 是本地独立的")
}

// 测试3: 验证数据隔离 - Spoke 的 Stream Hub 看不到
func Test_LeafNodeJetStream_Isolation(t *testing.T) {
	t.Log("=== 测试3: LeafNode JetStream 数据隔离 ===")

	// 启动 Hub (启用 JetStream)
	hubOpts := defaultOptions()
	hubOpts.LeafNode.Host = testHost
	hubOpts.LeafNode.Port = -1
	hubOpts.ServerName = "hub-server"
	hubOpts.JetStream = true
	hubOpts.JetStreamMaxMemory = 256 * 1024 * 1024
	hubOpts.JetStreamMaxStore = 1 * 1024 * 1024 * 1024
	hubOpts.StoreDir = t.TempDir()

	hub, err := startServer(hubOpts)
	if err != nil {
		t.Fatalf("start hub: %v", err)
	}
	defer hub.Shutdown()

	// 启动 Spoke
	spokeURL, _ := url.Parse(fmt.Sprintf("nats://%s:%d", testHost, hubOpts.LeafNode.Port))
	spokeOpts := defaultOptions()
	spokeOpts.Cluster.Host = testHost
	spokeOpts.Cluster.Port = -1
	spokeOpts.LeafNode.Host = testHost
	spokeOpts.LeafNode.Remotes = []*server.RemoteLeafOpts{{URLs: []*url.URL{spokeURL}}}
	spokeOpts.ServerName = "spoke-1"
	spokeOpts.JetStream = true
	spokeOpts.JetStreamMaxMemory = 256 * 1024 * 1024
	spokeOpts.JetStreamMaxStore = 1 * 1024 * 1024 * 1024
	spokeOpts.StoreDir = t.TempDir()

	spoke, err := startServer(spokeOpts)
	if err != nil {
		t.Fatalf("start spoke: %v", err)
	}
	defer spoke.Shutdown()

	checkFor(t, 10*time.Second, 100*time.Millisecond, func() error {
		if n := hub.NumLeafNodes(); n != 1 {
			return fmt.Errorf("hub has %d leafnodes, want 1", n)
		}
		return nil
	})

	// 在 Spoke 创建 Stream
	ncSpoke, _ := nats.Connect(fmt.Sprintf("nats://%s:%d", testHost, spokeOpts.Port))
	defer ncSpoke.Close()
	jsSpoke, _ := ncSpoke.JetStream()
	_, err = jsSpoke.AddStream(&nats.StreamConfig{
		Name:     "SPOKE_ONLY",
		Subjects: []string{"spoke.only.>"},
	})
	if err != nil {
		t.Fatalf("AddStream on spoke: %v", err)
	}
	t.Log("✅ Stream created on Spoke")

	// 在 Hub 尝试查看这个 Stream - 应该看不到
	ncHub, _ := nats.Connect(fmt.Sprintf("nats://%s:%d", testHost, hubOpts.Port))
	defer ncHub.Close()
	jsHub, _ := ncHub.JetStream()
	_, err = jsHub.StreamInfo("SPOKE_ONLY")
	if err == nil {
		t.Fatal("Hub should NOT see Spoke's Stream!")
	}
	t.Log("✅ Hub cannot see Spoke's Stream (isolation works)")

	t.Log("=== 测试3通过: 数据隔离正常 ===")
}

func main() {
	fmt.Println("Run with: go test -v -timeout 60s verify_jetstream.go")
}
