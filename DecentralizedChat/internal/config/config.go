package config

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"
)

type Config struct {
	User    UserConfig    `json:"user"`
	Network NetworkConfig `json:"network"`
	NATS    NATSConfig    `json:"nats"`
	Routes  RoutesConfig  `json:"routes"`
	UI      UIConfig      `json:"ui"`
	NSC     NSCConfig     `json:"nsc"`
}

type UserConfig struct {
	ID       string `json:"id"`
	Nickname string `json:"nickname"`
	Avatar   string `json:"avatar"`
}

type NetworkConfig struct {
	TailscaleEnabled bool     `json:"tailscale_enabled"`
	AutoDiscovery    bool     `json:"auto_discovery"`
	SeedNodes        []string `json:"seed_nodes"`
	LocalIP          string   `json:"local_ip"`
}

type NATSConfig struct {
	URL           string        `json:"url"`            // NATS服务器URL
	User          string        `json:"user"`           // 用户名（兼容遗留，不推荐）
	Password      string        `json:"password"`       // 密码（兼容遗留，不推荐）
	Token         string        `json:"token"`          // 令牌（可选）
	CredsFile     string        `json:"creds_file"`     // NSC 生成的 .creds 凭据文件路径
	Timeout       time.Duration `json:"timeout"`        // 连接超时
	MaxReconnect  int           `json:"max_reconnect"`  // 最大重连次数
	ReconnectWait time.Duration `json:"reconnect_wait"` // 重连等待时间
	Permissions   Permissions   `json:"permissions"`    // 主题权限配置
}

// Permissions NATS主题权限配置
type Permissions struct {
	Publish   PermissionRules `json:"publish"`   // 发布权限
	Subscribe PermissionRules `json:"subscribe"` // 订阅权限
}

// PermissionRules 权限规则
type PermissionRules struct {
	Allow []string `json:"allow"` // 允许的主题列表
	Deny  []string `json:"deny"`  // 拒绝的主题列表
}

type RoutesConfig struct {
	Enabled        bool     `json:"enabled"`         // 是否启用Routes集群
	Host           string   `json:"host"`            // 主机地址
	ClientPort     int      `json:"client_port"`     // 客户端端口
	ClusterPort    int      `json:"cluster_port"`    // 集群端口
	ClusterName    string   `json:"cluster_name"`    // 集群名称
	SeedRoutes     []string `json:"seed_routes"`     // 种子路由
	NodeName       string   `json:"node_name"`       // 节点名称
	ResolverConfig string   `json:"resolver_config"` // nsc 生成的 resolver.conf 路径（启用基于 JWT 的鉴权）
}

type UIConfig struct {
	Theme    string `json:"theme"`
	Language string `json:"language"`
}

// NSCConfig 用于持久化 nsc 生成的关键文件路径
type NSCConfig struct {
	Operator      string `json:"operator"`      // 如 local
	StoreDir      string `json:"store_dir"`     // nsc 存储目录
	KeysDir       string `json:"keys_dir"`      // nsc 密钥目录
	SysAccountJWT string `json:"sys_jwt_path"`  // SYS 账户 JWT 文件路径
	SysPubPath    string `json:"sys_pub_path"`  // SYS 公钥文件路径（本程序写入）
	SysSeedPath   string `json:"sys_seed_path"` // SYS 种子私钥路径（若可获取）
}

var defaultConfig = Config{
	User: UserConfig{
		ID:       "",
		Nickname: "Anonymous",
		Avatar:   "",
	},
	Network: NetworkConfig{
		TailscaleEnabled: true,
		AutoDiscovery:    true,
		SeedNodes:        []string{},
		LocalIP:          "", // 需要动态获取或用户配置
	},
	NATS: NATSConfig{
		URL:           "", // 需要用户配置
		User:          "",
		Password:      "",
		Token:         "",
		CredsFile:     "",
		Timeout:       5 * time.Second,
		MaxReconnect:  -1,
		ReconnectWait: time.Second,
		Permissions: Permissions{
			Publish: PermissionRules{
				Allow: []string{"*"}, // 默认允许发布到所有主题
				Deny:  []string{},
			},
			Subscribe: PermissionRules{
				Allow: []string{}, // 默认拒绝订阅所有主题
				Deny:  []string{"*"},
			},
		},
	},
	Routes: RoutesConfig{
		Enabled:        false,
		Host:           "", // 需要用户配置
		ClientPort:     0,  // 需要用户配置
		ClusterPort:    0,  // 需要用户配置
		ClusterName:    "dchat_network",
		SeedRoutes:     []string{},
		NodeName:       "dchat_node",
		ResolverConfig: "",
	},
	UI: UIConfig{
		Theme:    "dark",
		Language: "zh-CN",
	},
	NSC: NSCConfig{
		Operator:      "",
		StoreDir:      "",
		KeysDir:       "",
		SysAccountJWT: "",
		SysPubPath:    "",
		SysSeedPath:   "",
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
		"url":            c.NATS.URL,
		"user":           c.NATS.User,
		"password":       c.NATS.Password,
		"token":          c.NATS.Token,
		"creds_file":     c.NATS.CredsFile,
		"name":           c.User.Nickname,
		"timeout":        c.NATS.Timeout,
		"max_reconnect":  c.NATS.MaxReconnect,
		"reconnect_wait": c.NATS.ReconnectWait,
	}
}

// GetRoutesConfig 获取Routes集群配置
func (c *Config) GetRoutesConfig() map[string]interface{} {
	return map[string]interface{}{
		"enabled":         c.Routes.Enabled,
		"host":            c.Routes.Host,
		"client_port":     c.Routes.ClientPort,
		"cluster_port":    c.Routes.ClusterPort,
		"cluster_name":    c.Routes.ClusterName,
		"seed_routes":     c.Routes.SeedRoutes,
		"node_name":       c.Routes.NodeName,
		"resolver_config": c.Routes.ResolverConfig,
	}
}

// GetClusterConfig 获取集群配置用于ClusterManager
func (c *Config) GetClusterConfig() map[string]interface{} {
	return map[string]interface{}{
		"host": c.Routes.Host,
	}
}

// UpdateNATSURL 更新NATS连接URL
func (c *Config) UpdateNATSURL(url string) {
	c.NATS.URL = url
}

// UpdateUserInfo 更新用户信息
func (c *Config) UpdateUserInfo(nickname, avatar string) {
	c.User.Nickname = nickname
	c.User.Avatar = avatar
}

// EnableRoutes 启用Routes集群
func (c *Config) EnableRoutes(host string, clientPort int, clusterPort int, seedRoutes []string) {
	c.Routes.Enabled = true
	c.Routes.Host = host
	c.Routes.ClientPort = clientPort
	c.Routes.ClusterPort = clusterPort
	c.Routes.SeedRoutes = seedRoutes
}

// GetLocalIP 获取本机IP地址（非回环地址）
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

// ValidateAndSetDefaults 验证并设置默认值
func (c *Config) ValidateAndSetDefaults() error {
	// 设置本地IP
	if c.Network.LocalIP == "" {
		localIP, err := GetLocalIP()
		if err != nil {
			// 如果无法获取，使用回环地址作为后备
			c.Network.LocalIP = "127.0.0.1"
		} else {
			c.Network.LocalIP = localIP
		}
	}

	// 设置Routes主机地址
	if c.Routes.Host == "" {
		c.Routes.Host = c.Network.LocalIP
	}

	// 设置NATS URL
	if c.NATS.URL == "" {
		c.NATS.URL = fmt.Sprintf("nats://%s:%d", c.Routes.Host, c.Routes.ClientPort)
	}

	// 确保权限配置完整
	c.ensurePermissionsDefaults()

	return nil
}

// ensurePermissionsDefaults 确保权限配置有默认值
func (c *Config) ensurePermissionsDefaults() {
	if len(c.NATS.Permissions.Publish.Allow) == 0 && len(c.NATS.Permissions.Publish.Deny) == 0 {
		c.NATS.Permissions.Publish.Allow = []string{"*"}
	}
	if len(c.NATS.Permissions.Subscribe.Allow) == 0 && len(c.NATS.Permissions.Subscribe.Deny) == 0 {
		c.NATS.Permissions.Subscribe.Deny = []string{"*"}
	}
}

// AddSubscribePermission 添加订阅权限
func (c *Config) AddSubscribePermission(subject string) {
	// 检查是否已存在
	for _, allowed := range c.NATS.Permissions.Subscribe.Allow {
		if allowed == subject {
			return
		}
	}

	// 添加到允许列表
	c.NATS.Permissions.Subscribe.Allow = append(c.NATS.Permissions.Subscribe.Allow, subject)

	// 从拒绝列表中移除（如果存在）
	c.removeFromDenyList(subject)
}

// RemoveSubscribePermission 移除订阅权限
func (c *Config) RemoveSubscribePermission(subject string) {
	// 从允许列表中移除
	var newAllow []string
	for _, allowed := range c.NATS.Permissions.Subscribe.Allow {
		if allowed != subject {
			newAllow = append(newAllow, allowed)
		}
	}
	c.NATS.Permissions.Subscribe.Allow = newAllow
}

// removeFromDenyList 从拒绝列表中移除主题
func (c *Config) removeFromDenyList(subject string) {
	var newDeny []string
	for _, denied := range c.NATS.Permissions.Subscribe.Deny {
		if denied != subject {
			newDeny = append(newDeny, denied)
		}
	}
	c.NATS.Permissions.Subscribe.Deny = newDeny
}

// CanPublish 检查是否可以发布到指定主题
func (c *Config) CanPublish(subject string) bool {
	return c.checkPermission(subject, c.NATS.Permissions.Publish)
}

// CanSubscribe 检查是否可以订阅指定主题
func (c *Config) CanSubscribe(subject string) bool {
	return c.checkPermission(subject, c.NATS.Permissions.Subscribe)
}

// checkPermission 检查主题权限
func (c *Config) checkPermission(subject string, rules PermissionRules) bool {
	// 如果在拒绝列表中，直接返回false
	for _, deny := range rules.Deny {
		if c.matchSubject(subject, deny) {
			return false
		}
	}

	// 如果在允许列表中，返回true
	for _, allow := range rules.Allow {
		if c.matchSubject(subject, allow) {
			return true
		}
	}

	// 默认拒绝
	return false
}

// matchSubject 匹配主题模式（支持通配符）
func (c *Config) matchSubject(subject, pattern string) bool {
	if pattern == "*" {
		return true
	}
	if pattern == subject {
		return true
	}

	// 简单的通配符匹配（可以扩展为更复杂的模式匹配）
	if len(pattern) > 0 && pattern[len(pattern)-1] == '*' {
		prefix := pattern[:len(pattern)-1]
		return len(subject) >= len(prefix) && subject[:len(prefix)] == prefix
	}

	return false
}
