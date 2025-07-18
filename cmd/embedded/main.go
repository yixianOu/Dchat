package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

func main() {
	var (
		host           string
		port           int
		certFile       string
		keyFile        string
		clientCertFile string
		clientKeyFile  string
		caFile         string
	)

	dir, err := os.Getwd()
	if err != nil {
		fmt.Println("获取当前目录失败:", err)
		return
	}
	fmt.Printf("Current working directory: %s\n", dir)

	flag.StringVar(&host, "host", "localhost", "Client connection host/IP.")
	flag.IntVar(&port, "port", 4222, "Client connection port.")
	flag.StringVar(&certFile, "tls.cert.server", "cert.pem", "TLS cert file.")
	flag.StringVar(&keyFile, "tls.key.server", "key.pem", "TLS key file.")
	flag.StringVar(&caFile, "tls.ca", "ca.pem", "TLS CA file.")
	flag.StringVar(&clientCertFile, "tls.cert.client", "client-cert.pem", "TLS cert file.")
	flag.StringVar(&clientKeyFile, "tls.key.client", "client-key.pem", "TLS key file.")

	flag.Parse()

	serverTlsConfig, err := server.GenTLSConfig(&server.TLSConfigOpts{
		CertFile: certFile,
		KeyFile:  keyFile,
		CaFile:   caFile,
		Verify:   true,
		Timeout:  2,
	})
	if err != nil {
		log.Fatalf("tls config: %v", err)
	}

	opts := server.Options{
		Host:      host,
		Port:      port,
		TLSConfig: serverTlsConfig,
	}

	ns, err := server.NewServer(&opts)
	if err != nil {
		log.Fatalf("server init: %v", err)
	}

	go ns.Start()
	defer ns.Shutdown()

	time.Sleep(time.Second)

	nc, err := nats.Connect(
		fmt.Sprintf("tls://%s:%d", host, port),
		nats.RootCAs(caFile),
		nats.ClientCert(clientCertFile, clientKeyFile),
	)
	if err != nil {
		log.Fatalf("client connect: %v", err)
	}
	defer nc.Drain()

	sub, _ := nc.SubscribeSync("hello")
	defer sub.Drain()

	nc.Publish("hello", []byte("world"))

	msg, _ := sub.NextMsg(time.Second)
	fmt.Println(string(msg.Data))
}
