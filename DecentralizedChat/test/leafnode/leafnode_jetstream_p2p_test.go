// E2E 集成测试：两个开启JetStream的LeafNode通过公网Hub通信
package e2e_test

import (
	"testing"
	"time"

	"DecentralizedChat/internal/config"
	"DecentralizedChat/internal/leafnode"
	"DecentralizedChat/internal/nscsetup"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 测试常量
const (
	publicHubLeafURLJetStream = "nats://121.199.173.116:7422"
	testJetStreamSubject      = "dchat.test.jetstream.p2p"
	testStreamName            = "TEST_STREAM_P2P"
)

func TestLeafNode_JetStream_P2P_Through_Hub_E2E(t *testing.T) {
	t.Log("=== E2E 测试: 两个开启JetStream的LeafNode通过公网Hub通信 ===")
	t.Log("")

	// ========== Step 0: 初始化公共配置 ==========
	cfg, _ := config.LoadConfig()
	_ = nscsetup.EnsureSimpleSetup(cfg)
	testHost := "127.0.0.1"
	tempDir := t.TempDir()

	// ========== Step 1: 启动第一个 LeafNode 节点 A (开启JetStream) ==========
	t.Log("Step 1: 启动 LeafNode 节点 A (开启JetStream)...")
	cfgA := &config.LeafNodeConfig{
		LocalHost:         testHost,
		LocalPort:         42230, // 固定端口避免冲突
		HubURLs:           []string{publicHubLeafURLJetStream},
		CredsFile:         cfg.Keys.UserCredsPath,
		ConnectTimeout:    15 * time.Second,
		EnableJetStream:   true,
		JetStreamStoreDir: tempDir + "/nodeA",
	}

	mgrA := leafnode.NewManager(cfgA)
	err := mgrA.Start()
	require.NoError(t, err, "启动带JetStream的LeafNode A失败")
	defer mgrA.Stop()

	require.True(t, mgrA.IsRunning(), "LeafNode A 应该正在运行")
	urlA := mgrA.GetLocalNATSURL()
	t.Logf("✅ LeafNode A 启动成功，本地地址: %s，JetStream已开启", urlA)

	// 等待连接到 Hub
	time.Sleep(3 * time.Second)

	// ========== Step 2: 启动第二个 LeafNode 节点 B (开启JetStream) ==========
	t.Log("\nStep 2: 启动 LeafNode 节点 B (开启JetStream)...")
	cfgB := &config.LeafNodeConfig{
		LocalHost:         testHost,
		LocalPort:         42231, // 固定端口避免冲突
		HubURLs:           []string{publicHubLeafURLJetStream},
		CredsFile:         cfg.Keys.UserCredsPath,
		ConnectTimeout:    15 * time.Second,
		EnableJetStream:   true,
		JetStreamStoreDir: tempDir + "/nodeB",
	}

	mgrB := leafnode.NewManager(cfgB)
	err = mgrB.Start()
	require.NoError(t, err, "启动带JetStream的LeafNode B失败")
	defer mgrB.Stop()

	require.True(t, mgrB.IsRunning(), "LeafNode B 应该正在运行")
	urlB := mgrB.GetLocalNATSURL()
	t.Logf("✅ LeafNode B 启动成功，本地地址: %s，JetStream已开启", urlB)

	// 等待连接到 Hub 和路由同步
	time.Sleep(5 * time.Second)
	t.Log("✅ 两个开启JetStream的LeafNode均已连接到公网Hub")

	// ========== Step 3: 测试跨节点基础通信（验证P2P功能不受JetStream影响） ==========
	t.Log("\nStep 3: 测试跨节点基础消息通信...")

	// 连接到节点A，订阅测试主题
	ncA, err := nats.Connect(urlA, nats.Timeout(5*time.Second))
	require.NoError(t, err, "连接 LeafNode A 失败")
	defer ncA.Close()
	t.Log("✅ 客户端已连接到 LeafNode A")

	// 连接到节点B，用于发布消息
	ncB, err := nats.Connect(urlB, nats.Timeout(5*time.Second))
	require.NoError(t, err, "连接 LeafNode B 失败")
	defer ncB.Close()
	t.Log("✅ 客户端已连接到 LeafNode B")

	// 节点A订阅主题
	received := make(chan string, 1)
	_, err = ncA.Subscribe(testJetStreamSubject, func(msg *nats.Msg) {
		t.Logf("📥 收到实时消息: %q", string(msg.Data))
		received <- string(msg.Data)
	})
	require.NoError(t, err, "订阅主题失败")
	t.Logf("✅ LeafNode A 已订阅主题: %s", testJetStreamSubject)

	// 等待订阅同步
	time.Sleep(2 * time.Second)

	// 节点B发布消息
	testMsg := "Hello from LeafNode B with JetStream through Hub!"
	err = ncB.Publish(testJetStreamSubject, []byte(testMsg))
	require.NoError(t, err, "发布消息失败")
	t.Logf("✅ LeafNode B 已发布消息: %q", testMsg)

	// 等待接收消息
	select {
	case receivedMsg := <-received:
		assert.Equal(t, testMsg, receivedMsg, "收到的消息内容不匹配")
		t.Log("✅ 跨LeafNode通信成功！开启JetStream不影响P2P消息转发")
	case <-time.After(10 * time.Second): // 公网环境增加超时时间
		t.Fatal("❌ 等待消息超时，跨节点通信失败")
	}

	// ========== Step 4: 验证LeafNode A本地JetStream功能正常 ==========
	t.Log("\nStep 4: 验证LeafNode A本地JetStream功能...")
	jsA, err := ncA.JetStream()
	require.NoError(t, err, "获取LeafNode A的JetStream上下文失败")

	// 创建流
	_, err = jsA.AddStream(&nats.StreamConfig{
		Name:     testStreamName + "_A",
		Subjects: []string{testJetStreamSubject + ".a.>"},
		MaxAge:   1 * time.Hour,
		Storage:  nats.FileStorage,
	})
	require.NoError(t, err, "在LeafNode A上创建JetStream流失败")
	t.Log("✅ LeafNode A本地JetStream流创建成功")

	// 发布消息到本地JetStream
	testMsgA := "Message stored in LeafNode A local JetStream"
	pubAck, err := jsA.Publish(testJetStreamSubject+".a.test", []byte(testMsgA))
	require.NoError(t, err, "发布消息到LeafNode A JetStream失败")
	t.Logf("✅ 消息已发布到LeafNode A JetStream，序列号: %d", pubAck.Sequence)

	// 读取消息验证
	subA, err := jsA.PullSubscribe(testJetStreamSubject+".a.>", "consumer_a", nats.DeliverAll())
	require.NoError(t, err, "创建Pull订阅失败")

	msgs, err := subA.Fetch(1, nats.MaxWait(2*time.Second))
	require.NoError(t, err, "拉取消息失败")
	require.Len(t, msgs, 1, "应该拉取到1条消息")
	assert.Equal(t, testMsgA, string(msgs[0].Data), "拉取的消息内容不匹配")
	t.Log("✅ LeafNode A本地JetStream读写功能正常")

	// ========== Step 5: 验证LeafNode B本地JetStream功能正常 ==========
	t.Log("\nStep 5: 验证LeafNode B本地JetStream功能...")
	jsB, err := ncB.JetStream()
	require.NoError(t, err, "获取LeafNode B的JetStream上下文失败")

	// 创建流
	_, err = jsB.AddStream(&nats.StreamConfig{
		Name:     testStreamName + "_B",
		Subjects: []string{testJetStreamSubject + ".b.>"},
		MaxAge:   1 * time.Hour,
		Storage:  nats.FileStorage,
	})
	require.NoError(t, err, "在LeafNode B上创建JetStream流失败")
	t.Log("✅ LeafNode B本地JetStream流创建成功")

	// 发布消息到本地JetStream
	testMsgB := "Message stored in LeafNode B local JetStream"
	pubAckB, err := jsB.Publish(testJetStreamSubject+".b.test", []byte(testMsgB))
	require.NoError(t, err, "发布消息到LeafNode B JetStream失败")
	t.Logf("✅ 消息已发布到LeafNode B JetStream，序列号: %d", pubAckB.Sequence)

	// 读取消息验证
	subB, err := jsB.PullSubscribe(testJetStreamSubject+".b.>", "consumer_b", nats.DeliverAll())
	require.NoError(t, err, "创建Pull订阅失败")

	msgsB, err := subB.Fetch(1, nats.MaxWait(2*time.Second))
	require.NoError(t, err, "拉取消息失败")
	require.Len(t, msgsB, 1, "应该拉取到1条消息")
	assert.Equal(t, testMsgB, string(msgsB[0].Data), "拉取的消息内容不匹配")
	t.Log("✅ LeafNode B本地JetStream读写功能正常")

	// ========== 测试结论 ==========
	t.Log("\n=== 测试全部通过 ✅ ===")
	t.Log("")
	t.Log("✅ LeafNode可以正常开启JetStream功能")
	t.Log("✅ 开启JetStream不影响LeafNode之间通过Hub的P2P通信")
	t.Log("✅ 每个LeafNode的本地JetStream功能完全独立、正常工作")
}
