package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/micro"

	"golang.org/x/exp/slices"
)

func main() {

	url, exists := os.LookupEnv("NATS_URL")
	if !exists {
		url = nats.DefaultURL
	} else {
		url = strings.TrimSpace(url)
	}

	if strings.TrimSpace(url) == "" {
		url = nats.DefaultURL
	}

	nc, err := nats.Connect(url)
	if err != nil {
		log.Fatal(err)
		return
	}

	srv, err := micro.AddService(nc, micro.Config{
		Name:        "minmax",
		Version:     "0.0.1",
		Description: "Returns the min/max number in a request",
	})

	fmt.Printf("Created service: %s (%s)\n", srv.Info().Name, srv.Info().ID)

	if err != nil {
		log.Fatal(err)
		return
	}

	root := srv.AddGroup("minmax")

	root.AddEndpoint("min", micro.HandlerFunc(handleMin))
	root.AddEndpoint("max", micro.HandlerFunc(handleMax))

	requestSlice := []int{-1, 2, 100, -2000}

	requestData, _ := json.Marshal(requestSlice)

	msg, _ := nc.Request("minmax.min", requestData, 2*time.Second)

	result := decode(msg)
	fmt.Printf("Requested min value, got %d\n", result.Min)

	msg, _ = nc.Request("minmax.max", requestData, 2*time.Second)
	result = decode(msg)
	fmt.Printf("Requested max value, got %d\n", result.Max)

	fmt.Printf("Endpoint '%s' requests: %d\n", srv.Stats().Endpoints[0].Name, srv.Stats().Endpoints[0].NumRequests)
	fmt.Printf("Endpoint '%s' requests: %d\n", srv.Stats().Endpoints[1].Name, srv.Stats().Endpoints[1].NumRequests)
}

func handleMin(req micro.Request) {
	var arr []int
	_ = json.Unmarshal([]byte(req.Data()), &arr)
	slices.Sort(arr)

	res := ServiceResult{Min: arr[0]}
	req.RespondJSON(res)
}

func handleMax(req micro.Request) {
	var arr []int
	_ = json.Unmarshal([]byte(req.Data()), &arr)
	slices.Sort(arr)

	res := ServiceResult{Max: arr[len(arr)-1]}
	req.RespondJSON(res)
}

func decode(msg *nats.Msg) ServiceResult {
	var res ServiceResult
	json.Unmarshal(msg.Data, &res)
	return res
}

type ServiceResult struct {
	Min int `json:"min,omitempty"`
	Max int `json:"max,omitempty"`
}
