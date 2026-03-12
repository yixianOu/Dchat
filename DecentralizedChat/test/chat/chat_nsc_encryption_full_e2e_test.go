// E2E 集成测试：完整NSC密钥派生+加密通信流程
package e2e

import (
	"fmt"
	"testing"
	"time"

	"DecentralizedChat/internal/chat"
	"DecentralizedChat/internal/nats"
	"DecentralizedChat/internal/storage"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nkeys"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testHost = "127.0.0.1"

// 启动测试NATS服务器
func startTestNATSServer(t *testing.T) (*server.Server, string) {
	t.Helper()
	opts := &server.Options{
		Host:     testHost,
		Port:     -1,
		HTTPPort: -1,
		JetStream: false, // 不需要JetStream，纯pub/sub即可
		NoLog:    true,
		NoSigs:   true,
	}

	s, err := server.NewServer(opts)
	if err != nil {
		t.Fatalf("启动NATS服务器失败: %v", err)
	}
	s.Start()
	if !s.ReadyForConnections(10 * time.Second) {
		t.Fatal("NATS服务器未在超时时间内就绪")
	}

	url := fmt.Sprintf("nats://%s:%d", testHost, opts.Port)
	return s, url
}

// 测试完整流程：NSC密钥生成 → 派生聊天密钥 → 加密通信 → 解密验证
func TestChat_Full_NSC_Encryption_E2E(t *testing.T) {
	t.Log("=== E2E 测试: 完整NSC加密通信流程 ===")
	t.Log("")

	// =================== Step 1: 初始化环境 ===================
	t.Log("Step 1: 初始化测试环境...")
	// 启动NATS服务器
	natssrv, natsURL := startTestNATSServer(t)
	defer natssrv.Shutdown()
	t.Logf("✅ 测试NATS服务器启动成功: %s", natsURL)

	// 生成两个真实的NSC用户密钥对（模拟Alice和Bob注册账号）
	aliceKey, _ := nkeys.CreateUser()
	aliceSeed, _ := aliceKey.Seed()
	aliceNscPub, _ := aliceKey.PublicKey() // Alice的公开身份ID
	aliceUID := "alice_001"

	bobKey, _ := nkeys.CreateUser()
	bobSeed, _ := bobKey.Seed()
	bobNscPub, _ := bobKey.PublicKey() // Bob的公开身份ID
	bobUID := "bob_001"
	t.Log("✅ 生成Alice和Bob的NSC身份密钥对成功")
	t.Logf("   Alice NSC公钥: %s", aliceNscPub)
	t.Logf("   Bob NSC公钥: %s", bobNscPub)

	// =================== Step 2: 初始化双方Chat服务 ===================
	t.Log("\nStep 2: 初始化双方Chat服务...")
	// Alice端初始化
	natsAlice, err := nats.NewService(nats.ClientConfig{
		URL:  natsURL,
		Name: "alice-client",
	})
	require.NoError(t, err)
	defer natsAlice.Close()
	storageAlice, err := storage.NewSQLiteStorage(t.TempDir() + "/alice.db")
	require.NoError(t, err)
	defer storageAlice.Close()
	chatAlice := chat.NewService(natsAlice, storageAlice)
	chatAlice.SetUserID(aliceUID)
	// 加载Alice的NSC密钥，自动派生聊天密钥对
	err = chatAlice.LoadNSCKeys(string(aliceSeed))
	require.NoError(t, err)
	t.Log("✅ Alice Chat服务初始化完成，NSC密钥已加载")

	// Bob端初始化
	natsBob, err := nats.NewService(nats.ClientConfig{
		URL:  natsURL,
		Name: "bob-client",
	})
	require.NoError(t, err)
	defer natsBob.Close()
	storageBob, err := storage.NewSQLiteStorage(t.TempDir() + "/bob.db")
	require.NoError(t, err)
	defer storageBob.Close()
	chatBob := chat.NewService(natsBob, storageBob)
	chatBob.SetUserID(bobUID)
	// 加载Bob的NSC密钥，自动派生聊天密钥对
	err = chatBob.LoadNSCKeys(string(bobSeed))
	require.NoError(t, err)
	t.Log("✅ Bob Chat服务初始化完成，NSC密钥已加载")

	// =================== Step 3: 添加好友（仅使用NSC公钥，不需要交换聊天公钥） ===================
	t.Log("\nStep 3: 双方仅通过NSC公钥添加好友...")
	// Alice添加Bob为好友：只需要知道Bob的NSC公钥，不需要Bob在线，不需要交换任何消息
	derivedBobUID, err := chatAlice.AddFriendNSCKey(bobNscPub)
	require.NoError(t, err)
	require.Equal(t, bobUID, derivedBobUID) // 验证派生的ID和预期一致
	t.Log("✅ Alice添加Bob为好友成功（仅用Bob的NSC公钥，自动派生用户ID匹配）")

	// Bob添加Alice为好友：同样只需要知道Alice的NSC公钥
	derivedAliceUID, err := chatBob.AddFriendNSCKey(aliceNscPub)
	require.NoError(t, err)
	require.Equal(t, aliceUID, derivedAliceUID) // 验证派生的ID和预期一致
	t.Log("✅ Bob添加Alice为好友成功（仅用Alice的NSC公钥，自动派生用户ID匹配）")

	// =================== Step 4: Bob订阅会话，等待消息 ===================
	t.Log("\nStep 4: Bob订阅与Alice的私聊会话...")
	err = chatBob.JoinDirect(aliceUID)
	require.NoError(t, err)
	t.Log("✅ Bob已订阅与Alice的私聊会话")

	// 注册Bob的消息接收回调
	receivedMsg := make(chan *chat.DecryptedMessage, 1)
	chatBob.OnDecrypted(func(msg *chat.DecryptedMessage) {
		t.Logf("📥 Bob收到消息: 发送者=%s, 内容=%q", msg.Sender, msg.Plain)
		receivedMsg <- msg
	})
	chatBob.OnError(func(err error) {
		t.Errorf("❌ Bob收到错误: %v", err)
	})

	// 等待订阅同步
	time.Sleep(1 * time.Second)

	// =================== Step 5: Alice发送加密消息给Bob ===================
	t.Log("\nStep 5: Alice发送加密消息给Bob...")
	testMessage := "Hello Bob! 这是通过NSC密钥派生加密的消息，不需要提前交换密钥哦！"
	err = chatAlice.SendDirect(bobUID, testMessage)
	require.NoError(t, err)
	t.Logf("✅ Alice已发送加密消息: %q", testMessage)

	// =================== Step 6: 验证Bob收到并解密消息 ===================
	t.Log("\nStep 6: 验证Bob收到并成功解密消息...")
	select {
	case msg := <-receivedMsg:
		assert.Equal(t, testMessage, msg.Plain, "解密后的消息内容不匹配")
		assert.Equal(t, aliceUID, msg.Sender, "发送者ID不匹配")
		assert.False(t, msg.IsGroup, "私聊消息不应该标记为群聊")
		t.Log("✅ 消息解密成功！内容完全匹配")
	case <-time.After(5 * time.Second):
		t.Fatal("❌ 等待消息超时，解密失败")
	}

	// =================== Step 7: 反向测试：Bob回复消息给Alice ===================
	t.Log("\nStep 7: 测试反向通信，Bob回复消息给Alice...")
	// Alice订阅会话
	err = chatAlice.JoinDirect(bobUID)
	require.NoError(t, err)
	aliceReceived := make(chan *chat.DecryptedMessage, 1)
	chatAlice.OnDecrypted(func(msg *chat.DecryptedMessage) {
		t.Logf("📥 Alice收到回复: 发送者=%s, 内容=%q", msg.Sender, msg.Plain)
		aliceReceived <- msg
	})
	time.Sleep(1 * time.Second)

	// Bob回复消息
	replyMessage := "Hi Alice! 我收到了你的消息，加密通信完全正常！"
	err = chatBob.SendDirect(aliceUID, replyMessage)
	require.NoError(t, err)
	t.Logf("✅ Bob已回复加密消息: %q", replyMessage)

	// 验证Alice收到
	select {
	case msg := <-aliceReceived:
		assert.Equal(t, replyMessage, msg.Plain, "解密后的回复内容不匹配")
		assert.Equal(t, bobUID, msg.Sender, "发送者ID不匹配")
		t.Log("✅ 回复消息解密成功！双向通信正常")
	case <-time.After(5 * time.Second):
		t.Fatal("❌ 等待回复超时，反向通信失败")
	}

	t.Log("\n=== 🎉 完整NSC加密通信测试全部通过！ ===")
	t.Log("")
	t.Log("✅ 无需密钥交换，仅通过NSC公钥就能完成端到端加密")
	t.Log("✅ 双向通信正常，解密结果100%匹配")
	t.Log("✅ 完全符合去中心化设计，不需要任何中心存储或信令服务")
}
