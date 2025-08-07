package main

import (
	"context"
	"fmt"
	"log"

	"DecentralizedChat/internal/chat"
	"DecentralizedChat/internal/config"
	"DecentralizedChat/internal/nats"
	"DecentralizedChat/internal/network"
	"DecentralizedChat/internal/routes"
)

// App struct
type App struct {
	ctx         context.Context
	chatSvc     *chat.Service
	natsSvc     *nats.Service
	tailscale   *network.TailscaleManager
	nodeManager *routes.NodeManager
	config      *config.Config
}

// TailscaleStatus 返回给前端的网络状态
type TailscaleStatus struct {
	Connected bool   `json:"connected"`
	IP        string `json:"ip"`
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{}
}

// OnStartup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) OnStartup(ctx context.Context) {
	a.ctx = ctx

	// 加载配置
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Printf("Failed to load config: %v", err)
		return
	}
	a.config = cfg

	// 初始化Tailscale管理器
	a.tailscale = network.NewTailscaleManager()

	// 获取本机Tailscale IP
	localIP, err := a.tailscale.GetLocalIP()
	if err != nil {
		log.Printf("Warning: Failed to get Tailscale IP, using localhost: %v", err)
		localIP = "127.0.0.1"
	}

	// 更新配置中的网络信息
	a.config.Network.LocalIP = localIP
	a.config.EnableRoutes(localIP, 4222, 6222, []string{})

	// 初始化节点管理器
	a.nodeManager = routes.NewNodeManager("dchat-network", localIP)

	// 启动本地NATS节点
	nodeID := fmt.Sprintf("dchat-%s", localIP)
	clientPort := 4222
	clusterPort := 6222
	var seedRoutes []string // TODO: 从Tailscale网络发现其他节点

	// 默认允许的订阅主题
	defaultSubscribePermissions := []string{
		"chat.*",   // 聊天室主题
		"_INBOX.*", // 响应主题
		"system.*", // 系统主题
	}

	// 创建带权限配置的节点
	nodeConfig := a.nodeManager.CreateNodeConfigWithPermissions(
		nodeID, clientPort, clusterPort, seedRoutes, defaultSubscribePermissions)

	err = a.nodeManager.StartLocalNodeWithConfig(nodeConfig)
	if err != nil {
		log.Printf("Failed to start local NATS node: %v", err)
		return
	}

	// 获取连接凭据
	username, password := a.nodeManager.GetNodeCredentials()

	// 初始化NATS客户端服务（使用服务器配置的用户凭据）
	natsConfig := nats.ClientConfig{
		URL:      a.nodeManager.GetClientURL(),
		User:     username,
		Password: password,
		Name:     "DChatClient",
	}

	a.natsSvc, err = nats.NewService(natsConfig)
	if err != nil {
		log.Printf("Warning: Failed to start NATS service: %v", err)
	}

	// 初始化聊天服务
	if a.natsSvc != nil {
		a.chatSvc = chat.NewService(a.natsSvc)
	}

	// 保存配置
	if err := config.SaveConfig(a.config); err != nil {
		log.Printf("Warning: Failed to save config: %v", err)
	}

	log.Println("DChat application started successfully")
}

// GetTailscaleStatus 获取Tailscale连接状态
func (a *App) GetTailscaleStatus() TailscaleStatus {
	if a.tailscale == nil {
		return TailscaleStatus{Connected: false, IP: ""}
	}

	status, err := a.tailscale.GetStatus()
	if err != nil {
		return TailscaleStatus{Connected: false, IP: ""}
	}

	return TailscaleStatus{
		Connected: status.Connected,
		IP:        status.IP,
	}
}

// GetConnectedRooms 获取已连接的聊天室列表
func (a *App) GetConnectedRooms() []string {
	if a.chatSvc == nil {
		return []string{"general"} // 默认聊天室
	}

	rooms := a.chatSvc.GetRooms()
	if len(rooms) == 0 {
		return []string{"general"}
	}

	return rooms
}

// JoinChatRoom 加入聊天室
func (a *App) JoinChatRoom(roomName string) error {
	if a.chatSvc == nil {
		return fmt.Errorf("chat service not initialized")
	}

	// 聊天室主题权限已在服务器启动时配置（chat.*）
	return a.chatSvc.JoinRoom(roomName)
} // SendMessage 发送消息
func (a *App) SendMessage(roomName, message string) error {
	if a.chatSvc == nil {
		return fmt.Errorf("chat service not initialized")
	}

	return a.chatSvc.SendMessage(roomName, message)
}

// GetChatHistory 获取聊天历史
func (a *App) GetChatHistory(roomName string) ([]*chat.Message, error) {
	if a.chatSvc == nil {
		return nil, fmt.Errorf("chat service not initialized")
	}

	return a.chatSvc.GetHistory(roomName)
}

// SetUserInfo 设置用户信息
func (a *App) SetUserInfo(nickname, avatar string) error {
	if a.chatSvc == nil {
		return fmt.Errorf("chat service not initialized")
	}

	a.chatSvc.SetUser(nickname, avatar)
	return nil
}

// GetNetworkStats 获取网络统计信息
func (a *App) GetNetworkStats() map[string]interface{} {
	stats := make(map[string]interface{})

	// Tailscale状态
	if a.tailscale != nil {
		tailscaleStatus, _ := a.tailscale.GetStatus()
		stats["tailscale"] = map[string]interface{}{
			"connected": tailscaleStatus.Connected,
			"ip":        tailscaleStatus.IP,
			"peers":     len(tailscaleStatus.Peers),
		}
	}

	// NATS节点状态
	if a.nodeManager != nil {
		stats["nats_node"] = a.nodeManager.GetClusterInfo()
	}

	// NATS客户端状态
	if a.natsSvc != nil {
		stats["nats_client"] = a.natsSvc.GetStats()
	}

	return stats
}

// GetNodeCredentials 获取NATS连接凭据
func (a *App) GetNodeCredentials() (string, string) {
	if a.nodeManager != nil {
		return a.nodeManager.GetNodeCredentials()
	}
	return "", ""
}

// RestartNodeWithPermissions 重启节点并应用新权限
func (a *App) RestartNodeWithPermissions(subscribePermissions []string) error {
	if a.nodeManager == nil {
		return fmt.Errorf("节点管理器未初始化")
	}

	// 停止当前节点
	if err := a.nodeManager.StopLocalNode(); err != nil {
		log.Printf("停止节点时出错: %v", err)
	}

	// 关闭当前NATS连接
	if a.natsSvc != nil {
		a.natsSvc.Close()
	}

	// 获取当前配置信息
	localIP := a.config.Network.LocalIP
	nodeID := fmt.Sprintf("dchat-%s", localIP)

	// 创建新的权限配置
	nodeConfig := a.nodeManager.CreateNodeConfigWithPermissions(
		nodeID, 4222, 6222, []string{}, subscribePermissions)

	// 启动节点
	if err := a.nodeManager.StartLocalNodeWithConfig(nodeConfig); err != nil {
		return fmt.Errorf("重启节点失败: %v", err)
	}

	// 重新初始化NATS客户端
	username, password := a.nodeManager.GetNodeCredentials()
	natsConfig := nats.ClientConfig{
		URL:      a.nodeManager.GetClientURL(),
		User:     username,
		Password: password,
		Name:     "DChatClient",
	}

	var err error
	a.natsSvc, err = nats.NewService(natsConfig)
	if err != nil {
		return fmt.Errorf("重新连接NATS失败: %v", err)
	}

	// 重新初始化聊天服务
	if a.natsSvc != nil {
		a.chatSvc = chat.NewService(a.natsSvc)
	}

	return nil
}
