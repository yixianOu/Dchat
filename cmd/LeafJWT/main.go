package main

import (
	"fmt"
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
	)

	mainClientOpt := NATSClient{
		url: fmt.Sprintf("nats://%s:%d", address, mainPort),
		opts: []nats.Option{
			nats.Name("Main Client"),
			nats.ReconnectWait(time.Second),
			nats.MaxReconnects(-1),
			nats.UserCredentials(credsFileAppUser)},
	}
	leafClientOpt := NATSClient{
		url: fmt.Sprintf("nats://%s:%d", address, leafPort),
		opts: []nats.Option{
			nats.Name("Leaf Client"),
			nats.ReconnectWait(time.Second),
			nats.MaxReconnects(-1),
		},
	}
	mainServerOpt := server.Options{}

}
