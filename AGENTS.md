# xlip-relay Agent Guidelines

## 项目概述

xlip-relay 是 xlip 剪贴板同步的中继服务器,在多设备间路由加密剪贴板数据。Relay **不存储密钥、不解密内容**,仅做密文转发和设备在线状态管理。

- 语言: Go 1.26+
- 外部依赖:
  - `github.com/coder/websocket`
  - `github.com/pelletier/go-toml/v2`
  - `github.com/hashicorp/golang-lru/v2`
- 日志: `log/slog` 结构化日志,通过 `internal/logging` 初始化

## 构建 & 运行

```bash
cd xlip-relay && go build -o relay .
go run main.go [-config config.toml]   # 默认监听 :8080,无配置文件时以 mode=none 运行
go vet ./... && go test ./...
```

`LOG_LEVEL` 环境变量控制日志级别(debug/info/warn/error),默认 `info`。

## 架构

```
internal/
  config/      TOML 配置解析
  logging/     slog logger 初始化 + Fatal 封装
  auth/        认证接口 + none/tokens/webhook 三种实现
  server/      HTTP → WebSocket 升级(升级前认证)
  client/      单 WS 连接读写协程 + Ping/Pong 心跳
  hub/         同步组管理 + 消息广播
  protocol/    消息类型定义 + 两步 JSON 反序列化
```

核心流程: `Server.handleWebSocket` → WS 升级前认证 device_id → `Client.Run` → `readPump` 解码 → `Hub` 路由/广播 → `writePump` 写出 + Ping 心跳。

消息协议详见 [doc/protocol.md](../doc/protocol.md) 和 [doc/sync-architecture.md](../doc/sync-architecture.md)。

## 配置

配置项与认证模式(none/tokens/webhook)的完整说明见 [config.toml](config.toml)(含逐行注释)。通过 `-config` flag 指定路径。

## 认证

客户端通过 URL query 传递 device_id(`ws://relay:8080/ws?device_id=xxx`),认证在 WS 升级阶段完成:

- `device_id` 缺失或认证拒绝(`allowed=false`)→ HTTP 401,不升级 WS
- 认证器内部错误(如 Webhook 调用失败)→ HTTP 503
- `join_group` 中的 `device_id` 必须与 URL query 中的 device_id 一致,否则 error + 断开

## 编码规范

- 使用 `log/slog`(由 `internal/logging.Init()` 统一初始化);fatal 退出用 `logging.Fatal`,禁止标准库 `log`
- 中文注释和日志
- 错误处理: 格式错误不断开(`return nil`),IO 错误断开(`return`)
- 并发安全: `Hub` 用 `sync.RWMutex`,广播时先快照接收者列表再释放锁
- Client 读写分离: `readPump` 关闭 `send` channel 通知 `writePump` 退出
- 发送 channel 满时静默丢弃(`select { case c.send <- data: default: }`)
- 消息反序列化两步法: 先提取 `type` 字段,再 switch 到具体 struct

## 注意事项

- 客户端必须先 `join_group` 才能发送其他消息,否则返回 `auth_failed`
- `clipboard_sync` 校验 `ci.GroupID == msg.GroupID`,防止跨组路由
- 设备重连时 `Hub.Register` 自动将其从旧组移除再注册到新组
- `hub.ClientInfo.Send` 是只写 channel(`chan<- []byte`),Client 内部用双向 channel 暴露给 Hub
- 错误码常量集中在 `internal/protocol/message.go`:`auth_failed` / `invalid_message` / `internal_error`
- 心跳参数(Ping 间隔/Pong 超时/缓冲区大小)、WebSocket 读限制(`max_message_size`)均由对应代码常量定义,具体值见源文件
