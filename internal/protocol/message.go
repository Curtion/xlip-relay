package protocol

import (
	"encoding/json"
	"fmt"
	"time"
)

const (
	TypeJoinGroup     = "join_group"
	TypeJoinGroupResp = "join_group_resp"
	TypeClipboardSync = "clipboard_sync"
	TypeDeviceOnline  = "device_online"
	TypeDeviceOffline = "device_offline"
	TypeError         = "error"
)

const (
	CodeAuthFailed     = "auth_failed"
	CodeInvalidMessage = "invalid_message"
	CodeInternalError  = "internal_error"
)

// NowMillis 返回当前毫秒级 Unix 时间戳。
func NowMillis() int64 {
	return time.Now().UnixMilli()
}

// rawType 用于第一步反序列化，提取 "type" 字段。
type rawType struct {
	Type string `json:"type"`
}

// DecodeMessage 两步反序列化：
// 1. 从原始 JSON 中提取 "type" 字段。
// 2. 根据 type 反序列化为具体的消息结构体。
// 返回 (消息类型, 具体消息, 错误)。
func DecodeMessage(data []byte) (string, any, error) {
	var rt rawType
	if err := json.Unmarshal(data, &rt); err != nil {
		return "", nil, fmt.Errorf("invalid json: %w", err)
	}

	switch rt.Type {
	case TypeJoinGroup:
		var msg JoinGroupMsg
		if err := json.Unmarshal(data, &msg); err != nil {
			return "", nil, fmt.Errorf("invalid %s message: %w", rt.Type, err)
		}
		return rt.Type, msg, nil

	case TypeClipboardSync:
		var msg ClipboardSyncMsg
		if err := json.Unmarshal(data, &msg); err != nil {
			return "", nil, fmt.Errorf("invalid %s message: %w", rt.Type, err)
		}
		return rt.Type, msg, nil

	case TypeJoinGroupResp:
		var msg JoinGroupRespMsg
		if err := json.Unmarshal(data, &msg); err != nil {
			return "", nil, fmt.Errorf("invalid %s message: %w", rt.Type, err)
		}
		return rt.Type, msg, nil

	case TypeDeviceOnline:
		var msg DeviceOnlineMsg
		if err := json.Unmarshal(data, &msg); err != nil {
			return "", nil, fmt.Errorf("invalid %s message: %w", rt.Type, err)
		}
		return rt.Type, msg, nil

	case TypeDeviceOffline:
		var msg DeviceOfflineMsg
		if err := json.Unmarshal(data, &msg); err != nil {
			return "", nil, fmt.Errorf("invalid %s message: %w", rt.Type, err)
		}
		return rt.Type, msg, nil

	case TypeError:
		var msg ErrorMsg
		if err := json.Unmarshal(data, &msg); err != nil {
			return "", nil, fmt.Errorf("invalid %s message: %w", rt.Type, err)
		}
		return rt.Type, msg, nil

	default:
		return "", nil, fmt.Errorf("unknown message type: %s", rt.Type)
	}
}

// JoinGroupMsg 设备上线/重连时发送，注册到同步组。
type JoinGroupMsg struct {
	Type       string `json:"type"`
	GroupID    string `json:"group_id"`
	DeviceID   string `json:"device_id"`
	DeviceName string `json:"device_name"`
	Timestamp  int64  `json:"timestamp"`
}

// JoinGroupRespMsg Relay 响应 join_group，返回已知设备列表和最新剪贴板。
type JoinGroupRespMsg struct {
	Type            string          `json:"type"`
	GroupID         string          `json:"group_id"`
	Timestamp       int64           `json:"timestamp"`
	KnownDevices    []DeviceInfo    `json:"known_devices"`
	LatestClipboard json.RawMessage `json:"latest_clipboard"`
}

// ClipboardSyncMsg 加密的剪贴板内容同步，Relay 透传给同组设备。
type ClipboardSyncMsg struct {
	Type      string `json:"type"`
	GroupID   string `json:"group_id"`
	DeviceID  string `json:"device_id"`
	Timestamp int64  `json:"timestamp"`
	Payload   string `json:"payload"`
	Nonce     string `json:"nonce"`
}

// DeviceOnlineMsg 设备上线时广播给同组其他设备。
type DeviceOnlineMsg struct {
	Type       string `json:"type"`
	GroupID    string `json:"group_id"`
	DeviceID   string `json:"device_id"`
	DeviceName string `json:"device_name"`
	Timestamp  int64  `json:"timestamp"`
}

// DeviceOfflineMsg 设备下线（断开或心跳超时）时广播给同组其他设备。
type DeviceOfflineMsg struct {
	Type      string `json:"type"`
	GroupID   string `json:"group_id"`
	DeviceID  string `json:"device_id"`
	Timestamp int64  `json:"timestamp"`
}

// ErrorMsg Relay 返回的错误消息。
type ErrorMsg struct {
	Type      string `json:"type"`
	Code      string `json:"code"`
	Message   string `json:"message"`
	Timestamp int64  `json:"timestamp"`
}

// DeviceInfo 组内已知设备信息。
type DeviceInfo struct {
	DeviceID   string `json:"device_id"`
	DeviceName string `json:"device_name"`
}

// NewErrorMsg 创建一条带当前时间戳的错误消息。
func NewErrorMsg(code, message string) ErrorMsg {
	return ErrorMsg{
		Type:      TypeError,
		Code:      code,
		Message:   message,
		Timestamp: NowMillis(),
	}
}
