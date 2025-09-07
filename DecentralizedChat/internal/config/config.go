package config

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"

	"github.com/nats-io/nats-server/v2/server"
)

type Config struct {
	User    UserConfig        `json:"user"`
	Network NetworkConfig     `json:"network"`
	UI      UIConfig          `json:"ui"`
	Keys    KeysConfig        `json:"keys"` // 更名为Keys，不再是NSC
	Server  ServerOptionsLite `json:"server"`
}

type UserConfig struct {
	ID       string `json:"id"`
	Nickname string `json:"nickname"`
	Avatar   string `json:"avatar"`
}

type NetworkConfig struct {
	AutoDiscovery bool     `json:"auto_discovery"`
	SeedNodes     []string `json:"seed_nodes"`
	LocalIP       string   `json:"local_ip"`
}

// Legacy NATSConfig/Permissions 结构已移除：直接使用 ServerOptionsLite 提供的 Import/Export 权限。

type UIConfig struct {
	Theme    string `json:"theme"`
	Language string `json:"language"`
}

// KeysConfig 简化的密钥配置（替代NSCConfig）
type KeysConfig struct {
	Operator      string `json:"operator"`        // 操作者名称 (e.g. dchat)
	KeysDir       string `json:"keys_dir"`        // 密钥存储目录
	UserCredsPath string `json:"user_creds_path"` // 用户凭据文件路径 (.creds)
	UserSeedPath  string `json:"user_seed_path"`  // 用户私钥种子文件路径
	UserPubKey    string `json:"user_pub_key"`    // 用户公钥 (U...)
	Account       string `json:"account"`         // 账户名称 (e.g. USERS)
	User          string `json:"user"`            // 用户名称 (e.g. default)
}

// ServerOptionsLite 扁平化的服务端配置，减少嵌套 & 贴近 server.Options
type ServerOptionsLite struct {
	Host         string   `json:"host"`
	ClientPort   int      `json:"client_port"`
	ClusterPort  int      `json:"cluster_port"`
	ClusterName  string   `json:"cluster_name"`
	SeedRoutes   []string `json:"seed_routes"`
	ResolverConf string   `json:"resolver_config"`
	CredsFile    string   `json:"creds_file"`
	ImportAllow  []string `json:"import_allow"`
	ImportDeny   []string `json:"import_deny"`
	ExportAllow  []string `json:"export_allow"`
	ExportDeny   []string `json:"export_deny"`
	EnableTLS    bool     `json:"enable_tls,omitempty"`
	NodeID       string   `json:"node_id,omitempty"` // 对应routes.NodeConfig.NodeID
}

var defaultConfig = Config{
	User: UserConfig{
		ID:       "",
		Nickname: "Anonymous",
		Avatar:   "",
	},
	Network: NetworkConfig{
		AutoDiscovery: true,
		SeedNodes:     []string{},
		LocalIP:       "", // Will be resolved dynamically or provided by user
	},
	// NATS 字段已删除
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
	Server: ServerOptionsLite{
		Host:         "",
		ClientPort:   0,
		ClusterPort:  0,
		ClusterName:  "dchat_network",
		SeedRoutes:   []string{},
		ResolverConf: "",
		CredsFile:    "",
		ImportAllow:  []string{},
		ImportDeny:   []string{},
		ExportAllow:  []string{"*"},
		ExportDeny:   []string{},
		EnableTLS:    true, // ⭐ 默认启用TLS，与routes.go一致
		NodeID:       "",   // 运行时生成
	},
}

func GetConfigPath() (string, error) {
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
		"url":        fmt.Sprintf("nats://%s:%d", c.Server.Host, c.Server.ClientPort),
		"creds_file": c.Server.CredsFile,
		"name":       c.User.Nickname,
	}
}

// GetClusterConfig returns cluster related config for higher level managers
func (c *Config) GetClusterConfig() map[string]interface{} {
	return map[string]interface{}{
		"host": c.Server.Host,
	}
}

// UpdateUserInfo updates user profile information
func (c *Config) UpdateUserInfo(nickname, avatar string) {
	c.User.Nickname = nickname
	c.User.Avatar = avatar
}

// EnableRoutes enables embedded routes cluster with provided parameters
func (c *Config) EnableRoutes(host string, clientPort int, clusterPort int, seedRoutes []string) {
	c.Server.Host = host
	c.Server.ClientPort = clientPort
	c.Server.ClusterPort = clusterPort
	c.Server.SeedRoutes = seedRoutes
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
	// ⭐ 自动生成用户ID（如果为空）
	if c.User.ID == "" {
		userID, err := generateUserID()
		if err != nil {
			return fmt.Errorf("generate user ID: %w", err)
		}
		c.User.ID = userID
	}

	// 设置本地IP
	if c.Network.LocalIP == "" {
		localIP, err := GetLocalIP()
		if err != nil {
			// 如果无法获取，使用回环地址作为后备
			c.Network.LocalIP = "localhost"
		} else {
			c.Network.LocalIP = localIP
		}
	}

	if c.Server.Host == "" {
		c.Server.Host = c.Network.LocalIP
	}

	// ⭐ 设置默认端口（如果为0）
	if c.Server.ClientPort == 0 {
		c.Server.ClientPort = 4222
	}
	if c.Server.ClusterPort == 0 {
		c.Server.ClusterPort = 6222
	}

	// ⭐ 设置默认集群名称
	if c.Server.ClusterName == "" {
		c.Server.ClusterName = "dchat_network"
	}

	// 默认 URL 由 Server 构造； import/export 权限已直接在 Server 中维护
	if len(c.Server.ExportAllow) == 0 {
		c.Server.ExportAllow = []string{"*"}
	}

	// ⭐ 设置其他默认值
	if c.User.Nickname == "" {
		c.User.Nickname = "Anonymous"
	}

	// ⭐ 确保keys.user与user.nickname保持一致（如果keys.user为空）
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

// AddSubscribePermissionAndSave 添加订阅权限并立即持久化
func (c *Config) AddSubscribePermissionAndSave(subject string) error {
	if subject == "" {
		return nil
	}
	for _, allowed := range c.Server.ImportAllow { // 已存在
		if allowed == subject {
			return SaveConfig(c)
		}
	}
	c.Server.ImportAllow = append(c.Server.ImportAllow, subject)
	return SaveConfig(c)
}

// RemoveSubscribePermissionAndSave 移除订阅权限并立即持久化
func (c *Config) RemoveSubscribePermissionAndSave(subject string) error {
	if subject == "" {
		return nil
	}
	var na []string
	for _, s := range c.Server.ImportAllow {
		if s != subject {
			na = append(na, s)
		}
	}
	c.Server.ImportAllow = na
	return SaveConfig(c)
}

// Trusted 公钥不再持久化到本地配置：改为使用 NATS KV 存储（好友/群密钥）。

// BuildServerOptions 基于扁平 Server 配置生成 server.Options
func (c *Config) BuildServerOptions() (*server.Options, error) {
	opts := &server.Options{}
	opts.Host = c.Server.Host
	opts.Port = c.Server.ClientPort
	opts.Cluster.Host = c.Server.Host
	opts.Cluster.Port = c.Server.ClusterPort
	opts.Cluster.Name = c.Server.ClusterName
	if len(c.Server.SeedRoutes) > 0 {
		var routes []*url.URL
		for _, r := range c.Server.SeedRoutes {
			u, err := url.Parse(r)
			if err != nil {
				return nil, fmt.Errorf("invalid route url %s: %w", r, err)
			}
			routes = append(routes, u)
		}
		opts.Routes = routes
	}
	opts.Cluster.Permissions = &server.RoutePermissions{
		Import: &server.SubjectPermission{Allow: c.Server.ImportAllow, Deny: c.Server.ImportDeny},
		Export: &server.SubjectPermission{Allow: c.Server.ExportAllow, Deny: c.Server.ExportDeny},
	}
	if c.Server.ResolverConf != "" {
		if err := opts.ProcessConfigFile(c.Server.ResolverConf); err != nil {
			return nil, fmt.Errorf("failed to load resolver config: %w", err)
		}
	}
	return opts, nil
}
