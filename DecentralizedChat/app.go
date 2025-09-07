package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"

	"DecentralizedChat/internal/chat"
	"DecentralizedChat/internal/config"
	"DecentralizedChat/internal/nats"
	"DecentralizedChat/internal/nscsetup"
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
	// 1. 加载配置
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Printf("load config failed: %v", err)
		cfg = &config.Config{}
	}
	if cfg.Network.LocalIP == "" {
		cfg.Network.LocalIP = "localhost"
	}

	// 启用Routes集群，使用自动检测的本地IP
	cfg.EnableRoutes(cfg.Network.LocalIP, DefaultClientPort, DefaultClusterPort, []string{})

	// 首次运行：通过 nsc 初始化 SYS 账户与 resolver.conf，并将路径写入配置
	if err := nscsetup.EnsureSysAccountSetup(cfg); err != nil {
		log.Printf("初始化 NSC/SYS 失败: %v", err)
		return
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
	// 设置resolver配置路径（如果已生成）
	if a.config.Server.ResolverConf != "" {
		nodeConfig.ResolverConfigPath = a.config.Server.ResolverConf
	}
	if err := a.nodeManager.StartLocalNodeWithConfig(nodeConfig); err != nil {
		log.Printf("start node failed: %v", err)
		return
	}
	a.natsSvc, err = nats.NewService(nats.ClientConfig{
		URL:       a.nodeManager.GetClientURL(),
		Name:      "DChatClient",
		CredsFile: a.config.NSC.UserCredsPath, // 使用NSC生成的凭据文件
	})
	if err != nil {
		log.Printf("start nats client failed: %v", err)
		return
	}
	a.chatSvc = chat.NewService(a.natsSvc)

	// ⭐ 自动加载NSC密钥用于聊天加密
	if a.config.NSC.UserSeedPath != "" {
		seed, err := a.GetNSCUserSeed()
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
	log.Println("DChat application started (minimal mode)")
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

// GetConversationID returns the conversation ID for a direct chat with peerID
func (a *App) GetConversationID(peerID string) (string, error) {
	if a.chatSvc == nil {
		return "", fmt.Errorf("chat service not initialized")
	}
	return a.chatSvc.GetConversationID(peerID), nil
}

// GetNetworkStatus returns current network and cluster status
func (a *App) GetNetworkStatus() (map[string]interface{}, error) {
	if a.natsSvc == nil || a.nodeManager == nil {
		return nil, fmt.Errorf("services not initialized")
	}

	result := make(map[string]interface{})

	// NATS客户端状态
	result["nats"] = a.natsSvc.GetStats()

	// 集群节点状态
	result["cluster"] = a.nodeManager.GetClusterInfo()

	// 配置信息
	if a.config != nil {
		result["config"] = map[string]interface{}{
			"local_ip":     a.config.Network.LocalIP,
			"client_port":  a.config.Server.ClientPort,
			"cluster_port": a.config.Server.ClusterPort,
			"cluster_name": a.config.Server.ClusterName,
		}
	}

	return result, nil
}

// LoadNSCKeys 从NSC seed加载聊天密钥对 ⭐ 新增API
func (a *App) LoadNSCKeys(nscSeed string) error {
	if a.chatSvc == nil {
		return fmt.Errorf("chat service not initialized")
	}
	return a.chatSvc.LoadNSCKeys(nscSeed)
}

// AddFriendNSCKey 通过NSC公钥添加好友 ⭐ 新增API
func (a *App) AddFriendNSCKey(uid, nscPubKey string) error {
	if a.chatSvc == nil {
		return fmt.Errorf("chat service not initialized")
	}
	return a.chatSvc.AddFriendNSCKey(uid, nscPubKey)
}

// GetNSCUserSeed 获取当前用户的NSC seed (从配置中读取) ⭐ 新增API
func (a *App) GetNSCUserSeed() (string, error) {
	if a.config == nil {
		return "", fmt.Errorf("config not loaded")
	}

	// 从NSC用户seed文件读取
	if a.config.NSC.UserSeedPath != "" {
		// 读取seed文件内容
		data, err := os.ReadFile(a.config.NSC.UserSeedPath)
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

// GetNetworkStats aggregates network statistics
// （权限热重启、网络统计、凭据导出接口已移除，保持核心最小 API）
