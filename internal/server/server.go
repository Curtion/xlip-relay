package server

import (
	"context"
	"log"
	"net/http"

	"github.com/coder/websocket"

	"xlip-relay/internal/client"
	"xlip-relay/internal/hub"
)

// Server 是 Relay HTTP/WebSocket 服务。
type Server struct {
	hub        *hub.Hub
	httpServer *http.Server
}

// New 创建一个监听指定地址的 Server。
func New(addr string) *Server {
	h := hub.New()

	s := &Server{
		hub: h,
		httpServer: &http.Server{
			Addr: addr,
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
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"*"},
	})
	if err != nil {
		log.Printf("websocket accept error: %v", err)
		return
	}

	c := client.New(conn, s.hub)
	go c.Run(r.Context())
}
