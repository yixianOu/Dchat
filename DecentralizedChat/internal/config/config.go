package config

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"
)

type Config struct {
	User       UserConfig     `json:"user"`
	LeafNode   LeafNodeConfig `json:"leafnode"`
	SQLitePath string         `json:"sqlite_path"`
	UI         UIConfig       `json:"ui"`
	Keys       KeysConfig     `json:"keys"`
}

type UserConfig struct {
	ID       string `json:"id"`
	Nickname string `json:"nickname"`
	Avatar   string `json:"avatar"`
}

// LeafNodeConfig LeafNode 配置
type LeafNodeConfig struct {
	LocalHost      string        `json:"local_host"`
	LocalPort      int           `json:"local_port"`
	HubURLs        []string      `json:"hub_urls"`
	CredsFile      string        `json:"creds_file"`
	EnableTLS      bool          `json:"enable_tls"`
	ConnectTimeout time.Duration `json:"connect_timeout"`
}

type UIConfig struct {
	Theme    string `json:"theme"`
	Language string `json:"language"`
}

// KeysConfig 简化的密钥配置
type KeysConfig struct {
	Operator      string `json:"operator"`        // 操作者名称 (e.g. dchat)
	KeysDir       string `json:"keys_dir"`        // 密钥存储目录
	UserCredsPath string `json:"user_creds_path"` // 用户凭据文件路径 (.creds)
	UserSeedPath  string `json:"user_seed_path"`  // 用户私钥种子文件路径
	UserPubKey    string `json:"user_pub_key"`    // 用户公钥 (U...)
	Account       string `json:"account"`         // 账户名称 (e.g. USERS)
	User          string `json:"user"`            // 用户名称 (e.g. default)
}

var defaultConfig = Config{
	User: UserConfig{
		ID:       "",
		Nickname: "Anonymous",
		Avatar:   "",
	},
	LeafNode: LeafNodeConfig{
		LocalHost:      "127.0.0.1",
		LocalPort:      4222,
		HubURLs:        []string{"nats://hub1.dchat.example.com:7422"},
		CredsFile:      "",
		EnableTLS:      false,
		ConnectTimeout: 10 * time.Second,
	},
	SQLitePath: "", // 默认 ~/.dchat/chat.db
	UI: UIConfig{
		Theme:    "dark",
		Language: "zh-CN",
	},
	Keys: KeysConfig{
		Operator:      "",
		KeysDir:       "",
		UserCredsPath: "",
		UserSeedPath:  "",
		UserPubKey:    "",
		Account:       "",
		User:          "Anonymous", // 默认与user.nickname保持一致
	},
}

// DefaultLeafNodeConfig 返回默认的 LeafNode 配置
func DefaultLeafNodeConfig() *LeafNodeConfig {
	return &LeafNodeConfig{
		LocalHost:      "127.0.0.1",
		LocalPort:      4222,
		HubURLs:        []string{"nats://hub1.dchat.example.com:7422", "nats://hub2.dchat.example.com:7422"},
		CredsFile:      "",
		EnableTLS:      false,
		ConnectTimeout: 10 * time.Second,
	}
}

// GetConfigPathFunc 类型用于获取配置路径的函数
type GetConfigPathFunc func() (string, error)

// DefaultGetConfigPath 默认的获取配置路径函数
var DefaultGetConfigPath GetConfigPathFunc = func() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	configDir := filepath.Join(homeDir, ".dchat")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return "", err
	}

	return filepath.Join(configDir, "config.json"), nil
}

// GetConfigPath 获取配置文件路径（可被测试重写）
var GetConfigPath = DefaultGetConfigPath

func LoadConfig() (*Config, error) {
	configPath, err := GetConfigPath()
	if err != nil {
		return nil, err
	}

	// 如果配置文件不存在，返回默认配置
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		config := defaultConfig
		if err := config.ValidateAndSetDefaults(); err != nil {
			return nil, fmt.Errorf("failed to set default config: %w", err)
		}
		if err := SaveConfig(&config); err != nil {
			return &config, nil // 返回配置，忽略保存错误
		}
		return &config, nil
	}

	// 读取配置文件
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// 验证并设置默认值
	if err := config.ValidateAndSetDefaults(); err != nil {
		return nil, fmt.Errorf("failed to validate config: %w", err)
	}

	return &config, nil
}

func SaveConfig(config *Config) error {
	configPath, err := GetConfigPath()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

func GetDefaultConfig() Config {
	return defaultConfig
}

// GetNATSClientConfig 获取NATS客户端配置
func (c *Config) GetNATSClientConfig() map[string]interface{} {
	return map[string]interface{}{
		"url":        fmt.Sprintf("nats://%s:%d", c.LeafNode.LocalHost, c.LeafNode.LocalPort),
		"creds_file": c.Keys.UserCredsPath,
		"name":       c.User.Nickname,
	}
}

// UpdateUserInfo updates user profile information
func (c *Config) UpdateUserInfo(nickname, avatar string) {
	c.User.Nickname = nickname
	c.User.Avatar = avatar
}

// GetLocalIP returns first non-loopback IPv4 address
func GetLocalIP() (string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", err
	}

	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String(), nil
			}
		}
	}

	return "", fmt.Errorf("未找到有效的本地IP地址")
}

// ValidateAndSetDefaults validates config and fills missing defaults
func (c *Config) ValidateAndSetDefaults() error {
	// 自动生成用户ID（如果为空）
	if c.User.ID == "" {
		userID, err := generateUserID()
		if err != nil {
			return fmt.Errorf("generate user ID: %w", err)
		}
		c.User.ID = userID
	}

	// 设置默认本地监听地址
	if c.LeafNode.LocalHost == "" {
		c.LeafNode.LocalHost = "127.0.0.1"
	}

	// 设置默认端口
	if c.LeafNode.LocalPort == 0 {
		c.LeafNode.LocalPort = 4222
	}

	// 设置默认 Hub URLs
	if len(c.LeafNode.HubURLs) == 0 {
		c.LeafNode.HubURLs = []string{"nats://hub1.dchat.example.com:7422"}
	}

	// 设置默认昵称
	if c.User.Nickname == "" {
		c.User.Nickname = "Anonymous"
	}

	// 确保keys.user与user.nickname保持一致（如果keys.user为空）
	if c.Keys.User == "" {
		c.Keys.User = c.User.Nickname
	}

	if c.UI.Theme == "" {
		c.UI.Theme = "dark"
	}
	if c.UI.Language == "" {
		c.UI.Language = "zh-CN"
	}

	return nil
}

// generateUserID 生成一个唯一的用户ID
func generateUserID() (string, error) {
	// 生成12字节的随机数据，转为24字符的十六进制字符串
	bytes := make([]byte, 12)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
