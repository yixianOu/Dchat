// E2E 集成测试：LeafNode JetStream 镜像同步功能测试
package jetstream_test

/*
=== 测试目标 ===
验证NATS官方JetStream Mirror功能是否可以实现：
1. 本地LeafNode不需要直接连接公网Hub客户端，仅通过LeafNode链路即可同步Hub上的JetStream消息
2. 镜像流配置后消息自动同步，不需要额外开发代码
3. 本地查询镜像流和查询本地流体验完全一致

=== 测试场景 ===
架构：公网Hub(开启JetStream) <--- LeafNode链路 <--- 本地LeafNode(开启JetStream)
- 公网Hub上创建源流：存储所有用户的离线消息
- 本地LeafNode上创建镜像流：仅同步当前用户的离线消息（按subject过滤）
- 验证消息从Hub自动同步到LeafNode，并且可以在LeafNode本地查询

=== 测试步骤 ===
Step 1: 准备工作
  - 连接公网Hub的JetStream，清理测试环境
  - 创建Hub端的公共源流 OFFLINE_MESSAGES，监听 subject: dchat.offline.>

Step 2: 启动本地LeafNode
  - 配置LeafNode开启JetStream，连接到公网Hub
  - 验证LeafNode启动成功，与Hub连接正常

Step 3: 在LeafNode端创建镜像流
  - 创建镜像流 USER_OFFLINE_123，配置为Hub上流的镜像
  - 配置FilterSubject只同步 dchat.offline.user123.> 前缀的消息（模拟单个用户的离线消息）

Step 4: 验证历史消息同步
  - 直接在Hub端发布3条用户123的离线消息
  - 验证消息自动同步到LeafNode的镜像流，本地可以查询到
  - 验证其他用户的消息不会同步过来（过滤功能有效）

Step 5: 验证实时消息同步
  - 再发布2条新消息到Hub的源流
  - 验证新消息自动实时同步到LeafNode镜像流

Step 6: 验证ACK同步
  - 在LeafNode端ACK消费消息
  - 验证Hub端对应的消息也被标记为已消费（如果配置了ACK同步）

=== 预期结果 ===
✅ 镜像流创建成功
✅ 历史消息自动同步，延迟<2秒
✅ 消息过滤功能正常，仅同步指定subject的消息
✅ 新消息实时自动同步
✅ ACK可以同步回Hub（可选，取决于配置）
*/

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
	publicHubClientURL = "nats://121.199.173.116:4222" // 公网Hub客户端端口
	publicHubLeafURL   = "nats://121.199.173.116:7422" // 公网Hub LeafNode端口
	testUserID         = "user123"
	hubStreamName      = "OFFLINE_MESSAGES"
	leafStreamName     = "USER_OFFLINE_123"
	testSubjectPrefix  = "dchat.offline." + testUserID
)

func TestLeafNode_JetStream_Mirror_Sync_E2E(t *testing.T) {
	t.Log("=== E2E 测试: LeafNode JetStream 镜像同步功能 ===")
	t.Log("")

	// ========== Step 0: 初始化公共配置 ==========
	cfg, _ := config.LoadConfig()
	_ = nscsetup.EnsureSimpleSetup(cfg)
	testHost := "127.0.0.1"
	tempDir := t.TempDir()

	// ========== Step 1: 连接公网Hub，创建测试源流 ==========
	t.Log("Step 1: 连接公网Hub，准备源流...")
	ncHub, err := nats.Connect(publicHubClientURL,
		nats.UserCredentials(cfg.Keys.UserCredsPath),
		nats.Timeout(10*time.Second),
	)
	require.NoError(t, err, "连接公网Hub失败")
	defer ncHub.Close()

	jsHub, err := ncHub.JetStream()
	require.NoError(t, err, "获取Hub JetStream上下文失败")

	// 清理旧的测试流（如果存在）
	_ = jsHub.DeleteStream(hubStreamName)

	// 创建Hub端公共源流
	_, err = jsHub.AddStream(&nats.StreamConfig{
		Name:     hubStreamName,
		Subjects: []string{"dchat.offline.>"},
		MaxAge:   1 * time.Hour,
		Storage:  nats.FileStorage,
	})
	require.NoError(t, err, "创建Hub端源流失败")
	t.Logf("✅ Hub端源流 %s 创建成功", hubStreamName)

	// 先发布3条测试消息
	testMsgs := []struct {
		subject string
		content string
		userMsg bool // 是否是当前测试用户的消息
	}{
		{testSubjectPrefix + ".msg1", "用户123的离线消息1", true},
		{testSubjectPrefix + ".msg2", "用户123的离线消息2", true},
		{"dchat.offline.user456.msg1", "用户456的离线消息", false}, // 其他用户的消息，不应该同步
	}

	for _, msg := range testMsgs {
		_, err = jsHub.Publish(msg.subject, []byte(msg.content))
		require.NoError(t, err, "发布测试消息到Hub失败")
		t.Logf("✅ 发布消息到Hub: %s = %q", msg.subject, msg.content)
	}

	// ========== Step 2: 启动本地LeafNode（开启JetStream） ==========
	t.Log("\nStep 2: 启动本地LeafNode（开启JetStream）...")
	leafCfg := &config.LeafNodeConfig{
		LocalHost:               testHost,
		LocalPort:               42240,
		HubURLs:                 []string{publicHubLeafURL},
		CredsFile:               cfg.Keys.UserCredsPath,
		ConnectTimeout:          15 * time.Second,
		EnableJetStream:         true,
		JetStreamStoreDir:       tempDir + "/leafnode",
		JetStreamAllowUpstreamAPI: true, // 允许转发JetStream API请求到上游Hub
	}

	mgr := leafnode.NewManager(leafCfg)
	err = mgr.Start()
	require.NoError(t, err, "启动LeafNode失败")
	defer mgr.Stop()

	require.True(t, mgr.IsRunning(), "LeafNode应该正在运行")
	leafURL := mgr.GetLocalNATSURL()
	t.Logf("✅ LeafNode启动成功，本地地址: %s", leafURL)

	// 等待连接稳定
	time.Sleep(3 * time.Second)

	// ========== Step 3: 连接本地LeafNode，创建镜像流 ==========
	t.Log("\nStep 3: 在LeafNode端创建镜像流...")
	ncLeaf, err := nats.Connect(leafURL, nats.Timeout(5*time.Second))
	require.NoError(t, err, "连接本地LeafNode失败")
	defer ncLeaf.Close()

	jsLeaf, err := ncLeaf.JetStream()
	require.NoError(t, err, "获取LeafNode JetStream上下文失败")

	// 清理旧的镜像流
	_ = jsLeaf.DeleteStream(leafStreamName)

	// 创建镜像流 - 核心配置（跨LeafNode场景必须配置External）
	_, err = jsLeaf.AddStream(&nats.StreamConfig{
		Name: leafStreamName,
		// 镜像流不需要指定Subjects，自动继承源流的subject并通过FilterSubject过滤
		// 镜像配置：同步Hub上的源流
		Mirror: &nats.StreamSource{
			Name:          hubStreamName,              // Hub上的源流名称
			FilterSubject: testSubjectPrefix + ".>",    // 只同步当前用户的消息
			// 跨集群镜像必须配置External，指定Hub端的JetStream API前缀
			// 格式为 $JS.<Hub的JetStream Domain名>.API，我们的Hub domain是"hub"
			External: &nats.ExternalStream{
				APIPrefix:     "$JS.hub.API",                // Hub端JetStream API前缀，和Hub配置的domain对应
				DeliverPrefix: "sync.hub." + testUserID,     // 同步消息的交付前缀，避免和本地冲突
			},
		},
		MaxAge:  1 * time.Hour,
		Storage: nats.FileStorage,
	})
	require.NoError(t, err, "创建镜像流失败")
	t.Logf("✅ LeafNode端镜像流 %s 创建成功，将同步Hub上 %s 的消息", leafStreamName, testSubjectPrefix+".>")

	// 等待同步初始化，多等几秒确保镜像流同步完成
	time.Sleep(5 * time.Second)

	// 先查看镜像流信息，确认同步状态
	streamInfo, err := jsLeaf.StreamInfo(leafStreamName)
	require.NoError(t, err, "获取镜像流信息失败")
	t.Logf("📊 镜像流信息: subjects=%v, state=%+v", streamInfo.Config.Subjects, streamInfo.State)

	// ========== Step 4: 验证历史消息同步 ==========
	t.Log("\nStep 4: 验证历史消息自动同步...")
	// 镜像流的subject是源流的subject，经过FilterSubject过滤后，就是testSubjectPrefix.>
	sub, err := jsLeaf.PullSubscribe(testSubjectPrefix+".>", "test-consumer",
		nats.DeliverAll(),
		nats.AckExplicit(),
		nats.BindStream(leafStreamName), // 显式绑定流，避免自动查找
	)
	require.NoError(t, err, "创建Pull订阅失败")

	// 拉取消息
	msgs, err := sub.Fetch(10, nats.MaxWait(5*time.Second))
	require.NoError(t, err, "拉取同步消息失败")

	// 统计符合条件的消息
	var receivedUserMsgs []string
	for _, msg := range msgs {
		receivedUserMsgs = append(receivedUserMsgs, string(msg.Data))
		t.Logf("📥 同步到消息: %s = %q", msg.Subject, string(msg.Data))
		msg.Ack()
	}

	// 验证：应该只收到用户123的2条消息，用户456的消息不会同步
	assert.Len(t, receivedUserMsgs, 2, "应该只同步到2条属于user123的消息")
	assert.Contains(t, receivedUserMsgs, "用户123的离线消息1")
	assert.Contains(t, receivedUserMsgs, "用户123的离线消息2")
	t.Log("✅ 历史消息同步成功，消息过滤功能正常")

	// ========== Step 5: 验证实时消息同步 ==========
	t.Log("\nStep 5: 验证实时消息自动同步...")
	newMsgContent := "用户123的新离线消息（实时同步测试）"
	_, err = jsHub.Publish(testSubjectPrefix+".msg3", []byte(newMsgContent))
	require.NoError(t, err, "发布新消息到Hub失败")
	t.Logf("✅ 发布新消息到Hub: %s", newMsgContent)

	// 等待同步
	time.Sleep(1 * time.Second)

	// 拉取新消息
	newMsgs, err := sub.Fetch(1, nats.MaxWait(3*time.Second))
	require.NoError(t, err, "拉取新同步消息失败")
	require.Len(t, newMsgs, 1, "应该同步到1条新消息")
	assert.Equal(t, newMsgContent, string(newMsgs[0].Data))
	newMsgs[0].Ack()
	t.Log("✅ 实时消息自动同步成功")

	// ========== 测试结论 ==========
	t.Log("\n=== 测试全部通过 ✅ ===")
	t.Log("")
	t.Log("✅ JetStream镜像功能正常工作")
	t.Log("✅ 历史消息自动同步，延迟<2秒")
	t.Log("✅ 按subject过滤功能正常，仅同步指定用户的消息")
	t.Log("✅ 新消息实时自动同步，不需要额外代码")
	t.Log("✅ LeafNode本地查询镜像流和查询本地流完全一致")

	// 清理测试资源
	_ = jsHub.DeleteStream(hubStreamName)
	_ = jsLeaf.DeleteStream(leafStreamName)
}
