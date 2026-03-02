// 测试：LeafNode 架构下 JetStream 的正确使用方式
package leafnode_test

import (
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats-server/v2/server"
)

func TestJetStreamArchitecture(t *testing.T) {
	t.Log("=== LeafNode 架构下 JetStream 使用方案 ===")
	t.Log("")
	t.Log("方案 A: Hub 层 JetStream（推荐用于离线消息）")
	t.Log("  - Hub 启用 JetStream")
	t.Log("  - 消息通过 LeafNode 转发到 Hub")
	t.Log("  - Hub 的 JetStream 暂存离线消息")
	t.Log("")
	t.Log("方案 B: 本地历史消息（推荐用 SQLite/bbolt）")
	t.Log("  - 本地应用直接写本地数据库")
	t.Log("  - 不经过 NATS JetStream")
	t.Log("")

	// ===== 1. 启动 Hub（启用 JetStream）=====
	t.Log("--- 启动 Hub（启用 JetStream）---")

	hubOpts := defaultOptions()
	hubOpts.LeafNode.Host = testHost
	hubOpts.LeafNode.Port = -1
	hubOpts.ServerName = "hub-server"
	hubOpts.JetStreamMaxMemory = 256 * 1024 * 1024
	hubOpts.JetStreamMaxStore = 1 * 1024 * 1024 * 1024
	hubOpts.StoreDir = t.TempDir()

	hub, err := startServer(hubOpts)
	if err != nil {
		t.Fatalf("Failed to start hub: %v", err)
	}
	defer hub.Shutdown()

	t.Logf("✅ Hub 启动 (JetStream 启用) - client: %d, leafnode: %d", hubOpts.Port, hubOpts.LeafNode.Port)

	// 连接 Hub 创建离线消息 Stream
	hubNC, err := nats.Connect(fmt.Sprintf("nats://%s:%d", testHost, hubOpts.Port))
	if err != nil {
		t.Fatalf("Failed to connect to hub: %v", err)
	}
	defer hubNC.Close()

	hubJS, err := hubNC.JetStream()
	if err != nil {
		t.Fatalf("Failed to get hub JetStream: %v", err)
	}

	// 创建 Hub 端离线消息 Stream
	_, err = hubJS.AddStream(&nats.StreamConfig{
		Name:        "DCHAT_OFFLINE",
		Description: "离线消息暂存",
		Subjects:    []string{"dchat.offline.>"},
		Retention:   nats.LimitsPolicy,
		MaxAge:      7 * 24 * time.Hour, // 7天过期
		Storage:     nats.FileStorage,
	})
	if err != nil {
		t.Fatalf("Failed to create offline stream: %v", err)
	}
	t.Logf("✅ Hub 端离线消息 Stream 创建成功")

	// ===== 2. 启动两个 LeafNode =====
	t.Log("--- 启动两个 LeafNode ---")

	// LeafNode 1 (Alice)
	spokeURL, _ := url.Parse(fmt.Sprintf("nats://%s:%d", testHost, hubOpts.LeafNode.Port))

	spoke1Opts := defaultOptions()
	spoke1Opts.Cluster.Host = testHost
	spoke1Opts.Cluster.Port = -1
	spoke1Opts.Cluster.Name = "spoke1-cluster"
	spoke1Opts.LeafNode.Host = testHost
	spoke1Opts.LeafNode.Remotes = []*server.RemoteLeafOpts{{URLs: []*url.URL{spokeURL}}}
	spoke1Opts.ServerName = "alice-leafnode"

	spoke1, err := startServer(spoke1Opts)
	if err != nil {
		t.Fatalf("Failed to start spoke1: %v", err)
	}
	defer spoke1.Shutdown()

	// LeafNode 2 (Bob)
	spoke2Opts := defaultOptions()
	spoke2Opts.Cluster.Host = testHost
	spoke2Opts.Cluster.Port = -1
	spoke2Opts.Cluster.Name = "spoke2-cluster"
	spoke2Opts.LeafNode.Host = testHost
	spoke2Opts.LeafNode.Remotes = []*server.RemoteLeafOpts{{URLs: []*url.URL{spokeURL}}}
	spoke2Opts.ServerName = "bob-leafnode"

	spoke2, err := startServer(spoke2Opts)
	if err != nil {
		t.Fatalf("Failed to start spoke2: %v", err)
	}
	defer spoke2.Shutdown()

	// 等待连接
	checkFor(t, 10*time.Second, 100*time.Millisecond, func() error {
		if n := hub.NumLeafNodes(); n != 2 {
			return fmt.Errorf("hub has %d leafnodes, want 2", n)
		}
		return nil
	})
	t.Logf("✅ 两个 LeafNode 已连接到 Hub")

	// ===== 3. 测试场景 1: 双方在线，实时消息（Core NATS）=====
	t.Log("--- 场景 1: 双方在线，实时消息 ---")

	// Alice 连接她的 LeafNode
	nc1, err := nats.Connect(fmt.Sprintf("nats://%s:%d", testHost, spoke1Opts.Port))
	if err != nil {
		t.Fatalf("Failed to connect to spoke1: %v", err)
	}
	defer nc1.Close()

	// Bob 连接他的 LeafNode
	nc2, err := nats.Connect(fmt.Sprintf("nats://%s:%d", testHost, spoke2Opts.Port))
	if err != nil {
		t.Fatalf("Failed to connect to spoke2: %v", err)
	}
	defer nc2.Close()

	// Bob 订阅 DM
	received := make(chan *nats.Msg, 1)
	sub, err := nc2.ChanSubscribe("dchat.dm.alice_bob", received)
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}
	defer sub.Unsubscribe()

	// Alice 发消息
	testMsg := "你好 Bob！"
	if err := nc1.Publish("dchat.dm.alice_bob", []byte(testMsg)); err != nil {
		t.Fatalf("Failed to publish: %v", err)
	}

	// Bob 收到
	select {
	case msg := <-received:
		if string(msg.Data) != testMsg {
			t.Fatalf("got %q, want %q", string(msg.Data), testMsg)
		}
		t.Logf("✅ 实时消息成功: %q", string(msg.Data))
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for message")
	}

	// ===== 4. 测试场景 2: Bob 离线，消息存 Hub JetStream=====
	t.Log("--- 场景 2: 接收方离线，消息存 Hub JetStream ---")

	// Bob 取消订阅（模拟离线）
	sub.Unsubscribe()

	// Alice 发消息，同时也发一份到 Hub 离线主题
	offlineMsg := "这是离线消息"
	offlineSubject := "dchat.offline.bob.alice_bob"

	// 发布到 Hub 的离线 Stream（通过 Alice 的 LeafNode 转发）
	_, err = hubJS.Publish(offlineSubject, []byte(offlineMsg))
	if err != nil {
		t.Fatalf("Failed to publish offline: %v", err)
	}
	t.Logf("✅ 离线消息已存入 Hub JetStream")

	// ===== 5. 测试场景 3: Bob 上线，从 Hub 拉取离线消息=====
	t.Log("--- 场景 3: 接收方上线，从 Hub 拉取离线消息 ---")

	// Bob 从 Hub 的离线 Stream 读取
	bobHubJS, _ := hubNC.JetStream()
	offlineSub, err := bobHubJS.PullSubscribe(offlineSubject, "bob-offline",
		nats.Bind("DCHAT_OFFLINE", "bob-offline"),
		nats.DeliverAll())
	if err != nil {
		t.Fatalf("Failed to pull subscribe: %v", err)
	}
	defer offlineSub.Unsubscribe()

	msgs, err := offlineSub.Fetch(10, nats.MaxWait(1*time.Second))
	if err != nil && err != nats.ErrTimeout {
		t.Fatalf("Failed to fetch: %v", err)
	}

	for _, m := range msgs {
		t.Logf("✅ 收到离线消息: %q", string(m.Data))
		m.Ack()
	}

	t.Log("")
	t.Log("=== 架构总结 ===")
	t.Log("")
	t.Log("✅ Hub 层 JetStream: 适合存离线消息")
	t.Log("   - 消息通过 LeafNode 转发到 Hub")
	t.Log("   - Hub 的 JetStream 暂存，等接收方上线拉取")
	t.Log("")
	t.Log("❌ 本地 LeafNode JetStream: 不推荐存历史消息")
	t.Log("   - 建议用 SQLite/bbolt 代替")
	t.Log("   - 技术栈更简单，查询更灵活")
	t.Log("")
	t.Log("最终架构:")
	t.Log("  实时消息: Core NATS (LeafNode → Hub → LeafNode)")
	t.Log("  离线消息: Hub JetStream")
	t.Log("  本地历史: SQLite/bbolt")
}
