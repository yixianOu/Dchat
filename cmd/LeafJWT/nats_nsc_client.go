package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/nats-io/nats.go"
)

// jwtPath := "~/.local/share/nats/nsc/stores/local/accounts/APP/users/user.jwt"
// credsFile := "~/.local/share/nats/nsc/keys/creds/local/APP/user.creds" #(nsc)
// natsURL := "nats://0.0.0.0:4222"
func test() {
	// 从环境变量获取服务器 URL
	natsURL := os.Getenv("NATS_MAIN_URL")
	if natsURL == "" {
		log.Fatal("NATS_MAIN_URL 环境变量未设置")
	}
	credsFile := os.Getenv("NATS_CREDS")

	// 连接选项
	opts := []nats.Option{
		nats.Name("NSC Auth Demo"),
		nats.ReconnectWait(time.Second),
		nats.MaxReconnects(-1),

		nats.UserCredentials(credsFile),
	}

	// 连接到 NATS 服务器
	nc, err := nats.Connect(natsURL, opts...)
	if err != nil {
		log.Fatalf("连接失败: %v", err)
	}
	defer nc.Close()

	fmt.Println("成功连接到 NATS 服务器!")

	// 发布一条消息
	if err := nc.Publish("greetings", []byte("你好，NATS!")); err != nil {
		log.Fatalf("发布消息失败: %v", err)
	}

	// 订阅一个主题
	sub, err := nc.SubscribeSync("greetings")
	if err != nil {
		log.Fatalf("订阅失败: %v", err)
	}

	// 等待接收消息
	msg, err := sub.NextMsg(time.Second)
	if err != nil {
		log.Fatalf("接收消息失败: %v", err)
	}
	fmt.Printf("收到消息: %s\n", string(msg.Data))
}
