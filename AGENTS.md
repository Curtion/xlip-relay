# xlip-relay Agent Guidelines

xlip 剪贴板同步中继服务器。**不存储密钥、不解密内容**, 负责密文转发、设备在线状态及每组最新消息的内存缓存, 加入组时返回缓存。

## 构建与配置

Go 版本及依赖以 [go.mod](go.mod) 为准。在仓库根目录执行：

```bash
go build -o relay .
go run . -config config.toml
go vet ./... && go test ./...
```

- 默认读取工作目录的 [config.toml](config.toml), 随仓库配置监听 `:19090`；文件不存在时使用内置默认值 `:8080`、`mode=none`。
- 修改配置或认证模式时查阅 [config.toml](config.toml) 和 [config.go](internal/config/config.go)；`max_message_size` 可配置, 心跳和缓冲区参数见 [client.go](internal/client/client.go)。
- `LOG_LEVEL` 支持 `debug/info/warn/error`, 默认 `info`。

## 开发约束

- 使用 `log/slog`, 由 `internal/logging.Init()` 初始化；致命错误用 `logging.Fatal`, 不用标准库 `log`。
- 注释和日志使用中文, 括号和逗号使用半角, 逗号后加空格。
- 消息先解析 `type`, 再解码具体结构；格式错误返回 `invalid_message` 且不断开, IO 错误结束连接。协议定义和错误码见 [message.go](internal/protocol/message.go)。
- `Hub` 用 `sync.RWMutex` 保护索引, 广播先快照接收者再释放锁；Client 读写分离。
- 广播和错误响应在发送队列满时静默丢弃；加入组响应目前为阻塞发送。

## 认证与路由

- `/ws?device_id=...` 在 WebSocket 升级前认证：缺少 ID 或拒绝返回 HTTP 401, 认证器内部错误返回 HTTP 503。
- `join_group.device_id` 必须匹配连接身份, 否则尝试发送错误后断开。
- 成功解码的非 `join_group` 消息必须先加入组, 否则返回 `auth_failed`；非文本帧直接忽略。
- `clipboard_sync` 的组 ID 与连接所属组不一致时静默丢弃。
- 未知认证模式目前会回退到 `none`, 配置拼写错误不会阻止启动。

