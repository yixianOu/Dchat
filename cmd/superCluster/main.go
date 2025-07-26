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
		address          = "localhost"
		eastPort         = 4222
		westPort         = 4223
		eastHttpPort     = 8222
		westHttpPort     = 8223
		eastGatewayPort  = 7222
		westGatewayPort  = 7223
		subjectName      = "foo"
		credsFileAppUser = "~/.local/share/nats/nsc/keys/creds/local/APP/user.creds"
		// credsFileSysUser = "~/.local/share/nats/nsc/keys/creds/local/SYS/sys.creds"
	)
	expandedAppCreds := strings.Replace(credsFileAppUser, "~", os.Getenv("HOME"), 1)

	fmt.Println("Updating operator configuration...")
	execCmd := exec.Command("nsc", "edit", "operator", "--require-signing-keys",
		"--account-jwt-server-url", fmt.Sprintf("nats://%s:%d", address, eastPort))
	output, err := execCmd.CombinedOutput()
	if err != nil {
		panic(fmt.Sprintf("Failed to update operator: %v\nOutput: %s", err, string(output)))
	}
	fmt.Printf("Operator update output:\n%s\n", string(output))

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
	absResolverPath, _ := filepath.Abs(resolverFile)

	fmt.Println("Starting east NATS server using Go...")
	eastServerOpts := server.Options{}
	eastServerOpts.ProcessConfigFile(absResolverPath)

	eastServerOpts.Host = address
	eastServerOpts.Port = eastPort
	eastServerOpts.HTTPPort = eastHttpPort
	eastServerOpts.Gateway = server.GatewayOpts{
		Name: "east",
		Port: eastGatewayPort,
		Gateways: []*server.RemoteGatewayOpts{
			{
				Name: "west",
				URLs: []*url.URL{{Scheme: "nats", Host: fmt.Sprintf("%s:%d", address, westGatewayPort)}},
			},
			{
				Name: "east",
				URLs: []*url.URL{{Scheme: "nats", Host: fmt.Sprintf("%s:%d", address, eastGatewayPort)}},
			},
		},
	}
	eastServer, err := server.NewServer(&eastServerOpts)
	if err != nil {
		panic(fmt.Sprintf("Failed to create east server: %v", err))
	}
	// Start the server
	go eastServer.Start()
	defer eastServer.Shutdown()

	fmt.Println("Starting west NATS server using Go...")
	westServerOpts := server.Options{}
	westServerOpts.ProcessConfigFile(absResolverPath)
	westServerOpts.Host = address
	westServerOpts.Port = westPort
	westServerOpts.HTTPPort = westHttpPort
	westServerOpts.Gateway = server.GatewayOpts{
		Name: "west",
		Port: westGatewayPort,

		Gateways: []*server.RemoteGatewayOpts{
			{
				Name: "east",
				URLs: []*url.URL{{Scheme: "nats", Host: fmt.Sprintf("%s:%d", address, eastGatewayPort)}},
			},
			{
				Name: "west",
				URLs: []*url.URL{{Scheme: "nats", Host: fmt.Sprintf("%s:%d", address, westGatewayPort)}},
			},
		},
	}
	westServer, err := server.NewServer(&westServerOpts)
	if err != nil {
		panic(fmt.Sprintf("Failed to create west server: %v", err))
	}
	// Start the server
	go westServer.Start()
	defer westServer.Shutdown()

	time.Sleep(3 * time.Second)
	fmt.Println("Pushing APP account JWT...")
	pushCmd := exec.Command("nsc", "push", "-a", "APP", "-u",
		fmt.Sprintf("nats://%s:%d", address, eastPort))
	output, err = pushCmd.CombinedOutput()
	if err != nil {
		panic(fmt.Sprintf("Failed to push JWT: %v\nOutput: %s", err, string(output)))
	}
	fmt.Printf("JWT push output:\n%s\n", string(output))

	fmt.Println("Connecting to east NATS server...")
	eastClient, err := nats.Connect(fmt.Sprintf("nats://%s:%d", address, eastPort),
		nats.Name("East Client"),
		nats.ReconnectWait(time.Second),
		nats.MaxReconnects(-1),
		nats.UserCredentials(expandedAppCreds),
	)
	if err != nil {
		panic(fmt.Sprintf("Failed to connect to east NATS server: %v", err))
	}
	defer eastClient.Close()
	fmt.Println("Connecting to west NATS server...")
	westClient, err := nats.Connect(fmt.Sprintf("nats://%s:%d", address, westPort),
		nats.Name("West Client"),
		nats.ReconnectWait(time.Second),
		nats.MaxReconnects(-1),
		nats.UserCredentials(expandedAppCreds),
	)
	if err != nil {
		panic(fmt.Sprintf("Failed to connect to west NATS server: %v", err))
	}
	defer westClient.Close()

	fmt.Println("Subscribing to subject:", subjectName)
	subscription, err := eastClient.Subscribe(subjectName, func(msg *nats.Msg) {
		msg.Respond([]byte("Hello from East!"))
	})
	if err != nil {
		panic(fmt.Sprintf("Failed to subscribe to subject %q: %v", subjectName, err))
	}
	defer subscription.Unsubscribe()
	time.Sleep(1 * time.Second)
	fmt.Println("Request to subject:", subjectName)
	response, err := westClient.Request(subjectName, []byte("Hello from West!"), 5*time.Second)
	if err != nil {
		panic(fmt.Sprintf("Failed to request subject %q: %v", subjectName, err))
	}
	fmt.Printf("Received response: %s\n", string(response.Data))

}
