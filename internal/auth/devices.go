package auth

// DeviceAuth 基于静态白名单验证 device_id。
type DeviceAuth struct {
	devices map[string]bool // device_id -> true
}

// NewDeviceAuth 从 device_id 列表创建白名单认证器。
func NewDeviceAuth(devices []string) *DeviceAuth {
	set := make(map[string]bool, len(devices))
	for _, id := range devices {
		set[id] = true
	}
	return &DeviceAuth{devices: set}
}

// Authenticate 检查 device_id 是否在白名单中。
func (t *DeviceAuth) Authenticate(deviceID string) (bool, error) {
	return t.devices[deviceID], nil
}
