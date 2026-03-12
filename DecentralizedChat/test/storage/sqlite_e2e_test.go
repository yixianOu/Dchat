// E2E 测试：SQLite存储功能全流程验证
package storage_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"DecentralizedChat/internal/storage"

	"github.com/stretchr/testify/require"
)

func TestSQLite_E2E(t *testing.T) {
	t.Log("=== E2E 测试: SQLite存储功能全流程 ===")

	// 创建临时数据库文件
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_chat.db")

	// 1. 初始化存储
	t.Log("\nStep 1: 初始化SQLite存储...")
	store, err := storage.NewSQLiteStorage(dbPath)
	require.NoError(t, err, "初始化SQLite失败")
	defer store.Close()
	defer os.Remove(dbPath)
	t.Log("✅ SQLite存储初始化成功")

	// 2. 测试保存消息
	t.Log("\nStep 2: 测试消息保存功能...")
	testMsg1 := &storage.StoredMessage{
		ID:             "msg_test_001",
		ConversationID: "cid_test_001",
		SenderID:       "user_alice",
		SenderNickname: "Alice",
		Content:        "Hello Bob!",
		Timestamp:      time.Now(),
		IsRead:         false,
		IsGroup:        false,
		NatsSeq:        1001,
	}

	err = store.SaveMessage(testMsg1)
	require.NoError(t, err, "保存消息1失败")

	testMsg2 := &storage.StoredMessage{
		ID:             "msg_test_002",
		ConversationID: "cid_test_001",
		SenderID:       "user_bob",
		SenderNickname: "Bob",
		Content:        "Hi Alice!",
		Timestamp:      time.Now().Add(time.Second),
		IsRead:         false,
		IsGroup:        false,
		NatsSeq:        1002,
	}

	err = store.SaveMessage(testMsg2)
	require.NoError(t, err, "保存消息2失败")
	t.Log("✅ 两条消息保存成功")

	// 3. 测试消息去重
	t.Log("\nStep 3: 测试消息去重功能...")
	duplicateMsg := &storage.StoredMessage{
		ID:             "msg_test_003", // 不同的ID
		ConversationID: "cid_test_001",
		SenderID:       "user_alice",
		SenderNickname: "Alice",
		Content:        "Hello Bob!", // 和msg1内容相同
		Timestamp:      testMsg1.Timestamp, // 和msg1时间戳相同
		IsRead:         false,
		IsGroup:        false,
		NatsSeq:        1001, // 和msg1相同的NATS序列ID，应该被去重
	}

	err = store.SaveMessage(duplicateMsg)
	require.NoError(t, err, "保存重复消息不应该报错，应该被自动忽略")

	// 查询消息数量应该还是2条
	messages, err := store.GetMessages("cid_test_001", 10, nil)
	require.NoError(t, err, "查询消息失败")
	require.Equal(t, 2, len(messages), "重复消息应该被过滤，消息数量应该是2")
	t.Log("✅ 消息去重功能正常，重复消息被自动忽略")

	// 4. 测试消息查询
	t.Log("\nStep 4: 测试消息查询功能...")
	// 查询指定会话消息
	messages, err = store.GetMessages("cid_test_001", 10, nil)
	require.NoError(t, err, "查询会话消息失败")
	require.Equal(t, 2, len(messages), "应该查询到2条消息")
	require.Equal(t, "Hello Bob!", messages[0].Content, "第一条消息内容匹配")
	require.Equal(t, "Hi Alice!", messages[1].Content, "第二条消息内容匹配")

	// 查询所有消息
	allMessages, err := store.GetMessages("", 10, nil)
	require.NoError(t, err, "查询所有消息失败")
	require.Equal(t, 2, len(allMessages), "应该查询到2条消息")
	t.Log("✅ 消息查询功能正常")

	// 5. 测试会话存储
	t.Log("\nStep 5: 测试会话存储功能...")
	testConv := &storage.StoredConversation{
		ID:            "cid_test_001",
		Type:          "dm",
		LastMessageAt: time.Now(),
		CreatedAt:     time.Now(),
	}

	err = store.SaveConversation(testConv)
	require.NoError(t, err, "保存会话失败")

	conv, err := store.GetConversation("cid_test_001")
	require.NoError(t, err, "查询会话失败")
	require.NotNil(t, conv, "会话应该存在")
	require.Equal(t, "dm", conv.Type, "会话类型匹配")
	t.Log("✅ 会话存储功能正常")

	// 6. 测试好友公钥存储
	t.Log("\nStep 6: 测试好友公钥存储功能...")
	err = store.SaveFriendPubKey("user_bob", "test_pub_key_123")
	require.NoError(t, err, "保存好友公钥失败")

	pubKey, err := store.GetFriendPubKey("user_bob")
	require.NoError(t, err, "查询好友公钥失败")
	require.Equal(t, "test_pub_key_123", pubKey, "公钥内容匹配")
	t.Log("✅ 好友公钥存储功能正常")

	// 7. 测试群聊密钥存储
	t.Log("\nStep 7: 测试群聊密钥存储功能...")
	err = store.SaveGroupSymKey("group_test_001", "test_group_key_456")
	require.NoError(t, err, "保存群聊密钥失败")

	groupKey, err := store.GetGroupSymKey("group_test_001")
	require.NoError(t, err, "查询群聊密钥失败")
	require.Equal(t, "test_group_key_456", groupKey, "群密钥内容匹配")
	t.Log("✅ 群聊密钥存储功能正常")

	// 8. 测试消息搜索
	t.Log("\nStep 8: 测试消息搜索功能...")
	results, err := store.SearchMessages("Hello", 10)
	require.NoError(t, err, "搜索消息失败")
	require.Equal(t, 1, len(results), "应该搜索到1条包含'Hello'的消息")
	require.Equal(t, "Hello Bob!", results[0].Content, "搜索结果内容匹配")
	t.Log("✅ 消息搜索功能正常")

	t.Log("\n=== 所有测试通过 ===")
	t.Log("✅ SQLite存储功能全流程验证完成")
}
