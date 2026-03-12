// E2E 集成测试：使用公网Hub测试离线消息镜像同步+SQLite存储
package jetstream_test

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"DecentralizedChat/internal/chat"
	"DecentralizedChat/internal/config"
	"DecentralizedChat/internal/leafnode"
	natsservice "DecentralizedChat/internal/nats"
	"DecentralizedChat/internal/nscsetup"
	"DecentralizedChat/internal/storage"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/nacl/box"
)

const (
	testUserIDSync     = "test_user_e2e_001"
	testFriendID       = "test_friend_002"
	testGroupID        = "test_group_003"
	testDM_CID         = "test_dm_cid_000123456789abc" // 测试用私聊CID
	// publicHubClientURL = "nats://121.199.173.116:4222"   // 公网Hub客户端端口
	// publicHubLeafURL   = "nats://121.199.173.116:7422"   // 公网Hub LeafNode端口
)

func TestOfflineSync_PublicHub_E2E(t *testing.T) {
	t.Log("=== E2E 测试: 公网Hub离线消息镜像同步+SQLite存储全流程 ===")
	t.Log("")

	tempDir := t.TempDir()
	testHost := "127.0.0.1"

	// ========== 生成测试用密钥 ==========
	// 生成用户密钥对（测试用户）
	testUserPub, testUserPriv, err := box.GenerateKey(rand.Reader)
	require.NoError(t, err, "生成测试用户密钥失败")
	testUserPrivB64 := chat.B64(testUserPriv[:])
	testUserPubB64 := chat.B64(testUserPub[:])

	// 生成好友密钥对
	friendPub, friendPriv, err := box.GenerateKey(rand.Reader)
	require.NoError(t, err, "生成好友密钥失败")
	friendPrivB64 := chat.B64(friendPriv[:])
	friendPubB64 := chat.B64(friendPub[:])

	// 生成群聊密钥
	var groupKey [32]byte
	_, err = rand.Read(groupKey[:])
	require.NoError(t, err, "生成群聊密钥失败")
	groupKeyB64 := chat.B64(groupKey[:])

	// ========== Step 1: 加载配置，连接公网Hub发布测试消息 ==========
	t.Log("Step 1: 连接公网Hub，发布测试消息...")
	cfg, _ := config.LoadConfig()
	_ = nscsetup.EnsureSimpleSetup(cfg)

	// 连接公网Hub发布测试消息
	ncHub, err := nats.Connect(publicHubClientURL, nats.UserCredentials(cfg.Keys.UserCredsPath))
	require.NoError(t, err, "连接公网Hub失败")
	defer ncHub.Close()

	jsHub, err := ncHub.JetStream()
	require.NoError(t, err, "获取公网Hub JetStream失败")

	// 测试消息：私聊消息（好友发给测试用户）
	dmSubject := fmt.Sprintf("dchat.dm.%s.msg", testDM_CID)
	// 真实加密消息，和业务逻辑完全一致
	nonceDM, cipherDM, err := chat.EncryptDirect(friendPrivB64, testUserPubB64, []byte("Hello from friend!"))
	require.NoError(t, err, "加密私聊消息失败")
	dmMsg := chat.EncWire{
		Sender: testFriendID,
		CID:    testDM_CID,
		Nonce:  nonceDM,
		Cipher: cipherDM,
		TS:     time.Now().Unix(),
	}
	dmData, _ := json.Marshal(dmMsg)
	_, err = jsHub.Publish(dmSubject, dmData)
	require.NoError(t, err, "发布私聊消息失败")

	// 测试消息：群聊消息（好友发在群里）
	grpSubject := fmt.Sprintf("dchat.grp.%s.msg", testGroupID)
	nonceGrp, cipherGrp, err := chat.EncryptGroup(groupKeyB64, []byte("Hello from group!"))
	require.NoError(t, err, "加密群聊消息失败")
	grpMsg := chat.EncWire{
		Sender: testFriendID,
		CID:    testGroupID,
		Nonce:  nonceGrp,
		Cipher: cipherGrp,
		TS:     time.Now().Unix(),
	}
	grpData, _ := json.Marshal(grpMsg)
	_, err = jsHub.Publish(grpSubject, grpData)
	require.NoError(t, err, "发布群聊消息失败")

	t.Log("✅ 测试消息发布到公网Hub完成：1条私聊+1条群聊")
	// 等待消息持久化
	time.Sleep(2 * time.Second)

	// ========== 调试：确认消息已经存入Hub的流中 ==========
	t.Log("\n调试：查询Hub流中的消息数量...")
	// 查询DChatGroups流信息
	grpInfo, err := jsHub.StreamInfo("DChatGroups")
	if err == nil {
		t.Logf("Hub DChatGroups流总消息数: %d", grpInfo.State.Msgs)
	} else {
		t.Logf("查询DChatGroups失败: %v", err)
	}
	// 查询DChatDirect流信息
	dmInfo, err := jsHub.StreamInfo("DChatDirect")
	if err == nil {
		t.Logf("Hub DChatDirect流总消息数: %d", dmInfo.State.Msgs)
	} else {
		t.Logf("查询DChatDirect失败: %v", err)
	}

	// ========== Step 2: 启动本地LeafNode连接公网Hub ==========
	t.Log("\nStep 2: 启动本地LeafNode（连接公网Hub，开启JetStream）...")
	leafCfg := &config.LeafNodeConfig{
		LocalHost:               testHost,
		LocalPort:               42280,
		HubURLs:                 []string{publicHubLeafURL},
		CredsFile:               cfg.Keys.UserCredsPath,
		ConnectTimeout:          15 * time.Second,
		EnableJetStream:         true,
		JetStreamStoreDir:       tempDir + "/leafnode",
		JetStreamAllowUpstreamAPI: true,
	}

	leafMgr := leafnode.NewManager(leafCfg)
	err = leafMgr.Start()
	require.NoError(t, err, "启动LeafNode失败")
	defer leafMgr.Stop()
	require.True(t, leafMgr.IsRunning(), "LeafNode应该正在运行")
	t.Log("✅ LeafNode启动成功，已连接到公网Hub")

	// 等待连接稳定
	time.Sleep(3 * time.Second)

	// ========== Step 3: 初始化服务 ==========
	t.Log("\nStep 3: 初始化NATS、Storage、Chat服务...")
	// 初始化NATS客户端连接本地LeafNode
	nc, err := natsservice.NewService(natsservice.ClientConfig{
		URL:       leafMgr.GetLocalNATSURL(),
		Name:      "DChatTestClient",
		CredsFile: cfg.Keys.UserCredsPath,
		Timeout:   10 * time.Second,
	})
	require.NoError(t, err, "连接本地LeafNode失败")
	defer nc.Close()

	// 初始化SQLite存储
	sqlitePath := filepath.Join(tempDir, "test_chat.db")
	store, err := storage.NewSQLiteStorage(sqlitePath)
	require.NoError(t, err, "初始化SQLite失败")
	defer store.Close()
	defer os.Remove(sqlitePath)

	// 初始化Chat服务
	chatSvc := chat.NewService(nc, store)
	chatSvc.SetUserID(testUserIDSync)
	// 添加测试用的密钥，确保能解密测试消息
	chatSvc.SetKeyPair(testUserPrivB64, testUserPubB64)
	chatSvc.AddFriendKey(testFriendID, friendPubB64)
	chatSvc.AddGroupKey(testGroupID, groupKeyB64)
	t.Log("✅ 所有服务初始化完成")

	// ========== Step 4: 启动离线同步 ==========
	t.Log("\nStep 4: 启动离线消息同步...")
	err = chatSvc.InitOfflineSync()
	require.NoError(t, err, "初始化离线同步失败")
	t.Log("✅ 离线同步启动成功，等待同步完成...")

	// 等待同步完成，公网环境等待30秒
	time.Sleep(30 * time.Second)

	// ========== Step 5: 验证结果 ==========
	t.Log("\nStep 5: 验证同步结果...")
	// 查询所有消息
	messages, err := store.GetMessages("", 100, nil)
	require.NoError(t, err, "查询消息失败")

	// 打印同步结果
	t.Logf("📊 同步到消息数量: %d", len(messages))
	for _, msg := range messages {
		t.Logf("   - 会话ID: %s, 发送者: %s, 内容: %s", msg.ConversationID, msg.SenderID, msg.Content)
	}

	// 验证消息数量和内容，防止假阳性
	require.Equal(t, 2, len(messages), "应该同步到2条消息（1条私聊+1条群聊）")

	// 验证私聊消息内容
	dmFound := false
	grpFound := false
	for _, msg := range messages {
		switch msg.ConversationID {
case testDM_CID:
			require.Equal(t, testFriendID, msg.SenderID, "私聊消息发送者应该是好友")
			require.Equal(t, "Hello from friend!", msg.Content, "私聊消息内容不匹配")
			dmFound = true
		case testGroupID:
			require.Equal(t, testFriendID, msg.SenderID, "群聊消息发送者应该是好友")
			require.Equal(t, "Hello from group!", msg.Content, "群聊消息内容不匹配")
			grpFound = true
		}
	}
	require.True(t, dmFound, "应该找到私聊消息")
	require.True(t, grpFound, "应该找到群聊消息")

	// ========== 测试结论 ==========
	t.Log("\n=== 测试完成 ===")
	t.Log("✅ LeafNode镜像流创建成功")
	t.Log("✅ 公网Hub消息同步链路正常")
	t.Log("✅ 本地JetStream和SQLite存储工作正常")
	t.Log("✅ 消息解密和处理逻辑正常")
}
