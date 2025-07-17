package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"time"

	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
)

func main() {
	// 命令行参数
	serverURL := flag.String("server", "nats://localhost:4222", "NATS Server URL")
	operatorSeedFile := flag.String("operator-seed", "", "Operator seed file")
	accountName := flag.String("account-name", "APP", "Account name")
	sysCredFile := flag.String("sys-creds", "", "System account credentials file")
	outputJWT := flag.String("output", "", "Output JWT file path (optional)")
	flag.Parse()

	// 1. 读取操作员种子密钥
	seedBytes, err := ioutil.ReadFile(*operatorSeedFile)
	if err != nil {
		log.Fatalf("读取操作员种子密钥失败: %v", err)
	}
	operatorKP, err := nkeys.FromSeed(seedBytes)
	if err != nil {
		log.Fatalf("解析操作员种子密钥失败: %v", err)
	}
	defer operatorKP.Wipe() // 安全擦除内存中的私钥

	// 2. 创建账户密钥对
	accountKP, err := nkeys.CreateAccount()
	if err != nil {
		log.Fatalf("创建账户密钥对失败: %v", err)
	}
	defer accountKP.Wipe()

	accountPub, err := accountKP.PublicKey()
	if err != nil {
		log.Fatalf("获取账户公钥失败: %v", err)
	}

	// 3. 创建账户JWT声明
	ac := jwt.NewAccountClaims(accountPub)
	ac.Name = *accountName
	ac.Expires = time.Now().Add(365 * 24 * time.Hour).Unix() // 一年有效期

	// 设置账户限制和权限
	ac.Limits.Conn = 1000         // 最大连接数
	ac.Limits.Imports = 1000      // 最大导入数
	ac.Limits.Exports = 1000      // 最大导出数
	ac.Limits.Data = 10_000_000   // 每月数据量限制 (10MB)
	ac.Limits.Payload = 1_000_000 // 最大负载大小 (1MB)
	ac.Limits.Subs = 1000         // 最大订阅数

	// 4. 使用操作员密钥签名账户JWT
	accountJWT, err := ac.Encode(operatorKP)
	if err != nil {
		log.Fatalf("签名账户JWT失败: %v", err)
	}

	// 可选：保存JWT到文件
	if *outputJWT != "" {
		err = ioutil.WriteFile(*outputJWT, []byte(accountJWT), 0644)
		if err != nil {
			log.Fatalf("保存JWT到文件失败: %v", err)
		}
		fmt.Printf("账户JWT已保存到: %s\n", *outputJWT)
	}

	// 5. 连接NATS服务器并推送JWT
	opts := []nats.Option{
		nats.Name("Account JWT Publisher"),
		nats.ReconnectWait(time.Second),
		nats.MaxReconnects(5),
	}

	// 使用系统账户凭证连接
	if *sysCredFile != "" {
		opts = append(opts, nats.UserCredentials(*sysCredFile))
	}

	// 连接到NATS服务器
	nc, err := nats.Connect(*serverURL, opts...)
	if err != nil {
		log.Fatalf("连接NATS服务器失败: %v", err)
	}
	defer nc.Close()

	// 6. 构建用于发布JWT的系统主题
	subject := fmt.Sprintf("$SYS.REQ.ACCOUNT.%s.CLAIMS.UPDATE", accountPub)

	// 7. 发布账户JWT
	response, err := nc.Request(subject, []byte(accountJWT), 5*time.Second)
	if err != nil {
		log.Fatalf("发布账户JWT失败: %v", err)
	}

	// 检查响应
	if string(response.Data) != "+OK" {
		log.Fatalf("账户JWT推送失败，服务器响应: %s", string(response.Data))
	}

	fmt.Printf("账户JWT成功推送到服务器！\n")
	fmt.Printf("账户公钥: %s\n", accountPub)
}
