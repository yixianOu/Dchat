// E2E 集成测试：两个 LeafNode 通过公网 Hub 通信
package e2e_test

import (
	"testing"
	"time"

	"DecentralizedChat/internal/config"
	"DecentralizedChat/internal/leafnode"
	internal_nats "DecentralizedChat/internal/nats"
	"DecentralizedChat/internal/nscsetup"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 测试常量
const (
	publicHubLeafURL = "nats://121.199.173.116:7422"
	testSubject      = "dchat.test.p2p.communication"
)

func TestLeafNode_P2P_Through_Public_Hub_E2E(t *testing.T) {
	t.Log("=== E2E 测试: 两个 LeafNode 通过公网 Hub 通信 ===")
	t.Log("")

	// ========== Step 1: 启动第一个 LeafNode 节点 A ==========
	t.Log("Step 1: 启动 LeafNode 节点 A...")
  // 先初始化NSC配置得到creds路径
  cfg, _ := config.LoadConfig()
  _ = nscsetup.EnsureSimpleSetup(cfg)
	cfgA := &config.LeafNodeConfig{
		LocalHost:      testHost,
		LocalPort:      42222, // 固定端口避免冲突
		HubURLs:        []string{publicHubLeafURL},
		CredsFile:  cfg.Keys.UserCredsPath,
		ConnectTimeout: 15 * time.Second, // 公网环境增加超时时间
	}

	mgrA := leafnode.NewManager(cfgA)
	err := mgrA.Start()
	require.NoError(t, err, "启动 LeafNode A 失败")
	defer mgrA.Stop()

	require.True(t, mgrA.IsRunning(), "LeafNode A 应该正在运行")
	urlA := mgrA.GetLocalNATSURL()
	t.Logf("✅ LeafNode A 启动成功，本地地址: %s", urlA)

	// 等待连接到 Hub
	time.Sleep(3 * time.Second)

	// ========== Step 2: 启动第二个 LeafNode 节点 B ==========
	t.Log("\nStep 2: 启动 LeafNode 节点 B...")
	cfgB := &config.LeafNodeConfig{
		LocalHost:      testHost,
		LocalPort:      42223, // 固定端口避免冲突
		HubURLs:        []string{publicHubLeafURL},
		CredsFile:  cfg.Keys.UserCredsPath,
		ConnectTimeout: 15 * time.Second,
	}

	mgrB := leafnode.NewManager(cfgB)
	err = mgrB.Start()
	require.NoError(t, err, "启动 LeafNode B 失败")
	defer mgrB.Stop()

	require.True(t, mgrB.IsRunning(), "LeafNode B 应该正在运行")
	urlB := mgrB.GetLocalNATSURL()
	t.Logf("✅ LeafNode B 启动成功，本地地址: %s", urlB)

	// 等待连接到 Hub 和路由同步
	time.Sleep(5 * time.Second)
	t.Log("✅ 两个 LeafNode 均已连接到公网 Hub")

	// ========== Step 3: 测试跨节点通信 ==========
	t.Log("\nStep 3: 测试跨节点消息通信...")

	// 连接到节点A，订阅测试主题
	clientA, err := internal_nats.NewService(internal_nats.ClientConfig{
		URL:     urlA,
		Timeout: 5 * time.Second,
	})
	require.NoError(t, err, "连接 LeafNode A 失败")
	defer clientA.Close()
	t.Log("✅ 客户端已连接到 LeafNode A")

	// 连接到节点B，用于发布消息
	clientB, err := internal_nats.NewService(internal_nats.ClientConfig{
		URL:     urlB,
		Timeout: 5 * time.Second,
	})
	require.NoError(t, err, "连接 LeafNode B 失败")
	defer clientB.Close()
	t.Log("✅ 客户端已连接到 LeafNode B")

	// 节点A订阅主题
	received := make(chan string, 1)
	err = clientA.Subscribe(testSubject, func(msg *nats.Msg) {
		t.Logf("📥 收到消息: %q", string(msg.Data))
		received <- string(msg.Data)
	})
	require.NoError(t, err, "订阅主题失败")
	t.Logf("✅ LeafNode A 已订阅主题: %s", testSubject)

	// 等待订阅同步
	time.Sleep(2 * time.Second)

	// 节点B发布消息
	testMsg := "Hello from LeafNode B through public Hub!"
	err = clientB.Publish(testSubject, []byte(testMsg))
	require.NoError(t, err, "发布消息失败")
	t.Logf("✅ LeafNode B 已发布消息: %q", testMsg)

	// 等待接收消息
	select {
	case receivedMsg := <-received:
		assert.Equal(t, testMsg, receivedMsg, "收到的消息内容不匹配")
		t.Log("✅ 跨 LeafNode 通信成功！消息通过公网 Hub 正常转发")
	case <-time.After(10 * time.Second): // 公网环境增加超时时间
		t.Fatal("❌ 等待消息超时，跨节点通信失败")
	}

	t.Log("\n=== 测试全部通过 ✅ ===")
	t.Log("")
	t.Log("✅ 两个 LeafNode 成功连接到公网 Hub")
	t.Log("✅ 跨节点消息通过 Hub 正常路由转发")
	t.Log("✅ P2P 通信功能正常")
}
