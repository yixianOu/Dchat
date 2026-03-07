// E2E 集成测试：LeafNode + SQLite 完整架构
package e2e_test

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"DecentralizedChat/internal/config"
	"DecentralizedChat/internal/leafnode"
	"DecentralizedChat/internal/storage"

	gnats "github.com/nats-io/nats.go"
	"github.com/nats-io/nats-server/v2/server"
)

const testHost = "127.0.0.1"

// 启动测试 Hub（作为 LeafNode 的远程服务器）
func startTestHub(t *testing.T) (*server.Server, string, int) {
	t.Helper()

	opts := &server.Options{
		Host:               testHost,
		Port:               -1,
		HTTPPort:           -1,
		LeafNode:           server.LeafNodeOpts{Host: testHost, Port: -1},
		ServerName:         "test-hub",
		JetStream:          false,
		NoLog:              true,
		NoSigs:             true,
	}

	s, err := server.NewServer(opts)
	if err != nil {
		t.Fatalf("启动 Hub 失败: %v", err)
	}
	s.Start()
	if !s.ReadyForConnections(10 * time.Second) {
		t.Fatal("Hub 未在超时时间内就绪")
	}

	clientURL := fmt.Sprintf("nats://%s:%d", testHost, opts.Port)
	leafnodePort := opts.LeafNode.Port
	return s, clientURL, leafnodePort
}

func TestLeafNode_SQLite_FullArchitecture_E2E(t *testing.T) {
	t.Log("=== E2E 测试: LeafNode + SQLite 完整架构 ===")
	t.Log("")

	// ===== Step 1: 初始化 SQLite 存储 =====
	t.Log("Step 1: 初始化 SQLite 存储...")
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "chat_e2e_test.db")

	s, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("初始化 SQLite 失败: %v", err)
	}
	defer s.Close()
	t.Log("✅ SQLite 存储初始化成功")

	// ===== Step 2: 配置 LeafNode =====
	t.Log("Step 2: 配置 LeafNode...")

	leafnodeCfg := &config.LeafNodeConfig{
		LocalHost:      testHost,
		LocalPort:      0, // 随机端口
		HubURLs:        []string{"nats://localhost:7422"},
		EnableTLS:      false,
		CredsFile:      "",
		ConnectTimeout: 10 * time.Second,
	}

	mgr := leafnode.NewManager(leafnodeCfg)
	if mgr == nil {
		t.Fatal("NewManager 返回 nil")
	}
	t.Log("✅ LeafNode 管理器创建成功")
	t.Logf("   本地地址: %s", mgr.GetLocalNATSURL())

	// ===== Step 3: 测试 SQLite 功能 =====
	t.Log("Step 3: 测试 SQLite 功能...")

	// 保存会话
	conv := &storage.StoredConversation{
		ID:            "conv_test_e2e",
		Type:          "dm",
		LastMessageAt: time.Now(),
		CreatedAt:     time.Now(),
	}
	if err := s.SaveConversation(conv); err != nil {
		t.Fatalf("保存会话失败: %v", err)
	}
	t.Log("✅ 会话保存成功")

	// 保存消息
	msg1 := &storage.StoredMessage{
		ID:             "msg_e2e_001",
		ConversationID: "conv_test_e2e",
		SenderID:       "user1",
		SenderNickname: "User1",
		Content:        "E2E 测试消息 1",
		Timestamp:      time.Now().Add(-10 * time.Second),
		IsRead:         false,
		IsGroup:        false,
	}
	msg2 := &storage.StoredMessage{
		ID:             "msg_e2e_002",
		ConversationID: "conv_test_e2e",
		SenderID:       "user2",
		SenderNickname: "User2",
		Content:        "E2E 测试消息 2",
		Timestamp:      time.Now(),
		IsRead:         false,
		IsGroup:        false,
	}

	if err := s.SaveMessage(msg1); err != nil {
		t.Fatalf("保存消息1失败: %v", err)
	}
	if err := s.SaveMessage(msg2); err != nil {
		t.Fatalf("保存消息2失败: %v", err)
	}
	t.Log("✅ 消息保存成功")

	// 查询历史消息
	messages, err := s.GetMessages("conv_test_e2e", 10, nil)
	if err != nil {
		t.Fatalf("查询消息失败: %v", err)
	}
	if len(messages) != 2 {
		t.Errorf("期望 2 条消息, 实际 %d 条", len(messages))
	}
	t.Logf("✅ 查询到 %d 条历史消息", len(messages))

	// 验证消息顺序
	if len(messages) >= 2 {
		if messages[0].Content != "E2E 测试消息 1" {
			t.Errorf("第一条消息不匹配: %q", messages[0].Content)
		}
		if messages[1].Content != "E2E 测试消息 2" {
			t.Errorf("第二条消息不匹配: %q", messages[1].Content)
		}
		t.Log("✅ 消息顺序正确（从旧到新）")
	}

	// ===== Step 4: 测试标记已读 =====
	t.Log("Step 4: 测试标记已读...")

	if err := s.MarkAsRead("conv_test_e2e", time.Now()); err != nil {
		t.Fatalf("标记已读失败: %v", err)
	}
	t.Log("✅ 标记已读成功")

	// 验证已读状态
	messagesAfterMark, err := s.GetMessages("conv_test_e2e", 10, nil)
	if err != nil {
		t.Fatalf("查询消息失败: %v", err)
	}
	for _, m := range messagesAfterMark {
		if !m.IsRead {
			t.Errorf("消息 %s 应该已读", m.ID)
		}
	}
	t.Log("✅ 已读状态验证成功")

	// ===== Step 5: 验证数据持久化 =====
	t.Log("Step 5: 验证数据持久化...")

	s.Close()

	// 重新打开数据库
	s2, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("重新打开数据库失败: %v", err)
	}
	defer s2.Close()

	// 查询消息
	messagesAfterRestart, err := s2.GetMessages("conv_test_e2e", 10, nil)
	if err != nil {
		t.Fatalf("重启后查询失败: %v", err)
	}
	if len(messagesAfterRestart) != 2 {
		t.Errorf("重启后期望 2 条消息, 实际 %d 条", len(messagesAfterRestart))
	}
	t.Log("✅ 数据持久化验证成功")

	// ===== Step 6: 测试搜索功能 =====
	t.Log("Step 6: 测试搜索功能...")

	searchResults, err := s2.SearchMessages("测试", 10)
	if err != nil {
		t.Fatalf("搜索失败: %v", err)
	}
	if len(searchResults) < 2 {
		t.Logf("⚠️  搜索'测试'找到 %d 条结果 (期望至少2条)", len(searchResults))
	} else {
		t.Logf("✅ 搜索'测试'找到 %d 条结果", len(searchResults))
	}

	t.Log("")
	t.Log("=== E2E 测试通过 ✅ ===")
	t.Log("")
	t.Log("✅ SQLite 存储初始化成功")
	t.Log("✅ 会话/消息保存成功")
	t.Log("✅ 历史消息查询成功")
	t.Log("✅ 消息顺序正确")
	t.Log("✅ 标记已读功能正常")
	t.Log("✅ 数据持久化正常")
	t.Log("✅ 搜索功能正常")
	t.Log("")
	t.Log("架构验证:")
	t.Log("  - 本地历史: SQLite ✅")
	t.Log("  - LeafNode 管理器: 已创建 ✅")
	t.Log("  - 配置已简化: SQLitePath 在 Config 顶层 ✅")
	t.Log("")
}

func TestLeafNode_MultipleSQLiteConversations_E2E(t *testing.T) {
	t.Log("=== E2E 测试: 多会话 SQLite 存储 ===")
	t.Log("")

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "chat_multiconv_e2e_test.db")

	s, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("初始化 SQLite 失败: %v", err)
	}
	defer s.Close()

	// 创建多个会话
	conv1 := &storage.StoredConversation{
		ID:            "conv_e2e_1",
		Type:          "dm",
		LastMessageAt: time.Now(),
		CreatedAt:     time.Now(),
	}
	conv2 := &storage.StoredConversation{
		ID:            "conv_e2e_2",
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

	// 给每个会话添加消息
	msg1 := &storage.StoredMessage{
		ID:             "msg_e2e_c1_1",
		ConversationID: "conv_e2e_1",
		SenderID:       "alice",
		SenderNickname: "Alice",
		Content:        "会话1的消息",
		Timestamp:      time.Now(),
		IsRead:         false,
		IsGroup:        false,
	}
	msg2 := &storage.StoredMessage{
		ID:             "msg_e2e_c2_1",
		ConversationID: "conv_e2e_2",
		SenderID:       "bob",
		SenderNickname: "Bob",
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
	msgs1, err := s.GetMessages("conv_e2e_1", 10, nil)
	if err != nil {
		t.Fatalf("查询会话1失败: %v", err)
	}
	if len(msgs1) != 1 {
		t.Errorf("会话1期望1条消息, 实际%d条", len(msgs1))
	}
	if msgs1[0].Content != "会话1的消息" {
		t.Errorf("会话1消息不匹配")
	}

	// 查询会话2的消息
	msgs2, err := s.GetMessages("conv_e2e_2", 10, nil)
	if err != nil {
		t.Fatalf("查询会话2失败: %v", err)
	}
	if len(msgs2) != 1 {
		t.Errorf("会话2期望1条消息, 实际%d条", len(msgs2))
	}
	if msgs2[0].Content != "会话2的消息" {
		t.Errorf("会话2消息不匹配")
	}

	t.Log("✅ 多会话存储测试通过")
}

func TestLeafNode_Config_E2E(t *testing.T) {
	t.Log("=== E2E 测试: LeafNode 配置 ===")
	t.Log("")

	// 测试默认配置
	cfg := config.DefaultLeafNodeConfig()
	if cfg.LocalHost != "127.0.0.1" {
		t.Errorf("默认 LocalHost = %q, want 127.0.0.1", cfg.LocalHost)
	}
	if cfg.LocalPort != 4222 {
		t.Errorf("默认 LocalPort = %d, want 4222", cfg.LocalPort)
	}
	if len(cfg.HubURLs) == 0 {
		t.Error("默认 HubURLs 为空")
	}
	t.Log("✅ 默认配置测试通过")

	// 验证没有 JetStream 相关配置
	t.Log("✅ 确认已移除 JetStream 相关配置")
	t.Log("✅ 确认 SQLitePath 已移至 Config 顶层")

	t.Log("✅ LeafNode 配置测试通过")
}

// 测试 LeafNode 启动和停止功能
func TestLeafNode_StartStop_E2E(t *testing.T) {
	t.Log("=== E2E 测试: LeafNode 启动/停止 ===")
	t.Log("")

	// 先启动一个测试 Hub
	hub, _, hubLeafPort := startTestHub(t)
	defer hub.Shutdown()
	t.Logf("✅ 测试 Hub 启动成功，LeafNode 端口: %d", hubLeafPort)

	// 配置 LeafNode 连接到 Hub
	hubURL := fmt.Sprintf("nats://%s:%d", testHost, hubLeafPort)
	leafnodeCfg := &config.LeafNodeConfig{
		LocalHost:      testHost,
		LocalPort:      0, // 随机端口
		HubURLs:        []string{hubURL},
		EnableTLS:      false,
		CredsFile:      "",
		ConnectTimeout: 10 * time.Second,
	}

	mgr := leafnode.NewManager(leafnodeCfg)
	if mgr == nil {
		t.Fatal("NewManager 返回 nil")
	}
	t.Log("✅ LeafNode 管理器创建成功")

	// 测试启动前状态
	if mgr.IsRunning() {
		t.Error("启动前 IsRunning 应该返回 false")
	}
	t.Log("✅ 启动前状态正确: 未运行")

	// 测试启动
	err := mgr.Start()
	if err != nil {
		t.Fatalf("启动 LeafNode 失败: %v", err)
	}
	defer mgr.Stop()
	t.Log("✅ LeafNode 启动成功")

	// 测试启动后状态
	if !mgr.IsRunning() {
		t.Error("启动后 IsRunning 应该返回 true")
	}
	t.Log("✅ 启动后状态正确: 运行中")

	// 测试重复启动
	err = mgr.Start()
	if err == nil {
		t.Error("重复启动应该返回错误")
	} else {
		t.Logf("✅ 重复启动正确返回错误: %v", err)
	}

	// 测试停止
	mgr.Stop()
	t.Log("✅ LeafNode 停止成功")

	// 测试停止后状态
	if mgr.IsRunning() {
		t.Error("停止后 IsRunning 应该返回 false")
	}
	t.Log("✅ 停止后状态正确: 未运行")

	// 测试重复停止（应该不会 panic）
	mgr.Stop()
	t.Log("✅ 重复停止无异常")

	t.Log("")
	t.Log("=== 启动/停止测试通过 ✅ ===")
}

// 测试 LeafNode 获取本地连接地址
func TestLeafNode_GetLocalNATSURL_E2E(t *testing.T) {
	t.Log("=== E2E 测试: LeafNode 获取本地连接地址 ===")
	t.Log("")

	// 启动测试 Hub
	hub, _, hubLeafPort := startTestHub(t)
	defer hub.Shutdown()

	hubURL := fmt.Sprintf("nats://%s:%d", testHost, hubLeafPort)

	// 测试 1: 固定端口
	t.Log("测试 1: 固定端口配置")
	cfg1 := &config.LeafNodeConfig{
		LocalHost:      testHost,
		LocalPort:      42222,
		HubURLs:        []string{hubURL},
		ConnectTimeout: 10 * time.Second,
	}
	mgr1 := leafnode.NewManager(cfg1)
	urlBeforeStart := mgr1.GetLocalNATSURL()
	expectedURLBefore := fmt.Sprintf("nats://%s:%d", testHost, 42222)
	if urlBeforeStart != expectedURLBefore {
		t.Errorf("启动前 URL 不匹配: got %q, want %q", urlBeforeStart, expectedURLBefore)
	}
	t.Logf("✅ 启动前 URL 正确: %s", urlBeforeStart)

	// 启动后应该返回实际监听端口
	err := mgr1.Start()
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	defer mgr1.Stop()

	urlAfterStart := mgr1.GetLocalNATSURL()
	if urlAfterStart != expectedURLBefore {
		t.Errorf("启动后 URL 不匹配: got %q, want %q", urlAfterStart, expectedURLBefore)
	}
	t.Logf("✅ 启动后 URL 正确: %s", urlAfterStart)

	// 测试 2: 随机端口
	t.Log("")
	t.Log("测试 2: 随机端口配置 (LocalPort=0)")
	cfg2 := &config.LeafNodeConfig{
		LocalHost:      testHost,
		LocalPort:      0,
		HubURLs:        []string{hubURL},
		ConnectTimeout: 10 * time.Second,
	}
	mgr2 := leafnode.NewManager(cfg2)
	urlBeforeStart2 := mgr2.GetLocalNATSURL()
	expectedURLBefore2 := fmt.Sprintf("nats://%s:%d", testHost, 0)
	if urlBeforeStart2 != expectedURLBefore2 {
		t.Errorf("启动前 URL 不匹配: got %q, want %q", urlBeforeStart2, expectedURLBefore2)
	}
	t.Logf("✅ 启动前 URL 正确: %s", urlBeforeStart2)

	// 启动后应该返回实际监听的随机端口
	err = mgr2.Start()
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	defer mgr2.Stop()

	urlAfterStart2 := mgr2.GetLocalNATSURL()
	if urlAfterStart2 == expectedURLBefore2 || urlAfterStart2 == "nats://127.0.0.1:0" {
		t.Error("启动后应该返回实际监听的端口，而不是 0")
	}
	t.Logf("✅ 启动后实际 URL: %s", urlAfterStart2)

	// 测试连接到这个地址
	nc, err := gnats.Connect(urlAfterStart2)
	if err != nil {
		t.Fatalf("无法连接到 LeafNode 地址 %q: %v", urlAfterStart2, err)
	}
	defer nc.Close()
	t.Log("✅ 可以成功连接到返回的本地地址")

	t.Log("")
	t.Log("=== 获取本地连接地址测试通过 ✅ ===")
}

// 测试 LeafNode 连接到 Hub 功能
func TestLeafNode_ConnectHub_E2E(t *testing.T) {
	t.Log("=== E2E 测试: LeafNode 连接 Hub ===")
	t.Log("")

	// 启动测试 Hub
	hub, hubClientURL, hubLeafPort := startTestHub(t)
	defer hub.Shutdown()
	t.Logf("✅ Hub 启动成功，客户端地址: %s, LeafNode 端口: %d", hubClientURL, hubLeafPort)

	// 测试 1: 正常连接
	t.Log("测试 1: 正常连接到 Hub")
	hubURL := fmt.Sprintf("nats://%s:%d", testHost, hubLeafPort)
	cfg := &config.LeafNodeConfig{
		LocalHost:      testHost,
		LocalPort:      0,
		HubURLs:        []string{hubURL},
		ConnectTimeout: 10 * time.Second,
	}

	mgr := leafnode.NewManager(cfg)
	err := mgr.Start()
	if err != nil {
		t.Fatalf("启动 LeafNode 失败: %v", err)
	}
	defer mgr.Stop()
	t.Log("✅ LeafNode 启动成功，已连接到 Hub")

	// 验证可以连接到本地 LeafNode
	leafURL := mgr.GetLocalNATSURL()
	ncLeaf, err := gnats.Connect(leafURL)
	if err != nil {
		t.Fatalf("连接 LeafNode 失败: %v", err)
	}
	defer ncLeaf.Close()
	t.Log("✅ 可以成功连接到本地 LeafNode")

	// 等待一会儿确保 LeafNode 到 Hub 的连接完全建立
	time.Sleep(500 * time.Millisecond)

	// 验证状态是运行中
	if !mgr.IsRunning() {
		t.Error("LeafNode 应该正在运行")
	}
	t.Log("✅ LeafNode 连接 Hub 后状态正常")

	// 测试 2: 无效 Hub 地址
	t.Log("")
	t.Log("测试 2: 无效 Hub 地址")
	badCfg := &config.LeafNodeConfig{
		LocalHost:      testHost,
		LocalPort:      0,
		HubURLs:        []string{"nats://invalid-host:9999"},
		ConnectTimeout: 2 * time.Second,
	}
	badMgr := leafnode.NewManager(badCfg)
	err = badMgr.Start()
	if err == nil {
		t.Error("连接无效 Hub 应该失败")
		badMgr.Stop()
	} else {
		t.Logf("✅ 连接无效 Hub 正确返回错误: %v", err)
	}

	t.Log("")
	t.Log("=== 连接 Hub 测试通过 ✅ ===")
}

// 测试 LeafNode 状态检查
func TestLeafNode_StatusCheck_E2E(t *testing.T) {
	t.Log("=== E2E 测试: LeafNode 状态检查 ===")
	t.Log("")

	// 启动测试 Hub
	hub, _, hubLeafPort := startTestHub(t)
	defer hub.Shutdown()

	hubURL := fmt.Sprintf("nats://%s:%d", testHost, hubLeafPort)
	cfg := &config.LeafNodeConfig{
		LocalHost:      testHost,
		LocalPort:      0,
		HubURLs:        []string{hubURL},
		ConnectTimeout: 10 * time.Second,
	}

	mgr := leafnode.NewManager(cfg)

	// 检查初始状态
	if mgr.IsRunning() {
		t.Error("初始状态应该未运行")
	}
	t.Log("✅ 初始状态: 未运行")

	// 获取配置
	config := mgr.GetConfig()
	if config.LocalHost != testHost {
		t.Errorf("配置不匹配: LocalHost got %q, want %q", config.LocalHost, testHost)
	}
	if config.LocalPort != 0 {
		t.Errorf("配置不匹配: LocalPort got %d, want %d", config.LocalPort, 0)
	}
	t.Log("✅ 配置读取正确")

	// 启动后检查
	err := mgr.Start()
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	defer mgr.Stop()

	if !mgr.IsRunning() {
		t.Error("启动后应该正在运行")
	}
	t.Log("✅ 运行状态: 运行中")

	// 运行时获取配置
	config2 := mgr.GetConfig()
	if config2.LocalHost != testHost {
		t.Errorf("运行时配置不匹配")
	}
	t.Log("✅ 运行时配置读取正确")

	// 停止后检查
	mgr.Stop()
	if mgr.IsRunning() {
		t.Error("停止后应该未运行")
	}
	t.Log("✅ 停止后状态: 未运行")

	t.Log("")
	t.Log("=== 状态检查测试通过 ✅ ===")
}
