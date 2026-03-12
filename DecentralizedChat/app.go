package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
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
	startupErrs []error // 启动阶段错误集合
	initialized bool    // 是否已完成启动
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{}
}

// 初始化日志输出到文件和控制台，支持级别区分
func initLog(level string) error {
	logFile, err := os.OpenFile("log.txt", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}

	// 解析日志级别
	var logLevel slog.Level
	switch strings.ToLower(level) {
	case "debug":
		logLevel = slog.LevelDebug
	case "info":
		logLevel = slog.LevelInfo
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	// 同时输出到控制台和文件
	mw := io.MultiWriter(os.Stdout, logFile)

	// 创建日志handler，带级别和源文件信息
	handler := slog.NewTextHandler(mw, &slog.HandlerOptions{
		Level:     logLevel,
		AddSource: true,
	})

	// 设置全局默认logger
	slog.SetDefault(slog.New(handler))
	return nil
}

// addStartupError 添加启动错误并记录日志
func (a *App) addStartupError(err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.startupErrs = append(a.startupErrs, err)
	slog.Error("startup error", "error", err)
}

// OnStartup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) OnStartup(ctx context.Context) {
	a.ctx = ctx

	// 1. 先加载配置，获取日志级别
	cfg, err := config.LoadConfig()
	if err != nil {
		fmt.Printf("load config failed: %v, using default config\n", err)
		cfg = &config.Config{}
	}

	// 2. 初始化日志，使用配置的日志级别
	if err := initLog(cfg.LogLevel); err != nil {
		fmt.Printf("init log failed: %v\n", err)
	}
	if cfg.LeafNode.LocalHost == "" {
		cfg.LeafNode.LocalHost = "127.0.0.1"
	}

	// 简化版NATS初始化：无需NSC CLI，直接使用Go库
	if err := nscsetup.EnsureSimpleSetup(cfg); err != nil {
		a.addStartupError(fmt.Errorf("初始化简化NATS设置失败: %w", err))
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

	slog.Info("Starting LeafNode...")
	if err := a.leafnodeMgr.Start(); err != nil {
		a.addStartupError(fmt.Errorf("start leafnode failed: %w", err))
		return
	}
	slog.Info("✅ LeafNode started successfully")

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
				a.addStartupError(fmt.Errorf("init storage failed: %w", err))
			} else {
				slog.Info("✅ SQLite storage initialized")
			}
		} else {
			a.addStartupError(fmt.Errorf("create sqlite directory failed: %w", err))
		}
	}

	// 4. 创建本地 NATS Client（连接到本地 LeafNode）
	a.natsSvc, err = nats.NewService(nats.ClientConfig{
		URL:       a.leafnodeMgr.GetLocalNATSURL(),
		Name:      "DChatClient",
		CredsFile: a.config.Keys.UserCredsPath,
	})
	if err != nil {
		a.addStartupError(fmt.Errorf("start nats client failed: %w", err))
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
			a.addStartupError(fmt.Errorf("failed to load NSC seed: %w", err))
		} else {
			if err := a.chatSvc.LoadNSCKeys(seed); err != nil {
				a.addStartupError(fmt.Errorf("failed to load NSC chat keys: %w", err))
			} else {
				slog.Info("NSC chat keys loaded successfully")
			}
		}
	}

	// 初始化离线消息同步（所有依赖都准备就绪：LeafNode已连接、NATS客户端已连接、密钥已加载）
	if a.chatSvc != nil && a.config.LeafNode.EnableJetStream {
		go func() {
			// 等待3秒确保LeafNode和Hub的连接完全稳定
			time.Sleep(3 * time.Second)
			if err := a.chatSvc.InitOfflineSync(); err != nil {
				slog.Error("初始化离线同步失败", "error", err)
				// 异步初始化的错误推送到前端
				runtime.EventsEmit(a.ctx, "message:error", map[string]any{
					"error":     fmt.Sprintf("离线同步初始化失败: %v", err),
					"timestamp": fmt.Sprintf("%d", time.Now().Unix()),
				})
			} else {
				slog.Info("✅ 离线消息同步初始化成功")
			}
		}()
	}

	if err := config.SaveConfig(a.config); err != nil {
		slog.Warn("save config warn", "error", err)
	}

	// 标记启动完成，推送启动错误到前端
	a.mu.Lock()
	a.initialized = true
	startupErrs := a.startupErrs
	a.mu.Unlock()

	if len(startupErrs) > 0 {
		// 把所有启动错误合并成一条消息推送给前端
		errMsg := "启动过程中出现以下错误:\n"
		for _, err := range startupErrs {
			errMsg += fmt.Sprintf("- %v\n", err)
		}
		runtime.EventsEmit(a.ctx, "message:error", map[string]any{
			"error":     errMsg,
			"timestamp": fmt.Sprintf("%d", time.Now().Unix()),
		})
	}

	// 自动恢复所有会话（异步执行，不阻塞启动）
	if a.storage != nil && a.chatSvc != nil {
		go func() {
			// 等待1秒确保服务完全稳定
			time.Sleep(1 * time.Second)

			// 恢复好友会话
			friends, err := a.storage.GetAllFriends()
			if err != nil {
				slog.Warn("failed to get friends list", "error", err)
			} else {
				successCount := 0
				for _, peerID := range friends {
					if err := a.chatSvc.JoinDirect(peerID); err != nil {
						slog.Warn("failed to rejoin direct chat", "peer", peerID, "error", err)
					} else {
						successCount++
					}
				}
				slog.Info("direct chats restored", "total", len(friends), "success", successCount)
			}

			// 恢复群聊会话
			groups, err := a.storage.GetAllGroups()
			if err != nil {
				slog.Warn("failed to get groups list", "error", err)
			} else {
				successCount := 0
				for _, gid := range groups {
					if err := a.chatSvc.JoinGroup(gid); err != nil {
						slog.Warn("failed to rejoin group chat", "gid", gid, "error", err)
					} else {
						successCount++
					}
				}
				slog.Info("group chats restored", "total", len(groups), "success", successCount)
			}
		}()
	}

	slog.Info("DChat application started (LeafNode mode)")
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

// GetStartupStatus 获取启动状态和错误信息
func (a *App) GetStartupStatus() (map[string]any, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	errStrs := make([]string, len(a.startupErrs))
	for i, err := range a.startupErrs {
		errStrs[i] = err.Error()
	}

	return map[string]any{
		"initialized": a.initialized,
		"errors":      errStrs,
	}, nil
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
