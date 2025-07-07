package main

import (
	"github.com/nats-io/nats.go"
)

func main() {
	// Connect to NATS server
	nc, err := nats.Connect(nats.DefaultURL)
	if err != nil {
		panic(err)
	}
	defer nc.Close()

	// Create a simple subscriber
	nc.Subscribe("updates", func(m *nats.Msg) {
		println("Received message:", string(m.Data))
	})

	// Publish a message
	nc.Publish("updates", []byte("Hello, NATS!"))

	// Flush connection to ensure messages are sent
	nc.Flush()

	// Wait for a while to receive messages
	select {}
}
