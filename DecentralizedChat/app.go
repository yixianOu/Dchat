package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"DecentralizedChat/internal/chat"
	"DecentralizedChat/internal/config"
	"DecentralizedChat/internal/leafnode"
	"DecentralizedChat/internal/nats"
	"DecentralizedChat/internal/nscsetup"
	"DecentralizedChat/internal/storage"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// CreateGroupResult 创建群聊返回结果
type CreateGroupResult struct {
	GID      string `json:"gid"`
	GroupKey string `json:"groupKey"`
}

// App struct
type App struct {
	ctx         context.Context
	chatSvc     *chat.Service
	natsSvc     *nats.Service
	leafnodeMgr *leafnode.Manager
	storage     *storage.Storage
	config      *config.Config
	mu          sync.RWMutex
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

	// 2. 初始化并启动 LeafNode Manager
	leafnodeCfg := &config.LeafNodeConfig{
		LocalHost:               cfg.LeafNode.LocalHost,
		LocalPort:               cfg.LeafNode.LocalPort,
		HubURLs:                 cfg.LeafNode.HubURLs,
		CredsFile:               cfg.Keys.UserCredsPath,
		EnableTLS:               cfg.LeafNode.EnableTLS,
		ConnectTimeout:          cfg.LeafNode.ConnectTimeout,
		EnableJetStream:         cfg.LeafNode.EnableJetStream,
		JetStreamStoreDir:       cfg.LeafNode.JetStreamStoreDir,
		JetStreamAllowUpstreamAPI: cfg.LeafNode.JetStreamAllowUpstreamAPI,
	}
	if leafnodeCfg.ConnectTimeout == 0 {
		leafnodeCfg.ConnectTimeout = 10 * time.Second
	}
	a.leafnodeMgr = leafnode.NewManager(leafnodeCfg)

	log.Println("Starting LeafNode...")
	if err := a.leafnodeMgr.Start(); err != nil {
		log.Printf("start leafnode failed: %v", err)
		return
	}
	log.Println("✅ LeafNode started successfully")

	// 3. 初始化 SQLite 本地存储
	sqlitePath := cfg.SQLitePath
	if sqlitePath == "" {
		homeDir, err := os.UserHomeDir()
		if err == nil {
			sqlitePath = filepath.Join(homeDir, ".dchat", "chat.db")
		}
	}
	if sqlitePath != "" {
		// 确保目录存在
		if err := os.MkdirAll(filepath.Dir(sqlitePath), 0755); err == nil {
			a.storage, err = storage.NewSQLiteStorage(sqlitePath)
			if err != nil {
				log.Printf("init storage failed: %v", err)
			} else {
				log.Println("✅ SQLite storage initialized")
			}
		}
	}

	// 4. 创建本地 NATS Client（连接到本地 LeafNode）
	a.natsSvc, err = nats.NewService(nats.ClientConfig{
		URL:       a.leafnodeMgr.GetLocalNATSURL(),
		Name:      "DChatClient",
		CredsFile: a.config.Keys.UserCredsPath,
	})
	if err != nil {
		log.Printf("start nats client failed: %v", err)
		return
	}
	// 把storage实例传给chat服务
	a.chatSvc = chat.NewService(a.natsSvc, a.storage)

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

	// 初始化离线消息同步（所有依赖都准备就绪：LeafNode已连接、NATS客户端已连接、密钥已加载）
	if a.chatSvc != nil && a.config.LeafNode.EnableJetStream {
		go func() {
			// 等待3秒确保LeafNode和Hub的连接完全稳定
			time.Sleep(3 * time.Second)
			if err := a.chatSvc.InitOfflineSync(); err != nil {
				log.Printf("初始化离线同步失败: %v", err)
			} else {
				log.Println("✅ 离线消息同步初始化成功")
			}
		}()
	}

	if err := config.SaveConfig(a.config); err != nil {
		log.Printf("save config warn: %v", err)
	}
	log.Println("DChat application started (LeafNode mode)")
}

// OnShutdown is called when the app stops
func (a *App) OnShutdown(ctx context.Context) {
	// 先停止离线消息同步
	if a.natsSvc != nil {
		a.natsSvc.StopSync()
		a.natsSvc.Close()
	}
	if a.leafnodeMgr != nil {
		a.leafnodeMgr.Stop()
	}
	if a.storage != nil {
		a.storage.Close()
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

// 旧方法已废弃，请使用带groupKey参数的JoinGroup

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
	if a.natsSvc == nil || a.leafnodeMgr == nil {
		return nil, fmt.Errorf("services not initialized")
	}

	result := make(map[string]interface{})

	// NATS客户端状态
	result["nats"] = a.natsSvc.GetStats()

	// LeafNode 状态
	result["leafnode"] = map[string]interface{}{
		"connected":     a.leafnodeMgr.IsRunning(),
		"hub_urls":      a.config.LeafNode.HubURLs,
		"local_url":     a.leafnodeMgr.GetLocalNATSURL(),
		"sqlite_path":   a.config.SQLitePath,
		"storage_ready": a.storage != nil,
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

// GetMessages 获取会话历史消息
func (a *App) GetMessages(conversationID string, limit int, before *time.Time) ([]*storage.StoredMessage, error) {
	if a.chatSvc == nil {
		return nil, fmt.Errorf("chat service not initialized")
	}
	return a.chatSvc.GetMessages(conversationID, limit, before)
}

// MarkAsRead 标记会话消息已读
func (a *App) MarkAsRead(conversationID string, before time.Time) error {
	if a.chatSvc == nil {
		return fmt.Errorf("chat service not initialized")
	}
	return a.chatSvc.MarkAsRead(conversationID, before)
}

// GetConversation 获取会话信息
func (a *App) GetConversation(conversationID string) (*storage.StoredConversation, error) {
	if a.chatSvc == nil {
		return nil, fmt.Errorf("chat service not initialized")
	}
	return a.chatSvc.GetConversation(conversationID)
}

// CreateGroup 创建新群聊，返回群ID和群密钥
func (a *App) CreateGroup() (*CreateGroupResult, error) {
	if a.chatSvc == nil {
		return nil, fmt.Errorf("chat service not initialized")
	}
	gid, groupKey, err := a.chatSvc.CreateGroup()
	if err != nil {
		return nil, err
	}
	return &CreateGroupResult{
		GID:      gid,
		GroupKey: groupKey,
	}, nil
}

// JoinGroup 加入群聊，需要群ID和群密钥
func (a *App) JoinGroup(gid, groupKey string) error {
	if a.chatSvc == nil {
		return fmt.Errorf("chat service not initialized")
	}
	// 存储群密钥
	a.chatSvc.AddGroupKey(gid, groupKey)
	// 订阅群消息
	return a.chatSvc.JoinGroup(gid)
}

// SearchMessages 搜索消息
func (a *App) SearchMessages(query string, limit int) ([]*storage.StoredMessage, error) {
	if a.chatSvc == nil {
		return nil, fmt.Errorf("chat service not initialized")
	}
	return a.chatSvc.SearchMessages(query, limit)
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
