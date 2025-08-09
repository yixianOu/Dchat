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

// TailscaleStatus returned to frontend representing network status
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

	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Printf("Failed to load config: %v", err)
		return
	}
	a.config = cfg

	// Initialize Tailscale manager
	a.tailscale = network.NewTailscaleManager()

	// Obtain local Tailscale IP
	localIP, err := a.tailscale.GetLocalIP()
	if err != nil {
		log.Printf("Warning: Failed to get Tailscale IP, using localhost: %v", err)
		localIP = "127.0.0.1"
	}

	// Update network info in config
	a.config.Network.LocalIP = localIP
	a.config.EnableRoutes(localIP, 4222, 6222, []string{})

	// Initialize node manager
	a.nodeManager = routes.NewNodeManager("dchat-network", localIP)

	// Start local NATS node
	nodeID := fmt.Sprintf("dchat-%s", localIP)
	clientPort := 4222
	clusterPort := 6222
	var seedRoutes []string // TODO: discover other nodes via Tailscale network

	// Default allowed subscribe subjects
	defaultSubscribePermissions := []string{
		"chat.*",   // chat rooms
		"_INBOX.*", // inbox responses
		"system.*", // system topics
	}

	// Create node config with permissions
	nodeConfig := a.nodeManager.CreateNodeConfigWithPermissions(
		nodeID, clientPort, clusterPort, seedRoutes, defaultSubscribePermissions)

	err = a.nodeManager.StartLocalNodeWithConfig(nodeConfig)
	if err != nil {
		log.Printf("Failed to start local NATS node: %v", err)
		return
	}

	// Get auth credentials (unused with creds/JWT model currently)
	username, password := a.nodeManager.GetNodeCredentials()

	// Initialize NATS client service (legacy user/pass path)
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

	// Initialize chat service
	if a.natsSvc != nil {
		a.chatSvc = chat.NewService(a.natsSvc)
	}

	// Persist configuration
	if err := config.SaveConfig(a.config); err != nil {
		log.Printf("Warning: Failed to save config: %v", err)
	}

	log.Println("DChat application started successfully")
}

// GetTailscaleStatus returns Tailscale connectivity status
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

// GetConnectedRooms returns list of joined chat rooms
func (a *App) GetConnectedRooms() []string {
	if a.chatSvc == nil {
		return []string{"general"} // default room
	}

	rooms := a.chatSvc.GetRooms()
	if len(rooms) == 0 {
		return []string{"general"}
	}

	return rooms
}

// JoinChatRoom joins a chat room
func (a *App) JoinChatRoom(roomName string) error {
	if a.chatSvc == nil {
		return fmt.Errorf("chat service not initialized")
	}

	// Room subject permissions enforced at server start (chat.*)
	return a.chatSvc.JoinRoom(roomName)
} // SendMessage sends a message
func (a *App) SendMessage(roomName, message string) error {
	if a.chatSvc == nil {
		return fmt.Errorf("chat service not initialized")
	}

	return a.chatSvc.SendMessage(roomName, message)
}

// GetChatHistory returns history of a room
func (a *App) GetChatHistory(roomName string) ([]*chat.Message, error) {
	if a.chatSvc == nil {
		return nil, fmt.Errorf("chat service not initialized")
	}

	return a.chatSvc.GetHistory(roomName)
}

// SetUserInfo sets current user metadata
func (a *App) SetUserInfo(nickname, avatar string) error {
	if a.chatSvc == nil {
		return fmt.Errorf("chat service not initialized")
	}

	a.chatSvc.SetUser(nickname, avatar)
	return nil
}

// GetNetworkStats aggregates network statistics
func (a *App) GetNetworkStats() map[string]interface{} {
	stats := make(map[string]interface{})

	// Tailscale status
	if a.tailscale != nil {
		tailscaleStatus, _ := a.tailscale.GetStatus()
		stats["tailscale"] = map[string]interface{}{
			"connected": tailscaleStatus.Connected,
			"ip":        tailscaleStatus.IP,
			"peers":     len(tailscaleStatus.Peers),
		}
	}

	// NATS node status
	if a.nodeManager != nil {
		stats["nats_node"] = a.nodeManager.GetClusterInfo()
	}

	// NATS client status
	if a.natsSvc != nil {
		stats["nats_client"] = a.natsSvc.GetStats()
	}

	return stats
}

// GetNodeCredentials returns client credentials
func (a *App) GetNodeCredentials() (string, string) {
	if a.nodeManager != nil {
		return a.nodeManager.GetNodeCredentials()
	}
	return "", ""
}

// RestartNodeWithPermissions restarts node applying new subscribe permissions
func (a *App) RestartNodeWithPermissions(subscribePermissions []string) error {
	if a.nodeManager == nil {
		return fmt.Errorf("node manager not initialized")
	}

	// Stop current node
	if err := a.nodeManager.StopLocalNode(); err != nil {
		log.Printf("停止节点时出错: %v", err)
	}

	// Close current NATS connection
	if a.natsSvc != nil {
		a.natsSvc.Close()
	}

	// Read current config
	localIP := a.config.Network.LocalIP
	nodeID := fmt.Sprintf("dchat-%s", localIP)

	// Build new permission config
	nodeConfig := a.nodeManager.CreateNodeConfigWithPermissions(
		nodeID, 4222, 6222, []string{}, subscribePermissions)

	// Start node
	if err := a.nodeManager.StartLocalNodeWithConfig(nodeConfig); err != nil {
		return fmt.Errorf("restart node failed: %v", err)
	}

	// Recreate NATS client
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
		return fmt.Errorf("reconnect NATS failed: %v", err)
	}

	// Recreate chat service
	if a.natsSvc != nil {
		a.chatSvc = chat.NewService(a.natsSvc)
	}

	return nil
}
