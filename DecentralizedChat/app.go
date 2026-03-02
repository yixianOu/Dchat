package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"DecentralizedChat/internal/chat"
	"DecentralizedChat/internal/config"
	"DecentralizedChat/internal/nats"
	"DecentralizedChat/internal/nscsetup"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App struct
type App struct {
	ctx     context.Context
	chatSvc *chat.Service
	natsSvc *nats.Service
	config  *config.Config
	mu      sync.RWMutex
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{}
}

// OnStartup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) OnStartup(ctx context.Context) {
	a.ctx = ctx
	// 1. 加载配置
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Printf("load config failed: %v", err)
		cfg = &config.Config{}
	}
	if cfg.LeafNode.LocalHost == "" {
		cfg.LeafNode.LocalHost = "127.0.0.1"
	}

	// 简化版NATS初始化：无需NSC CLI，直接使用Go库
	if err := nscsetup.EnsureSimpleSetup(cfg); err != nil {
		log.Printf("初始化简化NATS设置失败: %v", err)
		return
	}
	a.config = cfg

	// TODO: Phase 3-5: 初始化并启动 LeafNode Manager
	// 目前暂时直接连接本地 NATS（需要手动启动）
	a.natsSvc, err = nats.NewService(nats.ClientConfig{
		URL:       fmt.Sprintf("nats://%s:%d", cfg.LeafNode.LocalHost, cfg.LeafNode.LocalPort),
		Name:      "DChatClient",
		CredsFile: a.config.Keys.UserCredsPath,
	})
	if err != nil {
		log.Printf("start nats client failed: %v", err)
		return
	}
	a.chatSvc = chat.NewService(a.natsSvc)

	// 设置默认的消息处理器，将解密后的消息推送给前端
	a.chatSvc.OnDecrypted(func(msg *chat.DecryptedMessage) {
		runtime.EventsEmit(a.ctx, "message:decrypted", msg)
	})

	// 设置默认的错误处理器，将错误推送给前端
	a.chatSvc.OnError(func(err error) {
		runtime.EventsEmit(a.ctx, "message:error", map[string]interface{}{
			"error":     err.Error(),
			"timestamp": fmt.Sprintf("%d", time.Now().Unix()),
		})
	})

	// 自动加载NSC密钥用于聊天加密
	if a.config.Keys.UserSeedPath != "" {
		seed, err := a.getNSCUserSeed()
		if err != nil {
			log.Printf("failed to load NSC seed: %v", err)
		} else {
			if err := a.chatSvc.LoadNSCKeys(seed); err != nil {
				log.Printf("failed to load NSC chat keys: %v", err)
			} else {
				log.Println("NSC chat keys loaded successfully")
			}
		}
	}

	if err := config.SaveConfig(a.config); err != nil {
		log.Printf("save config warn: %v", err)
	}
	log.Println("DChat application started (LeafNode mode - placeholder)")
}

// OnShutdown is called when the app stops
func (a *App) OnShutdown(ctx context.Context) {
	// TODO: Phase 5: Stop LeafNode Manager
	if a.natsSvc != nil {
		a.natsSvc.Close()
	}
}

// SetUserInfo sets current user metadata
func (a *App) SetUserInfo(nickname string) error {
	if a.chatSvc == nil {
		return fmt.Errorf("chat service not initialized")
	}

	a.chatSvc.SetUser(nickname)
	return nil
}

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

// GetUser returns current user info
func (a *App) GetUser() (chat.User, error) {
	if a.chatSvc == nil {
		return chat.User{}, fmt.Errorf("chat service not initialized")
	}
	return a.chatSvc.GetUser(), nil
}

// GetConversationID returns the conversation ID for a direct chat with peerID
func (a *App) GetConversationID(peerID string) (string, error) {
	if a.chatSvc == nil {
		return "", fmt.Errorf("chat service not initialized")
	}
	return a.chatSvc.GetConversationID(peerID), nil
}

// GetNetworkStatus returns current network and cluster status
func (a *App) GetNetworkStatus() (map[string]interface{}, error) {
	if a.natsSvc == nil {
		return nil, fmt.Errorf("services not initialized")
	}

	result := make(map[string]interface{})

	// NATS客户端状态
	result["nats"] = a.natsSvc.GetStats()

	// TODO: Phase 5: Add LeafNode status
	result["leafnode"] = map[string]interface{}{
		"status": "placeholder",
		"note":   "LeafNode manager not implemented yet",
	}

	// 配置信息
	if a.config != nil {
		result["config"] = map[string]interface{}{
			"local_host": a.config.LeafNode.LocalHost,
			"local_port": a.config.LeafNode.LocalPort,
			"hub_urls":   a.config.LeafNode.HubURLs,
		}
	}

	return result, nil
}

// LoadNSCKeys 从NSC seed加载聊天密钥对
func (a *App) LoadNSCKeys(nscSeed string) error {
	if a.chatSvc == nil {
		return fmt.Errorf("chat service not initialized")
	}
	return a.chatSvc.LoadNSCKeys(nscSeed)
}

// AddFriendNSCKey 通过NSC公钥添加好友
func (a *App) AddFriendNSCKey(uid, nscPubKey string) error {
	if a.chatSvc == nil {
		return fmt.Errorf("chat service not initialized")
	}
	return a.chatSvc.AddFriendNSCKey(uid, nscPubKey)
}

// getNSCUserSeed 获取当前用户的NSC seed (从配置中读取)
func (a *App) getNSCUserSeed() (string, error) {
	if a.config == nil {
		return "", fmt.Errorf("config not loaded")
	}

	// 从NSC用户seed文件读取
	if a.config.Keys.UserSeedPath != "" {
		// 读取seed文件内容
		data, err := os.ReadFile(a.config.Keys.UserSeedPath)
		if err != nil {
			return "", fmt.Errorf("read NSC seed file: %w", err)
		}

		seed := strings.TrimSpace(string(data))
		if seed == "" {
			return "", fmt.Errorf("NSC seed file is empty")
		}

		// 验证seed格式
		if !strings.HasPrefix(seed, "SU") {
			return "", fmt.Errorf("invalid NSC seed format in file")
		}

		return seed, nil
	}

	return "", fmt.Errorf("NSC user seed path not configured")
}
