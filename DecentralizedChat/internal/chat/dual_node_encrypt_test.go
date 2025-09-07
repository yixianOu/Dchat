package chat

// 集成测试：启动两个本地嵌入 NATS 节点，通过 Routes 建立连接，
// 使用最小 chat.Service 建立私聊加密通道，验证 A->B 与 B->A 往返加密消息解密成功。
// 说明：该测试在单机上模拟"跨机"双节点，通过不同端口 + Routes 连接，
// 若需真实跨机，可将 host 改为实际局域网 IP，并在第二节点 seedRoutes 指向第一节点 cluster 端口。

import (
	crand "crypto/rand"
	"encoding/base64"
	"fmt"
	"net"
	"net/url"
	"testing"
	"time"

	natsservice "DecentralizedChat/internal/nats"

	"github.com/nats-io/nats-server/v2/server"
	"golang.org/x/crypto/nacl/box"
)

// getFreePort 获取一个可用端口（监听后立即关闭）
func getFreePort() (int, error) { // 保留单端口探测工具
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// allocatePortSet 选择一个基础端口，返回四个连续端口（确保未被占用）
func allocatePortSet() (c1p, cl1p, c2p, cl2p int, err error) {
	// 使用固定端口集，若被占用则跳过测试（提高确定性）
	candidates := [][4]int{{4222, 6222, 4322, 6322}, {5022, 6022, 5122, 6122}}
	for _, set := range candidates {
		ok := true
		ls := []net.Listener{}
		for _, p := range set {
			l, e := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p))
			if e != nil {
				ok = false
				break
			}
			ls = append(ls, l)
		}
		for _, l := range ls {
			l.Close()
		}
		if ok {
			return set[0], set[1], set[2], set[3], nil
		}
	}
	return 0, 0, 0, 0, fmt.Errorf("no free fixed port set")
}

// genKeyPair 生成 NaCl box 密钥对（32 字节 raw）并 base64 编码
func genKeyPair(t *testing.T) (privB64, pubB64 string) {
	pub, priv, err := box.GenerateKey(crand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return base64.StdEncoding.EncodeToString(priv[:]), base64.StdEncoding.EncodeToString(pub[:])
}

func TestDualNodeDirectRoundTrip(t *testing.T) {
	c1p, cl1p, c2p, cl2p, err := allocatePortSet()
	if err != nil {
		t.Fatalf("port alloc: %v", err)
	}

	start := func(name string, clientPort, clusterPort int, seed []string) (*server.Server, func(), error) {
		opts := &server.Options{
			ServerName: name,
			Host:       "127.0.0.1",
			Port:       clientPort,
		}
		opts.Cluster.Name = "dchat-test"
		opts.Cluster.Host = "127.0.0.1"
		opts.Cluster.Port = clusterPort
		// 禁用 JetStream，聚焦加密消息测试
		opts.JetStream = false
		if len(seed) > 0 {
			for _, r := range seed {
				u, e := url.Parse(r)
				if e != nil {
					return nil, nil, e
				}
				opts.Routes = append(opts.Routes, u)
			}
		}
		srv, e := server.NewServer(opts)
		if e != nil {
			return nil, nil, e
		}
		go srv.Start()
		if !srv.ReadyForConnections(5 * time.Second) {
			return nil, nil, fmt.Errorf("%s not ready", name)
		}
		stop := func() { srv.Shutdown() }
		return srv, stop, nil
	}

	seedRoute := fmt.Sprintf("nats://127.0.0.1:%d", cl1p)
	srvA, stopA, err := start("A", c1p, cl1p, []string{})
	if err != nil {
		t.Fatalf("start A: %v", err)
	}
	defer stopA()
	srvB, stopB, err := start("B", c2p, cl2p, []string{seedRoute})
	if err != nil {
		t.Fatalf("start B: %v", err)
	}
	defer stopB()

	// 等待路由形成
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if srvA.NumRoutes() >= 1 && srvB.NumRoutes() >= 1 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if srvA.NumRoutes() == 0 || srvB.NumRoutes() == 0 {
		t.Fatalf("routes not connected: A=%d B=%d", srvA.NumRoutes(), srvB.NumRoutes())
	}

	nA, err := natsservice.NewService(natsservice.ClientConfig{URL: fmt.Sprintf("nats://127.0.0.1:%d", c1p), Name: "clientA"})
	if err != nil {
		t.Fatalf("nats A: %v", err)
	}
	defer nA.Close()
	nB, err := natsservice.NewService(natsservice.ClientConfig{URL: fmt.Sprintf("nats://127.0.0.1:%d", c2p), Name: "clientB"})
	if err != nil {
		t.Fatalf("nats B: %v", err)
	}
	defer nB.Close()

	// chat 服务
	svcA := NewService(nA)
	svcB := NewService(nB)
	defer svcA.Close()
	defer svcB.Close()

	// 生成并设置密钥对
	privA, pubA := genKeyPair(t)
	privB, pubB := genKeyPair(t)
	svcA.SetKeyPair(privA, pubA)
	svcB.SetKeyPair(privB, pubB)

	// 获取各自用户 ID
	userA := svcA.GetUser().ID
	userB := svcB.GetUser().ID

	// 互相缓存公钥
	svcA.AddFriendKey(userB, pubB)
	svcB.AddFriendKey(userA, pubA)

	// 注册回调（使用缓冲 channel 防止阻塞）
	chA := make(chan *DecryptedMessage, 2)
	chB := make(chan *DecryptedMessage, 2)
	svcA.OnDecrypted(func(m *DecryptedMessage) { chA <- m })
	svcB.OnDecrypted(func(m *DecryptedMessage) { chB <- m })

	// 加入会话（订阅）
	if err := svcA.JoinDirect(userB); err != nil {
		t.Fatalf("A join: %v", err)
	}
	if err := svcB.JoinDirect(userA); err != nil {
		t.Fatalf("B join: %v", err)
	}

	// 等待订阅传播到路由（避免立即发送造成首条消息丢失）
	time.Sleep(200 * time.Millisecond)

	// 发送往返消息
	if err := svcA.SendDirect(userB, "hello B"); err != nil {
		t.Fatalf("A->B send: %v", err)
	}
	if err := svcB.SendDirect(userA, "hello A"); err != nil {
		t.Fatalf("B->A send: %v", err)
	}

	// 等待消息
	wait := func(ch <-chan *DecryptedMessage, wantSender, wantPlain string) {
		select {
		case m := <-ch:
			if m.Sender != wantSender || m.Plain != wantPlain {
				t.Fatalf("unexpected message: sender=%s plain=%s", m.Sender, m.Plain)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("timeout waiting message %s %s", wantSender, wantPlain)
		}
	}
	wait(chB, userA, "hello B") // B 收到来自 A
	wait(chA, userB, "hello A") // A 收到来自 B
}
