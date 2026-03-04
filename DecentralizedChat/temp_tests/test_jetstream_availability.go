// 验证 LeafNode 模式下 JetStream 的实际可用性
package leafnode_avail_test

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
		Cluster:  server.ClusterOpts{Port: -1, Name: "abc"},
		NoLog:    false,
		NoSigs:   true,
		Debug:    true,
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

// 测试 1: 独立 NATS Server（非 LeafNode）启用 JetStream - 应该工作
func TestStandaloneJetStream(t *testing.T) {
	t.Log("=== 测试 1: 独立 NATS Server + JetStream ===")

	opts := defaultOptions()
	opts.JetStreamMaxMemory = 256 * 1024 * 1024
	opts.JetStreamMaxStore = 1 * 1024 * 1024 * 1024
	opts.StoreDir = t.TempDir()

	s, err := startServer(opts)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer s.Shutdown()

	nc, err := nats.Connect(fmt.Sprintf("nats://%s:%d", testHost, opts.Port))
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer nc.Close()

	js, err := nc.JetStream()
	if err != nil {
		t.Fatalf("JetStream not available: %v", err)
	}
	t.Log("✅ JetStream context created")

	_, err = js.AddStream(&nats.StreamConfig{
		Name:     "TEST_STREAM",
		Subjects: []string{"test.>"},
	})
	if err != nil {
		t.Fatalf("Failed to create stream: %v", err)
	}
	t.Log("✅ Stream created successfully")

	t.Log("=== 测试 1 通过: 独立 NATS Server + JetStream 正常工作 ===")
}

// 测试 2: LeafNode (Spoke) 启用 JetStream - 看看是否工作
func TestLeafNodeJetStream(t *testing.T) {
	t.Log("=== 测试 2: LeafNode (Spoke) + JetStream ===")

	// 启动 Hub（不启用 JetStream）
	hubOpts := defaultOptions()
	hubOpts.LeafNode.Host = testHost
	hubOpts.LeafNode.Port = -1
	hubOpts.ServerName = "hub-server"

	hub, err := startServer(hubOpts)
	if err != nil {
		t.Fatalf("Failed to start hub: %v", err)
	}
	defer hub.Shutdown()

	t.Logf("Hub started - client: %d, leafnode: %d", hubOpts.Port, hubOpts.LeafNode.Port)

	// 启动 Spoke (LeafNode)，启用 JetStream
	spokeURL, _ := url.Parse(fmt.Sprintf("nats://%s:%d", testHost, hubOpts.LeafNode.Port))
	spokeOpts := defaultOptions()
	spokeOpts.Cluster.Host = testHost
	spokeOpts.Cluster.Port = -1  // 注意：没有配置 cluster port！
	spokeOpts.Cluster.Name = "spoke-cluster"
	spokeOpts.LeafNode.Host = testHost
	spokeOpts.LeafNode.Remotes = []*server.RemoteLeafOpts{{URLs: []*url.URL{spokeURL}}}
	spokeOpts.ServerName = "spoke-1"
	spokeOpts.JetStreamMaxMemory = 256 * 1024 * 1024
	spokeOpts.JetStreamMaxStore = 1 * 1024 * 1024 * 1024
	spokeOpts.StoreDir = t.TempDir()

	spoke, err := startServer(spokeOpts)
	if err != nil {
		t.Fatalf("Failed to start spoke: %v", err)
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

	// 连接到 Spoke 并测试 JetStream
	nc, err := nats.Connect(fmt.Sprintf("nats://%s:%d", testHost, spokeOpts.Port))
	if err != nil {
		t.Fatalf("Failed to connect to spoke: %v", err)
	}
	defer nc.Close()

	// 检查 JetStream 是否可用
	js, err := nc.JetStream()
	if err != nil {
		t.Fatalf("JetStream not available: %v", err)
	}
	t.Log("✅ JetStream context created (but will it work?)")

	// 尝试创建 Stream - 这是关键测试！
	t.Log("Attempting to create stream...")
	_, err = js.AddStream(&nats.StreamConfig{
		Name:     "SPOKE_STREAM",
		Subjects: []string{"spoke.>"},
	})
	if err != nil {
		t.Logf("❌ Failed to create stream: %v", err)
		t.Log("")
		t.Log("=== 问题分析 ===")
		t.Log("LeafNode 模式下，虽然配置了 JetStreamMaxMemory/JetStreamMaxStore，")
		t.Log("但 JetStream 可能没有真正启用！")
		t.Log("")
		t.Log("让我们检查一下服务器是否真正启用了 JetStream...")

		// 尝试用另一种方式检查
		accountInfo, err := nc.JetStream().AccountInfo()
		if err != nil {
			t.Logf("AccountInfo also failed: %v", err)
		} else {
			t.Logf("AccountInfo: %+v", accountInfo)
		}
	} else {
		t.Log("✅ Stream created successfully!")
		t.Log("=== 测试 2 通过: LeafNode + JetStream 正常工作 ===")
	}
}

// 测试 3: LeafNode + Cluster.Port 配置 + JetStream
func TestLeafNodeWithClusterPortJetStream(t *testing.T) {
	t.Log("=== 测试 3: LeafNode + Cluster.Port + JetStream ===")

	// 启动 Hub
	hubOpts := defaultOptions()
	hubOpts.LeafNode.Host = testHost
	hubOpts.LeafNode.Port = -1
	hubOpts.ServerName = "hub-server"

	hub, err := startServer(hubOpts)
	if err != nil {
		t.Fatalf("Failed to start hub: %v", err)
	}
	defer hub.Shutdown()

	t.Logf("Hub started - client: %d, leafnode: %d", hubOpts.Port, hubOpts.LeafNode.Port)

	// 启动 Spoke - 这次配置 Cluster.Port！
	spokeURL, _ := url.Parse(fmt.Sprintf("nats://%s:%d", testHost, hubOpts.LeafNode.Port))
	spokeOpts := defaultOptions()
	spokeOpts.Cluster.Host = testHost
	spokeOpts.Cluster.Port = -1  // 分配一个端口
	spokeOpts.Cluster.Name = "spoke-cluster"
	spokeOpts.LeafNode.Host = testHost
	spokeOpts.LeafNode.Remotes = []*server.RemoteLeafOpts{{URLs: []*url.URL{spokeURL}}}
	spokeOpts.ServerName = "spoke-1"
	spokeOpts.JetStreamMaxMemory = 256 * 1024 * 1024
	spokeOpts.JetStreamMaxStore = 1 * 1024 * 1024 * 1024
	spokeOpts.StoreDir = t.TempDir()

	spoke, err := startServer(spokeOpts)
	if err != nil {
		t.Fatalf("Failed to start spoke: %v", err)
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

	// 连接到 Spoke 并测试 JetStream
	nc, err := nats.Connect(fmt.Sprintf("nats://%s:%d", testHost, spokeOpts.Port))
	if err != nil {
		t.Fatalf("Failed to connect to spoke: %v", err)
	}
	defer nc.Close()

	js, err := nc.JetStream()
	if err != nil {
		t.Fatalf("JetStream not available: %v", err)
	}
	t.Log("✅ JetStream context created")

	_, err = js.AddStream(&nats.StreamConfig{
		Name:     "SPOKE_STREAM_2",
		Subjects: []string{"spoke2.>"},
	})
	if err != nil {
		t.Logf("❌ Failed to create stream: %v", err)
	} else {
		t.Log("✅ Stream created successfully!")
		t.Log("=== 测试 3 通过: LeafNode + Cluster.Port + JetStream 正常工作 ===")
	}
}

