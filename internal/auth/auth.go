package auth

import (
	"fmt"
	"log/slog"

	"xlip-relay/internal/config"
)

// Authenticator 验证 device_id 是否允许连接。
type Authenticator interface {
	Authenticate(deviceID string) (bool, error)
}

// noneAuth 不做任何验证，始终允许。
type noneAuth struct{}

func (noneAuth) Authenticate(string) (bool, error) { return true, nil }

// NewAuthenticator 根据配置创建对应的认证器。
func NewAuthenticator(cfg *config.AuthConfig) Authenticator {
	switch cfg.Mode {
	case "tokens":
		slog.Info(fmt.Sprintf("认证模式：静态白名单(%d 台设备)", len(cfg.Tokens)))
		return NewTokenAuth(cfg.Tokens)
	case "webhook":
		slog.Info(fmt.Sprintf("认证模式：Webhook(%s)", cfg.Webhook.URL))
		return NewWebhookAuth(cfg.Webhook)
	case "none":
		slog.Info("认证模式：无认证")
		return noneAuth{}
	default:
		slog.Warn(fmt.Sprintf("认证模式配置「%q」不支持, 已回退到无认证", cfg.Mode))
		return noneAuth{}
	}
}
