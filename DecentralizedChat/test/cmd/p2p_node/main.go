// P2P节点程序 - 可在两台不同设备上独立运行
//
// 使用方式:
// 设备A: go run p2p_node.go -node-id Alice -listen-port 10001
// 设备B: go run p2p_node.go -node-id Bob   -listen-port 10002 -peer-id Alice
//
// 两台设备需要能够访问同一个信令服务器

package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

// 配置
var (
	signalServerURL = "http://121.199.173.116:8080" // 公网信令服务器
	stunServers     = []string{
		"stun.chat.bilibili.com:3478", // B站STUN服务器(国内可用)
	}
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
	Type      string   `json:"type"`
	From      string   `json:"from"`
	To        string   `json:"to"`
	Payload   PeerInfo `json:"payload"`
}

// P2PNode P2P节点
type P2PNode struct {
	NodeID     string
	LocalAddr  *net.UDPAddr
	PublicAddr *net.UDPAddr
	NATType    string
	conn       *net.UDPConn
	peerInfo   *PeerInfo
	mu         sync.RWMutex
	connected  bool
	msgCount   int
}

// NewP2PNode 创建P2P节点
func NewP2PNode(nodeID string, listenPort int) (*P2PNode, error) {
	addr := &net.UDPAddr{IP: net.ParseIP("0.0.0.0"), Port: listenPort}
	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		return nil, fmt.Errorf("创建UDP监听失败: %v", err)
	}

	return &P2PNode{
		NodeID:    nodeID,
		LocalAddr: conn.LocalAddr().(*net.UDPAddr),
		conn:      conn,
	}, nil
}

// Close 关闭节点
func (n *P2PNode) Close() {
	n.conn.Close()
}

// GetSTUNInfo 通过STUN获取公网地址
// 重要：必须使用监听连接发送STUN请求，这样获取的公网端口才与监听端口一致
func (n *P2PNode) GetSTUNInfo() error {
	for _, server := range stunServers {
		publicAddr, natType, err := n.querySTUN(server)
		if err == nil {
			n.PublicAddr = publicAddr
			n.NATType = natType
			return nil
		}
		fmt.Printf("STUN服务器 %s 失败: %v\n", server, err)
	}
	return fmt.Errorf("所有STUN服务器都失败")
}

// querySTUN 查询STUN服务器
// 使用节点的监听连接发送请求，确保STUN获取的公网端口与监听端口一致
func (n *P2PNode) querySTUN(server string) (*net.UDPAddr, string, error) {
	serverAddr, err := net.ResolveUDPAddr("udp4", server)
	if err != nil {
		return nil, "", err
	}

	// STUN Binding Request
	request := make([]byte, 20)
	request[0], request[1] = 0x00, 0x01 // Binding Request
	request[2], request[3] = 0x00, 0x00 // 长度
	request[4], request[5], request[6], request[7] = 0x21, 0x12, 0xA4, 0x42 // Magic Cookie

	// 使用监听连接发送请求（关键！）
	if err := n.conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return nil, "", fmt.Errorf("设置写入超时失败: %v", err)
	}
	if _, err := n.conn.WriteToUDP(request, serverAddr); err != nil {
		return nil, "", fmt.Errorf("发送STUN请求失败: %v", err)
	}

	// 读取响应
	response := make([]byte, 512)
	if err := n.conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return nil, "", fmt.Errorf("设置读取超时失败: %v", err)
	}
	num, _, err := n.conn.ReadFromUDP(response)
	if err != nil {
		return nil, "", fmt.Errorf("读取STUN响应失败: %v", err)
	}

	// 解析XOR-MAPPED-ADDRESS
	publicAddr, err := parseXORMappedAddress(response[:num])
	if err != nil {
		return nil, "", err
	}

	return publicAddr, "Cone NAT", nil
}

// parseXORMappedAddress 解析XOR-MAPPED-ADDRESS
func parseXORMappedAddress(data []byte) (*net.UDPAddr, error) {
	if len(data) < 28 {
		return nil, fmt.Errorf("响应太短")
	}

	// 查找属性
	pos := 20
	for pos < len(data)-4 {
		attrType := uint16(data[pos])<<8 | uint16(data[pos+1])
		attrLen := uint16(data[pos+2])<<8 | uint16(data[pos+3])

		if attrType == 0x0020 && pos+12 <= len(data) { // XOR-MAPPED-ADDRESS
			xorPort := uint16(data[pos+6])<<8 | uint16(data[pos+7])
			port := xorPort ^ 0x2112

			xorIP := uint32(data[pos+8])<<24 | uint32(data[pos+9])<<16 |
				uint32(data[pos+10])<<8 | uint32(data[pos+11])
			ip := xorIP ^ 0x2112A442

			return &net.UDPAddr{
				IP:   net.IPv4(byte(ip>>24), byte(ip>>16), byte(ip>>8), byte(ip)),
				Port: int(port),
			}, nil
		}

		pos += 4 + int(attrLen)
		if attrLen%4 != 0 {
			pos += 4 - int(attrLen%4)
		}
	}

	return nil, fmt.Errorf("未找到XOR-MAPPED-ADDRESS")
}

// GetInfo 获取节点信息
func (n *P2PNode) GetInfo() PeerInfo {
	publicAddrStr := ""
	if n.PublicAddr != nil {
		publicAddrStr = n.PublicAddr.String()
	}
	return PeerInfo{
		NodeID:     n.NodeID,
		LocalAddr:  n.LocalAddr.String(),
		PublicAddr: publicAddrStr,
		NATType:    n.NATType,
		Timestamp:  time.Now().Unix(),
	}
}

// RegisterToSignalServer 注册到信令服务器
func (n *P2PNode) RegisterToSignalServer() error {
	info := n.GetInfo()
	msg := SignalMessage{
		Type:    "register",
		From:    n.NodeID,
		Payload: info,
	}

	data, _ := json.Marshal(msg)
	resp, err := http.Post(signalServerURL+"/register", "application/json", bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("注册失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("注册失败，状态码: %d", resp.StatusCode)
	}

	fmt.Printf("✅ 已注册到信令服务器\n")
	return nil
}

// QueryPeer 查询对等节点
func (n *P2PNode) QueryPeer(peerID string) (*PeerInfo, error) {
	resp, err := http.Get(fmt.Sprintf("%s/query?peer_id=%s", signalServerURL, peerID))
	if err != nil {
		return nil, fmt.Errorf("查询失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("对等节点不存在")
	}

	var peer PeerInfo
	if err := json.NewDecoder(resp.Body).Decode(&peer); err != nil {
		return nil, fmt.Errorf("解析响应失败: %v", err)
	}

	return &peer, nil
}

// ListPeers 列出所有在线节点
func (n *P2PNode) ListPeers() (map[string]*PeerInfo, error) {
	resp, err := http.Get(fmt.Sprintf("%s/list", signalServerURL))
	if err != nil {
		return nil, fmt.Errorf("查询失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("服务器错误")
	}

	var peers map[string]*PeerInfo
	if err := json.NewDecoder(resp.Body).Decode(&peers); err != nil {
		return nil, fmt.Errorf("解析响应失败: %v", err)
	}

	return peers, nil
}

// HolePunch 执行UDP打洞 (使用协议格式)
func (n *P2PNode) HolePunch(peer *PeerInfo) error {
	fmt.Printf("\n🎯 开始UDP打洞到 %s\n", peer.NodeID)
	fmt.Printf("   公网地址: %s\n", peer.PublicAddr)
	fmt.Printf("   内网地址: %s\n", peer.LocalAddr)

	// 解析地址
	publicAddr, err := net.ResolveUDPAddr("udp4", peer.PublicAddr)
	if err != nil {
		fmt.Printf("   ❌ 解析公网地址失败: %v\n", err)
	}
	localAddr, err := net.ResolveUDPAddr("udp4", peer.LocalAddr)
	if err != nil {
		fmt.Printf("   ❌ 解析内网地址失败: %v\n", err)
	}

	if publicAddr == nil && localAddr == nil {
		return fmt.Errorf("无法解析任何有效地址")
	}

	// 构建打洞消息
	holePunchMsg := &ProtocolMessage{
		Type:    MsgTypeHolePunch,
		Version: 1,
		Data:    []byte(n.NodeID),
	}
	data := holePunchMsg.Encode()

	// 同时向公网和内网地址发送打洞包
	fmt.Printf("   正在发送打洞包...\n")
	fmt.Printf("   本机地址: %s\n", n.LocalAddr)
	fmt.Printf("   目标公网: %s, 目标内网: %s\n", publicAddr, localAddr)

	// 清除写超时（UDP 写操作是异步的，不应该设置超时）
	n.conn.SetWriteDeadline(time.Time{})

	var publicSent, localSent int
	for i := 0; i < 10; i++ {
		if publicAddr != nil {
			if _, err := n.conn.WriteToUDP(data, publicAddr); err != nil {
				fmt.Printf("   ❌ 发送到公网 [%s] 失败 [%d]: %v\n", publicAddr, i, err)
			} else {
				publicSent++
			}
		}
		if localAddr != nil {
			if _, err := n.conn.WriteToUDP(data, localAddr); err != nil {
				fmt.Printf("   ❌ 发送到内网 [%s] 失败 [%d]: %v\n", localAddr, i, err)
			} else {
				localSent++
			}
		}
		time.Sleep(100 * time.Millisecond)
	}

	fmt.Printf("   打洞包发送完成 (公网:%d/10, 内网:%d/10)\n", publicSent, localSent)
	if publicSent == 0 && localSent == 0 {
		fmt.Printf("   ❌ 所有打洞包都发送失败，请检查:\n")
		fmt.Printf("      1. 防火墙是否阻止 UDP 出站\n")
		fmt.Printf("      2. 路由器是否为 Symmetric NAT\n")
		fmt.Printf("      3. 是否有运营商级 NAT\n")
	} else {
		fmt.Printf("   ✅ 打洞包已发送，等待对方响应...\n")
	}
	fmt.Printf("   ⚠️  注意: 打洞成功需要对方也同时向你发送打洞包\n")
	return nil
}

// StartListening 开始监听
func (n *P2PNode) StartListening() {
	go func() {
		buf := make([]byte, 4096)
		for {
			if err := n.conn.SetReadDeadline(time.Time{}); err != nil {
				fmt.Printf("❌ 设置读取超时失败: %v\n", err)
				return
			}
			num, addr, err := n.conn.ReadFromUDP(buf)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue
				}
				fmt.Printf("❌ 读取UDP数据失败: %v\n", err)
				return
			}

			data := buf[:num]
			n.handleMessage(addr, data)
		}
	}()
}

// MessageType 消息类型
type MessageType byte

const (
	MsgTypeHolePunch MessageType = iota // 打洞消息
	MsgTypeAck                          // 确认消息
	MsgTypeData                         // 业务数据
	MsgTypeHeartbeat                    // 心跳保活
)

// ProtocolMessage 协议消息格式
// [1字节类型][1字节版本][2字节长度][N字节数据]
type ProtocolMessage struct {
	Type    MessageType
	Version byte
	Data    []byte
}

// Encode 编码消息
func (m *ProtocolMessage) Encode() []byte {
	buf := make([]byte, 4+len(m.Data))
	buf[0] = byte(m.Type)
	buf[1] = m.Version
	buf[2] = byte(len(m.Data) >> 8)
	buf[3] = byte(len(m.Data))
	copy(buf[4:], m.Data)
	return buf
}

// DecodeProtocolMessage 解码消息
func DecodeProtocolMessage(data []byte) (*ProtocolMessage, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("消息太短")
	}
	msgLen := int(data[2])<<8 | int(data[3])
	if len(data) < 4+msgLen {
		return nil, fmt.Errorf("消息长度不匹配")
	}
	return &ProtocolMessage{
		Type:    MessageType(data[0]),
		Version: data[1],
		Data:    data[4 : 4+msgLen],
	}, nil
}

// handleMessage 处理消息 (支持共享端口)
func (n *P2PNode) handleMessage(addr *net.UDPAddr, data []byte) {
	// 尝试解析协议消息
	msg, err := DecodeProtocolMessage(data)
	if err != nil {
		// 兼容旧格式：纯文本消息
		n.handlePlainTextMessage(addr, string(data))
		return
	}

	switch msg.Type {
	case MsgTypeHolePunch:
		// 打洞消息
		peerID := string(msg.Data)
		fmt.Printf("\n📨 收到打洞包 from %s@%s\n", peerID, addr)
		n.mu.Lock()
		n.connected = true
		n.mu.Unlock()
		// 回复确认
		reply := &ProtocolMessage{
			Type:    MsgTypeAck,
			Version: 1,
			Data:    []byte(n.NodeID),
		}
		if _, err := n.conn.WriteToUDP(reply.Encode(), addr); err != nil {
			fmt.Printf("   ❌ 发送打洞确认失败: %v\n", err)
		} else {
			fmt.Printf("   ✅ 已发送打洞确认 to %s\n", addr)
		}

	case MsgTypeAck:
		// 打洞确认
		peerID := string(msg.Data)
		fmt.Printf("\n✅ 收到打洞确认 from %s@%s\n", peerID, addr)
		n.mu.Lock()
		n.connected = true
		n.mu.Unlock()

	case MsgTypeData:
		// 业务数据
		fmt.Printf("\n💬 [%s]: %s\n", addr, string(msg.Data))
		n.mu.Lock()
		n.msgCount++
		n.mu.Unlock()

	case MsgTypeHeartbeat:
		// 心跳消息，更新连接状态
		n.mu.Lock()
		n.connected = true
		n.mu.Unlock()
	}
}

// handlePlainTextMessage 处理纯文本消息 (兼容模式)
func (n *P2PNode) handlePlainTextMessage(addr *net.UDPAddr, data string) {
	// 打洞消息 (旧格式)
	if len(data) > 11 && data[:11] == "HOLE_PUNCH:" {
		peerID := data[11:]
		fmt.Printf("\n📨 收到打洞包(旧格式) from %s@%s\n", peerID, addr)
		n.mu.Lock()
		n.connected = true
		n.mu.Unlock()
		reply := []byte(fmt.Sprintf("PUNCH_ACK:%s", n.NodeID))
		if _, err := n.conn.WriteToUDP(reply, addr); err != nil {
			fmt.Printf("   ❌ 发送打洞确认(旧格式)失败: %v\n", err)
		} else {
			fmt.Printf("   ✅ 已发送打洞确认(旧格式) to %s\n", addr)
		}
		return
	}

	// 确认消息 (旧格式)
	if len(data) > 10 && data[:10] == "PUNCH_ACK:" {
		peerID := data[10:]
		fmt.Printf("\n✅ 收到打洞确认(旧格式) from %s@%s\n", peerID, addr)
		n.mu.Lock()
		n.connected = true
		n.mu.Unlock()
		return
	}

	// 普通消息
	fmt.Printf("\n💬 [%s]: %s\n", addr, data)
	n.mu.Lock()
	n.msgCount++
	n.mu.Unlock()
}

// SendMessage 发送业务消息 (使用协议格式)
func (n *P2PNode) SendMessage(peer *PeerInfo, message string) error {
	addr, err := n.resolvePeerAddr(peer)
	if err != nil {
		return fmt.Errorf("解析对等节点地址失败: %v", err)
	}

	fmt.Printf("   正在发送消息到 %s...\n", addr)

	msg := &ProtocolMessage{
		Type:    MsgTypeData,
		Version: 1,
		Data:    []byte(message),
	}
	data := msg.Encode()
	_, err = n.conn.WriteToUDP(data, addr)
	if err != nil {
		return fmt.Errorf("发送消息失败: %v", err)
	}

	fmt.Printf("   ✅ 消息已发送 (%d 字节)\n", len(data))
	return nil
}

// SendHolePunch 发送打洞消息 (使用协议格式)
func (n *P2PNode) SendHolePunch(peer *PeerInfo) error {
	addr, err := n.resolvePeerAddr(peer)
	if err != nil {
		return fmt.Errorf("解析地址失败: %v", err)
	}

	msg := &ProtocolMessage{
		Type:    MsgTypeHolePunch,
		Version: 1,
		Data:    []byte(n.NodeID),
	}
	_, err = n.conn.WriteToUDP(msg.Encode(), addr)
	if err != nil {
		return fmt.Errorf("发送打洞消息失败: %v", err)
	}
	fmt.Printf("   ✅ 打洞消息已发送到 %s\n", addr)
	return nil
}

// SendHeartbeat 发送心跳消息
func (n *P2PNode) SendHeartbeat(peer *PeerInfo) error {
	addr, err := n.resolvePeerAddr(peer)
	if err != nil {
		return fmt.Errorf("解析地址失败: %v", err)
	}

	msg := &ProtocolMessage{
		Type:    MsgTypeHeartbeat,
		Version: 1,
		Data:    []byte{1}, // 简单的心跳数据
	}
	_, err = n.conn.WriteToUDP(msg.Encode(), addr)
	if err != nil {
		return fmt.Errorf("发送心跳消息失败: %v", err)
	}
	return nil
}

// resolvePeerAddr 解析对等节点地址
func (n *P2PNode) resolvePeerAddr(peer *PeerInfo) (*net.UDPAddr, error) {
	// 优先使用公网地址
	if peer.PublicAddr != "" {
		addr, err := net.ResolveUDPAddr("udp4", peer.PublicAddr)
		if err == nil {
			fmt.Printf("   使用公网地址: %s\n", addr)
			return addr, nil
		}
		fmt.Printf("   ⚠️  解析公网地址失败: %v\n", err)
	}

	// 尝试内网地址
	if peer.LocalAddr != "" {
		addr, err := net.ResolveUDPAddr("udp4", peer.LocalAddr)
		if err == nil {
			fmt.Printf("   使用内网地址: %s\n", addr)
			return addr, nil
		}
		fmt.Printf("   ⚠️  解析内网地址失败: %v\n", err)
	}

	return nil, fmt.Errorf("无法解析任何有效地址 (公网:%s, 内网:%s)", peer.PublicAddr, peer.LocalAddr)
}

// IsConnected 检查是否已连接
func (n *P2PNode) IsConnected() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.connected
}

// PrintStatus 打印状态
func (n *P2PNode) PrintStatus() {
	fmt.Println("\n========================================")
	fmt.Printf("节点ID: %s\n", n.NodeID)
	fmt.Printf("内网地址: %s\n", n.LocalAddr)
	if n.PublicAddr != nil {
		fmt.Printf("公网地址: %s\n", n.PublicAddr)
		fmt.Printf("NAT类型: %s\n", n.NATType)
	} else {
		fmt.Printf("公网地址: (未获取)\n")
	}
	n.mu.RLock()
	fmt.Printf("连接状态: %v\n", n.connected)
	fmt.Printf("收到消息: %d\n", n.msgCount)
	n.mu.RUnlock()
	fmt.Println("========================================")
}

func main() {
	// 命令行参数
	nodeID := flag.String("node-id", "", "节点ID (必需)")
	listenPort := flag.Int("listen-port", 0, "监听端口 (0=随机)")
	peerID := flag.String("peer-id", "", "对等节点ID (可选)")
	signalServer := flag.String("signal-server", signalServerURL, "信令服务器地址")
	flag.Parse()

	if *nodeID == "" {
		fmt.Println("错误: 必须指定 -node-id")
		flag.Usage()
		os.Exit(1)
	}

	signalServerURL = *signalServer

	fmt.Println("========================================")
	fmt.Println("P2P NAT穿透测试节点")
	fmt.Println("========================================")

	// 创建节点
	node, err := NewP2PNode(*nodeID, *listenPort)
	if err != nil {
		fmt.Printf("创建节点失败: %v\n", err)
		os.Exit(1)
	}
	defer node.Close()

	fmt.Printf("节点已创建: %s\n", node.NodeID)
	fmt.Printf("监听地址: %s\n", node.LocalAddr)

	// 获取STUN信息
	fmt.Println("\n正在通过STUN获取公网地址...")
	if err := node.GetSTUNInfo(); err != nil {
		fmt.Printf("⚠️ 获取公网地址失败: %v\n", err)
		fmt.Println("   继续以内网模式运行")
	} else {
		fmt.Printf("✅ 公网地址: %s\n", node.PublicAddr)
		fmt.Printf("   NAT类型: %s\n", node.NATType)
	}

	// 注册到信令服务器
	fmt.Println("\n正在注册到信令服务器...")
	if err := node.RegisterToSignalServer(); err != nil {
		fmt.Printf("⚠️ 注册失败: %v\n", err)
		fmt.Println("   可能无法与其他节点通信")
	}

	// 开始监听
	node.StartListening()

	// 如果有指定对等节点，尝试连接
	var peer *PeerInfo
	if *peerID != "" {
		fmt.Printf("\n正在查询对等节点 %s...\n", *peerID)
		peer, err = node.QueryPeer(*peerID)
		if err != nil {
			fmt.Printf("⚠️ 查询失败: %v\n", err)
		} else {
			fmt.Printf("✅ 找到对等节点:\n")
			fmt.Printf("   公网地址: %s\n", peer.PublicAddr)
			fmt.Printf("   内网地址: %s\n", peer.LocalAddr)

			// 执行打洞
			node.HolePunch(peer)
		}
	}

	// 打印状态
	node.PrintStatus()

	// 交互式命令
	fmt.Println("\n命令:")
	fmt.Println("  s - 显示状态")
	fmt.Println("  l - 列出在线节点")
	fmt.Println("  c <节点ID> - 连接到指定节点")
	fmt.Println("  m <消息> - 发送消息给对等节点")
	fmt.Println("  h - 再次打洞")
	fmt.Println("  q - 退出")

	// 信号处理
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// 输入处理
	inputChan := make(chan string)
	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		for {
			fmt.Print("> ")
			if scanner.Scan() {
				inputChan <- strings.TrimSpace(scanner.Text())
			}
		}
	}()

	// 定时打印状态
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-sigChan:
			fmt.Println("\n正在退出...")
			return

		case input := <-inputChan:
			switch {
			case input == "s":
				node.PrintStatus()

			case input == "q":
				fmt.Println("正在退出...")
				return

			case input == "l":
				peers, err := node.ListPeers()
				if err != nil {
					fmt.Printf("查询失败: %v\n", err)
				} else {
					fmt.Println("\n在线节点:")
					for id, info := range peers {
						if id != node.NodeID {
							fmt.Printf("  - %s @ %s (%s)\n", id, info.PublicAddr, info.NATType)
						}
					}
				}

			case len(input) > 2 && input[:2] == "c ":
				connectToID := input[2:]
				fmt.Printf("\n🔍 正在查询节点 %s...\n", connectToID)
				newPeer, err := node.QueryPeer(connectToID)
				if err != nil {
					fmt.Printf("❌ 查询节点失败: %v\n", err)
				} else {
					peer = newPeer
					fmt.Printf("✅ 找到节点 %s\n", connectToID)
					if err := node.HolePunch(peer); err != nil {
						fmt.Printf("❌ 打洞失败: %v\n", err)
					}
				}

			case input == "h":
				if peer != nil {
					if err := node.HolePunch(peer); err != nil {
						fmt.Printf("❌ 打洞失败: %v\n", err)
					}
				} else {
					fmt.Println("❌ 未指定对等节点，使用 c <节点ID> 连接")
				}

			case len(input) > 2 && input[:2] == "m ":
				if peer == nil {
					fmt.Println("❌ 未指定对等节点，使用 c <节点ID> 连接")
					continue
				}
				message := input[2:]
				fmt.Printf("\n📤 准备发送消息: %s\n", message)
				if err := node.SendMessage(peer, message); err != nil {
					fmt.Printf("❌ 发送失败: %v\n", err)
				}

			default:
				fmt.Println("未知命令")
			}

		case <-ticker.C:
			if node.IsConnected() {
				fmt.Printf("\n[状态] 已连接，收到 %d 条消息\n", node.msgCount)
			}
		}
	}
}
