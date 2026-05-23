package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"xlip-relay/internal/config"
	"xlip-relay/internal/server"
)

func main() {
	configPath := flag.String("config", "config.toml", "配置文件路径")
	addr := flag.String("addr", "", "监听地址（覆盖配置文件中的 server.addr）")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	// 命令行 flag 覆盖配置文件。
	if *addr != "" {
		cfg.Server.Addr = *addr
	}

	srv := server.New(cfg)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := srv.Start(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	sig := <-sigCh
	log.Printf("收到 %s 信号，正在关闭...", sig)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("shutdown error: %v", err)
	}
	log.Println("服务已停止")
}
