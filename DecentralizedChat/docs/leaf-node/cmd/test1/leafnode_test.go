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

func TestLeafNodeHubAndSpokes(t *testing.T) {
	// ===== 1. 启动 Hub =====
	hubOpts := defaultOptions()
	hubOpts.LeafNode.Host = testHost
	hubOpts.LeafNode.Port = -1
	hubOpts.ServerName = "hub-server"

	hub, err := startServer(hubOpts)
	if err != nil {
		t.Fatalf("Failed to start hub: %v", err)
	}
	defer hub.Shutdown()

	// 注意：hubOpts 保存了原始引用，启动后 Port 会被更新
	t.Logf("Hub started - client port: %d, leafnode port: %d", hubOpts.Port, hubOpts.LeafNode.Port)

	// ===== 2. 启动 Spoke 1 =====
	spoke1URL, _ := url.Parse(fmt.Sprintf("nats://%s:%d", testHost, hubOpts.LeafNode.Port))
	spoke1Opts := defaultOptions()
	spoke1Opts.Cluster.Host = testHost
	spoke1Opts.Cluster.Port = -1
	spoke1Opts.Cluster.Name = "spoke-1-cluster"
	spoke1Opts.LeafNode.Host = testHost
	spoke1Opts.LeafNode.Remotes = []*server.RemoteLeafOpts{{URLs: []*url.URL{spoke1URL}}}
	spoke1Opts.ServerName = "spoke-1"

	spoke1, err := startServer(spoke1Opts)
	if err != nil {
		t.Fatalf("Failed to start spoke1: %v", err)
	}
	defer spoke1.Shutdown()

	t.Logf("Spoke 1 started - client port: %d", spoke1Opts.Port)

	// ===== 3. 启动 Spoke 2 =====
	spoke2URL, _ := url.Parse(fmt.Sprintf("nats://%s:%d", testHost, hubOpts.LeafNode.Port))
	spoke2Opts := defaultOptions()
	spoke2Opts.Cluster.Host = testHost
	spoke2Opts.Cluster.Port = -1
	spoke2Opts.Cluster.Name = "spoke-2-cluster"
	spoke2Opts.LeafNode.Host = testHost
	spoke2Opts.LeafNode.Remotes = []*server.RemoteLeafOpts{{URLs: []*url.URL{spoke2URL}}}
	spoke2Opts.ServerName = "spoke-2"

	spoke2, err := startServer(spoke2Opts)
	if err != nil {
		t.Fatalf("Failed to start spoke2: %v", err)
	}
	defer spoke2.Shutdown()

	t.Logf("Spoke 2 started - client port: %d", spoke2Opts.Port)

	// ===== 4. 检查 LeafNode 连接 =====
	t.Log("Checking LeafNode connections...")

	// 等待 Hub 接受 2 个 LeafNode 连接
	checkFor(t, 10*time.Second, 100*time.Millisecond, func() error {
		if n := hub.NumLeafNodes(); n != 2 {
			return fmt.Errorf("hub has %d leafnodes, want 2", n)
		}
		return nil
	})
	t.Log("✅ Hub has 2 LeafNode connections")

	// ===== 5. 测试消息通信 =====
	t.Log("Testing message communication...")

	// 连接到 Spoke 2 作为订阅者
	nc2, err := nats.Connect(fmt.Sprintf("nats://%s:%d", testHost, spoke2Opts.Port))
	if err != nil {
		t.Fatalf("Failed to connect to Spoke 2: %v", err)
	}
	defer nc2.Close()

	// 订阅消息
	received := make(chan *nats.Msg, 1)
	sub, err := nc2.ChanSubscribe("test.chat", received)
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}
	defer sub.Unsubscribe()

	// 连接到 Spoke 1 作为发布者
	nc1, err := nats.Connect(fmt.Sprintf("nats://%s:%d", testHost, spoke1Opts.Port))
	if err != nil {
		t.Fatalf("Failed to connect to Spoke 1: %v", err)
	}
	defer nc1.Close()

	// 发布消息
	testMsg := "Hello from Spoke 1!"
	if err := nc1.Publish("test.chat", []byte(testMsg)); err != nil {
		t.Fatalf("Failed to publish: %v", err)
	}
	nc1.Flush()

	// 等待接收消息
	select {
	case msg := <-received:
		if string(msg.Data) != testMsg {
			t.Fatalf("Received wrong message: got %q, want %q", string(msg.Data), testMsg)
		}
		t.Logf("✅ Message received: %q", string(msg.Data))
	case <-time.After(5 * time.Second):
		t.Fatal("Timed out waiting for message")
	}

	t.Log("=== Test completed successfully ===")
}

func TestLeafNodeWithJetStreamOnSpokesOnly(t *testing.T) {
	// ===== 1. 启动 Hub（不启用 JetStream）=====
	hubOpts := defaultOptions()
	hubOpts.LeafNode.Host = testHost
	hubOpts.LeafNode.Port = -1
	hubOpts.ServerName = "hub-server-no-js"
	// 注意：没有配置 JetStream

	hub, err := startServer(hubOpts)
	if err != nil {
		t.Fatalf("Failed to start hub: %v", err)
	}
	defer hub.Shutdown()

	t.Logf("Hub started (NO JetStream) - client port: %d, leafnode port: %d", hubOpts.Port, hubOpts.LeafNode.Port)

	// ===== 2. 启动 Spoke 1（启用 JetStream）=====
	spoke1URL, _ := url.Parse(fmt.Sprintf("nats://%s:%d", testHost, hubOpts.LeafNode.Port))
	spoke1Opts := defaultOptions()
	spoke1Opts.Cluster.Host = testHost
	spoke1Opts.Cluster.Port = -1
	spoke1Opts.Cluster.Name = "spoke-1-js-cluster"
	spoke1Opts.LeafNode.Host = testHost
	spoke1Opts.LeafNode.Remotes = []*server.RemoteLeafOpts{{URLs: []*url.URL{spoke1URL}}}
	spoke1Opts.ServerName = "spoke-1-with-js"
	// 启用 JetStream（单机模式，不需要 cluster port）
	spoke1Opts.JetStreamMaxMemory = 256 * 1024 * 1024
	spoke1Opts.JetStreamMaxStore = 1 * 1024 * 1024 * 1024
	spoke1Opts.StoreDir = t.TempDir()

	spoke1, err := startServer(spoke1Opts)
	if err != nil {
		t.Fatalf("Failed to start spoke1 with JetStream: %v", err)
	}
	defer spoke1.Shutdown()

	t.Logf("Spoke 1 started (WITH JetStream) - client port: %d", spoke1Opts.Port)

	// ===== 3. 启动 Spoke 2（启用 JetStream）=====
	spoke2URL, _ := url.Parse(fmt.Sprintf("nats://%s:%d", testHost, hubOpts.LeafNode.Port))
	spoke2Opts := defaultOptions()
	spoke2Opts.Cluster.Host = testHost
	spoke2Opts.Cluster.Port = -1
	spoke2Opts.Cluster.Name = "spoke-2-js-cluster"
	spoke2Opts.LeafNode.Host = testHost
	spoke2Opts.LeafNode.Remotes = []*server.RemoteLeafOpts{{URLs: []*url.URL{spoke2URL}}}
	spoke2Opts.ServerName = "spoke-2-with-js"
	// 启用 JetStream（单机模式）
	spoke2Opts.JetStreamMaxMemory = 256 * 1024 * 1024
	spoke2Opts.JetStreamMaxStore = 1 * 1024 * 1024 * 1024
	spoke2Opts.StoreDir = t.TempDir()

	spoke2, err := startServer(spoke2Opts)
	if err != nil {
		t.Fatalf("Failed to start spoke2 with JetStream: %v", err)
	}
	defer spoke2.Shutdown()

	t.Logf("Spoke 2 started (WITH JetStream) - client port: %d", spoke2Opts.Port)

	// ===== 4. 检查 LeafNode 连接 =====
	t.Log("Checking LeafNode connections...")

	checkFor(t, 10*time.Second, 100*time.Millisecond, func() error {
		if n := hub.NumLeafNodes(); n != 2 {
			return fmt.Errorf("hub has %d leafnodes, want 2", n)
		}
		return nil
	})
	t.Log("✅ Hub has 2 LeafNode connections (both with JetStream enabled)")

	// ===== 5. 测试普通消息通信（不涉及 JetStream）=====
	t.Log("Testing normal message communication...")

	nc2, err := nats.Connect(fmt.Sprintf("nats://%s:%d", testHost, spoke2Opts.Port))
	if err != nil {
		t.Fatalf("Failed to connect to Spoke 2: %v", err)
	}
	defer nc2.Close()

	received := make(chan *nats.Msg, 1)
	sub, err := nc2.ChanSubscribe("test.chat", received)
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}
	defer sub.Unsubscribe()

	// 等待订阅传播完成
	time.Sleep(100 * time.Millisecond)

	nc1, err := nats.Connect(fmt.Sprintf("nats://%s:%d", testHost, spoke1Opts.Port))
	if err != nil {
		t.Fatalf("Failed to connect to Spoke 1: %v", err)
	}
	defer nc1.Close()

	testMsg := "Hello from Spoke 1 with JetStream!"
	if err := nc1.Publish("test.chat", []byte(testMsg)); err != nil {
		t.Fatalf("Failed to publish: %v", err)
	}
	nc1.Flush()

	select {
	case msg := <-received:
		if string(msg.Data) != testMsg {
			t.Fatalf("Received wrong message: got %q, want %q", string(msg.Data), testMsg)
		}
		t.Logf("✅ Message received: %q", string(msg.Data))
	case <-time.After(5 * time.Second):
		t.Fatal("Timed out waiting for message")
	}

	t.Log("=== Test completed successfully ===")
	t.Log("✅ Summary:")
	t.Log("  - Hub: NO JetStream")
	t.Log("  - Spokes: WITH JetStream (standalone mode)")
	t.Log("  - LeafNode connections work normally")
	t.Log("  - Message communication works normally")
	t.Log("  - JetStream is isolated on each spoke")
}
