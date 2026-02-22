// 信令服务器 - 用于P2P节点交换地址信息
//
// 运行方式:
//   go run signal_server.go -port 8080
//
// 部署到公网服务器后，两台不同局域网的设备可以通过此服务器交换STUN信息

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

// PeerInfo 节点信息
type PeerInfo struct {
	NodeID     string `json:"node_id"`
	LocalAddr  string `json:"local_addr"`
	PublicAddr string `json:"public_addr"`
	NATType    string `json:"nat_type"`
	Timestamp  int64  `json:"timestamp"`
}

// SignalMessage 信令消息
type SignalMessage struct {
	Type    string   `json:"type"`
	From    string   `json:"from"`
	To      string   `json:"to"`
	Payload PeerInfo `json:"payload"`
}

// SignalServer 信令服务器
type SignalServer struct {
	peers map[string]*PeerInfo
	mu    sync.RWMutex
}

// NewSignalServer 创建信令服务器
func NewSignalServer() *SignalServer {
	return &SignalServer{
		peers: make(map[string]*PeerInfo),
	}
}

// RegisterHandler 处理注册请求
func (s *SignalServer) RegisterHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var msg SignalMessage
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if msg.Type != "register" || msg.From == "" {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	s.peers[msg.From] = &msg.Payload
	s.mu.Unlock()

	fmt.Printf("[%s] 节点注册: %s@%s (NAT: %s)\n",
		time.Now().Format("15:04:05"),
		msg.From,
		msg.Payload.PublicAddr,
		msg.Payload.NATType)

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// QueryHandler 处理查询请求
func (s *SignalServer) QueryHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	peerID := r.URL.Query().Get("peer_id")
	if peerID == "" {
		http.Error(w, "Missing peer_id", http.StatusBadRequest)
		return
	}

	s.mu.RLock()
	peer, exists := s.peers[peerID]
	s.mu.RUnlock()

	if !exists {
		http.Error(w, "Peer not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(peer)
}

// ListHandler 列出所有节点
func (s *SignalServer) ListHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	peers := make(map[string]*PeerInfo)
	for k, v := range s.peers {
		peers[k] = v
	}
	s.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(peers)
}

// StatusHandler 状态检查
func (s *SignalServer) StatusHandler(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	count := len(s.peers)
	s.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":     "running",
		"peer_count": count,
	})
}

// CleanupRoutine 清理过期节点
func (s *SignalServer) CleanupRoutine() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now().Unix()
		s.mu.Lock()
		for id, peer := range s.peers {
			if now-peer.Timestamp > 300 { // 5分钟过期
				delete(s.peers, id)
				fmt.Printf("[%s] 清理过期节点: %s\n",
					time.Now().Format("15:04:05"), id)
			}
		}
		s.mu.Unlock()
	}
}

func main() {
	port := flag.Int("port", 8080, "服务器端口")
	flag.Parse()

	server := NewSignalServer()

	// 启动清理协程
	go server.CleanupRoutine()

	// 注册路由
	http.HandleFunc("/register", server.RegisterHandler)
	http.HandleFunc("/query", server.QueryHandler)
	http.HandleFunc("/list", server.ListHandler)
	http.HandleFunc("/status", server.StatusHandler)

	// 首页
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head>
    <title>P2P信令服务器</title>
    <style>
        body { font-family: Arial, sans-serif; max-width: 800px; margin: 50px auto; padding: 20px; }
        h1 { color: #333; }
        code { background: #f4f4f4; padding: 2px 6px; border-radius: 3px; }
        pre { background: #f4f4f4; padding: 15px; border-radius: 5px; overflow-x: auto; }
    </style>
</head>
<body>
    <h1>P2P NAT穿透信令服务器</h1>
    <p>状态: <strong>运行中</strong></p>
    
    <h2>API接口</h2>
    <ul>
        <li><code>POST /register</code> - 注册节点</li>
        <li><code>GET /query?peer_id=&lt;id&gt;</code> - 查询节点</li>
        <li><code>GET /list</code> - 列出所有节点</li>
        <li><code>GET /status</code> - 服务器状态</li>
    </ul>
    
    <h2>使用示例</h2>
    <pre>
# 注册节点
curl -X POST http://%s/register \
  -H "Content-Type: application/json" \
  -d '{
    "type": "register",
    "from": "Alice",
    "payload": {
      "node_id": "Alice",
      "local_addr": "192.168.1.100:10001",
      "public_addr": "120.239.59.111:50001",
      "nat_type": "Cone NAT"
    }
  }'

# 查询节点
curl http://%s/query?peer_id=Alice
    </pre>
</body>
</html>`, r.Host, r.Host)
	})

	addr := fmt.Sprintf(":%d", *port)
	fmt.Printf("信令服务器启动: http://0.0.0.0%s\n", addr)
	fmt.Printf("API端点:\n")
	fmt.Printf("  POST /register - 注册节点\n")
	fmt.Printf("  GET  /query?peer_id=<id> - 查询节点\n")
	fmt.Printf("  GET  /list - 列出所有节点\n")
	fmt.Printf("  GET  /status - 服务器状态\n")
	fmt.Println()

	log.Fatal(http.ListenAndServe(addr, nil))
}
