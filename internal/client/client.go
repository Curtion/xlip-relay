package client

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/coder/websocket"

	"xlip-relay/internal/hub"
	"xlip-relay/internal/protocol"
)

const (
	PongTimeout  = 30 * time.Second
	PingInterval = 30 * time.Second
	SendBufSize  = 64
)

// Client 表示一个 WebSocket 连接。
type Client struct {
	conn     *websocket.Conn
	hub      *hub.Hub
	info     hub.ClientInfo
	send     chan []byte
	deviceID string // WS 升级阶段认证的 device_id
	joined   bool   // 首次 join_group 处理后置为 true
}

// New 为给定的 WebSocket 连接创建 Client。
func New(conn *websocket.Conn, h *hub.Hub, deviceID string) *Client {
	c := &Client{
		conn:     conn,
		hub:      h,
		send:     make(chan []byte, SendBufSize),
		deviceID: deviceID,
	}
	c.info.Send = c.send
	return c
}

// Run 启动读写协程，阻塞直到连接关闭。
func (c *Client) Run(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	done := make(chan struct{})

	go func() {
		c.writePump(ctx)
		close(done)
	}()

	c.readPump(ctx)
	cancel()
	<-done

	// 清理：如果已注册，从 Hub 中注销。
	if c.joined {
		c.hub.Unregister(&c.info)
	}

	c.conn.CloseNow()
}

// displayDevice 返回用于日志的设备标识字符串。
func (c *Client) displayDevice() string {
	if c.joined {
		name := c.info.DeviceName
		if name == "" {
			name = c.info.DeviceID
		}
		return fmt.Sprintf("组「%s」的设备「%s」", c.info.GroupID, name)
	}
	return fmt.Sprintf("设备「%s」(未加入同步组)", c.deviceID)
}

// readPump 从 WebSocket 连接读取消息。
func (c *Client) readPump(ctx context.Context) {
	defer close(c.send)

	for {
		msgType, data, err := c.conn.Read(ctx)

		if err != nil {
			// 正常关闭或 context 取消是预期行为。
			if ctx.Err() != nil {
				return
			}
			slog.Info(fmt.Sprintf("%s 离线(读取失败：%v)", c.displayDevice(), err))
			return
		}

		// 只接受文本消息（JSON）。
		if msgType != websocket.MessageText {
			continue
		}

		if err := c.handleMessage(data); err != nil {
			slog.Warn(fmt.Sprintf("%s 消息处理失败：%v", c.displayDevice(), err))
			return
		}
	}
}

// writePump 从 send channel 取消息写入连接，同时周期发送 Ping。
func (c *Client) writePump(ctx context.Context) {
	ticker := time.NewTicker(PingInterval)
	defer ticker.Stop()

	for {
		select {
		case msg, ok := <-c.send:
			if !ok {
				// Channel 已关闭，发送关闭帧。
				c.conn.Close(websocket.StatusNormalClosure, "")
				return
			}
			writeCtx, writeCancel := context.WithTimeout(ctx, 10*time.Second)
			err := c.conn.Write(writeCtx, websocket.MessageText, msg)
			writeCancel()
			if err != nil {
				slog.Info(fmt.Sprintf("%s 推送消息失败, 已断开连接(%v)", c.displayDevice(), err))
				return
			}
		case <-ticker.C:
			pingCtx, pingCancel := context.WithTimeout(ctx, PongTimeout)
			err := c.conn.Ping(pingCtx)
			pingCancel()
			if err != nil {
				slog.Info(fmt.Sprintf("%s 长时间无响应, 已判定离线(%v)", c.displayDevice(), err))
				return
			}
		case <-ctx.Done():
			c.conn.Close(websocket.StatusNormalClosure, "")
			return
		}
	}
}

// handleMessage 处理单条收到的 JSON 消息。
func (c *Client) handleMessage(data []byte) error {
	msgType, msg, err := protocol.DecodeMessage(data)
	if err != nil {
		c.sendError(protocol.CodeInvalidMessage, err.Error())
		return nil
	}

	switch msgType {
	case protocol.TypeJoinGroup:
		joinMsg := msg.(protocol.JoinGroupMsg)
		// 校验 join_group 中的 device_id 与 WS 升级阶段认证的 device_id 一致。
		if joinMsg.DeviceID != c.deviceID {
			c.sendError(protocol.CodeAuthFailed, "device_id mismatch")
			return fmt.Errorf("device_id mismatch: join_group=%s, ws_auth=%s", joinMsg.DeviceID, c.deviceID)
		}
		c.hub.Register(&c.info, joinMsg)
		c.joined = true
		return nil

	case protocol.TypeClipboardSync:
		if !c.joined {
			c.sendError(protocol.CodeAuthFailed, "must join_group first")
			return nil
		}
		syncMsg := msg.(protocol.ClipboardSyncMsg)
		c.hub.RouteClipboardSync(&c.info, syncMsg, data)
		return nil

	default:
		// 客户端只允许发送 join_group 和 clipboard_sync。
		// 未认证前必须先发送 join_group。
		if !c.joined {
			c.sendError(protocol.CodeAuthFailed, "must join_group first")
			return nil
		}
		c.sendError(protocol.CodeInvalidMessage, "unexpected message type: "+msgType)
		return nil
	}
}

// sendError 向客户端发送错误消息。
func (c *Client) sendError(code, message string) {
	errMsg := protocol.NewErrorMsg(code, message)
	data, err := json.Marshal(errMsg)
	if err != nil {
		slog.Warn(fmt.Sprintf("%s 错误消息序列化失败(理论不应发生, 请反馈 bug)：%v", c.displayDevice(), err))
		return
	}

	select {
	case c.send <- data:
	default:
		// 发送缓冲区已满时丢弃。
	}
}
