// package main

// import (
// 	"fmt"
// 	"net/url"
// 	"time"

// 	"github.com/nats-io/nats-server/v2/server"
// 	"github.com/nats-io/nats.go"
// )

// func main() {
// 	var (
// 		address     = "localhost"
// 		mainPort    = 4222
// 		clusterPort = 7422
// 		leafPort    = 4223
// 	)

// 	mainLeafConf := server.LeafNodeOpts{
// 		Port: clusterPort,
// 	}
// 	mainConf := server.Options{
// 		Host:     address,
// 		Port:     mainPort,
// 		LeafNode: mainLeafConf,
// 	}
// 	LeafNodeConf := server.LeafNodeOpts{
// 		Remotes: []*server.RemoteLeafOpts{
// 			{
// 				URLs: []*url.URL{
// 					{Scheme: "nats", Host: fmt.Sprintf("%s:%d", address, mainPort)},
// 				},
// 			},
// 		},
// 	}
// 	leafConf := server.Options{
// 		Host:     address,
// 		Port:     leafPort,
// 		LeafNode: LeafNodeConf,
// 	}
// 	mainServer, err := server.NewServer(&mainConf)
// 	if err != nil {
// 		fmt.Printf("Failed to create main server: %v\n", err)
// 		return
// 	}
// 	leafServer, err := server.NewServer(&leafConf)
// 	if err != nil {
// 		fmt.Printf("Failed to create leaf server: %v\n", err)
// 		return
// 	}

// 	go mainServer.Start()
// 	defer mainServer.Shutdown()
// 	go leafServer.Start()
// 	defer leafServer.Shutdown()
// 	time.Sleep(time.Millisecond * 1000)

// 	mainClient, err := nats.Connect(fmt.Sprintf("nats://%s:%d", address, mainPort))
// 	if err != nil {
// 		fmt.Printf("Failed to connect to main server: %v\n", err)
// 		return
// 	}
// 	defer mainClient.Drain()
// 	leafClient, err := nats.Connect(fmt.Sprintf("nats://%s:%d", address, leafPort))
// 	if err != nil {
// 		fmt.Printf("Failed to connect to leaf server: %v\n", err)
// 		return
// 	}
// 	defer leafClient.Drain()

// 	mainSub, err := mainClient.Subscribe("foo", func(msg *nats.Msg) {
// 		msg.Respond([]byte("response from main server"))
// 	})
// 	if err != nil {
// 		fmt.Printf("Failed to subscribe on main server: %v\n", err)
// 		return
// 	}
// 	defer mainSub.Drain()
// 	time.Sleep(time.Millisecond * 1000)
// 	respond, err := leafClient.Request("foo", []byte("request from leaf server"), time.Second)
// 	if err != nil {
// 		fmt.Printf("Request failed: %v\n", err)
// 		return
// 	}
// 	fmt.Printf("Received response: %s\n", string(respond.Data))

// 	leafSub, err := leafClient.Subscribe("foo", func(msg *nats.Msg) {
// 		msg.Respond([]byte("response from leaf server"))
// 	})
// 	if err != nil {
// 		fmt.Printf("Failed to subscribe on leaf server: %v\n", err)
// 		return
// 	}
// 	defer leafSub.Drain()
// 	respond, err = leafClient.Request("foo", []byte("request from leaf server"), time.Second)
// 	if err != nil {
// 		fmt.Printf("Request failed: %v\n", err)
// 		return
// 	}
// 	fmt.Printf("Received response: %s\n", string(respond.Data))

// 	mainServer.Shutdown()
// 	time.Sleep(time.Millisecond * 100)
// 	respond, err = leafClient.Request("foo", []byte("request from leaf server"), time.Second)
// 	if err != nil {
// 		fmt.Printf("Request failed: %v\n", err)
// 		return
// 	}
// 	fmt.Printf("Received response: %s\n", string(respond.Data))
// }
