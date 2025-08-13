package main

import (
	"crypto/rand"
	"encoding/base64"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"DecentralizedChat/internal/chat"
	natsservice "DecentralizedChat/internal/nats"

	"github.com/nats-io/nats-server/v2/server"
	"golang.org/x/crypto/nacl/box"
)

func genKeyPair() (privB64, pubB64 string) {
	pub, priv, err := box.GenerateKey(rand.Reader)
	if err != nil {
		log.Fatalf("generate key: %v", err)
	}
	return base64.StdEncoding.EncodeToString(priv[:]), base64.StdEncoding.EncodeToString(pub[:])
}

func startServer(clientPort, clusterPort int, advertise string, seedRoutes []string) (*server.Server, error) {
	opts := &server.Options{Host: "0.0.0.0", Port: clientPort}
	opts.ServerName = fmt.Sprintf("dchat-%d", clientPort)
	opts.Cluster.Name = "dchat-peer"
	opts.Cluster.Host = "0.0.0.0"
	opts.Cluster.Port = clusterPort
	if strings.TrimSpace(advertise) != "" {
		// 公共节点在无 Tailscale 场景下通过对外可达地址向其他节点公告
		// 形如 "1.2.3.4:6222" 或 "example.com:6222"
		// 兼容用户误传如 "nats://1.2.3.4:6222"，去除 scheme。
		adv := strings.TrimSpace(advertise)
		adv = strings.TrimPrefix(adv, "nats://")
		adv = strings.TrimPrefix(adv, "tls://")
		opts.Cluster.Advertise = adv
	}
	opts.JetStream = false
	for _, r := range seedRoutes {
		if strings.TrimSpace(r) == "" {
			continue
		}
		u, err := url.Parse(r)
		if err != nil {
			return nil, err
		}
		opts.Routes = append(opts.Routes, u)
	}
	srv, err := server.NewServer(opts)
	if err != nil {
		return nil, err
	}
	go srv.Start()
	if !srv.ReadyForConnections(5 * time.Second) {
		return nil, fmt.Errorf("nats server not ready on %d/%d", clientPort, clusterPort)
	}
	return srv, nil
}

// identity 文件格式（简单 KV）：
// ID=<userID>
// PRIV=<base64>
// PUB=<base64>
func loadIdentity(path string) (id, priv, pub string, err error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", "", "", err
	}
	lines := strings.Split(string(b), "\n")
	for _, ln := range lines {
		if strings.HasPrefix(ln, "ID=") {
			id = strings.TrimPrefix(ln, "ID=")
		}
		if strings.HasPrefix(ln, "PRIV=") {
			priv = strings.TrimPrefix(ln, "PRIV=")
		}
		if strings.HasPrefix(ln, "PUB=") {
			pub = strings.TrimPrefix(ln, "PUB=")
		}
	}
	return
}

func saveIdentity(path, id, priv, pub string) error {
	data := fmt.Sprintf("ID=%s\nPRIV=%s\nPUB=%s\n", id, priv, pub)
	return os.WriteFile(path, []byte(data), 0o600)
}

func main() {
	clientPort := flag.Int("client-port", 4222, "本地 NATS 客户端端口")
	clusterPort := flag.Int("cluster-port", 6222, "本地 NATS 集群端口")
	seed := flag.String("seed-route", "", "可选：种子路由 nats://host:clusterPort")
	clusterAdv := flag.String("cluster-advertise", "", "公共节点对外公告地址 host:port（例如 1.2.3.4:6222）")
	privIn := flag.String("priv", "", "本地私钥 base64 (32 bytes)")
	pubIn := flag.String("pub", "", "本地公钥 base64 (32 bytes)")
	idIn := flag.String("id", "", "覆盖/指定固定用户ID (可与 --identity 配合)")
	peerID := flag.String("peer-id", "", "对端用户 ID")
	peerPub := flag.String("peer-pub", "", "对端公钥 base64")
	nickname := flag.String("nick", "Peer", "昵称")
	sendMsg := flag.String("send", "hello from peer", "发送消息内容")
	waitOnly := flag.Bool("wait", false, "仅等待消息（不主动发送）")
	identityPath := flag.String("identity", "dchat_identity.txt", "持久身份文件 (保存/加载 ID+密钥)")
	noSave := flag.Bool("no-save", false, "不回写 identity 文件")
	flag.Parse()

	seedRoutes := []string{}
	if *seed != "" {
		// 允许逗号/空格分隔的多路由输入
		raw := strings.ReplaceAll(*seed, ";", ",")
		parts := strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ' ' || r == '\n' || r == '\t' })
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			seedRoutes = append(seedRoutes, p)
		}
	}

	srv, err := startServer(*clientPort, *clusterPort, *clusterAdv, seedRoutes)
	if err != nil {
		log.Fatalf("start server: %v", err)
	}
	defer srv.Shutdown()

	// 加载/生成身份与密钥
	priv := *privIn
	pub := *pubIn
	userID := *idIn
	if _, err := os.Stat(*identityPath); err == nil && (priv == "" || pub == "" || userID == "") {
		idFile, pFile, pubFile, err := loadIdentity(*identityPath)
		if err == nil {
			if userID == "" {
				userID = idFile
			}
			if priv == "" {
				priv = pFile
			}
			if pub == "" {
				pub = pubFile
			}
		}
	}
	if priv == "" || pub == "" {
		priv, pub = genKeyPair()
	}

	natsURL := fmt.Sprintf("nats://127.0.0.1:%d", *clientPort)
	nc, err := natsservice.NewService(natsservice.ClientConfig{URL: natsURL, Name: "chatpeer"})
	if err != nil {
		log.Fatalf("nats connect: %v", err)
	}
	defer nc.Close()

	svc := chat.NewService(nc)
	svc.SetUser(*nickname)
	if userID != "" {
		svc.SetUserID(userID)
	}
	svc.SetKeyPair(priv, pub)
	self := svc.GetUser()

	fmt.Println("====================")
	fmt.Println("本地节点启动成功")
	fmt.Println("UserID:", self.ID)
	fmt.Println("PubKey:", pub)
	fmt.Println("ClientURL:", natsURL)
	fmt.Println("ClusterPort:", *clusterPort)
	if strings.TrimSpace(*clusterAdv) != "" {
		adv := strings.TrimPrefix(strings.TrimSpace(*clusterAdv), "nats://")
		adv = strings.TrimPrefix(adv, "tls://")
		fmt.Println("ClusterAdvertise:", adv)
	}
	fmt.Println("SeedRoute(提供给其它节点):", fmt.Sprintf("nats://%s:%d", "<your_ip>", *clusterPort))
	fmt.Println("====================")

	if !*noSave {
		// 保存身份（若路径在目录中不存在则创建目录）
		if dir := filepath.Dir(*identityPath); dir != "." && dir != "" {
			_ = os.MkdirAll(dir, 0o755)
		}
		_ = saveIdentity(*identityPath, self.ID, priv, pub)
	}

	if *peerID == "" || *peerPub == "" {
		fmt.Println("未提供 peer-id / peer-pub，当前等待对端。将本端 userID/pubKey 发给对方后重启携带 --peer-id --peer-pub 即可。")
		fmt.Println("若已提前交换 identity 文件，可直接补充参数再运行。")
		// 继续运行接收模式
		svc.OnDecrypted(func(m *chat.DecryptedMessage) {
			fmt.Printf("[RECV] from=%s cid=%s plain=%q time=%s\n", m.Sender, m.CID, m.Plain, m.Ts.Format(time.RFC3339))
		})
		svc.OnError(func(e error) { fmt.Println("[ERR]", e) })
		fmt.Println("监听中（未配置对端，当前不会发送）。按 Ctrl+C 退出 ...")
		select {}
	}

	svc.AddFriendKey(*peerID, *peerPub)
	if err := svc.JoinDirect(*peerID); err != nil {
		log.Fatalf("join direct: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	svc.OnDecrypted(func(m *chat.DecryptedMessage) {
		fmt.Printf("[RECV] from=%s cid=%s plain=%q time=%s\n", m.Sender, m.CID, m.Plain, m.Ts.Format(time.RFC3339))
	})
	svc.OnError(func(e error) { fmt.Println("[ERR]", e) })

	if !*waitOnly {
		if err := svc.SendDirect(*peerID, *sendMsg); err != nil {
			log.Fatalf("send: %v", err)
		}
		fmt.Println("已发送消息:", *sendMsg)
	} else {
		fmt.Println("wait 模式：不主动发送，仅接收对端消息")
	}

	fmt.Println("进入监听，按 Ctrl+C 退出 ...")
	select {}
}
