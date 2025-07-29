package main

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
		address          = "0.0.0.0"
		mainPort         = 4222
		clusterPort      = 7422
		leafPort         = 4223
		subjectName      = "foo"
		credsFileAppUser = "~/.local/share/nats/nsc/keys/creds/local/APP/user.creds"
		credsFileSysUser = "~/.local/share/nats/nsc/keys/creds/local/SYS/sys.creds"
	)

	// Step 1: Update operator configuration
	fmt.Println("Updating operator configuration...")
	execCmd := exec.Command("nsc", "edit", "operator", "--require-signing-keys",
		"--account-jwt-server-url", fmt.Sprintf("nats://%s:%d", address, mainPort))
	output, err := execCmd.CombinedOutput()
	if err != nil {
		panic(fmt.Sprintf("Failed to update operator: %v\nOutput: %s", err, string(output)))
	}
	fmt.Printf("Operator update output:\n%s\n", string(output))

	// Step 2: Update APP account
	fmt.Println("Updating APP account configuration...")
	execCmd = exec.Command("nsc", "edit", "account", "APP", "--sk", "generate")
	output, err = execCmd.CombinedOutput()
	if err != nil {
		panic(fmt.Sprintf("Failed to update APP account: %v\nOutput: %s", err, string(output)))
	}
	fmt.Printf("APP account update output:\n%s\n", string(output))

	// Step 3: Generate resolver configuration
	fmt.Println("Generating resolver configuration...")
	execCmd = exec.Command("nsc", "generate", "config", "--nats-resolver", "--sys-account", "SYS")
	output, err = execCmd.CombinedOutput()
	if err != nil {
		panic(fmt.Sprintf("Failed to generate resolver configuration: %v\nOutput: %s", err, string(output)))
	}

	resolverFile := "resolver.conf"
	err = os.WriteFile(resolverFile, output, 0644)
	if err != nil {
		panic(fmt.Sprintf("Failed to write resolver.conf: %v", err))
	}
	fmt.Println("resolver.conf created")

	// Get NATS server configuration
	absResolverPath, _ := filepath.Abs(resolverFile)

	// Step 5: Start the main NATS server using Go library
	fmt.Println("Starting main NATS server using Go...")
	mainServerOpts := server.Options{
		ConfigFile: absResolverPath,
		Host:       address,
		Port:       mainPort,
		LeafNode: server.LeafNodeOpts{
			Host:      address,
			Port:      clusterPort,
			Advertise: fmt.Sprintf("%s:%d", address, clusterPort),
		},
		NoSigs: true, // Disable signal handling
	}
	mainServerOpts.ProcessConfigFile(absResolverPath)
	mainServer, err := server.NewServer(&mainServerOpts)
	if err != nil {
		panic(fmt.Sprintf("Failed to create main server: %v", err))
	}

	// Start the server
	go mainServer.Start()
	defer mainServer.Shutdown()

	if !mainServer.ReadyForConnections(10 * time.Second) {
		panic("Main server not ready for connections within timeout")
	}

	fmt.Println("Main NATS server started")

	// Step 6: Test system account connection
	fmt.Println("Testing system account connection...")
	expandedSysCreds := strings.Replace(credsFileSysUser, "~", os.Getenv("HOME"), 1)
	sysClientOpt := NATSClient{
		url: fmt.Sprintf("nats://%s:%d", address, mainPort),
		opts: []nats.Option{
			nats.Name("System Client"),
			nats.ReconnectWait(time.Second),
			nats.MaxReconnects(-1),
			nats.UserCredentials(expandedSysCreds),
		},
	}

	sysConn, err := nats.Connect(sysClientOpt.url, sysClientOpt.opts...)
	if err != nil {
		panic(fmt.Sprintf("System account connection failed: %v", err))
	}
	fmt.Println("System account connected successfully")
	sysConn.Close()

	// Step 7: Push APP account JWT
	fmt.Println("Pushing APP account JWT...")
	pushCmd := exec.Command("nsc", "push", "-a", "APP", "-u",
		fmt.Sprintf("nats://%s:%d", address, mainPort))
	output, err = pushCmd.CombinedOutput()
	if err != nil {
		panic(fmt.Sprintf("Failed to push JWT: %v\nOutput: %s", err, string(output)))
	}
	fmt.Printf("JWT push output:\n%s\n", string(output))

	// Step 8: Start the leaf node NATS server using Go library
	fmt.Println("Starting leaf node NATS server using Go...")
	expandedAppCreds := strings.Replace(credsFileAppUser, "~", os.Getenv("HOME"), 1)

	leafServerOpts := server.Options{
		Host: address,
		Port: leafPort,
		LeafNode: server.LeafNodeOpts{
			ReconnectInterval: 2 * time.Second,
			Remotes: []*server.RemoteLeafOpts{
				{
					URLs:        []*url.URL{{Scheme: "nats", Host: fmt.Sprintf("%s:%d", address, clusterPort)}},
					Credentials: expandedAppCreds,
				},
			},
		},
		NoSigs: true, // Disable signal handling
	}

	leafServer, err := server.NewServer(&leafServerOpts)
	if err != nil {
		panic(fmt.Sprintf("Failed to create leaf node server: %v", err))
	}

	// Start leaf node server
	go leafServer.Start()
	defer leafServer.Shutdown()
	// leafServer.ReloadOptions()

	if !leafServer.ReadyForConnections(10 * time.Second) {
		panic("Leaf node server not ready for connections within timeout")
	}

	fmt.Println("Leaf node NATS server started")

	// Wait for leaf node connection to establish
	time.Sleep(3 * time.Second)

	// Step 9: Create main client connection and subscription
	fmt.Println("Creating main client connection...")
	mainClientOpt := NATSClient{
		url: fmt.Sprintf("nats://%s:%d", address, mainPort),
		opts: []nats.Option{
			nats.Name("Main Client"),
			nats.ReconnectWait(time.Second),
			nats.MaxReconnects(-1),
			nats.UserCredentials(expandedAppCreds),
		},
	}

	mainConn, err := nats.Connect(mainClientOpt.url, mainClientOpt.opts...)
	if err != nil {
		panic(fmt.Sprintf("Failed to connect to main server: %v", err))
	}
	defer mainConn.Close()

	mainSub, err := mainConn.Subscribe(subjectName, func(msg *nats.Msg) {
		fmt.Printf("Main client received message: %s\n", string(msg.Data))
		msg.Respond([]byte("Hello from main client!"))
	})
	if err != nil {
		panic(fmt.Sprintf("Failed to create subscription on main client: %v", err))
	}
	defer mainSub.Unsubscribe()

	// Step 10: Create leaf client connection and make request
	fmt.Println("Creating leaf client connection...")
	leafClientOpt := NATSClient{
		url: fmt.Sprintf("nats://%s:%d", address, leafPort),
		opts: []nats.Option{
			nats.Name("Leaf Client"),
			nats.ReconnectWait(time.Second),
			nats.MaxReconnects(-1),
		},
	}

	leafConn, err := nats.Connect(leafClientOpt.url, leafClientOpt.opts...)
	if err != nil {
		panic(fmt.Sprintf("Failed to connect to leaf server: %v", err))
	}
	defer leafConn.Close()

	// Wait for subscription propagation
	time.Sleep(1 * time.Second)

	// Send request and wait for response
	fmt.Println("Sending request from leaf node to main server...")
	respond, err := leafConn.Request(subjectName, []byte("Hello from leaf client!"), 5*time.Second)
	if err != nil {
		panic(fmt.Sprintf("Failed to make request from leaf client: %v", err))
	}
	fmt.Printf("Leaf client received response: %s\n", string(respond.Data))

	fmt.Println("Test completed successfully!")
	time.Sleep(1 * time.Second)
}
