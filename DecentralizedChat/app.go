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
// (Tailscale 相关逻辑已移除，后续需要可再扩展)

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
		cfg.Network.LocalIP = "127.0.0.1"
	}
	a.config = cfg

	// 启动最小节点（权限策略后续可细化）
	a.nodeManager = routes.NewNodeManager("dchat-network", a.config.Network.LocalIP)
	nodeID := fmt.Sprintf("dchat-%s", a.config.Network.LocalIP)
	nodeConfig := a.nodeManager.CreateNodeConfigWithPermissions(
		nodeID,
		DefaultClientPort,
		DefaultClusterPort,
		[]string{},
		[]string{"dchat.dm.*.msg", "dchat.grp.*.msg", "_INBOX.*"},
	)
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
	log.Println("DChat application started (minimal mode)")
}

// GetTailscaleStatus returns Tailscale connectivity status
// （房间/历史/尾部统计等功能已移除，只保留最小 Direct/Group 能力）

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

// SetKeyPair sets local user key pair (base64 encoded)
func (a *App) SetKeyPair(privB64, pubB64 string) error {
	if a.chatSvc == nil {
		return fmt.Errorf("chat service not initialized")
	}
	a.chatSvc.SetKeyPair(privB64, pubB64)
	return nil
}

// OnDecrypted registers decrypted message callback
func (a *App) OnDecrypted(h func(*chat.DecryptedMessage)) error {
	if a.chatSvc == nil {
		return fmt.Errorf("chat service not initialized")
	}
	a.chatSvc.OnDecrypted(h)
	return nil
}

// OnError registers error callback
func (a *App) OnError(h func(error)) error {
	if a.chatSvc == nil {
		return fmt.Errorf("chat service not initialized")
	}
	a.chatSvc.OnError(h)
	return nil
}

// GetUser returns current user info
func (a *App) GetUser() (chat.User, error) {
	if a.chatSvc == nil {
		return chat.User{}, fmt.Errorf("chat service not initialized")
	}
	return a.chatSvc.GetUser(), nil
}

// GetNetworkStats aggregates network statistics
// （权限热重启、网络统计、凭据导出接口已移除，保持核心最小 API）
