package main

import (
	"context"
	"fmt"
	"log"
	"sync"

	"DecentralizedChat/internal/chat"
	"DecentralizedChat/internal/config"
	"DecentralizedChat/internal/nats"
	"DecentralizedChat/internal/routes"
)

// App struct
type App struct {
	ctx         context.Context
	chatSvc     *chat.Service
	natsSvc     *nats.Service
	nodeManager *routes.NodeManager
	config      *config.Config
	mu          sync.RWMutex
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
const (
	DefaultClientPort  = 4222
	DefaultClusterPort = 6222
)

func (a *App) OnStartup(ctx context.Context) {
	a.ctx = ctx
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Printf("load config failed: %v", err)
		cfg = &config.Config{}
	}
	if cfg.Network.LocalIP == "" {
		cfg.Network.LocalIP = "127.0.0.1" // fallback without tailscale
	}
	a.config = cfg

	// setup routes/node
	a.nodeManager = routes.NewNodeManager("dchat-network", a.config.Network.LocalIP)
	nodeID := fmt.Sprintf("dchat-%s", a.config.Network.LocalIP)
	defaultSubscribePermissions := []string{"chat.*", "_INBOX.*", "system.*"}
	nodeConfig := a.nodeManager.CreateNodeConfigWithPermissions(
		nodeID, DefaultClientPort, DefaultClusterPort, []string{}, defaultSubscribePermissions)
	if err := a.nodeManager.StartLocalNodeWithConfig(nodeConfig); err != nil {
		log.Printf("start node failed: %v", err)
		return
	}
	a.natsSvc, err = nats.NewService(nats.ClientConfig{URL: a.nodeManager.GetClientURL(), Name: "DChatClient"})
	if err != nil {
		log.Printf("start nats client failed: %v", err)
		return
	}
	a.chatSvc = chat.NewService(a.natsSvc)
	if err := config.SaveConfig(a.config); err != nil {
		log.Printf("save config warn: %v", err)
	}
	log.Println("DChat application started")
}

// GetTailscaleStatus returns Tailscale connectivity status
func (a *App) GetTailscaleStatus() TailscaleStatus {
	// tailscale removed in this refactor (network abstraction future extension)
	return TailscaleStatus{Connected: false, IP: a.config.Network.LocalIP}
}

// GetConnectedRooms returns list of joined chat rooms
// Deprecated room API placeholders (removed room concept). Return empty slice.
func (a *App) GetConnectedRooms() []string { return []string{} }

// JoinChatRoom joins a chat room
func (a *App) JoinChatRoom(_ string) error { return fmt.Errorf("room feature removed") }

// LeaveChatRoom leaves a room (unsubscribe & purge local state)
func (a *App) LeaveChatRoom(_ string) error { return fmt.Errorf("room feature removed") }

// SendMessage sends a message
func (a *App) SendMessage(_, _ string) error {
	return fmt.Errorf("room feature removed; use SendDirect/SendGroup")
}

// GetChatHistory returns history of a room
func (a *App) GetChatHistory(_ string) ([]interface{}, error) {
	return nil, fmt.Errorf("room feature removed")
}

// SetUserInfo sets current user metadata
func (a *App) SetUserInfo(nickname string) error {
	if a.chatSvc == nil {
		return fmt.Errorf("chat service not initialized")
	}

	a.chatSvc.SetUser(nickname)
	return nil
}

// Direct / Group facade wrappers
func (a *App) AddFriendKey(uid, pubB64 string) error {
	if a.chatSvc == nil {
		return fmt.Errorf("chat service not initialized")
	}
	a.chatSvc.AddFriendKey(uid, pubB64)
	return nil
}

func (a *App) AddGroupKey(gid, symB64 string) error {
	if a.chatSvc == nil {
		return fmt.Errorf("chat service not initialized")
	}
	a.chatSvc.AddGroupKey(gid, symB64)
	return nil
}

func (a *App) JoinDirect(peerID string) error {
	if a.chatSvc == nil {
		return fmt.Errorf("chat service not initialized")
	}
	return a.chatSvc.JoinDirect(peerID)
}

func (a *App) JoinGroup(gid string) error {
	if a.chatSvc == nil {
		return fmt.Errorf("chat service not initialized")
	}
	return a.chatSvc.JoinGroup(gid)
}

func (a *App) SendDirect(peerID, content string) error {
	if a.chatSvc == nil {
		return fmt.Errorf("chat service not initialized")
	}
	return a.chatSvc.SendDirect(peerID, content)
}

func (a *App) SendGroup(gid, content string) error {
	if a.chatSvc == nil {
		return fmt.Errorf("chat service not initialized")
	}
	return a.chatSvc.SendGroup(gid, content)
}

// GetNetworkStats aggregates network statistics
func (a *App) GetNetworkStats() map[string]interface{} {
	stats := make(map[string]interface{})
	// tailscale removed; report local IP only
	stats["network"] = map[string]interface{}{"ip": a.config.Network.LocalIP}

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
		nodeID, DefaultClientPort, DefaultClusterPort, []string{}, subscribePermissions)

	// Start node
	if err := a.nodeManager.StartLocalNodeWithConfig(nodeConfig); err != nil {
		return fmt.Errorf("restart node failed: %v", err)
	}

	// Recreate NATS client

	natsConfig := nats.ClientConfig{
		URL:  a.nodeManager.GetClientURL(),
		Name: "DChatClient",
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
