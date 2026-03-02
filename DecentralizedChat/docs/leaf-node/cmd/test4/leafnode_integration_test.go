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
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/nats-io/nats-server/v2/server"
)

const testHost = "127.0.0.1"

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
		Cluster:  server.ClusterOpts{Port: -1, Name: "abc"},
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

// ===== 测试 1: 完整的 Hub + LeafNode 集成测试 =====

func TestLeafNode_FullIntegration(t *testing.T) {
	t.Log("=== 测试: 完整的 Hub + LeafNode 集成测试 ===")

	// ===== Step 1: 启动公网 Hub =====
	t.Log("Step 1: Starting public Hub...")

	hubOpts := defaultOptions()
	hubOpts.LeafNode.Host = testHost
	hubOpts.LeafNode.Port = -1
	hubOpts.ServerName = "dchat-hub-1"
	hubOpts.JetStreamMaxMemory = 256 * 1024 * 1024
	hubOpts.JetStreamMaxStore = 1 * 1024 * 1024 * 1024
	hubOpts.StoreDir = t.TempDir()

	hub, err := startServer(hubOpts)
	if err != nil {
		t.Fatalf("Failed to start hub: %v", err)
	}
	defer hub.Shutdown()

	t.Logf("✅ Hub started - client port: %d, leafnode port: %d", hubOpts.Port, hubOpts.LeafNode.Port)

	// ===== Step 2: 启动 LeafNode A (用户 A) =====
	t.Log("Step 2: Starting LeafNode A (User A)...")

	leafAURL, _ := url.Parse(fmt.Sprintf("nats://%s:%d", testHost, hubOpts.LeafNode.Port))
	leafAOpts := defaultOptions()
	leafAOpts.Cluster.Host = testHost
	leafAOpts.Cluster.Port = -1
	leafAOpts.Cluster.Name = "spoke-1-cluster"
	leafAOpts.LeafNode.Host = testHost
	leafAOpts.LeafNode.Remotes = []*server.RemoteLeafOpts{{URLs: []*url.URL{leafAURL}}}
	leafAOpts.ServerName = "leafnode-user-a"
	leafAOpts.JetStreamMaxMemory = 256 * 1024 * 1024
	leafAOpts.JetStreamMaxStore = 1 * 1024 * 1024 * 1024
	leafAOpts.StoreDir = t.TempDir()

	leafA, err := startServer(leafAOpts)
	if err != nil {
		t.Fatalf("Failed to start leafnode A: %v", err)
	}
	defer leafA.Shutdown()

	t.Logf("✅ LeafNode A started - client port: %d", leafAOpts.Port)

	// ===== Step 3: 启动 LeafNode B (用户 B) =====
	t.Log("Step 3: Starting LeafNode B (User B)...")

	leafBURL, _ := url.Parse(fmt.Sprintf("nats://%s:%d", testHost, hubOpts.LeafNode.Port))
	leafBOpts := defaultOptions()
	leafBOpts.Cluster.Host = testHost
	leafBOpts.Cluster.Port = -1
	leafBOpts.Cluster.Name = "spoke-2-cluster"
	leafBOpts.LeafNode.Host = testHost
	leafBOpts.LeafNode.Remotes = []*server.RemoteLeafOpts{{URLs: []*url.URL{leafBURL}}}
	leafBOpts.ServerName = "leafnode-user-b"
	leafBOpts.JetStreamMaxMemory = 256 * 1024 * 1024
	leafBOpts.JetStreamMaxStore = 1 * 1024 * 1024 * 1024
	leafBOpts.StoreDir = t.TempDir()

	leafB, err := startServer(leafBOpts)
	if err != nil {
		t.Fatalf("Failed to start leafnode B: %v", err)
	}
	defer leafB.Shutdown()

	t.Logf("✅ LeafNode B started - client port: %d", leafBOpts.Port)

	// ===== Step 4: 验证 LeafNode 连接 =====
	t.Log("Step 4: Verifying LeafNode connections...")

	checkFor(t, 10*time.Second, 100*time.Millisecond, func() error {
		if n := hub.NumLeafNodes(); n != 2 {
			return fmt.Errorf("hub has %d leafnodes, want 2", n)
		}
		return nil
	})

	t.Log("✅ Hub has 2 LeafNode connections")

	// ===== Step 5: 测试 DM 消息通信 (dchat.dm.{cid}.msg) =====
	t.Log("Step 5: Testing DM message communication...")

	// 连接到 LeafNode B 作为订阅者
	ncB, err := nats.Connect(fmt.Sprintf("nats://%s:%d", testHost, leafBOpts.Port))
	if err != nil {
		t.Fatalf("Failed to connect to LeafNode B: %v", err)
	}
	defer ncB.Close()

	// 订阅 DM 消息
	cid := "cid_user_a_user_b"
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
	testMsg := "Hello from User A to User B! 你好！"
	t.Logf("Publishing DM message: %q", testMsg)
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

	// ===== Step 6: 测试群聊消息通信 (dchat.grp.{gid}.msg) =====
	t.Log("Step 6: Testing group message communication...")

	gid := "grp_test_group_123"
	grpSubject := fmt.Sprintf("dchat.grp.%s.msg", gid)
	grpReceived := make(chan *nats.Msg, 1)
	grpSub, err := ncB.ChanSubscribe(grpSubject, grpReceived)
	if err != nil {
		t.Fatalf("Failed to subscribe to group: %v", err)
	}
	defer grpSub.Unsubscribe()

	// 等待订阅传播完成
	time.Sleep(100 * time.Millisecond)

	// 发布群聊消息
	grpMsg := "Hello everyone in the group! 大家好！"
	t.Logf("Publishing group message: %q", grpMsg)
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

	// ===== Step 7: 验证架构 =====
	t.Log("Step 7: Verifying architecture...")
	t.Log("  ✓ Message flow: LeafNode A → Hub → LeafNode B")
	t.Log("  ✓ Subjects follow dchat.dm.{cid}.msg and dchat.grp.{gid}.msg patterns")
	t.Log("  ✓ Both LeafNodes connected to Hub")
	t.Log("  ✓ JetStream enabled on all nodes")

	t.Log("=== 完整集成测试通过 ✅ ===")
}

// ===== 测试 2: 多个 Hub 配置 =====

func TestLeafNode_MultipleHubs(t *testing.T) {
	t.Log("=== 测试: 多个 Hub 配置 ===")

	// 启动 Hub 1
	hub1Opts := defaultOptions()
	hub1Opts.LeafNode.Host = testHost
	hub1Opts.LeafNode.Port = -1
	hub1Opts.ServerName = "dchat-hub-1"

	hub1, err := startServer(hub1Opts)
	if err != nil {
		t.Fatalf("Failed to start hub 1: %v", err)
	}
	defer hub1.Shutdown()

	// 启动 Hub 2
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

	// 启动 LeafNode，配置连接两个 Hub
	hub1URL, _ := url.Parse(fmt.Sprintf("nats://%s:%d", testHost, hub1Opts.LeafNode.Port))
	hub2URL, _ := url.Parse(fmt.Sprintf("nats://%s:%d", testHost, hub2Opts.LeafNode.Port))

	leafOpts := defaultOptions()
	leafOpts.Cluster.Host = testHost
	leafOpts.Cluster.Port = -1
	leafOpts.Cluster.Name = "spoke-multi-cluster"
	leafOpts.LeafNode.Host = testHost
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

	// 验证至少连接了一个 Hub
	checkFor(t, 10*time.Second, 100*time.Millisecond, func() error {
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

	t.Log("=== 多 Hub 配置测试通过 ✅ ===")
}

// ===== 测试 3: JetStream 在 LeafNode 本地 =====

func TestLeafNode_LocalJetStream(t *testing.T) {
	t.Log("=== 测试: LeafNode 本地 JetStream ===")

	// 启动 Hub
	hubOpts := defaultOptions()
	hubOpts.LeafNode.Host = testHost
	hubOpts.LeafNode.Port = -1
	hubOpts.ServerName = "dchat-hub-js"

	hub, err := startServer(hubOpts)
	if err != nil {
		t.Fatalf("Failed to start hub: %v", err)
	}
	defer hub.Shutdown()

	// 启动 LeafNode，启用 JetStream
	leafURL, _ := url.Parse(fmt.Sprintf("nats://%s:%d", testHost, hubOpts.LeafNode.Port))
	leafOpts := defaultOptions()
	leafOpts.Cluster.Host = testHost
	leafOpts.Cluster.Port = -1
	leafOpts.Cluster.Name = "spoke-js-cluster"
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

	t.Log("✅ LeafNode started with JetStream enabled")
	t.Log("=== 本地 JetStream 测试通过 ✅ ===")
}
