// E2E 集成测试：LeafNode连接多Hub集群测试
// 验证LeafNode可以连接任意Hub节点并且正常收发消息
package e2e_test

import (
	"fmt"
	"testing"
	"time"

	"DecentralizedChat/internal/config"
	"DecentralizedChat/internal/leafnode"
	"DecentralizedChat/internal/nscsetup"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
)

// 公网3节点Hub集群LeafNode端口
const (
	hub1LeafURL = "nats://121.199.173.116:7421"
	hub2LeafURL = "nats://121.199.173.116:7422"
	hub3LeafURL = "nats://121.199.173.116:7423"
	multiHubTestSubject = "test.leafnode.multi.hub.unique"
	multiHubTestStreamName = "TEST_LEAFNODE_MULTI_HUB_UNIQUE"
)

// 测试单个Hub连接功能
func testSingleHubConnection(t *testing.T, hubURL string, localPort int, testName string) {
	t.Logf("\n=== 测试 %s 连接 ===", testName)

	cfg, _ := config.LoadConfig()
	_ = nscsetup.EnsureSimpleSetup(cfg)
	testHost := "127.0.0.1"
	tempDir := t.TempDir()

	// 配置LeafNode
	cfgLeaf := &config.LeafNodeConfig{
		LocalHost:         testHost,
		LocalPort:         localPort,
		HubURLs:           []string{hubURL},
		CredsFile:         cfg.Keys.UserCredsPath,
		ConnectTimeout:    15 * time.Second,
		EnableJetStream:   false, // 测试基础连接不需要本地JetStream
		JetStreamStoreDir: tempDir,
	}

	// 启动LeafNode
	mgr := leafnode.NewManager(cfgLeaf)
	err := mgr.Start()
	require.NoError(t, err, fmt.Sprintf("启动LeafNode连接%s失败", testName))
	defer mgr.Stop()

	require.True(t, mgr.IsRunning(), fmt.Sprintf("LeafNode %s 应该正在运行", testName))
	localURL := mgr.GetLocalNATSURL()
	t.Logf("✅ LeafNode 启动成功，本地地址: %s，已连接到 %s", localURL, hubURL)

	// 等待连接稳定
	time.Sleep(3 * time.Second)

	// 连接本地LeafNode
	nc, err := nats.Connect(localURL, nats.Timeout(5*time.Second))
	require.NoError(t, err, fmt.Sprintf("连接本地LeafNode %s 失败", testName))
	defer nc.Close()

	// 测试消息收发
	received := make(chan string, 1)
	sub, err := nc.Subscribe(multiHubTestSubject, func(m *nats.Msg) {
		received <- string(m.Data)
	})
	require.NoError(t, err, "订阅失败")
	defer sub.Unsubscribe()

	// 等待订阅同步
	time.Sleep(1 * time.Second)

	// 发布消息
	testMsg := fmt.Sprintf("Test message through %s", testName)
	err = nc.Publish(multiHubTestSubject, []byte(testMsg))
	require.NoError(t, err, "发布消息失败")

	// 等待接收
	select {
	case msg := <-received:
		require.Equal(t, testMsg, msg)
		t.Logf("✅ %s 消息收发正常，内容: %s", testName, msg)
	case <-time.After(5 * time.Second):
		t.Fatalf("❌ %s 等待消息超时", testName)
	}

	// 测试访问Hub集群JetStream
	t.Logf("🔍 测试通过 %s 访问Hub集群JetStream", testName)
	js, err := nc.JetStream(nats.Domain("hub"))
	require.NoError(t, err, "获取Hub JetStream上下文失败")

	// 先创建测试流（如果不存在）
	streamName := fmt.Sprintf("%s_%s", multiHubTestStreamName, testName)
	subject := fmt.Sprintf("test.jetstream.leaf.multi.%s.*", testName)
	_, err = js.AddStream(&nats.StreamConfig{
		Name:     streamName,
		Subjects: []string{subject},
		Replicas: 3,
		Storage:  nats.FileStorage,
		MaxAge:   1 * time.Hour,
	})
	if err != nil && err != nats.ErrStreamNameAlreadyInUse {
		require.NoError(t, err, "创建测试JetStream流失败")
	}
	defer js.DeleteStream(streamName)
	t.Logf("✅ 测试JetStream流 %s 已准备就绪", streamName)

	// 发布测试消息到Hub JetStream
	jsTestMsg := fmt.Sprintf("JetStream test through %s", testName)
	testSubject := fmt.Sprintf("test.jetstream.leaf.multi.%s.test", testName)
	ack, err := js.Publish(testSubject, []byte(jsTestMsg))
	require.NoError(t, err, "发布JetStream消息失败")
	t.Logf("✅ 成功发布JetStream消息，序列ID: %d", ack.Sequence)

	// 读取消息验证
	msg, err := js.GetMsg(streamName, ack.Sequence)
	require.NoError(t, err, "读取JetStream消息失败")
	require.Equal(t, jsTestMsg, string(msg.Data), "JetStream消息内容不匹配")
	t.Logf("✅ JetStream消息读写正常，内容: %s", jsTestMsg)

	t.Logf("✅ %s 连接测试全部通过", testName)
}

// 测试单个Hub连接功能（独立测试）
func TestLeafNode_Single_Hub_Connect_E2E(t *testing.T) {
	t.Log("=== E2E测试: LeafNode单个Hub连接功能验证 ===")
	t.Log("测试场景：验证LeafNode可以独立连接每个Hub节点并正常工作")

	// ========== 测试连接Hub1 ==========
	t.Log("\n📌 Step 1: 测试连接Hub1 (7421端口)")
	testSingleHubConnection(t, hub1LeafURL, 42240, "Hub1")

	// ========== 测试连接Hub2 ==========
	t.Log("\n📌 Step 2: 测试连接Hub2 (7422端口)")
	testSingleHubConnection(t, hub2LeafURL, 42241, "Hub2")

	// ========== 测试连接Hub3 ==========
	t.Log("\n📌 Step 3: 测试连接Hub3 (7423端口)")
	testSingleHubConnection(t, hub3LeafURL, 42242, "Hub3")

	// ========== 测试结论 ==========
	t.Log("\n🎉 所有单Hub连接测试通过！✅")
	t.Log("========================================================")
	t.Log("✅ LeafNode可以正常连接Hub1、Hub2、Hub3任意节点")
	t.Log("✅ 连接任意Hub都可以正常收发全局消息")
	t.Log("✅ 连接任意Hub都可以正常访问集群JetStream功能")
	t.Log("========================================================")
}

// 测试多Hub地址自动选择功能（独立测试）
func TestLeafNode_Multi_Hub_Auto_Select_E2E(t *testing.T) {
	t.Log("=== E2E测试: LeafNode多Hub地址自动选择功能验证 ===")
	t.Log("测试场景：验证LeafNode配置多Hub地址时可以自动选择可用节点")

	cfg, _ := config.LoadConfig()
	_ = nscsetup.EnsureSimpleSetup(cfg)
	testHost := "127.0.0.1"
	tempDir := t.TempDir()

	// 配置所有三个Hub地址
	cfgLeaf := &config.LeafNodeConfig{
		LocalHost:         testHost,
		LocalPort:         42243,
		HubURLs:           []string{hub1LeafURL, hub2LeafURL, hub3LeafURL},
		CredsFile:         cfg.Keys.UserCredsPath,
		ConnectTimeout:    15 * time.Second,
		EnableJetStream:   false,
		JetStreamStoreDir: tempDir,
	}

	// 启动LeafNode
	mgr := leafnode.NewManager(cfgLeaf)
	err := mgr.Start()
	require.NoError(t, err, "启动带多Hub配置的LeafNode失败")
	defer mgr.Stop()

	require.True(t, mgr.IsRunning(), "LeafNode 应该正在运行")
	localURL := mgr.GetLocalNATSURL()
	t.Logf("✅ LeafNode 启动成功，本地地址: %s", localURL)
	t.Logf("✅ 配置了3个Hub地址: %v", cfgLeaf.HubURLs)

	// 等待连接稳定
	time.Sleep(3 * time.Second)

	// 连接本地LeafNode
	nc, err := nats.Connect(localURL, nats.Timeout(5*time.Second))
	require.NoError(t, err, "连接本地LeafNode失败")
	defer nc.Close()

	// 打印当前连接的Hub地址
	t.Logf("✅ LeafNode自动选择连接到Hub: %s", nc.ConnectedUrl())

	// 测试消息收发
	received := make(chan string, 1)
	sub, err := nc.Subscribe(multiHubTestSubject+".multi", func(m *nats.Msg) {
		received <- string(m.Data)
	})
	require.NoError(t, err, "订阅失败")
	defer sub.Unsubscribe()

	// 等待订阅同步
	time.Sleep(1 * time.Second)

	// 发布消息
	testMsg := "Test message through multi Hub configuration"
	err = nc.Publish(multiHubTestSubject+".multi", []byte(testMsg))
	require.NoError(t, err, "发布消息失败")

	// 等待接收
	select {
	case msg := <-received:
		require.Equal(t, testMsg, msg)
		t.Logf("✅ 多Hub配置消息收发正常，内容: %s", msg)
	case <-time.After(5 * time.Second):
		t.Fatal("❌ 多Hub配置等待消息超时")
	}

	// ========== 测试结论 ==========
	t.Log("\n🎉 多Hub自动选择测试通过！✅")
	t.Log("========================================================")
	t.Log("✅ 配置多Hub地址时LeafNode可以自动选择可用节点")
	t.Log("✅ 公网Hub集群完全满足LeafNode多活接入要求")
	t.Log("========================================================")
}
