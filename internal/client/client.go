package client

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/coder/websocket"

	"xlip-relay/internal/hub"
	"xlip-relay/internal/protocol"
)

const (
	// PongTimeout 发送 Ping 后等待 Pong 的超时时间，超时则判定连接已死。
	PongTimeout = 30 * time.Second
	// PingInterval 发送 Ping 的间隔。
	PingInterval = 30 * time.Second
	// SendBufSize 出站消息 channel 缓冲区大小。
	SendBufSize = 64
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
// deviceID 来自 WS 升级阶段的 URL query 认证。
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

// readPump 从 WebSocket 连接读取消息。
func (c *Client) readPump(ctx context.Context) {
	defer func() {
		close(c.send) // 通知 writePump 退出。
	}()

	for {
		msgType, data, err := c.conn.Read(ctx)

		if err != nil {
			// 正常关闭或 context 取消是预期行为。
			if ctx.Err() != nil {
				return
			}
			log.Printf("read error: %v", err)
			return
		}

		// 只接受文本消息（JSON）。
		if msgType != websocket.MessageText {
			continue
		}

		if err := c.handleMessage(data); err != nil {
			log.Printf("handle message error: %v", err)
			return
		}
	}
}

// writePump 从 send channel 读取消息并写入 WebSocket 连接。
// 同时每 PingInterval 发送一次 Ping 帧保持连接活跃。
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
				log.Printf("write error: %v", err)
				return
			}
		case <-ticker.C:
			pingCtx, pingCancel := context.WithTimeout(ctx, PongTimeout)
			err := c.conn.Ping(pingCtx)
			pingCancel()
			if err != nil {
				log.Printf("ping error: %v", err)
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
		return nil // 格式错误不断开连接。
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
		log.Printf("error marshaling error message: %v", err)
		return
	}

	select {
	case c.send <- data:
	default:
		// 发送缓冲区已满时丢弃。
	}
}
