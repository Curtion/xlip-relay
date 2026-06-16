package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"xlip-relay/internal/config"
	"xlip-relay/internal/logging"
	"xlip-relay/internal/server"
)

func main() {
	logging.Init()

	configPath := flag.String("config", "config.toml", "配置文件路径")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		logging.Fatal(fmt.Sprintf("加载配置失败：%v", err))
	}

	srv := server.New(cfg)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := srv.Start(); err != nil && err != http.ErrServerClosed {
			logging.Fatal(fmt.Sprintf("中继服务发生错误, 正在退出：%v", err))
		}
	}()

	sig := <-sigCh
	slog.Info(fmt.Sprintf("收到 %s 信号, 正在关闭...", sig))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logging.Fatal(fmt.Sprintf("服务关闭出错：%v", err))
	}
	slog.Info("服务已停止")
}
