package network

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

type TailscaleManager struct {
	localIP string
	peers   []string
}

type TailscaleStatus struct {
	Connected bool   `json:"connected"`
	IP        string `json:"ip"`
	Peers     []Peer `json:"peers"`
}

type Peer struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	IPs      []string `json:"ips"`
	Online   bool     `json:"online"`
	LastSeen string   `json:"last_seen"`
}

func NewTailscaleManager() *TailscaleManager {
	return &TailscaleManager{}
}

func (tm *TailscaleManager) GetLocalIP() (string, error) {
	// 使用tailscale命令获取本机IP
	cmd := exec.Command("tailscale", "ip", "-4")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get tailscale IP: %w", err)
	}

	ip := strings.TrimSpace(string(output))
	tm.localIP = ip
	return ip, nil
}

func (tm *TailscaleManager) GetPeerIPs() ([]string, error) {
	status, err := tm.GetStatus()
	if err != nil {
		return nil, err
	}

	var ips []string
	for _, peer := range status.Peers {
		if peer.Online && len(peer.IPs) > 0 {
			ips = append(ips, peer.IPs[0])
		}
	}

	tm.peers = ips
	return ips, nil
}

func (tm *TailscaleManager) GetStatus() (*TailscaleStatus, error) {
	// 使用tailscale status命令获取状态
	cmd := exec.Command("tailscale", "status", "--json")
	output, err := cmd.Output()
	if err != nil {
		// 如果命令失败，返回模拟状态
		return &TailscaleStatus{
			Connected: false,
			IP:        "",
			Peers:     []Peer{},
		}, nil
	}

	// 解析JSON输出
	var rawStatus map[string]interface{}
	if err := json.Unmarshal(output, &rawStatus); err != nil {
		return nil, fmt.Errorf("failed to parse tailscale status: %w", err)
	}

	// 转换为我们的结构
	status := &TailscaleStatus{
		Connected: true,
		Peers:     []Peer{},
	}

	// 提取本机IP
	if self, ok := rawStatus["Self"].(map[string]interface{}); ok {
		if ips, ok := self["TailscaleIPs"].([]interface{}); ok && len(ips) > 0 {
			if ip, ok := ips[0].(string); ok {
				status.IP = ip
			}
		}
	}

	// 提取对等节点信息
	if peers, ok := rawStatus["Peer"].(map[string]interface{}); ok {
		for _, peer := range peers {
			if peerData, ok := peer.(map[string]interface{}); ok {
				p := Peer{
					Online: false,
					IPs:    []string{},
				}

				if name, ok := peerData["DNSName"].(string); ok {
					p.Name = name
				}

				if ips, ok := peerData["TailscaleIPs"].([]interface{}); ok {
					for _, ip := range ips {
						if ipStr, ok := ip.(string); ok {
							p.IPs = append(p.IPs, ipStr)
						}
					}
				}

				if lastSeen, ok := peerData["LastSeen"].(string); ok {
					p.LastSeen = lastSeen
					// 简单判断在线状态：最近5分钟内有活动
					if t, err := time.Parse(time.RFC3339, lastSeen); err == nil {
						p.Online = time.Since(t) < 5*time.Minute
					}
				}

				status.Peers = append(status.Peers, p)
			}
		}
	}

	return status, nil
}

func (tm *TailscaleManager) IsConnected() bool {
	status, err := tm.GetStatus()
	if err != nil {
		return false
	}
	return status.Connected && status.IP != ""
}

func (tm *TailscaleManager) WaitForConnection(ctx context.Context, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for tailscale connection")
		case <-ticker.C:
			if tm.IsConnected() {
				return nil
			}
		}
	}
}

// DiscoverNATSNodes 通过Tailscale网络发现其他NATS节点
func (tm *TailscaleManager) DiscoverNATSNodes(port int) ([]string, error) {
	peers, err := tm.GetPeerIPs()
	if err != nil {
		return nil, err
	}

	var natsNodes []string
	for _, peerIP := range peers {
		// 尝试连接到对等节点的NATS端口
		url := fmt.Sprintf("http://%s:%d/varz", peerIP, port)

		client := &http.Client{Timeout: 2 * time.Second}
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			// 如果能连接到NATS监控端口，说明这是一个NATS节点
			natsURL := fmt.Sprintf("nats://%s:%d", peerIP, port-1000) // 假设集群端口比监控端口小1000
			natsNodes = append(natsNodes, natsURL)
		}
	}

	return natsNodes, nil
}

// BroadcastNodeInfo 广播节点信息到Tailscale网络
func (tm *TailscaleManager) BroadcastNodeInfo(nodeInfo map[string]interface{}) error {
	// TODO: 实现节点信息广播机制
	// 可以使用NATS主题或者HTTP API
	return nil
}
