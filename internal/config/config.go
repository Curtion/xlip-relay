package config

import (
	"fmt"
	"os"

	"github.com/pelletier/go-toml/v2"
)

// Config 是 Relay 的完整配置。
type Config struct {
	Server ServerConfig `toml:"server"`
	Auth   AuthConfig   `toml:"auth"`
}

// ServerConfig 包含 HTTP 服务配置。
type ServerConfig struct {
	Addr string `toml:"addr"`
	// MaxMessageSize 单条 WebSocket 消息允许的最大字节数。
	MaxMessageSize int64 `toml:"max_message_size"`
}

// AuthConfig 包含认证相关配置。
type AuthConfig struct {
	Mode    string        `toml:"mode"`   // "tokens" | "webhook" | "none"
	Tokens  []string      `toml:"tokens"` // 允许连接的 device_id 白名单
	Webhook WebhookConfig `toml:"webhook"`
}

// WebhookConfig 包含 Webhook 动态认证配置。
type WebhookConfig struct {
	URL      string `toml:"url"`       // Auth Server 验证接口 URL
	CacheTTL int    `toml:"cache_ttl"` // 缓存有效期（秒），默认 300
	Timeout  int    `toml:"timeout"`   // HTTP 请求超时（秒），默认 3
}

// Default 返回默认配置。
func Default() *Config {
	return &Config{
		Server: ServerConfig{
			Addr:           ":8080",
			MaxMessageSize: 16 << 20, // 16 MiB
		},
		Auth: AuthConfig{
			Mode:   "none",
			Tokens: []string{},
			Webhook: WebhookConfig{
				CacheTTL: 300,
				Timeout:  3,
			},
		},
	}
}

// Load 从 TOML 文件加载配置。若文件不存在则返回默认配置。
func Load(path string) (*Config, error) {
	cfg := Default()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	if err := toml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	if cfg.Server.Addr == "" {
		cfg.Server.Addr = ":8080"
	}
	if cfg.Server.MaxMessageSize <= 0 {
		cfg.Server.MaxMessageSize = 16 << 20
	}
	if cfg.Auth.Webhook.CacheTTL == 0 {
		cfg.Auth.Webhook.CacheTTL = 300
	}
	if cfg.Auth.Webhook.Timeout == 0 {
		cfg.Auth.Webhook.Timeout = 3
	}
	if cfg.Auth.Tokens == nil {
		cfg.Auth.Tokens = []string{}
	}

	return cfg, nil
}
