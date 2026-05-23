package auth

// TokenAuth 基于静态白名单验证 device_id。
type TokenAuth struct {
	tokens map[string]bool // device_id -> true
}

// NewTokenAuth 从 device_id 列表创建白名单认证器。
func NewTokenAuth(tokens []string) *TokenAuth {
	set := make(map[string]bool, len(tokens))
	for _, id := range tokens {
		set[id] = true
	}
	return &TokenAuth{tokens: set}
}

// Authenticate 检查 device_id 是否在白名单中。
func (t *TokenAuth) Authenticate(deviceID string) (bool, error) {
	return t.tokens[deviceID], nil
}
