package auth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"

	"xlip-relay/internal/config"
	"xlip-relay/internal/logging"
)

// cacheEntry 是 LRU 缓存中的条目。
type cacheEntry struct {
	allowed  bool
	expireAt time.Time
}

// webhookRequest 是发往 Auth Server 的请求体。
type webhookRequest struct {
	DeviceID string `json:"device_id"`
}

// webhookResponse 是 Auth Server 返回的响应体。
type webhookResponse struct {
	Allowed bool `json:"allowed"`
}

// WebhookAuth 通过 HTTP 请求 Auth Server 验证 device_id。
type WebhookAuth struct {
	url      string
	cacheTTL time.Duration
	client   *http.Client
	cache    *lru.Cache[string, *cacheEntry]
}

// NewWebhookAuth 创建 Webhook 认证器。
func NewWebhookAuth(cfg config.WebhookConfig) *WebhookAuth {
	cache, err := lru.New[string, *cacheEntry](256)
	if err != nil {
		logging.Fatal(fmt.Sprintf("创建 LRU 缓存失败：%v", err))
	}

	return &WebhookAuth{
		url:      cfg.URL,
		cacheTTL: time.Duration(cfg.CacheTTL) * time.Second,
		client: &http.Client{
			Timeout: time.Duration(cfg.Timeout) * time.Second,
		},
		cache: cache,
	}
}

// Authenticate 通过 Webhook 验证 device_id。
// 先查 LRU 缓存，未命中则请求 Auth Server。
func (w *WebhookAuth) Authenticate(deviceID string) (bool, error) {
	if entry, ok := w.cache.Get(deviceID); ok {
		if time.Now().Before(entry.expireAt) {
			return entry.allowed, nil
		}
		w.cache.Remove(deviceID)
	}

	reqBody := webhookRequest{DeviceID: deviceID}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return false, fmt.Errorf("序列化请求体失败: %w", err)
	}

	resp, err := w.client.Post(w.url, "application/json", bytes.NewReader(body))
	if err != nil {
		return false, fmt.Errorf("请求 Auth Server 失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("Auth Server 返回状态码 %d", resp.StatusCode)
	}

	respData, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Errorf("读取 Auth Server 响应失败: %w", err)
	}

	var result webhookResponse
	if err := json.Unmarshal(respData, &result); err != nil {
		return false, fmt.Errorf("解析 Auth Server 响应失败: %w", err)
	}

	w.cache.Add(deviceID, &cacheEntry{
		allowed:  result.Allowed,
		expireAt: time.Now().Add(w.cacheTTL),
	})

	return result.Allowed, nil
}
