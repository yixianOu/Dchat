package main

import (
	"context"
	"fmt"
	"log"

	"DecentralizedChat/internal/chat"
	"DecentralizedChat/internal/nats"
	"DecentralizedChat/internal/network"
)

// App struct
type App struct {
	ctx       context.Context
	chatSvc   *chat.Service
	natsSvc   *nats.Service
	tailscale *network.TailscaleManager
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

	// 初始化Tailscale管理器
	a.tailscale = network.NewTailscaleManager()

	// 获取本机Tailscale IP
	localIP, err := a.tailscale.GetLocalIP()
	if err != nil {
		log.Printf("Warning: Failed to get Tailscale IP: %v", err)
		panic("")
	}

	// 初始化NATS服务
	natsConfig := nats.ClientConfig{
		URL:  fmt.Sprintf("nats://%s:4222", localIP),
		Name: "DChatClient",
	}

	a.natsSvc, err = nats.NewService(natsConfig)
	if err != nil {
		log.Printf("Warning: Failed to start NATS service: %v", err)
	}

	// 初始化聊天服务
	if a.natsSvc != nil {
		a.chatSvc = chat.NewService(a.natsSvc)
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

	return a.chatSvc.JoinRoom(roomName)
}

// SendMessage 发送消息
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

	// NATS状态
	if a.natsSvc != nil {
		stats["nats"] = a.natsSvc.GetStats()
	}

	return stats
}
