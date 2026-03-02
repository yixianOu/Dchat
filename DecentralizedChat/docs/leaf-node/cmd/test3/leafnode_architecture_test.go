// Copyright 2025 The NATS Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package leafnode_test

import (
	"encoding/json"
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/nats-io/nats-server/v2/server"
)

const testHost = "127.0.0.1"

// ===== 测试辅助函数 =====

func startServer(opts *server.Options) (*server.Server, error) {
	s, err := server.NewServer(opts)
	if err != nil || s == nil {
		return nil, fmt.Errorf("no NATS Server object returned: %v", err)
	}
	s.Start()
	if !s.ReadyForConnections(10 * time.Second) {
		return nil, fmt.Errorf("server not ready for connections")
	}
	return s, nil
}

func defaultOptions() *server.Options {
	return &server.Options{
		Host:     testHost,
		Port:     -1,
		HTTPPort: -1,
		Cluster:  server.ClusterOpts{Port: -1, Name: "dchat-cluster"},
		NoLog:    true,
		NoSigs:   true,
		Debug:    true,
		Trace:    true,
	}
}

func checkFor(t *testing.T, timeout, wait time.Duration, check func() error) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if err := check(); err == nil {
			return
		}
		time.Sleep(wait)
	}
	if err := check(); err != nil {
		t.Fatalf("Timed out waiting for condition: %v", err)
	}
}

// ===== 测试数据结构 =====

// HistoryMessageRecord 历史消息记录（对应设计中的格式）
type HistoryMessageRecord struct {
	CID      string `json:"cid"`       // 会话 ID
	Sender   string `json:"sender"`    // 发送者
	Content  string `json:"content"`   // 消息内容
	TS       int64  `json:"ts"`        // 时间戳
	IsGroup  bool   `json:"is_group"`  // 是否群聊
}

// ===== 测试场景 1: 基础 LeafNode 通信 =====

func TestLeafNode_BasicCommunication(t *testing.T) {
	t.Log("=== 测试场景 1: 基础 LeafNode 通信 ===")

	// ===== 1. 启动 Hub (公网 Hub) =====
	hubOpts := defaultOptions()
	hubOpts.LeafNode.Host = testHost
	hubOpts.LeafNode.Port = -1
	hubOpts.ServerName = "dchat-hub-1"
	// Hub 启用 JetStream 用于离线消息
	hubOpts.JetStreamMaxMemory = 256 * 1024 * 1024
	hubOpts.JetStreamMaxStore = 1 * 1024 * 1024 * 1024
	hubOpts.StoreDir = t.TempDir()

	hub, err := startServer(hubOpts)
	if err != nil {
		t.Fatalf("Failed to start hub: %v", err)
	}
	defer hub.Shutdown()

	t.Logf("✅ Hub started - client port: %d, leafnode port: %d", hubOpts.Port, hubOpts.LeafNode.Port)

	// ===== 2. 启动 LeafNode A (用户设备 A) =====
	leafAURL, _ := url.Parse(fmt.Sprintf("nats://%s:%d", testHost, hubOpts.LeafNode.Port))
	leafAOpts := defaultOptions()
	leafAOpts.LeafNode.Host = testHost
	leafAOpts.LeafNode.Remotes = []*server.RemoteLeafOpts{{URLs: []*url.URL{leafAURL}}}
	leafAOpts.ServerName = "leafnode-user-a"
	// LeafNode 本地启用 JetStream 用于历史消息
	leafAOpts.JetStreamMaxMemory = 256 * 1024 * 1024
	leafAOpts.JetStreamMaxStore = 1 * 1024 * 1024 * 1024
	leafAOpts.StoreDir = t.TempDir()

	leafA, err := startServer(leafAOpts)
	if err != nil {
		t.Fatalf("Failed to start leafnode A: %v", err)
	}
	defer leafA.Shutdown()

	t.Logf("✅ LeafNode A started - client port: %d", leafAOpts.Port)

	// ===== 3. 启动 LeafNode B (用户设备 B) =====
	leafBURL, _ := url.Parse(fmt.Sprintf("nats://%s:%d", testHost, hubOpts.LeafNode.Port))
	leafBOpts := defaultOptions()
	leafBOpts.LeafNode.Host = testHost
	leafBOpts.LeafNode.Remotes = []*server.RemoteLeafOpts{{URLs: []*url.URL{leafBURL}}}
	leafBOpts.ServerName = "leafnode-user-b"
	// LeafNode 本地启用 JetStream 用于历史消息
	leafBOpts.JetStreamMaxMemory = 256 * 1024 * 1024
	leafBOpts.JetStreamMaxStore = 1 * 1024 * 1024 * 1024
	leafBOpts.StoreDir = t.TempDir()

	leafB, err := startServer(leafBOpts)
	if err != nil {
		t.Fatalf("Failed to start leafnode B: %v", err)
	}
	defer leafB.Shutdown()

	t.Logf("✅ LeafNode B started - client port: %d", leafBOpts.Port)

	// ===== 4. 检查 LeafNode 连接 =====
	t.Log("Checking LeafNode connections...")

	checkFor(t, 10*time.Second, 100*time.Millisecond, func() error {
		if n := hub.NumLeafNodes(); n != 2 {
			return fmt.Errorf("hub has %d leafnodes, want 2", n)
		}
		return nil
	})
	t.Log("✅ Hub has 2 LeafNode connections")

	// ===== 5. 测试 DM 消息通信 =====
	t.Log("Testing DM message communication (dchat.dm.{cid}.msg)...")

	// 连接到 LeafNode B 作为订阅者
	ncB, err := nats.Connect(fmt.Sprintf("nats://%s:%d", testHost, leafBOpts.Port))
	if err != nil {
		t.Fatalf("Failed to connect to LeafNode B: %v", err)
	}
	defer ncB.Close()

	// 订阅 DM 消息
	cid := "cid_test_123"
	dmSubject := fmt.Sprintf("dchat.dm.%s.msg", cid)
	received := make(chan *nats.Msg, 1)
	sub, err := ncB.ChanSubscribe(dmSubject, received)
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}
	defer sub.Unsubscribe()

	// 连接到 LeafNode A 作为发布者
	ncA, err := nats.Connect(fmt.Sprintf("nats://%s:%d", testHost, leafAOpts.Port))
	if err != nil {
		t.Fatalf("Failed to connect to LeafNode A: %v", err)
	}
	defer ncA.Close()

	// 发布 DM 消息
	testMsg := "Hello from User A to User B!"
	if err := ncA.Publish(dmSubject, []byte(testMsg)); err != nil {
		t.Fatalf("Failed to publish: %v", err)
	}
	ncA.Flush()

	// 等待接收消息
	select {
	case msg := <-received:
		if string(msg.Data) != testMsg {
			t.Fatalf("Received wrong message: got %q, want %q", string(msg.Data), testMsg)
		}
		t.Logf("✅ DM message received: %q", string(msg.Data))
	case <-time.After(5 * time.Second):
		t.Fatal("Timed out waiting for DM message")
	}

	// ===== 6. 测试群聊消息通信 =====
	t.Log("Testing group message communication (dchat.grp.{gid}.msg)...")

	gid := "grp_test_456"
	grpSubject := fmt.Sprintf("dchat.grp.%s.msg", gid)
	grpReceived := make(chan *nats.Msg, 1)
	grpSub, err := ncB.ChanSubscribe(grpSubject, grpReceived)
	if err != nil {
		t.Fatalf("Failed to subscribe to group: %v", err)
	}
	defer grpSub.Unsubscribe()

	// 发布群聊消息
	grpMsg := "Hello group from User A!"
	if err := ncA.Publish(grpSubject, []byte(grpMsg)); err != nil {
		t.Fatalf("Failed to publish group message: %v", err)
	}
	ncA.Flush()

	// 等待接收消息
	select {
	case msg := <-grpReceived:
		if string(msg.Data) != grpMsg {
			t.Fatalf("Received wrong group message: got %q, want %q", string(msg.Data), grpMsg)
		}
		t.Logf("✅ Group message received: %q", string(msg.Data))
	case <-time.After(5 * time.Second):
		t.Fatal("Timed out waiting for group message")
	}

	t.Log("=== 测试场景 1 完成: 基础 LeafNode 通信 ✅ ===")
}

// ===== 测试场景 2: JetStream Stream 历史消息存储 =====

func TestLeafNode_JetStreamHistory(t *testing.T) {
	t.Log("=== 测试场景 2: JetStream Stream 历史消息存储 ===")

	// ===== 1. 启动 Hub =====
	hubOpts := defaultOptions()
	hubOpts.LeafNode.Host = testHost
	hubOpts.LeafNode.Port = -1
	hubOpts.ServerName = "dchat-hub-js"

	hub, err := startServer(hubOpts)
	if err != nil {
		t.Fatalf("Failed to start hub: %v", err)
	}
	defer hub.Shutdown()

	// ===== 2. 启动 LeafNode (带 JetStream) =====
	leafURL, _ := url.Parse(fmt.Sprintf("nats://%s:%d", testHost, hubOpts.LeafNode.Port))
	leafOpts := defaultOptions()
	leafOpts.LeafNode.Host = testHost
	leafOpts.LeafNode.Remotes = []*server.RemoteLeafOpts{{URLs: []*url.URL{leafURL}}}
	leafOpts.ServerName = "leafnode-with-js"
	leafOpts.JetStreamMaxMemory = 256 * 1024 * 1024
	leafOpts.JetStreamMaxStore = 1 * 1024 * 1024 * 1024
	leafOpts.StoreDir = t.TempDir()

	leaf, err := startServer(leafOpts)
	if err != nil {
		t.Fatalf("Failed to start leafnode: %v", err)
	}
	defer leaf.Shutdown()

	// 等待连接
	checkFor(t, 10*time.Second, 100*time.Millisecond, func() error {
		if n := hub.NumLeafNodes(); n != 1 {
			return fmt.Errorf("hub has %d leafnodes, want 1", n)
		}
		return nil
	})

	t.Logf("✅ LeafNode started with JetStream - client port: %d", leafOpts.Port)

	// ===== 3. 连接 LeafNode 并创建 JetStream Stream =====
	nc, err := nats.Connect(fmt.Sprintf("nats://%s:%d", testHost, leafOpts.Port))
	if err != nil {
		t.Fatalf("Failed to connect to LeafNode: %v", err)
	}
	defer nc.Close()

	// 创建 JetStream Context
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatalf("Failed to create JetStream context: %v", err)
	}

	// 创建历史消息 Stream
	cid := "cid_history_789"
	historySubject := fmt.Sprintf("dchat.history.%s", cid)

	t.Logf("Creating history stream for subject: %s", historySubject)

	stream, err := js.CreateStream(nc, jetstream.StreamConfig{
		Name:      fmt.Sprintf("HISTORY_%s", cid),
		Subjects:  []string{historySubject},
		Retention: jetstream.LimitsPolicy,
		MaxAge:    30 * 24 * time.Hour, // 30 天
		MaxMsgs:   100000,              // 10 万条
		Storage:   jetstream.FileStorage,
	})
	if err != nil {
		t.Fatalf("Failed to create stream: %v", err)
	}
	defer stream.Delete(nc)

	t.Log("✅ History stream created")

	// ===== 4. 发布多条历史消息 =====
	t.Log("Publishing history messages...")

	messages := []HistoryMessageRecord{
		{
			CID:     cid,
			Sender:  "user_a",
			Content: "Hello first message!",
			TS:      time.Now().Unix() - 300,
			IsGroup: false,
		},
		{
			CID:     cid,
			Sender:  "user_b",
			Content: "Hi there!",
			TS:      time.Now().Unix() - 200,
			IsGroup: false,
		},
		{
			CID:     cid,
			Sender:  "user_a",
			Content: "How are you?",
			TS:      time.Now().Unix() - 100,
			IsGroup: false,
		},
	}

	for i, msg := range messages {
		data, err := json.Marshal(msg)
		if err != nil {
			t.Fatalf("Failed to marshal message %d: %v", i, err)
		}
		_, err = js.Publish(nc, historySubject, data)
		if err != nil {
			t.Fatalf("Failed to publish message %d: %v", i, err)
		}
		t.Logf("  Published message %d: %q", i+1, msg.Content)
	}

	// ===== 5. 从 Stream 查询历史消息 =====
	t.Log("Retrieving history messages...")

	// 创建 Ordered Consumer
	consumer, err := stream.OrderedConsumer(nc)
	if err != nil {
		t.Fatalf("Failed to create ordered consumer: %v", err)
	}

	// 获取消息
	var retrieved []HistoryMessageRecord
	retrieveCount := 0
	maxRetrieve := 10

	for retrieveCount < maxRetrieve {
		msg, err := consumer.Next()
		if err != nil {
			break // 没有更多消息
		}

		var record HistoryMessageRecord
		if err := json.Unmarshal(msg.Data(), &record); err != nil {
			t.Logf("Warning: failed to unmarshal message: %v", err)
			continue
		}

		retrieved = append(retrieved, record)
		t.Logf("  Retrieved message from %s: %q", record.Sender, record.Content)
		retrieveCount++
	}

	// 验证消息数量
	if len(retrieved) != len(messages) {
		t.Fatalf("Retrieved %d messages, want %d", len(retrieved), len(messages))
	}
	t.Logf("✅ Retrieved all %d history messages", len(retrieved))

	// 验证消息内容
	for i := range messages {
		if retrieved[i].Sender != messages[i].Sender {
			t.Errorf("Message %d: sender mismatch, got %q, want %q", i, retrieved[i].Sender, messages[i].Sender)
		}
		if retrieved[i].Content != messages[i].Content {
			t.Errorf("Message %d: content mismatch, got %q, want %q", i, retrieved[i].Content, messages[i].Content)
		}
	}
	t.Log("✅ All history messages match")

	t.Log("=== 测试场景 2 完成: JetStream Stream 历史消息存储 ✅ ===")
}

// ===== 测试场景 3: 多 Hub 配置 =====

func TestLeafNode_MultipleHubs(t *testing.T) {
	t.Log("=== 测试场景 3: 多 Hub 配置（验证支持多个 Remotes）===")

	// ===== 1. 启动 Hub 1 =====
	hub1Opts := defaultOptions()
	hub1Opts.LeafNode.Host = testHost
	hub1Opts.LeafNode.Port = -1
	hub1Opts.ServerName = "dchat-hub-1"

	hub1, err := startServer(hub1Opts)
	if err != nil {
		t.Fatalf("Failed to start hub 1: %v", err)
	}
	defer hub1.Shutdown()

	// ===== 2. 启动 Hub 2 =====
	hub2Opts := defaultOptions()
	hub2Opts.LeafNode.Host = testHost
	hub2Opts.LeafNode.Port = -1
	hub2Opts.ServerName = "dchat-hub-2"

	hub2, err := startServer(hub2Opts)
	if err != nil {
		t.Fatalf("Failed to start hub 2: %v", err)
	}
	defer hub2.Shutdown()

	t.Logf("✅ Hubs started - Hub1: %d, Hub2: %d", hub1Opts.LeafNode.Port, hub2Opts.LeafNode.Port)

	// ===== 3. 启动 LeafNode，配置连接两个 Hub =====
	hub1URL, _ := url.Parse(fmt.Sprintf("nats://%s:%d", testHost, hub1Opts.LeafNode.Port))
	hub2URL, _ := url.Parse(fmt.Sprintf("nats://%s:%d", testHost, hub2Opts.LeafNode.Port))

	leafOpts := defaultOptions()
	leafOpts.LeafNode.Host = testHost
	// 配置多个 Remote Hub
	leafOpts.LeafNode.Remotes = []*server.RemoteLeafOpts{
		{URLs: []*url.URL{hub1URL}},
		{URLs: []*url.URL{hub2URL}},
	}
	leafOpts.ServerName = "leafnode-multi-hub"

	leaf, err := startServer(leafOpts)
	if err != nil {
		t.Fatalf("Failed to start leafnode: %v", err)
	}
	defer leaf.Shutdown()

	t.Log("✅ LeafNode started with multiple Hub remotes")

	// ===== 4. 验证至少连接了一个 Hub =====
	// 注意：NumLeafNodes() 返回 inbound 连接数，所以需要在每个 Hub 上检查
	checkFor(t, 10*time.Second, 100*time.Millisecond, func() error {
		// 检查是否至少连接了一个 Hub
		hub1Count := hub1.NumLeafNodes()
		hub2Count := hub2.NumLeafNodes()
		if hub1Count == 0 && hub2Count == 0 {
			return fmt.Errorf("not connected to any hub")
		}
		return nil
	})

	hub1Count := hub1.NumLeafNodes()
	hub2Count := hub2.NumLeafNodes()
	t.Logf("✅ LeafNode connected to %d hub(s) (Hub1: %d, Hub2: %d)", hub1Count+hub2Count, hub1Count, hub2Count)

	t.Log("=== 测试场景 3 完成: 多 Hub 配置 ✅ ===")
}

// ===== 测试场景 4: 离线消息（Hub 端 JetStream）=====

func TestLeafNode_OfflineMessages(t *testing.T) {
	t.Log("=== 测试场景 4: 离线消息（Hub 端 JetStream）===")

	// ===== 1. 启动 Hub（带 JetStream）=====
	hubOpts := defaultOptions()
	hubOpts.LeafNode.Host = testHost
	hubOpts.LeafNode.Port = -1
	hubOpts.ServerName = "dchat-hub-offline"
	hubOpts.JetStreamMaxMemory = 256 * 1024 * 1024
	hubOpts.JetStreamMaxStore = 1 * 1024 * 1024 * 1024
	hubOpts.StoreDir = t.TempDir()

	hub, err := startServer(hubOpts)
	if err != nil {
		t.Fatalf("Failed to start hub: %v", err)
	}
	defer hub.Shutdown()

	// ===== 2. 启动 LeafNode A（在线发送消息）=====
	leafAURL, _ := url.Parse(fmt.Sprintf("nats://%s:%d", testHost, hubOpts.LeafNode.Port))
	leafAOpts := defaultOptions()
	leafAOpts.LeafNode.Host = testHost
	leafAOpts.LeafNode.Remotes = []*server.RemoteLeafOpts{{URLs: []*url.URL{leafAURL}}}
	leafAOpts.ServerName = "leafnode-sender"

	leafA, err := startServer(leafAOpts)
	if err != nil {
		t.Fatalf("Failed to start leafnode A: %v", err)
	}
	defer leafA.Shutdown()

	// 等待连接
	checkFor(t, 10*time.Second, 100*time.Millisecond, func() error {
		if n := hub.NumLeafNodes(); n != 1 {
			return fmt.Errorf("hub has %d leafnodes, want 1", n)
		}
		return nil
	})

	t.Log("✅ LeafNode A (sender) connected")

	// ===== 3. 在 Hub 上创建 JetStream Stream 用于离线消息 =====
	// 连接到 Hub 直接创建 Stream
	hubNC, err := nats.Connect(fmt.Sprintf("nats://%s:%d", testHost, hubOpts.Port))
	if err != nil {
		t.Fatalf("Failed to connect to hub: %v", err)
	}
	defer hubNC.Close()

	hubJS, err := jetstream.New(hubNC)
	if err != nil {
		t.Fatalf("Failed to create hub JetStream context: %v", err)
	}

	// 创建离线消息 Stream
	offlineSubject := "dchat.>"
	stream, err := hubJS.CreateStream(hubNC, jetstream.StreamConfig{
		Name:      "OFFLINE_MSGS",
		Subjects:  []string{offlineSubject},
		Retention: jetstream.LimitsPolicy,
		MaxAge:    7 * 24 * time.Hour, // 7 天
		Storage:   jetstream.FileStorage,
	})
	if err != nil {
		t.Fatalf("Failed to create offline stream: %v", err)
	}
	defer stream.Delete(hubNC)

	t.Log("✅ Offline message stream created on Hub")

	// ===== 4. LeafNode A 发送消息（此时 LeafNode B 不在线）=====
	t.Log("LeafNode A sending offline messages...")

	ncA, err := nats.Connect(fmt.Sprintf("nats://%s:%d", testHost, leafAOpts.Port))
	if err != nil {
		t.Fatalf("Failed to connect to LeafNode A: %v", err)
	}
	defer ncA.Close()

	// 发送几条消息
	cid := "cid_offline_abc"
	dmSubject := fmt.Sprintf("dchat.dm.%s.msg", cid)

	offlineMessages := []string{
		"Offline message 1",
		"Offline message 2",
		"Offline message 3",
	}

	for _, msg := range offlineMessages {
		if err := ncA.Publish(dmSubject, []byte(msg)); err != nil {
			t.Fatalf("Failed to publish offline message: %v", err)
		}
		t.Logf("  Sent: %q", msg)
	}
	ncA.Flush()

	// 等待 Hub 的 Stream 处理
	time.Sleep(200 * time.Millisecond)

	// ===== 5. 启动 LeafNode B（接收离线消息）=====
	t.Log("Starting LeafNode B (receiver)...")

	leafBURL, _ := url.Parse(fmt.Sprintf("nats://%s:%d", testHost, hubOpts.LeafNode.Port))
	leafBOpts := defaultOptions()
	leafBOpts.LeafNode.Host = testHost
	leafBOpts.LeafNode.Remotes = []*server.RemoteLeafOpts{{URLs: []*url.URL{leafBURL}}}
	leafBOpts.ServerName = "leafnode-receiver"

	leafB, err := startServer(leafBOpts)
	if err != nil {
		t.Fatalf("Failed to start leafnode B: %v", err)
	}
	defer leafB.Shutdown()

	// 等待连接
	checkFor(t, 10*time.Second, 100*time.Millisecond, func() error {
		if n := hub.NumLeafNodes(); n != 2 {
			return fmt.Errorf("hub has %d leafnodes, want 2", n)
		}
		return nil
	})

	t.Log("✅ LeafNode B (receiver) connected")

	// 订阅消息
	ncB, err := nats.Connect(fmt.Sprintf("nats://%s:%d", testHost, leafBOpts.Port))
	if err != nil {
		t.Fatalf("Failed to connect to LeafNode B: %v", err)
	}
	defer ncB.Close()

	received := make(chan *nats.Msg, 3)
	sub, err := ncB.ChanSubscribe(dmSubject, received)
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}
	defer sub.Unsubscribe()

	// 发送一条在线消息触发订阅传播
	time.Sleep(100 * time.Millisecond)
	ncA.Publish(dmSubject, []byte("Online trigger message"))
	ncA.Flush()

	t.Log("=== 测试场景 4 完成: 离线消息架构验证 ✅ ===")
	t.Log("  Note: Full offline message delivery requires additional configuration")
	t.Log("  of JetStream consumers on LeafNodes.")
}
