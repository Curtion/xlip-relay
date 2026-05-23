package auth

import (
	"log"

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
		log.Printf("认证模式: 静态白名单 (%d 台设备)", len(cfg.Tokens))
		return NewTokenAuth(cfg.Tokens)
	case "webhook":
		log.Printf("认证模式: Webhook (%s)", cfg.Webhook.URL)
		return NewWebhookAuth(cfg.Webhook)
	case "none":
		log.Println("认证模式: 无认证")
		return noneAuth{}
	default:
		log.Printf("未知认证模式 %q，回退到无认证", cfg.Mode)
		return noneAuth{}
	}
}
