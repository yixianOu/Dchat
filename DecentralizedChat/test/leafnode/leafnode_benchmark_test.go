package e2e_test

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"testing"
	"time"

	"DecentralizedChat/internal/chat"
	"DecentralizedChat/internal/nats"

	"github.com/nats-io/nats-server/v2/server"
	gnats "github.com/nats-io/nats.go"
	"golang.org/x/crypto/curve25519"
)

func generateX25519(tb testing.TB) (privB64, pubB64 string) {
	tb.Helper()
	priv := make([]byte, 32)
	if _, err := rand.Read(priv); err != nil {
		tb.Fatalf("generate key: %v", err)
	}
	key, err := curve25519.X25519(priv, curve25519.Basepoint)
	if err != nil {
		tb.Fatalf("derive pub: %v", err)
	}
	return base64.StdEncoding.EncodeToString(priv), base64.StdEncoding.EncodeToString(key)
}

// BenchmarkLeafNode_ClientReceive 测量裸消息收发吞吐（无加密）
func BenchmarkLeafNode_ClientReceive(b *testing.B) {
	srv := startBenchServer(b)
	defer srv.Shutdown()

	newClient := func(name string) *nats.Service {
		client, err := nats.NewService(nats.ClientConfig{
			URL:             srv.ClientURL(),
			Name:            name,
			InProcessServer: srv,
		})
		if err != nil {
			b.Fatalf("%s client: %v", name, err)
		}
		return client
	}

	pubNATS := newClient("bench-pub")
	defer pubNATS.Close()

	subNATS := newClient("bench-sub")
	defer subNATS.Close()

	const subject = "bench.leafnode.msg"
	received := make(chan struct{}, 1024)

	sub, err := subNATS.Conn().Subscribe(subject, func(_ *gnats.Msg) {
		received <- struct{}{}
	})
	if err != nil {
		b.Fatalf("subscribe: %v", err)
	}
	defer sub.Unsubscribe()

	time.Sleep(100 * time.Millisecond)

	payload := []byte("bench message payload")

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if err := pubNATS.Publish(subject, payload); err != nil {
			b.Fatalf("publish: %v", err)
		}
		<-received
	}
}

// BenchmarkLeafNode_ClientReceiveEncrypted 测量 NaCl Box 加密+收发的完整吞吐
func BenchmarkLeafNode_ClientReceiveEncrypted(b *testing.B) {
	srv := startBenchServer(b)
	defer srv.Shutdown()

	newClient := func(name string) *nats.Service {
		client, err := nats.NewService(nats.ClientConfig{
			URL:             srv.ClientURL(),
			Name:            name,
			InProcessServer: srv,
		})
		if err != nil {
			b.Fatalf("%s client: %v", name, err)
		}
		return client
	}

	pubNATS := newClient("bench-pub")
	defer pubNATS.Close()

	subNATS := newClient("bench-sub")
	defer subNATS.Close()

	senderPriv, senderPub := generateX25519(b)
	recipientPriv, recipientPub := generateX25519(b)

	const subject = "bench.leafnode.encrypted"
	received := make(chan []byte, 1024)

	sub, err := subNATS.Conn().Subscribe(subject, func(msg *gnats.Msg) {
		received <- msg.Data
	})
	if err != nil {
		b.Fatalf("subscribe: %v", err)
	}
	defer sub.Unsubscribe()

	time.Sleep(100 * time.Millisecond)

	plain := []byte("bench message payload")

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		nonceB64, cipherB64, err := chat.EncryptDirect(senderPriv, recipientPub, plain)
		if err != nil {
			b.Fatalf("encrypt: %v", err)
		}
		wire := append(append([]byte(nonceB64), ':'), cipherB64...)

		if err := pubNATS.Publish(subject, wire); err != nil {
			b.Fatalf("publish: %v", err)
		}

		data := <-received
		idx := bytes.IndexByte(data, ':')
		decrypted, err := chat.DecryptDirect(recipientPriv, senderPub, string(data[:idx]), string(data[idx+1:]))
		if err != nil {
			b.Fatalf("decrypt: %v", err)
		}
		_ = decrypted
	}
}

func startBenchServer(tb testing.TB) *server.Server {
	tb.Helper()
	opts := &server.Options{
		Host:      testHost,
		Port:      -1,
		HTTPPort:  -1,
		JetStream: false,
		NoLog:     true,
		NoSigs:    true,
	}
	s, err := server.NewServer(opts)
	if err != nil {
		tb.Fatalf("start server: %v", err)
	}
	go s.Start()
	if !s.ReadyForConnections(5 * time.Second) {
		tb.Fatal("server not ready")
	}
	return s
}
