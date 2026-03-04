// E2E 集成测试：SQLite 本地存储
package e2e_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"DecentralizedChat/internal/storage"
)

func TestSQLiteStorage_E2E(t *testing.T) {
	t.Log("=== E2E 测试: SQLite 本地存储 ===")
	t.Log("")

	// 创建临时数据库文件
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "chat_test.db")

	t.Logf("数据库路径: %s", dbPath)

	// ===== Step 1: 初始化存储 =====
	t.Log("Step 1: 初始化 SQLite 存储...")

	s, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("初始化存储失败: %v", err)
	}
	defer s.Close()

	t.Log("✅ 存储初始化成功")

	// ===== Step 2: 保存会话 =====
	t.Log("Step 2: 保存会话...")

	conv := &storage.StoredConversation{
		ID:            "conv_alice_bob_123",
		Type:          "dm",
		LastMessageAt: time.Now(),
		CreatedAt:     time.Now(),
	}

	if err := s.SaveConversation(conv); err != nil {
		t.Fatalf("保存会话失败: %v", err)
	}

	t.Logf("✅ 会话保存成功: %s", conv.ID)

	// ===== Step 3: 保存消息 =====
	t.Log("Step 3: 保存消息...")

	now := time.Now()
	messages := []*storage.StoredMessage{
		{
			ID:             "msg_001",
			ConversationID: "conv_alice_bob_123",
			SenderID:       "alice_123",
			SenderNickname: "Alice",
			Content:        "你好！",
			Timestamp:      now.Add(-10 * time.Minute),
			IsRead:         false,
			IsGroup:        false,
		},
		{
			ID:             "msg_002",
			ConversationID: "conv_alice_bob_123",
			SenderID:       "bob_456",
			SenderNickname: "Bob",
			Content:        "你好 Alice！",
			Timestamp:      now.Add(-5 * time.Minute),
			IsRead:         false,
			IsGroup:        false,
		},
		{
			ID:             "msg_003",
			ConversationID: "conv_alice_bob_123",
			SenderID:       "alice_123",
			SenderNickname: "Alice",
			Content:        "今天天气怎么样？",
			Timestamp:      now,
			IsRead:         false,
			IsGroup:        false,
		},
	}

	for _, msg := range messages {
		if err := s.SaveMessage(msg); err != nil {
			t.Fatalf("保存消息失败: %v", err)
		}
		t.Logf("✅ 消息保存成功: %s", msg.ID)
	}

	// ===== Step 4: 查询历史消息 =====
	t.Log("Step 4: 查询历史消息...")

	fetched, err := s.GetMessages("conv_alice_bob_123", 10, nil)
	if err != nil {
		t.Fatalf("查询消息失败: %v", err)
	}

	t.Logf("📥 查到 %d 条消息", len(fetched))

	if len(fetched) != len(messages) {
		t.Errorf("消息数量不匹配: 期望 %d, 实际 %d", len(messages), len(fetched))
	}

	for i, msg := range fetched {
		t.Logf("   %d: [%s] %s: %s", i+1, msg.Timestamp.Format("15:04:05"), msg.SenderNickname, msg.Content)
	}

	// 验证消息顺序（从旧到新）
	if len(fetched) >= 3 {
		if fetched[0].Content != "你好！" {
			t.Errorf("第一条消息不匹配: got %q, want %q", fetched[0].Content, "你好！")
		}
		if fetched[2].Content != "今天天气怎么样？" {
			t.Errorf("第三条消息不匹配: got %q, want %q", fetched[2].Content, "今天天气怎么样？")
		}
	}

	// ===== Step 5: 标记已读 =====
	t.Log("Step 5: 标记已读...")

	if err := s.MarkAsRead("conv_alice_bob_123", now); err != nil {
		t.Fatalf("标记已读失败: %v", err)
	}

	t.Log("✅ 标记已读成功")

	// 验证已读状态
	fetchedAfterMark, err := s.GetMessages("conv_alice_bob_123", 10, nil)
	if err != nil {
		t.Fatalf("查询消息失败: %v", err)
	}

	for _, msg := range fetchedAfterMark {
		if !msg.IsRead {
			t.Errorf("消息 %s 应该已读", msg.ID)
		}
	}

	// ===== Step 6: 搜索消息 =====
	t.Log("Step 6: 搜索消息...")

	searchResults, err := s.SearchMessages("天气", 10)
	if err != nil {
		t.Fatalf("搜索失败: %v", err)
	}

	t.Logf("🔍 搜索结果: %d 条消息", len(searchResults))

	if len(searchResults) == 0 {
		t.Error("期望找到包含'天气'的消息")
	} else {
		t.Logf("   找到: %q", searchResults[0].Content)
	}

	// ===== Step 7: 测试重启后数据依然存在 =====
	t.Log("Step 7: 测试重启后数据持久化...")

	s.Close()

	// 重新打开
	s2, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("重新打开存储失败: %v", err)
	}
	defer s2.Close()

	fetchedAfterRestart, err := s2.GetMessages("conv_alice_bob_123", 10, nil)
	if err != nil {
		t.Fatalf("重启后查询失败: %v", err)
	}

	if len(fetchedAfterRestart) != len(messages) {
		t.Errorf("重启后消息数量不匹配: 期望 %d, 实际 %d", len(messages), len(fetchedAfterRestart))
	}

	t.Logf("✅ 重启后数据完整: %d 条消息", len(fetchedAfterRestart))

	t.Log("")
	t.Log("=== E2E 测试通过 ✅ ===")
	t.Log("")
	t.Log("✅ 存储初始化成功")
	t.Log("✅ 会话保存/查询成功")
	t.Log("✅ 消息保存/查询成功")
	t.Log("✅ 消息顺序正确（从旧到新）")
	t.Log("✅ 标记已读功能正常")
	t.Log("✅ 搜索功能正常")
	t.Log("✅ 数据持久化正常（重启后依然存在）")
	t.Log("")
}

func TestSQLiteStorage_MultipleConversations_E2E(t *testing.T) {
	t.Log("=== E2E 测试: 多会话存储 ===")
	t.Log("")

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "chat_multiconv_test.db")

	s, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("初始化存储失败: %v", err)
	}
	defer s.Close()

	// 创建两个会话
	conv1 := &storage.StoredConversation{
		ID:            "conv_1",
		Type:          "dm",
		LastMessageAt: time.Now(),
		CreatedAt:     time.Now(),
	}
	conv2 := &storage.StoredConversation{
		ID:            "conv_2",
		Type:          "group",
		LastMessageAt: time.Now(),
		CreatedAt:     time.Now(),
	}

	if err := s.SaveConversation(conv1); err != nil {
		t.Fatalf("保存会话1失败: %v", err)
	}
	if err := s.SaveConversation(conv2); err != nil {
		t.Fatalf("保存会话2失败: %v", err)
	}

	// 给每个会话保存消息
	msg1 := &storage.StoredMessage{
		ID:             "msg_c1_1",
		ConversationID: "conv_1",
		SenderID:       "user1",
		SenderNickname: "User1",
		Content:        "会话1的消息",
		Timestamp:      time.Now(),
		IsRead:         false,
		IsGroup:        false,
	}
	msg2 := &storage.StoredMessage{
		ID:             "msg_c2_1",
		ConversationID: "conv_2",
		SenderID:       "user2",
		SenderNickname: "User2",
		Content:        "会话2的消息",
		Timestamp:      time.Now(),
		IsRead:         false,
		IsGroup:        true,
	}

	if err := s.SaveMessage(msg1); err != nil {
		t.Fatalf("保存消息1失败: %v", err)
	}
	if err := s.SaveMessage(msg2); err != nil {
		t.Fatalf("保存消息2失败: %v", err)
	}

	// 查询会话1的消息
	msgs1, err := s.GetMessages("conv_1", 10, nil)
	if err != nil {
		t.Fatalf("查询会话1失败: %v", err)
	}
	if len(msgs1) != 1 {
		t.Errorf("会话1应该有1条消息，实际有%d条", len(msgs1))
	}

	// 查询会话2的消息
	msgs2, err := s.GetMessages("conv_2", 10, nil)
	if err != nil {
		t.Fatalf("查询会话2失败: %v", err)
	}
	if len(msgs2) != 1 {
		t.Errorf("会话2应该有1条消息，实际有%d条", len(msgs2))
	}

	// 验证消息内容
	if msgs1[0].Content != "会话1的消息" {
		t.Errorf("会话1消息不匹配")
	}
	if msgs2[0].Content != "会话2的消息" {
		t.Errorf("会话2消息不匹配")
	}

	t.Log("✅ 多会话存储测试通过")
}

func TestSQLiteStorage_GetConversation_E2E(t *testing.T) {
	t.Log("=== E2E 测试: 获取会话 ===")
	t.Log("")

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "chat_getconv_test.db")

	s, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("初始化存储失败: %v", err)
	}
	defer s.Close()

	// 保存会话
	conv := &storage.StoredConversation{
		ID:            "conv_test_get",
		Type:          "dm",
		LastMessageAt: time.Now(),
		CreatedAt:     time.Now(),
	}
	if err := s.SaveConversation(conv); err != nil {
		t.Fatalf("保存会话失败: %v", err)
	}

	// 获取会话
	fetched, err := s.GetConversation("conv_test_get")
	if err != nil {
		t.Fatalf("获取会话失败: %v", err)
	}
	if fetched == nil {
		t.Fatal("会话不存在")
	}
	if fetched.ID != conv.ID {
		t.Errorf("会话ID不匹配: got %s, want %s", fetched.ID, conv.ID)
	}
	if fetched.Type != conv.Type {
		t.Errorf("会话类型不匹配")
	}

	// 获取不存在的会话
	notFound, err := s.GetConversation("conv_not_exists")
	if err != nil {
		t.Fatalf("获取不存在的会话时出错: %v", err)
	}
	if notFound != nil {
		t.Error("不存在的会话应该返回nil")
	}

	t.Log("✅ 获取会话测试通过")
}

// 测试数据库文件权限和实际存储
func TestSQLiteStorage_FilePersistence_E2E(t *testing.T) {
	t.Log("=== E2E 测试: 文件持久化 ===")
	t.Log("")

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "chat_file_test.db")

	// 第一次打开并写入
	s1, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("第一次打开失败: %v", err)
	}

	msg := &storage.StoredMessage{
		ID:             "msg_file_1",
		ConversationID: "conv_file",
		SenderID:       "user1",
		SenderNickname: "User1",
		Content:        "持久化测试消息",
		Timestamp:      time.Now(),
		IsRead:         false,
		IsGroup:        false,
	}
	if err := s1.SaveMessage(msg); err != nil {
		t.Fatalf("保存消息失败: %v", err)
	}
	s1.Close()

	// 验证文件存在
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Fatal("数据库文件不存在")
	}
	t.Log("✅ 数据库文件已创建")

	// 第二次打开验证
	s2, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("第二次打开失败: %v", err)
	}
	defer s2.Close()

	msgs, err := s2.GetMessages("conv_file", 10, nil)
	if err != nil {
		t.Fatalf("查询消息失败: %v", err)
	}
	if len(msgs) != 1 {
		t.Errorf("应该有1条消息，实际有%d条", len(msgs))
	}

	t.Log("✅ 文件持久化测试通过")
}
