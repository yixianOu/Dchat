package config

import (
	"encoding/json"
	"fmt"
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
	User          string        `json:"user"`           // 用户名
	Password      string        `json:"password"`       // 密码
	Token         string        `json:"token"`          // 令牌
	Timeout       time.Duration `json:"timeout"`        // 连接超时
	MaxReconnect  int           `json:"max_reconnect"`  // 最大重连次数
	ReconnectWait time.Duration `json:"reconnect_wait"` // 重连等待时间
}

type RoutesConfig struct {
	Enabled           bool     `json:"enabled"`             // 是否启用Routes集群
	Host              string   `json:"host"`                // 主机地址
	ClientPort        int      `json:"client_port"`         // 客户端端口
	ClusterPort       int      `json:"cluster_port"`        // 集群端口
	ClusterPortOffset int      `json:"cluster_port_offset"` // 集群端口偏移量
	ClusterName       string   `json:"cluster_name"`        // 集群名称
	SeedRoutes        []string `json:"seed_routes"`         // 种子路由
	NodeName          string   `json:"node_name"`           // 节点名称
}

type UIConfig struct {
	Theme    string `json:"theme"`
	Language string `json:"language"`
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
		LocalIP:          "127.0.0.1",
	},
	NATS: NATSConfig{
		URL:           "nats://127.0.0.1:4222",
		User:          "",
		Password:      "",
		Token:         "",
		Timeout:       5 * time.Second,
		MaxReconnect:  -1,
		ReconnectWait: time.Second,
	},
	Routes: RoutesConfig{
		Enabled:           false,
		Host:              "127.0.0.1",
		ClientPort:        4222,
		ClusterPort:       6222,
		ClusterPortOffset: 2000,
		ClusterName:       "dchat_network",
		SeedRoutes:        []string{},
		NodeName:          "dchat_node",
	},
	UI: UIConfig{
		Theme:    "dark",
		Language: "zh-CN",
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
		if err := SaveConfig(&config); err != nil {
			return &config, nil // 返回默认配置，忽略保存错误
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
		"name":           c.User.Nickname,
		"timeout":        c.NATS.Timeout,
		"max_reconnect":  c.NATS.MaxReconnect,
		"reconnect_wait": c.NATS.ReconnectWait,
	}
}

// GetRoutesConfig 获取Routes集群配置
func (c *Config) GetRoutesConfig() map[string]interface{} {
	return map[string]interface{}{
		"enabled":             c.Routes.Enabled,
		"host":                c.Routes.Host,
		"client_port":         c.Routes.ClientPort,
		"cluster_port":        c.Routes.ClusterPort,
		"cluster_port_offset": c.Routes.ClusterPortOffset,
		"cluster_name":        c.Routes.ClusterName,
		"seed_routes":         c.Routes.SeedRoutes,
		"node_name":           c.Routes.NodeName,
	}
}

// GetClusterConfig 获取集群配置用于ClusterManager
func (c *Config) GetClusterConfig() map[string]interface{} {
	return map[string]interface{}{
		"host":                c.Routes.Host,
		"cluster_port_offset": c.Routes.ClusterPortOffset,
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
func (c *Config) EnableRoutes(host string, clientPort int, seedRoutes []string) {
	c.Routes.Enabled = true
	c.Routes.Host = host
	c.Routes.ClientPort = clientPort
	c.Routes.ClusterPort = clientPort + c.Routes.ClusterPortOffset
	c.Routes.SeedRoutes = seedRoutes
}
