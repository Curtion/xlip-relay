package hub

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"

	"xlip-relay/internal/protocol"
)

// Hub 管理同步组和已连接的客户端。
type Hub struct {
	mu      sync.RWMutex
	groups  map[string]*Group      // groupID -> Group
	clients map[string]*ClientInfo // deviceID -> ClientInfo
}

// Group 表示一个同步组，包含在线设备列表和最新剪贴板缓存。
type Group struct {
	id              string
	devices         map[string]*ClientInfo // deviceID -> ClientInfo
	latestClipboard json.RawMessage        // 原始 clipboard_sync JSON
}

// ClientInfo 保存 Hub 所需的客户端最小信息。
type ClientInfo struct {
	DeviceID   string
	DeviceName string
	GroupID    string
	Send       chan<- []byte // 只写 channel，用于向客户端推送消息
}

// New 创建一个新的 Hub。
func New() *Hub {
	return &Hub{
		groups:  make(map[string]*Group),
		clients: make(map[string]*ClientInfo),
	}
}

// Register 将客户端添加到同步组。
// 向客户端发送 join_group_resp，并向同组其他设备广播 device_online。
func (h *Hub) Register(ci *ClientInfo, msg protocol.JoinGroupMsg) {
	// 更新客户端信息。
	ci.DeviceID = msg.DeviceID
	ci.DeviceName = msg.DeviceName
	ci.GroupID = msg.GroupID

	h.mu.Lock()
	g, ok := h.groups[msg.GroupID]
	if !ok {
		g = &Group{
			id:      msg.GroupID,
			devices: make(map[string]*ClientInfo),
		}
		h.groups[msg.GroupID] = g
	}

	// 若该设备之前已注册（重连），从旧组中移除。
	if old, exists := h.clients[msg.DeviceID]; exists && old.GroupID != "" {
		if oldGroup, ok := h.groups[old.GroupID]; ok {
			delete(oldGroup.devices, msg.DeviceID)
			if len(oldGroup.devices) == 0 {
				delete(h.groups, old.GroupID)
			}
		}
	}

	g.devices[msg.DeviceID] = ci
	h.clients[msg.DeviceID] = ci
	latestClipboard := g.latestClipboard
	h.mu.Unlock()

	// 构建 join_group_resp。
	knownDevices := h.getOtherDevices(msg.GroupID, msg.DeviceID)

	resp := protocol.JoinGroupRespMsg{
		Type:            protocol.TypeJoinGroupResp,
		GroupID:         msg.GroupID,
		Timestamp:       protocol.NowMillis(),
		KnownDevices:    knownDevices,
		LatestClipboard: latestClipboard,
	}
	respBytes, err := json.Marshal(resp)
	if err != nil {
		slog.Warn(fmt.Sprintf("设备「%s」加入组「%s」响应序列化失败(理论不应发生)：%v", msg.DeviceID, msg.GroupID, err))
		return
	}
	ci.Send <- respBytes

	// 向同组其他设备广播 device_online。
	onlineMsg := protocol.DeviceOnlineMsg{
		Type:       protocol.TypeDeviceOnline,
		GroupID:    msg.GroupID,
		DeviceID:   msg.DeviceID,
		DeviceName: msg.DeviceName,
		Timestamp:  protocol.NowMillis(),
	}
	onlineBytes, err := json.Marshal(onlineMsg)
	if err != nil {
		slog.Warn(fmt.Sprintf("广播设备「%s」上线消息序列化失败(理论不应发生)：%v", msg.DeviceID, err))
		return
	}
	h.broadcastToGroup(msg.GroupID, msg.DeviceID, onlineBytes)
}

// Unregister 将客户端从组中移除并广播 device_offline。
func (h *Hub) Unregister(ci *ClientInfo) {
	if ci.DeviceID == "" {
		return
	}

	h.mu.Lock()
	g, ok := h.groups[ci.GroupID]
	if !ok {
		delete(h.clients, ci.DeviceID)
		h.mu.Unlock()
		return
	}

	delete(g.devices, ci.DeviceID)
	delete(h.clients, ci.DeviceID)
	groupID := ci.GroupID
	if len(g.devices) == 0 {
		delete(h.groups, ci.GroupID)
	}
	h.mu.Unlock()

	offlineMsg := protocol.DeviceOfflineMsg{
		Type:      protocol.TypeDeviceOffline,
		GroupID:   groupID,
		DeviceID:  ci.DeviceID,
		Timestamp: protocol.NowMillis(),
	}
	offlineBytes, err := json.Marshal(offlineMsg)
	if err != nil {
		slog.Warn(fmt.Sprintf("广播设备「%s」离线消息序列化失败(理论不应发生)：%v", ci.DeviceID, err))
		return
	}
	h.broadcastToGroup(groupID, ci.DeviceID, offlineBytes)
}

// RouteClipboardSync 缓存原始消息并广播给同组其他设备。
// rawMsg 是从客户端收到的原始 JSON 字节（用于透传）。
func (h *Hub) RouteClipboardSync(ci *ClientInfo, msg protocol.ClipboardSyncMsg, rawMsg []byte) {
	if ci.GroupID != msg.GroupID {
		return
	}

	h.mu.Lock()
	g, ok := h.groups[msg.GroupID]
	if !ok {
		h.mu.Unlock()
		return
	}
	g.latestClipboard = json.RawMessage(rawMsg)
	h.mu.Unlock()

	h.broadcastToGroup(msg.GroupID, msg.DeviceID, rawMsg)
}

// getOtherDevices 返回组内除发送者外所有设备的 DeviceInfo。
func (h *Hub) getOtherDevices(groupID, senderID string) []protocol.DeviceInfo {
	h.mu.RLock()
	defer h.mu.RUnlock()

	g, ok := h.groups[groupID]
	if !ok {
		return nil
	}

	devices := make([]protocol.DeviceInfo, 0, len(g.devices)-1)
	for _, ci := range g.devices {
		if ci.DeviceID == senderID {
			continue
		}
		devices = append(devices, protocol.DeviceInfo{
			DeviceID:   ci.DeviceID,
			DeviceName: ci.DeviceName,
		})
	}
	return devices
}

// broadcastToGroup 向组内除发送者外的所有设备发送 payload。
// 发送 channel 已满的设备会被静默跳过。
func (h *Hub) broadcastToGroup(groupID, senderID string, payload []byte) {
	h.mu.RLock()
	g, ok := h.groups[groupID]
	if !ok {
		h.mu.RUnlock()
		return
	}
	// 快照接收者列表，避免在发送时持锁。
	recipients := make([]chan<- []byte, 0, len(g.devices))
	for _, ci := range g.devices {
		if ci.DeviceID == senderID {
			continue
		}
		recipients = append(recipients, ci.Send)
	}
	h.mu.RUnlock()

	for _, ch := range recipients {
		select {
		case ch <- payload:
		default:
			// Drop message for slow clients.
		}
	}
}
