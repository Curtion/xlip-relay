package server

import (
	"context"
	"log"
	"net/http"

	"github.com/coder/websocket"

	"xlip-relay/internal/auth"
	"xlip-relay/internal/client"
	"xlip-relay/internal/config"
	"xlip-relay/internal/hub"
)

// Server 是 Relay HTTP/WebSocket 服务。
type Server struct {
	hub           *hub.Hub
	httpServer    *http.Server
	authenticator auth.Authenticator
}

// New 创建一个监听指定地址的 Server。
func New(cfg *config.Config) *Server {
	h := hub.New()
	authenticator := auth.NewAuthenticator(&cfg.Auth)

	s := &Server{
		hub:           h,
		authenticator: authenticator,
		httpServer: &http.Server{
			Addr: cfg.Server.Addr,
		},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", s.handleWebSocket)
	s.httpServer.Handler = mux

	return s
}

// Start 启动 HTTP 服务，阻塞直到服务退出。
func (s *Server) Start() error {
	log.Printf("relay 正在监听 %s", s.httpServer.Addr)
	return s.httpServer.ListenAndServe()
}

// Shutdown 优雅关闭服务。
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

// handleWebSocket 将 HTTP 连接升级为 WebSocket 并启动客户端读写协程。
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	// 从 URL query 提取 device_id。
	deviceID := r.URL.Query().Get("device_id")
	if deviceID == "" {
		http.Error(w, "missing device_id parameter", http.StatusUnauthorized)
		return
	}

	// WS 升级前认证。
	allowed, err := s.authenticator.Authenticate(deviceID)
	if err != nil {
		log.Printf("认证失败 device_id=%s: %v", deviceID, err)
		http.Error(w, "authentication error", http.StatusServiceUnavailable)
		return
	}
	if !allowed {
		log.Printf("认证拒绝 device_id=%s", deviceID)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"*"},
	})
	if err != nil {
		log.Printf("websocket accept error: %v", err)
		return
	}

	c := client.New(conn, s.hub, deviceID)
	go c.Run(context.Background())
}
