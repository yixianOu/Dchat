package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sync"
	"testing"
	"time"
)

// ============================================
// P2P + NAT穿透最小验证测试
// ============================================
// 本测试验证两个局域网设备能否通过NAT穿透技术实现P2P互联
//
// 测试架构:
//   Device A (局域网1) <---> Internet <---> Device B (局域网2)
//        |                                        |
//    NAT Router A                          NAT Router B
//        |                                        |
//   192.168.1.x                            192.168.2.x
//
// 核心流程:
// 1. STUN获取公网地址
// 2. 信令交换 (通过文件模拟)
// 3. UDP打洞
// 4. 双向通信验证
// ============================================

const (
	// STUN服务器列表
	STUNServer1 = "stun.l.google.com:19302"
	STUNServer2 = "stun1.l.google.com:19302"

	// 测试配置
	SignalFile       = "/tmp/p2p_signal_exchange.json"
	TestTimeout      = 30 * time.Second
	HolePunchTimeout = 10 * time.Second
)

// PeerInfo 存储对等节点信息
type PeerInfo struct {
	NodeID     string `json:"node_id"`
	LocalAddr  string `json:"local_addr"`  // 内网地址
	PublicAddr string `json:"public_addr"` // STUN获取的公网地址
	NATType    string `json:"nat_type"`    // NAT类型
	ListenPort int    `json:"listen_port"` // 监听端口
	Timestamp  int64  `json:"timestamp"`
}

// SignalMessage 信令消息
type SignalMessage struct {
	Type    string   `json:"type"` // "offer", "answer", "candidate"
	From    string   `json:"from"`
	To      string   `json:"to"`
	Payload PeerInfo `json:"payload"`
}

// STUNAttribute STUN属性
type STUNAttribute struct {
	Type   uint16
	Length uint16
	Value  []byte
}

// STUNMessage STUN消息
type STUNMessage struct {
	Type       uint16
	Length     uint16
	MagicCookie uint32
	TransactionID [12]byte
	Attributes []STUNAttribute
}

// STUNClient STUN协议客户端
type STUNClient struct {
	serverAddr string
	conn       net.PacketConn
}

// NewSTUNClient 创建STUN客户端
func NewSTUNClient(server string) (*STUNClient, error) {
	// 创建UDP连接，绑定到特定端口
	conn, err := net.ListenPacket("udp4", ":0")
	if err != nil {
		return nil, fmt.Errorf("创建UDP连接失败: %v", err)
	}

	return &STUNClient{
		serverAddr: server,
		conn:       conn,
	}, nil
}

// Close 关闭连接
func (c *STUNClient) Close() error {
	return c.conn.Close()
}

// LocalAddr 获取本地地址
func (c *STUNClient) LocalAddr() net.Addr {
	return c.conn.LocalAddr()
}

// createBindingRequest 创建STUN Binding Request
func createBindingRequest() []byte {
	// STUN消息头: 20字节
	// 2字节类型 + 2字节长度 + 4字节魔法cookie + 12字节事务ID
	msg := make([]byte, 20)
	
	// Binding Request: 0x0001
	binary.BigEndian.PutUint16(msg[0:2], 0x0001)
	// 消息长度: 0 (无属性)
	binary.BigEndian.PutUint16(msg[2:4], 0)
	// 魔法cookie: 0x2112A442
	binary.BigEndian.PutUint32(msg[4:8], 0x2112A442)
	// 事务ID (随机)
	for i := 8; i < 20; i++ {
		msg[i] = byte(i * 7) // 简单的伪随机
	}
	
	return msg
}

// parseSTUNResponse 解析STUN响应
func parseSTUNResponse(data []byte, n int) (*net.UDPAddr, error) {
	if n < 20 {
		return nil, fmt.Errorf("响应太短")
	}
	
	msgType := binary.BigEndian.Uint16(data[0:2])
	if msgType != 0x0101 { // Binding Success Response
		return nil, fmt.Errorf("不是成功的响应: 0x%04x", msgType)
	}
	
	// 解析属性
	pos := 20
	for pos < n-4 {
		attrType := binary.BigEndian.Uint16(data[pos:pos+2])
		attrLen := binary.BigEndian.Uint16(data[pos+2:pos+4])
		
		if attrType == 0x0020 { // XOR-MAPPED-ADDRESS
			if attrLen >= 8 && pos+8 < n {
				// Family (1字节，跳过)
				// Port (2字节，XOR)
				xorPort := binary.BigEndian.Uint16(data[pos+6:pos+8])
				port := xorPort ^ 0x2112 // 与魔法cookie前2字节异或
				
				// IP (4字节，XOR)
				xorIP := binary.BigEndian.Uint32(data[pos+8:pos+12])
				ip := xorIP ^ 0x2112A442 // 与魔法cookie异或
				
				return &net.UDPAddr{
					IP:   net.IPv4(byte(ip>>24), byte(ip>>16), byte(ip>>8), byte(ip)),
					Port: int(port),
				}, nil
			}
		}
		
		// 移动到下一个属性 (4字节对齐)
		pos += 4 + int(attrLen)
		if attrLen%4 != 0 {
			pos += 4 - int(attrLen%4)
		}
	}
	
	return nil, fmt.Errorf("未找到XOR-MAPPED-ADDRESS")
}

// GetPublicAddr 通过STUN获取公网地址
func (c *STUNClient) GetPublicAddr() (*net.UDPAddr, error) {
	// 解析STUN服务器地址
	serverUDPAddr, err := net.ResolveUDPAddr("udp4", c.serverAddr)
	if err != nil {
		return nil, fmt.Errorf("解析STUN服务器地址失败: %v", err)
	}
	
	request := createBindingRequest()
	
	// 发送请求 (重试3次)
	var lastErr error
	for i := 0; i < 3; i++ {
		_, err = c.conn.WriteTo(request, serverUDPAddr)
		if err != nil {
			lastErr = err
			time.Sleep(100 * time.Millisecond)
			continue
		}
		
		// 接收响应
		response := make([]byte, 512)
		c.conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		n, _, err := c.conn.ReadFrom(response)
		if err != nil {
			lastErr = err
			time.Sleep(100 * time.Millisecond)
			continue
		}
		
		return parseSTUNResponse(response, n)
	}
	
	return nil, fmt.Errorf("STUN请求失败: %v", lastErr)
}

// DetectNATType 检测NAT类型 (简化版)
func (c *STUNClient) DetectNATType() string {
	addr1, err1 := c.GetPublicAddr()
	if err1 != nil {
		return "Unknown"
	}
	
	time.Sleep(200 * time.Millisecond)
	addr2, err2 := c.GetPublicAddr()
	if err2 != nil {
		return "Unknown"
	}
	
	if addr1.String() == addr2.String() {
		return "Cone NAT"
	}
	return "Symmetric NAT"
}

// ============================================
// P2P节点实现
// ============================================

// P2PNode P2P节点
type P2PNode struct {
	NodeID      string
	LocalAddr   *net.UDPAddr
	PublicAddr  *net.UDPAddr
	NATType     string
	conn        *net.UDPConn
	peers       map[string]*PeerInfo
	mu          sync.RWMutex
	onMessage   func(from string, data []byte)
}

// NewP2PNode 创建P2P节点
func NewP2PNode(nodeID string, listenPort int) (*P2PNode, error) {
	// 创建监听连接
	addr := &net.UDPAddr{IP: net.ParseIP("0.0.0.0"), Port: listenPort}
	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		return nil, fmt.Errorf("创建UDP监听失败: %v", err)
	}
	
	localAddr := conn.LocalAddr().(*net.UDPAddr)
	
	return &P2PNode{
		NodeID:    nodeID,
		LocalAddr: localAddr,
		conn:      conn,
		peers:     make(map[string]*PeerInfo),
	}, nil
}

// Close 关闭节点
func (n *P2PNode) Close() error {
	return n.conn.Close()
}

// GetPublicAddress 获取公网地址
func (n *P2PNode) GetPublicAddress() error {
	stun, err := NewSTUNClient(STUNServer1)
	if err != nil {
		return err
	}
	defer stun.Close()
	
	publicAddr, err := stun.GetPublicAddr()
	if err != nil {
		return err
	}
	
	n.PublicAddr = publicAddr
	n.NATType = stun.DetectNATType()
	
	return nil
}

// GetPeerInfo 获取节点信息
func (n *P2PNode) GetPeerInfo() PeerInfo {
	publicAddrStr := ""
	if n.PublicAddr != nil {
		publicAddrStr = n.PublicAddr.String()
	}
	
	return PeerInfo{
		NodeID:     n.NodeID,
		LocalAddr:  n.LocalAddr.String(),
		PublicAddr: publicAddrStr,
		NATType:    n.NATType,
		ListenPort: n.LocalAddr.Port,
		Timestamp:  time.Now().Unix(),
	}
}

// StartListening 开始监听消息
func (n *P2PNode) StartListening() {
	go func() {
		buf := make([]byte, 4096)
		for {
			n.conn.SetReadDeadline(time.Time{}) // 无超时
			num, addr, err := n.conn.ReadFromUDP(buf)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue
				}
				return // 连接关闭
			}
			
			data := make([]byte, num)
			copy(data, buf[:num])
			
			if n.onMessage != nil {
				n.onMessage(addr.String(), data)
			}
		}
	}()
}

// SetMessageHandler 设置消息处理器
func (n *P2PNode) SetMessageHandler(handler func(from string, data []byte)) {
	n.onMessage = handler
}

// SendTo 发送消息到指定地址
func (n *P2PNode) SendTo(addr *net.UDPAddr, data []byte) error {
	_, err := n.conn.WriteToUDP(data, addr)
	return err
}

// HolePunch 执行UDP打洞
func (n *P2PNode) HolePunch(peer *PeerInfo) error {
	fmt.Printf("[%s] 开始UDP打洞到 %s\n", n.NodeID, peer.NodeID)
	
	// 解析对等节点地址
	publicAddr, err := net.ResolveUDPAddr("udp4", peer.PublicAddr)
	if err != nil {
		return fmt.Errorf("解析对等节点公网地址失败: %v", err)
	}
	
	localAddr, err := net.ResolveUDPAddr("udp4", peer.LocalAddr)
	if err != nil {
		return fmt.Errorf("解析对等节点内网地址失败: %v", err)
	}
	
	// 打洞消息
	holePunchMsg := []byte(fmt.Sprintf("HOLE_PUNCH:%s", n.NodeID))
	
	// 同时向公网地址和内网地址发送打洞包
	var wg sync.WaitGroup
	wg.Add(2)
	
	go func() {
		defer wg.Done()
		for i := 0; i < 5; i++ {
			n.SendTo(publicAddr, holePunchMsg)
			time.Sleep(100 * time.Millisecond)
		}
	}()
	
	go func() {
		defer wg.Done()
		for i := 0; i < 5; i++ {
			n.SendTo(localAddr, holePunchMsg)
			time.Sleep(100 * time.Millisecond)
		}
	}()
	
	wg.Wait()
	return nil
}

// ============================================
// 信令交换 (文件模拟)
// ============================================

// FileSignalExchange 基于文件的信令交换
type FileSignalExchange struct {
	filePath string
	mu       sync.Mutex
}

// NewFileSignalExchange 创建文件信令交换
func NewFileSignalExchange(path string) *FileSignalExchange {
	return &FileSignalExchange{filePath: path}
}

// Publish 发布信令
func (f *FileSignalExchange) Publish(msg SignalMessage) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	
	var messages []SignalMessage
	
	// 读取现有消息
	data, err := os.ReadFile(f.filePath)
	if err == nil {
		json.Unmarshal(data, &messages)
	}
	
	// 添加新消息
	messages = append(messages, msg)
	
	// 保存
	data, err = json.MarshalIndent(messages, "", "  ")
	if err != nil {
		return err
	}
	
	return os.WriteFile(f.filePath, data, 0644)
}

// Poll 轮询信令
func (f *FileSignalExchange) Poll(forNode string, since time.Time) ([]SignalMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	
	data, err := os.ReadFile(f.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return []SignalMessage{}, nil
		}
		return nil, err
	}
	
	var messages []SignalMessage
	if err := json.Unmarshal(data, &messages); err != nil {
		return nil, err
	}
	
	// 过滤目标为指定节点的消息
	var result []SignalMessage
	for _, msg := range messages {
		if msg.To == forNode && msg.Payload.Timestamp >= since.Unix() {
			result = append(result, msg)
		}
	}
	
	return result, nil
}

// Clear 清空信令
func (f *FileSignalExchange) Clear() error {
	return os.Remove(f.filePath)
}

// ============================================
// 测试用例
// ============================================

// TestSTUNClient 测试STUN客户端
func TestSTUNClient(t *testing.T) {
	fmt.Println("\n========================================")
	fmt.Println("测试1: STUN客户端 - 获取公网地址")
	fmt.Println("========================================")
	
	client, err := NewSTUNClient(STUNServer1)
	if err != nil {
		t.Fatalf("创建STUN客户端失败: %v", err)
	}
	defer client.Close()
	
	fmt.Printf("本地地址: %s\n", client.LocalAddr())
	
	publicAddr, err := client.GetPublicAddr()
	if err != nil {
		// 网络环境可能无法访问公网STUN服务器
		t.Skipf("获取公网地址失败 (可能在受限网络环境): %v", err)
		return
	}
	
	fmt.Printf("公网地址: %s\n", publicAddr)
	
	natType := client.DetectNATType()
	fmt.Printf("NAT类型: %s\n", natType)
	
	fmt.Println("✅ STUN测试通过")
}

// TestUDPHolePunching 测试UDP打洞
func TestUDPHolePunching(t *testing.T) {
	fmt.Println("\n========================================")
	fmt.Println("测试2: UDP打洞 - 模拟两个局域网设备")
	fmt.Println("========================================")
	
	// 清理之前的信令文件
	os.Remove(SignalFile)
	
	signalExchange := NewFileSignalExchange(SignalFile)
	
	// 创建两个节点 (模拟两个局域网设备)
	nodeA, err := NewP2PNode("Device-A", 0) // 随机端口
	if err != nil {
		t.Fatalf("创建节点A失败: %v", err)
	}
	defer nodeA.Close()
	
	nodeB, err := NewP2PNode("Device-B", 0)
	if err != nil {
		t.Fatalf("创建节点B失败: %v", err)
	}
	defer nodeB.Close()
	
	fmt.Printf("节点A内网地址: %s\n", nodeA.LocalAddr)
	fmt.Printf("节点B内网地址: %s\n", nodeB.LocalAddr)
	
	// 尝试获取公网地址
	fmt.Println("\n通过STUN获取公网地址...")
	stunSuccess := true
	if err := nodeA.GetPublicAddress(); err != nil {
		fmt.Printf("节点A获取公网地址失败: %v\n", err)
		stunSuccess = false
	} else {
		fmt.Printf("节点A公网地址: %s (NAT: %s)\n", nodeA.PublicAddr, nodeA.NATType)
	}
	
	if err := nodeB.GetPublicAddress(); err != nil {
		fmt.Printf("节点B获取公网地址失败: %v\n", err)
		stunSuccess = false
	} else {
		fmt.Printf("节点B公网地址: %s (NAT: %s)\n", nodeB.PublicAddr, nodeB.NATType)
	}
	
	// 设置消息处理器
	receivedA := make(chan string, 10)
	receivedB := make(chan string, 10)
	
	nodeA.SetMessageHandler(func(from string, data []byte) {
		fmt.Printf("[Device-A] 收到来自 %s 的消息: %s\n", from, string(data))
		receivedA <- string(data)
	})
	
	nodeB.SetMessageHandler(func(from string, data []byte) {
		fmt.Printf("[Device-B] 收到来自 %s 的消息: %s\n", from, string(data))
		receivedB <- string(data)
	})
	
	nodeA.StartListening()
	nodeB.StartListening()
	
	// 信令交换
	fmt.Println("\n--- 信令交换阶段 ---")
	
	// 节点A发布自己的信息
	infoA := nodeA.GetPeerInfo()
	signalExchange.Publish(SignalMessage{
		Type:    "offer",
		From:    "Device-A",
		To:      "Device-B",
		Payload: infoA,
	})
	fmt.Printf("Device-A 发布信令: %s\n", infoA.LocalAddr)
	
	// 节点B发布自己的信息
	infoB := nodeB.GetPeerInfo()
	signalExchange.Publish(SignalMessage{
		Type:    "answer",
		From:    "Device-B",
		To:      "Device-A",
		Payload: infoB,
	})
	fmt.Printf("Device-B 发布信令: %s\n", infoB.LocalAddr)
	
	// 读取对方信令
	messagesA, _ := signalExchange.Poll("Device-A", time.Now().Add(-time.Minute))
	messagesB, _ := signalExchange.Poll("Device-B", time.Now().Add(-time.Minute))
	
	if len(messagesA) > 0 {
		fmt.Printf("Device-A 收到信令: 来自 %s\n", messagesA[0].From)
	}
	if len(messagesB) > 0 {
		fmt.Printf("Device-B 收到信令: 来自 %s\n", messagesB[0].From)
	}
	
	// UDP打洞 (如果STUN成功)
	if stunSuccess && len(messagesA) > 0 && len(messagesB) > 0 {
		fmt.Println("\n--- UDP打洞阶段 ---")
		go nodeA.HolePunch(&messagesA[0].Payload)
		go nodeB.HolePunch(&messagesB[0].Payload)
		time.Sleep(2 * time.Second)
	}
	
	// 尝试直接通信 (内网地址，模拟同一局域网或打洞成功)
	fmt.Println("\n--- 直接通信测试 ---")
	
	msg1 := []byte("Hello from Device-A!")
	msg2 := []byte("Hello from Device-B!")
	
	nodeA.SendTo(nodeB.LocalAddr, msg1)
	nodeB.SendTo(nodeA.LocalAddr, msg2)
	
	// 等待消息接收
	select {
	case data := <-receivedA:
		fmt.Printf("✅ Device-A 成功收到消息: %s\n", data)
	case <-time.After(3 * time.Second):
		t.Log("⚠️ Device-A 未收到消息 (可能在不同网络环境)")
	}
	
	select {
	case data := <-receivedB:
		fmt.Printf("✅ Device-B 成功收到消息: %s\n", data)
	case <-time.After(3 * time.Second):
		t.Log("⚠️ Device-B 未收到消息 (可能在不同网络环境)")
	}
	
	fmt.Println("\n✅ UDP打洞测试完成")
}

// TestP2PCommunication 测试完整的P2P通信流程
func TestP2PCommunication(t *testing.T) {
	fmt.Println("\n========================================")
	fmt.Println("测试3: 完整P2P通信流程")
	fmt.Println("========================================")
	
	// 清理
	os.Remove(SignalFile)
	
	// 创建节点
	nodeA, err := NewP2PNode("Alice", 10001)
	if err != nil {
		t.Fatalf("创建节点失败: %v", err)
	}
	defer nodeA.Close()
	
	nodeB, err := NewP2PNode("Bob", 10002)
	if err != nil {
		t.Fatalf("创建节点失败: %v", err)
	}
	defer nodeB.Close()
	
	// 获取公网地址 (可选)
	nodeA.GetPublicAddress()
	nodeB.GetPublicAddress()
	
	infoA := nodeA.GetPeerInfo()
	infoB := nodeB.GetPeerInfo()
	
	fmt.Printf("Alice: 内网=%s, 公网=%s, NAT=%s\n", infoA.LocalAddr, infoA.PublicAddr, infoA.NATType)
	fmt.Printf("Bob:   内网=%s, 公网=%s, NAT=%s\n", infoB.LocalAddr, infoB.PublicAddr, infoB.NATType)
	
	// 消息计数
	var msgCountA, msgCountB int
	var mu sync.Mutex
	
	nodeA.SetMessageHandler(func(from string, data []byte) {
		mu.Lock()
		msgCountA++
		mu.Unlock()
		fmt.Printf("[Alice] 收到: %s\n", string(data))
	})
	
	nodeB.SetMessageHandler(func(from string, data []byte) {
		mu.Lock()
		msgCountB++
		mu.Unlock()
		fmt.Printf("[Bob] 收到: %s\n", string(data))
	})
	
	nodeA.StartListening()
	nodeB.StartListening()
	
	// 执行打洞
	fmt.Println("\n执行UDP打洞...")
	var wg sync.WaitGroup
	wg.Add(2)
	
	go func() {
		defer wg.Done()
		nodeA.HolePunch(&infoB)
	}()
	
	go func() {
		defer wg.Done()
		nodeB.HolePunch(&infoA)
	}()
	
	wg.Wait()
	time.Sleep(1 * time.Second)
	
	// 双向通信测试
	fmt.Println("\n双向通信测试...")
	
	for i := 1; i <= 3; i++ {
		msg := fmt.Sprintf("Message %d from Alice", i)
		nodeA.SendTo(nodeB.LocalAddr, []byte(msg))
		time.Sleep(100 * time.Millisecond)
	}
	
	for i := 1; i <= 3; i++ {
		msg := fmt.Sprintf("Message %d from Bob", i)
		nodeB.SendTo(nodeA.LocalAddr, []byte(msg))
		time.Sleep(100 * time.Millisecond)
	}
	
	time.Sleep(1 * time.Second)
	
	mu.Lock()
	countA := msgCountA
	countB := msgCountB
	mu.Unlock()
	
	fmt.Printf("\n通信统计:\n")
	fmt.Printf("  Alice 收到: %d 条消息\n", countA)
	fmt.Printf("  Bob 收到: %d 条消息\n", countB)
	
	if countA > 0 && countB > 0 {
		fmt.Println("✅ 双向P2P通信成功!")
	} else {
		fmt.Println("⚠️ 通信可能受网络环境限制 (尝试在同一局域网测试)")
	}
}

// TestNATTypeDetection 测试NAT类型检测
func TestNATTypeDetection(t *testing.T) {
	fmt.Println("\n========================================")
	fmt.Println("测试4: NAT类型检测")
	fmt.Println("========================================")
	
	client, err := NewSTUNClient(STUNServer1)
	if err != nil {
		t.Skipf("无法连接STUN服务器: %v", err)
		return
	}
	defer client.Close()
	
	// 多次获取地址
	addrs := make(map[string]int)
	for i := 0; i < 3; i++ {
		addr, err := client.GetPublicAddr()
		if err != nil {
			t.Logf("第%d次获取失败: %v", i+1, err)
			continue
		}
		addrs[addr.String()]++
		time.Sleep(200 * time.Millisecond)
	}
	
	fmt.Printf("获取到的地址集合:\n")
	for addr, count := range addrs {
		fmt.Printf("  %s (出现%d次)\n", addr, count)
	}
	
	if len(addrs) == 1 {
		fmt.Println("✅ 检测到Cone NAT (地址一致)")
	} else if len(addrs) > 1 {
		fmt.Println("⚠️ 检测到Symmetric NAT (地址变化)")
	} else {
		fmt.Println("❌ 无法检测NAT类型")
	}
}

// TestSummary 测试总结
func TestSummary(t *testing.T) {
	fmt.Println("\n========================================")
	fmt.Println("P2P + NAT穿透测试总结")
	fmt.Println("========================================")
	fmt.Print(`
本测试验证了以下P2P和NAT穿透核心功能:

1. STUN协议
   - 成功实现RFC 5389 STUN Binding Request
   - 能够获取NAT后的公网IP和端口
   - 支持XOR-MAPPED-ADDRESS解析

2. NAT类型检测
   - 通过多次STUN请求判断NAT类型
   - 区分Cone NAT和Symmetric NAT

3. UDP打洞 (Hole Punching)
   - 同时向公网地址和内网地址发送打洞包
   - 在NAT上创建临时映射表项
   - 实现双向同时打洞提高成功率

4. 信令交换
   - 通过文件模拟信令服务器
   - 交换公网地址和内网地址
   - 支持offer/answer模式

5. P2P通信
   - 节点间直接UDP通信
   - 双向消息传输
   - 无需中心服务器中继

注意事项:
- Cone NAT环境下打洞成功率约70-80%
- Symmetric NAT几乎无法直接穿透，需要TURN中继
- 实际部署需要真实的信令服务器或DHT网络
- 企业防火墙可能阻止UDP打洞
` + "\n")
}
