# xlip-relay Agent Guidelines

## 项目概述

xlip-relay 是 xlip 剪贴板同步服务的中继服务器，负责在多设备间路由加密剪贴板数据。Relay **不存储密钥、不解密内容**，仅做密文转发和设备在线状态管理。

- 语言: Go 1.26+
- 外部依赖: `github.com/coder/websocket`、`github.com/pelletier/go-toml/v2`、`github.com/hashicorp/golang-lru/v2`
- 单二进制部署

## 构建 & 运行

```bash
# 构建
cd xlip-relay && go build -o relay .

# 运行（默认读取 config.toml，监听 :8080）
go run main.go
go run main.go -config /path/to/config.toml
go run main.go -addr :9090

# 代码检查
go vet ./...
go test ./...
```

## 架构

```
main.go              入口，配置加载，信号处理，优雅关闭
internal/
  config/            TOML 配置文件解析，结构体定义
  auth/              认证模块：接口 + 三种实现（none/tokens/webhook）
    auth.go          Authenticator 接口 + 工厂函数
    tokens.go        静态 device_id 白名单认证
    webhook.go       Webhook 动态认证 + LRU 缓存
  server/            HTTP → WebSocket 升级，WS 升级前认证，创建 Client
  client/            单个 WebSocket 连接的读写协程（readPump / writePump），Ping/Pong 心跳
  hub/               同步组管理，设备注册/注销，消息广播
  protocol/          消息类型定义，两步 JSON 反序列化
```

核心流程: `Server.handleWebSocket` → WS 升级前认证 device_id → `Client.Run` → `readPump` 解码消息 → `Hub` 路由/广播 → `writePump` 写出 + Ping 心跳。

## 配置文件

配置文件默认为 `config.toml`，通过 `-config` flag 指定路径。配置文件不存在时以默认值（mode=none）运行。

```toml
[server]
addr = ":8080"

[auth]
mode = "none"    # "tokens" | "webhook" | "none"

# 静态白名单模式
# [auth.tokens]
# "device-id-xxx" = "Home Laptop"

# Webhook 动态验证模式
# [auth.webhook]
# url = "https://auth.xlip.com/api/check-device"
# cache_ttl = 300
# timeout = 3
```

### 认证模式

| 模式      | 说明                                  | 适用场景            |
| --------- | ------------------------------------- | ------------------- |
| `none`    | 不验证，任何客户端可连接              | 内网/开发环境       |
| `tokens`  | 静态 device_id 白名单                 | 自部署、个人/小团队 |
| `webhook` | HTTP 请求 Auth Server 验证 + LRU 缓存 | 官方托管、商业化    |

客户端连接时通过 URL query 传递 `device_id`：`ws://relay:8080/ws?device_id=xxx`。认证在 WS 升级阶段完成，失败返回 HTTP 401。

## 认证流程

```
客户端连接 ws://relay:8080/ws?device_id=device-uuid
  │
  ├─ Server 从 URL query 提取 device_id
  ├─ 调用 Authenticator.Authenticate(device_id)
  │   ├─ none → 直接通过
  │   ├─ tokens → 查内存白名单 map
  │   └─ webhook → 查 LRU 缓存 → 未命中则 POST Auth Server
  │
  ├─ 认证失败 → HTTP 401，不升级 WS
  └─ 认证成功 → 升级 WS → Client 接收 device_id
      │
      └─ join_group 时校验 msg.DeviceID == ws_auth_deviceID
         ├─ 一致 → 正常注册
         └─ 不一致 → error + 断开
```

## 心跳

- Relay 每 30s 向客户端发送 WebSocket Ping 帧
- 60s 无任何消息（含 Pong）判定离线 → 广播 device_offline
- `coder/websocket` 库自动处理 Pong 响应

## 消息协议

协议详见 [doc/protocol.md](../doc/protocol.md)和[doc/sync-architecture.md](../doc/sync-architecture.md)。Relay 处理的消息类型：

| type              | 方向                  | 说明                         |
| ----------------- | --------------------- | ---------------------------- |
| `join_group`      | Client → Relay        | 注册到同步组                 |
| `join_group_resp` | Relay → Client        | 返回已知设备和最新剪贴板缓存 |
| `clipboard_sync`  | Client → Relay → 同组 | 透传加密剪贴板内容           |
| `device_online`   | Relay → 同组          | 广播设备上线                 |
| `device_offline`  | Relay → 同组          | 广播设备下线                 |
| `error`           | Relay → Client        | 错误响应                     |

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
- `join_group` 中的 `device_id` 必须与 WS 升级阶段 URL query 中的 `device_id` 一致
- Webhook 认证的 LRU 缓存容量为 256 条，过期判断在 `Authenticate` 中完成
