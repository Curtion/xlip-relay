# xlip-relay Agent Guidelines

## 项目概述

xlip-relay 是 xlip 剪贴板同步服务的中继服务器，负责在多设备间路由加密剪贴板数据。Relay **不存储密钥、不解密内容**，仅做密文转发和设备在线状态管理。

- 语言: Go 1.26+
- 唯一外部依赖: `github.com/coder/websocket`
- 单二进制部署，零外部依赖

## 构建 & 运行

```bash
# 构建
cd xlip-relay && go build -o relay .

# 运行（默认监听 :8080）
go run main.go
go run main.go -addr :9090

# 代码检查
go vet ./...
go test ./...
```

## 架构

```
main.go              入口，信号处理，优雅关闭
internal/
  server/            HTTP → WebSocket 升级，创建 Client
  client/            单个 WebSocket 连接的读写协程（readPump / writePump）
  hub/               同步组管理，设备注册/注销，消息广播
  protocol/          消息类型定义，两步 JSON 反序列化
```

核心流程: `Server.handleWebSocket` → `Client.Run` → `readPump` 解码消息 → `Hub` 路由/广播 → `writePump` 写出。

## 消息协议

协议详见 [doc/protocol.md](../doc/protocol.md)。Relay 处理的消息类型：

| type | 方向 | 说明 |
|---|---|---|
| `join_group` | Client → Relay | 注册到同步组 |
| `join_group_resp` | Relay → Client | 返回已知设备和最新剪贴板缓存 |
| `clipboard_sync` | Client → Relay → 同组 | 透传加密剪贴板内容 |
| `device_online` | Relay → 同组 | 广播设备上线 |
| `device_offline` | Relay → 同组 | 广播设备下线 |
| `error` | Relay → Client | 错误响应 |

`clipboard_sync` 使用**原始字节透传**（不经二次序列化），通过 `json.RawMessage` 缓存并原样广播。

## 编码规范

- 标准库 `log` 做日志，中文注释和日志
- 错误处理: 格式错误不断开连接（`return nil`），IO 错误断开（`return`）
- 并发安全: `Hub` 使用 `sync.RWMutex`，广播时先快照接收者列表再释放锁
- Client 读写分离: `readPump` 关闭 `send` channel 通知 `writePump` 退出
- 发送 channel 满时静默丢弃（`select { case c.send <- data: default: }`）
- 消息反序列化: 两步法 — 先提取 `type` 字段，再 switch 到具体 struct

## 注意事项

- `hub.ClientInfo.Send` 是只写 channel（`chan<- []byte`），Client 内部用双向 channel 暴露给 Hub
- 设备重连时，`Hub.Register` 会自动将其从旧组移除再注册到新组
- `clipboard_sync` 校验 `ci.GroupID == msg.GroupID`，防止跨组路由
- 客户端必须先 `join_group` 才能发送其他消息，否则返回 `auth_failed` 错误
