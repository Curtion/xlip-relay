# Xlip-relay

[Xlip](https://Xlip.3gxk.net) 剪贴板同步的中继服务器, 负责在多设备间路由**端到端加密**的剪贴板数据。

Relay **不存储密钥、不解密内容**, 只做密文转发和设备在线状态管理。即使 Relay 运维方也无法读取任何剪贴板内容。

## 工作原理

```
Device A (Xlip) ──WSS/E2EE──► Relay (纯路由) ──WSS/E2EE──► Device B (Xlip)
                 密文上行      不存密钥/只转发      密文下行
```

客户端通过 WebSocket 连接 `/ws?device_id=xxx`, 先 `join_group` 加入同步组, 之后同组设备间实时互转 `clipboard_sync` 密文消息。

## 特性

- **纯密文路由**: AES-256-GCM 密文转发, Relay 无法解密
- **单二进制零依赖**: Go 编写, 静态编译, 裸机直接运行
- **三种认证模式**: `none`(开放) / `devices`(静态白名单) / `webhook`(动态验证)
- **多设备同步组**: 支持 N 台设备组成一个同步组, 实时广播
- **心跳保活**: Ping/Pong 自动维持长连接, 断线自动清理

## 安装与运行

### 方式一: 从源码构建

需要 Go 1.26+:

```bash
git clone https://github.com/Curtion/Xlip-relay.git
cd Xlip-relay
go build -o relay .
./relay                    # 默认监听 :19090, 无配置文件时以 mode=none 运行
./relay -config config.toml
```

### 方式二: Docker

```bash
docker build -f build/Dockerfile -t Xlip-relay .
docker run -d --name Xlip-relay \
  -p 19090:19090 \
  -v $(pwd)/config.toml:/etc/Xlip-relay/config.toml:ro \
  Xlip-relay
```

### 方式三: Docker Compose

```bash
cd deployments
# 按需要修改同目录下的 config.toml
docker compose up -d
```

## 配置

配置为 TOML 格式, 完整逐行注释见 [config.toml](config.toml):

```toml
[server]
addr = ":19090"

[auth]
mode = "none" # "devices" | "webhook" | "none"

# 静态白名单模式: 允许连接的 device_id 列表 (即 Xlip 客户端设置页显示的 Device ID)
# devices = [
#   "550e8400-e29b-41d4-a716-446655440000",    # Home Laptop
# ]

# Webhook 动态验证模式
# [auth.webhook]
# url = "https://example.com/api/check-device"
# cache_ttl = 300
# timeout = 3
```


## Webhook 认证协议

`mode = "webhook"` 时, Relay 会在 WebSocket 升级前向配置的 `url` 发起请求, 由 Auth Server 决定 device_id 是否允许接入。

请求 (Relay → Auth Server):

```http
POST /api/check-device HTTP/1.1
Content-Type: application/json

{"device_id": "550e8400-e29b-41d4-a716-446655440000"}
```

响应 (Auth Server → Relay): 必须返回 HTTP 200, 正文为 JSON:

```json
{"allowed": true}
```

判定规则:

- `allowed: false` → Relay 向客户端返回 HTTP 401, 不升级 WebSocket
- 非 200 状态码或请求失败/超时 → Relay 向客户端返回 HTTP 503
- 验证结果 (含拒绝) 会按 `cache_ttl` 缓存, 期间同一 device_id 不再请求 Auth Server; `cache_ttl = 0` 表示禁用缓存, 每次连接都实时验证

## 客户端接入

在 Xlip 客户端的同步设置中填入 Relay 地址 (如 `wss://your-server:19090/ws`), 即可开始跨设备同步。iOS 设备无需安装任何软件, 借助 macOS 的通用剪贴板 (Handoff) 由 Mac 上的 Xlip 中转, 详见 [Xlip](https://Xlip.3gxk.net)。
