package main

import (
	"fmt"
	"net/url"
	"os/exec"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

type NATSClient struct {
	url  string
	opts []nats.Option
}

func main() {
	const (
		address          = "localhost"
		mainPort         = 4222
		clusterPort      = 7422
		leafPort         = 4223
		subjectName      = "foo"
		credsFileAppUser = "~/.local/share/nats/nsc/keys/creds/local/APP/user.creds"
		credsFileSysUser = "/home/orician/.local/share/nats/nsc/keys/creds/local/SYS/sys.creds"
		resolverFile     = "/home/orician/workspace/learn/nats/Dchat/cmd/LeafJWT/resolver.conf"
	)

	mainClientOpt := NATSClient{
		url: fmt.Sprintf("nats://%s:%d", address, mainPort),
		opts: []nats.Option{
			nats.Name("Main Client"),
			nats.ReconnectWait(time.Second),
			nats.MaxReconnects(-1),
			nats.UserCredentials(credsFileAppUser)},
	}

	sysClientOpt := NATSClient{
		url: fmt.Sprintf("nats://%s:%d", address, mainPort),
		opts: []nats.Option{
			nats.Name("System Client"),
			nats.ReconnectWait(time.Second),
			nats.MaxReconnects(-1),
			nats.UserCredentials(credsFileSysUser),
		},
	}

	leafClientOpt := NATSClient{
		url: fmt.Sprintf("nats://%s:%d", address, leafPort),
		opts: []nats.Option{
			nats.Name("Leaf Client"),
			nats.ReconnectWait(time.Second),
			nats.MaxReconnects(-1),
		},
	}

	mainServerOpt := server.Options{
		ConfigFile: resolverFile,
		Host:       address,
		Port:       mainPort,
		LeafNode: server.LeafNodeOpts{
			Port:      clusterPort,
			Advertise: fmt.Sprintf("%s:%d", address, clusterPort),
		},
	}

	leafServerOpt := server.Options{
		Host: address,
		Port: leafPort,
		LeafNode: server.LeafNodeOpts{
			Remotes: []*server.RemoteLeafOpts{
				{
					URLs:        []*url.URL{{Scheme: "nats", Host: fmt.Sprintf("%s:%d", address, clusterPort)}},
					Credentials: credsFileAppUser,
				},
			},
		},
	}

	// 依次执行3条nsc命令，并打印输出
	nscCmds := [][]string{
		{"edit", "operator", "--require-signing-keys", "--account-jwt-server-url", fmt.Sprintf("nats://%s:%d", address, mainPort)},
		{"edit", "account", "APP", "--sk", "generate"},
		{"generate", "config", "--nats-resolver", "--sys-account", "SYS"},
	}
	for i, args := range nscCmds {
		fmt.Printf("执行 nsc 命令 %d: nsc %s\n", i+1, args)
		if i == 2 { // 第3条命令重定向输出到resolverFile
			execCmd := exec.Command("nsc", args...)
			err := exec.Command("touch", resolverFile).Run() // 确保文件存在
			if err != nil {
				panic(fmt.Sprintf("无法创建resolverFile: %v", err))
			}
			outFile, err := exec.Command("tee", resolverFile).StdinPipe()
			if err != nil {
				panic(fmt.Sprintf("无法打开resolverFile: %v", err))
			}
			execCmd.Stdout = outFile
			execCmd.Stderr = outFile
			err = execCmd.Run()
			outFile.Close()
			if err != nil {
				panic(fmt.Sprintf("nsc命令失败: nsc %v\n错误: %v", args, err))
			}
			fmt.Printf("nsc命令输出已写入: %s\n", resolverFile)
		} else {
			execCmd := exec.Command("nsc", args...)
			output, err := execCmd.CombinedOutput()
			if err != nil {
				panic(fmt.Sprintf("nsc命令失败: nsc %v\n错误: %v\n输出: %s", args, err, string(output)))
			}
			fmt.Printf("nsc命令输出:\n%s\n", string(output))
		}
	}

	// 创建并启动main服务器
	mainServer, err := server.NewServer(&mainServerOpt)
	if err != nil {
		panic(fmt.Sprintf("创建主服务器失败: %v", err))
	}
	go mainServer.Start()
	defer mainServer.Shutdown()

	// 等待服务器完全启动（增加等待时间）
	fmt.Println("等待主服务器启动...")
	time.Sleep(100 * time.Second)

	// 测试系统账户连接
	fmt.Println("测试系统账户连接...")
	sysConn, err := nats.Connect(sysClientOpt.url, sysClientOpt.opts...)
	if err != nil {
		panic(fmt.Sprintf("系统账户连接失败: %v", err))
	}
	fmt.Println("系统账户连接成功")
	sysConn.Close()

	// 执行nsc cli命令: nsc push -a APP
	fmt.Println("执行 nsc push 命令...")
	execCmd := exec.Command("nsc", "push", "-a", "APP")
	output, err := execCmd.CombinedOutput()
	if err != nil {
		panic(fmt.Sprintf("执行nsc命令失败: %v\n输出: %s", err, string(output)))
	}
	fmt.Printf("nsc命令输出:\n%s\n", string(output))

	// 创建并启动leaf服务器
	leafServer, err := server.NewServer(&leafServerOpt)
	if err != nil {
		panic(fmt.Sprintf("创建叶服务器失败: %v", err))
	}
	go leafServer.Start()
	defer leafServer.Shutdown()

	// 等待leaf服务器启动
	fmt.Println("等待叶服务器启动...")
	time.Sleep(5 * time.Second)

	// 创建NATS连接
	mainConn, err := nats.Connect(mainClientOpt.url, mainClientOpt.opts...)
	if err != nil {
		panic(fmt.Sprintf("连接到主服务器失败: %v", err))
	}
	defer mainConn.Drain()

	mainSub, err := mainConn.Subscribe(subjectName, func(msg *nats.Msg) {
		fmt.Printf("主客户端接收到消息: %s\n", string(msg.Data))
		msg.Respond([]byte("Hello from main client!"))
	})
	if err != nil {
		panic(fmt.Sprintf("在主客户端创建订阅失败: %v", err))
	}
	defer mainSub.Drain()

	leafConn, err := nats.Connect(leafClientOpt.url, leafClientOpt.opts...)
	if err != nil {
		panic(fmt.Sprintf("连接到叶服务器失败: %v", err))
	}
	defer leafConn.Drain()

	respond, err := leafConn.Request(subjectName, []byte("Hello from leaf client!"), time.Second)
	if err != nil {
		panic(fmt.Sprintf("在叶客户端创建请求失败: %v", err))
	}
	fmt.Printf("叶客户端接收到响应: %s\n", string(respond.Data))
}
