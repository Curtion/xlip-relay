// Package logging 提供全局日志初始化与 Fatal 封装。
package logging

import (
	"log/slog"
	"os"
	"strings"
	"time"
)

// Init 初始化全局 slog logger。
// 通过 LOG_LEVEL 环境变量控制日志级别(debug/info/warn/error), 默认 info。
func Init() {
	level := parseLevel(os.Getenv("LOG_LEVEL"))
	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level:       level,
		ReplaceAttr: replaceAttr,
	})
	slog.SetDefault(slog.New(handler))
}

// Fatal 输出 Error 级别日志后退出进程(替代标准库 log.Fatalf)。
func Fatal(msg string, args ...any) {
	slog.Error(msg, args...)
	os.Exit(1)
}

// parseLevel 将字符串解析为 slog.Level, 不区分大小写。
func parseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// replaceAttr 自定义时间格式为 2006-01-02 15:04:05, 更易读。
func replaceAttr(groups []string, a slog.Attr) slog.Attr {
	if a.Key == slog.TimeKey && len(groups) == 0 {
		if t, ok := a.Value.Any().(time.Time); ok {
			a.Value = slog.StringValue(t.Format("2006-01-02 15:04:05"))
		}
	}
	return a
}
