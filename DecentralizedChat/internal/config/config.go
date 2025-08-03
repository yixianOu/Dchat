package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	User    UserConfig    `json:"user"`
	Network NetworkConfig `json:"network"`
	NATS    NATSConfig    `json:"nats"`
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
}

type NATSConfig struct {
	ClientPort  int    `json:"client_port"`
	ClusterPort int    `json:"cluster_port"`
	ClusterName string `json:"cluster_name"`
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
	},
	NATS: NATSConfig{
		ClientPort:  4222,
		ClusterPort: 6222,
		ClusterName: "dchat_network",
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
