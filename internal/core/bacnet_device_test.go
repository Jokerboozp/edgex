package core

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	_ "github.com/anviod/edgeCore/internal/driver/bacnet"
	"github.com/anviod/edgeCore/internal/model"
	"github.com/stretchr/testify/require"
)

func TestBACnet_AddDeviceFromScanPayload(t *testing.T) {
	cm := NewChannelManager(nil, nil)
	channelID := "bacnet-ch"
	ch := &model.Channel{
		ID:       channelID,
		Name:     "BACnet",
		Protocol: "bacnet-ip",
		Enable:   false,
		Config:   map[string]any{},
	}
	require.NoError(t, cm.AddChannel(ch))

	raw := `[{
		"id": "bacnet-2228316",
		"name": "RoomController.Simulator",
		"interval": "10s",
		"enable": true,
		"config": {
			"bacnet_device_id": 2228316,
			"ip": "192.168.3.106",
			"port": 54103,
			"vendor_name": "Test Vendor",
			"model_name": "Room_FC_2014"
		},
		"points": []
	}]`
	var devices []model.Device
	require.NoError(t, json.Unmarshal([]byte(raw), &devices))
	for i := range devices {
		require.NoError(t, model.EnsureDeviceID(&devices[i]))
		err := cm.AddDevice(channelID, &devices[i])
		require.NoError(t, err, "AddDevice failed")
	}

	got := cm.GetChannelDevices(channelID)
	require.Len(t, got, 1)
	require.Equal(t, "bacnet-2228316", got[0].ID)
	require.Equal(t, 2228316, got[0].Config["bacnet_device_id"])
}

func TestBACnet_AddDeviceFromScanPayload_DuplicateInstance(t *testing.T) {
	cm := NewChannelManager(nil, nil)
	channelID := "bacnet-ch"
	ch := &model.Channel{
		ID:       channelID,
		Name:     "BACnet",
		Protocol: "bacnet-ip",
		Enable:   false,
		Config:   map[string]any{},
	}
	require.NoError(t, cm.AddChannel(ch))

	existing := &model.Device{
		ID:       "manual-device-name",
		Name:     "Existing Device",
		Interval: model.Duration(10 * time.Second),
		Enable:   true,
		Config: map[string]any{
			"bacnet_device_id": 2228316,
		},
	}
	require.NoError(t, cm.AddDevice(channelID, existing))

	scanDev := &model.Device{
		ID:       "bacnet-2228316",
		Name:     "Scanned Device",
		Interval: model.Duration(10 * time.Second),
		Enable:   true,
		Config: map[string]any{
			"bacnet_device_id": 2228316,
			"ip":               "192.168.3.106",
			"port":             54103,
		},
	}
	err := cm.AddDevice(channelID, scanDev)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Instance ID 2228316 already exists")
}

func TestBACnet_AddDeviceFromScanPayload_DuplicateInstance_BacnetDeviceID(t *testing.T) {
	cm := NewChannelManager(nil, nil)
	channelID := "bacnet-ch"
	ch := &model.Channel{
		ID:       channelID,
		Name:     "BACnet",
		Protocol: "bacnet-ip",
		Enable:   false,
		Config:   map[string]any{},
	}
	require.NoError(t, cm.AddChannel(ch))

	existing := &model.Device{
		ID:       "manual-device",
		Name:     "Manual Device",
		Interval: model.Duration(10 * time.Second),
		Enable:   true,
		Config: map[string]any{
			"bacnet_device_id": 2228316,
		},
	}
	require.NoError(t, cm.AddDevice(channelID, existing))

	scanDev := &model.Device{
		ID:       "bacnet-2228316",
		Name:     "Scanned Device",
		Interval: model.Duration(10 * time.Second),
		Enable:   true,
		Config: map[string]any{
			"bacnet_device_id": 2228316,
		},
	}
	err := cm.AddDevice(channelID, scanDev)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Instance ID 2228316 already exists")
}

func TestBACnet_BatchAddDevicesFromScanPayload(t *testing.T) {
	cm := NewChannelManager(nil, nil)
	channelID := "bacnet-ch"
	ch := &model.Channel{
		ID:       channelID,
		Name:     "BACnet",
		Protocol: "bacnet-ip",
		Enable:   false,
		Config:   map[string]any{},
	}
	require.NoError(t, cm.AddChannel(ch))

	for _, id := range []int{2228316, 2228317, 2228318} {
		dev := &model.Device{
			ID:       fmt.Sprintf("bacnet-%d", id),
			Name:     fmt.Sprintf("Device %d", id),
			Interval: model.Duration(10 * time.Second),
			Enable:   true,
			Config: map[string]any{
				"bacnet_device_id": id,
				"ip":               "192.168.3.106",
				"port":             47808,
			},
		}
		require.NoError(t, cm.AddDevice(channelID, dev))
	}

	got := cm.GetChannelDevices(channelID)
	require.Len(t, got, 3)
}

func TestBACnet_UpdateDevice_RejectsInstanceCollision(t *testing.T) {
	cm := NewChannelManager(nil, nil)
	channelID := "bacnet-ch"
	ch := &model.Channel{
		ID:       channelID,
		Name:     "BACnet",
		Protocol: "bacnet-ip",
		Enable:   false,
		Config:   map[string]any{},
	}
	require.NoError(t, cm.AddChannel(ch))

	// 先添加两台实例号不同的设备（2228316 / 2228317）
	for _, id := range []int{2228316, 2228317} {
		dev := &model.Device{
			ID:       fmt.Sprintf("bacnet-%d", id),
			Name:     fmt.Sprintf("Device %d", id),
			Interval: model.Duration(10 * time.Second),
			Enable:   true,
			Config: map[string]any{
				"bacnet_device_id": id,
				"ip":               "192.168.3.106",
				"port":             47808,
			},
		}
		require.NoError(t, cm.AddDevice(channelID, dev))
	}

	// 编辑 2228316，把实例号改成与 2228317 相同 —— 必须被拒绝，防止点位被合并成一台
	colliding := &model.Device{
		ID:       "bacnet-2228316",
		Name:     "Device 2228316",
		Interval: model.Duration(10 * time.Second),
		Enable:   true,
		Config: map[string]any{
			"bacnet_device_id": 2228317, // 撞了 2228317 的实例号
			"ip":               "192.168.3.106",
			"port":             47808,
		},
	}
	err := cm.UpdateDevice(channelID, colliding)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Device ID 2228317 is already in use")

	// 冲突编辑被拒绝后，两台设备结构与点位应保持完好
	got := cm.GetChannelDevices(channelID)
	require.Len(t, got, 2)
	require.Equal(t, "bacnet-2228316", got[0].ID)
	require.Equal(t, "bacnet-2228317", got[1].ID)

	// 正常编辑（保留自身实例号）仍然允许
	ok := &model.Device{
		ID:       "bacnet-2228316",
		Name:     "Renamed Device",
		Interval: model.Duration(20 * time.Second),
		Enable:   true,
		Config: map[string]any{
			"bacnet_device_id": 2228316,
			"ip":               "192.168.3.110",
			"port":             54103,
		},
	}
	require.NoError(t, cm.UpdateDevice(channelID, ok))
	got = cm.GetChannelDevices(channelID)
	require.Len(t, got, 2)
	require.Equal(t, 2228316, got[0].Config["bacnet_device_id"])
}

func TestDeviceID_GloballyUniqueAcrossChannels(t *testing.T) {
	cm := NewChannelManager(nil, nil)
	t.Cleanup(func() { cm.cancel() })

	// 直接注入两个不同协议/通道，避免依赖驱动注册
	injectChannel := func(id string) {
		cm.channels[id] = &model.Channel{
			ID:       id,
			Name:     "Channel " + id,
			Protocol: "modbus-tcp",
			Enable:   false,
			Config:   map[string]any{},
		}
	}
	injectChannel("a")
	injectChannel("b")

	devA := &model.Device{
		ID:   "shared-device",
		Name: "A",
		Config: map[string]any{
			"slave_id": 1,
			"address":  "192.168.3.10",
		},
	}
	require.NoError(t, cm.AddDevice("a", devA))

	devB := &model.Device{
		ID:   "shared-device", // 与 ch A 重复，全系统唯一约束应拒绝
		Name: "B",
		Config: map[string]any{
			"slave_id": 1,
			"address":  "192.168.3.20",
		},
	}
	err := cm.AddDevice("b", devB)
	require.Error(t, err)
	require.Contains(t, err.Error(), "already exists")

	// 确认 ch B 未被污染，仍为空
	gotB := cm.GetChannelDevices("b")
	require.Len(t, gotB, 0)
}

func TestBatchAddModbus_DeviceIDsChannelPrefixed(t *testing.T) {
	cm := NewChannelManager(nil, nil)
	t.Cleanup(func() { cm.cancel() })

	injectChannel := func(id string) {
		cm.channels[id] = &model.Channel{
			ID:       id,
			Name:     "Channel " + id,
			Protocol: "modbus-tcp",
			Enable:   false,
			Config:   map[string]any{},
		}
	}
	injectChannel("ch-a")
	injectChannel("ch-b")

	// 两个 Modbus 通道各自批量添加从站 1~2，ID 需带通道前缀以避免全局冲突
	resA, err := cm.BatchAddModbusSlaves("ch-a", 1, 2, 0, 100, model.Duration(time.Second), true, "int16", "R", model.RegHolding, 3)
	require.NoError(t, err)
	require.Len(t, resA.Created, 2)

	resB, err := cm.BatchAddModbusSlaves("ch-b", 1, 2, 0, 100, model.Duration(time.Second), true, "int16", "R", model.RegHolding, 3)
	require.NoError(t, err)
	require.Len(t, resB.Created, 2)

	gotA := cm.GetChannelDevices("ch-a")
	gotB := cm.GetChannelDevices("ch-b")
	require.Equal(t, "ch-a-modbus-slave-1", gotA[0].ID)
	require.Equal(t, "ch-b-modbus-slave-1", gotB[0].ID)
}
