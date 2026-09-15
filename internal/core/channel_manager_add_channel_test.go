package core

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/driver"
	"github.com/anviod/edgeCore/internal/model"
)

const addChannelMockProtocol = "add-channel-mock"

type addChannelMockDriver struct {
	initErr error
}

func (m *addChannelMockDriver) Init(_ model.DriverConfig) error { return m.initErr }
func (m *addChannelMockDriver) Connect(_ context.Context) error { return nil }
func (m *addChannelMockDriver) Disconnect() error               { return nil }
func (m *addChannelMockDriver) ReadPoints(_ context.Context, _ []model.Point) (map[string]model.Value, error) {
	return nil, nil
}
func (m *addChannelMockDriver) WritePoint(_ context.Context, _ model.Point, _ any) error { return nil }
func (m *addChannelMockDriver) Health() driver.HealthStatus                              { return driver.HealthStatusGood }
func (m *addChannelMockDriver) SetSlaveID(_ uint8) error                                 { return nil }
func (m *addChannelMockDriver) SetDeviceConfig(_ map[string]any) error                   { return nil }
func (m *addChannelMockDriver) GetConnectionMetrics() (int64, int64, string, string, time.Time) {
	return 0, 0, "", "", time.Time{}
}

func init() {
	driver.RegisterDriver(addChannelMockProtocol, func() driver.Driver {
		return &addChannelMockDriver{}
	})
}

func TestChannelManager_AddChannel_RejectsDuplicate(t *testing.T) {
	cm := NewChannelManager(nil, nil)
	defer cm.cancel()

	ch := &model.Channel{
		ID:       "ch-dup",
		Name:     "Duplicate",
		Protocol: addChannelMockProtocol,
		Config:   map[string]any{},
	}
	if err := cm.AddChannel(ch); err != nil {
		t.Fatalf("first AddChannel: %v", err)
	}
	err := cm.AddChannel(ch)
	if err == nil {
		t.Fatal("expected duplicate channel error")
	}
}

func TestChannelManager_AddChannel_RejectsUnknownProtocol(t *testing.T) {
	cm := NewChannelManager(nil, nil)
	defer cm.cancel()

	err := cm.AddChannel(&model.Channel{
		ID:       "ch-unknown",
		Name:     "Unknown",
		Protocol: "not-a-real-protocol",
	})
	if err == nil {
		t.Fatal("expected unknown protocol error")
	}
}

func TestChannelManager_AddChannel_RejectsEmptyID(t *testing.T) {
	cm := NewChannelManager(nil, nil)
	defer cm.cancel()

	err := cm.AddChannel(&model.Channel{
		ID:       "",
		Name:     "",
		Protocol: addChannelMockProtocol,
	})
	if err == nil {
		t.Fatal("expected empty ID error")
	}
}

func TestChannelManager_AddChannel_RejectsDriverInitFailure(t *testing.T) {
	const failProtocol = "add-channel-init-fail"
	driver.RegisterDriver(failProtocol, func() driver.Driver {
		return &addChannelMockDriver{initErr: fmt.Errorf("invalid config")}
	})

	cm := NewChannelManager(nil, nil)
	defer cm.cancel()

	err := cm.AddChannel(&model.Channel{
		ID:       "ch-init-fail",
		Name:     "Init Fail",
		Protocol: failProtocol,
		Config:   map[string]any{},
	})
	if err == nil {
		t.Fatal("expected init failure error")
	}
}

func TestChannelManager_AddChannel_NilConfig(t *testing.T) {
	cm := NewChannelManager(nil, nil)
	defer cm.cancel()

	ch := &model.Channel{
		ID:       "ch-nil-config",
		Name:     "Nil Config",
		Protocol: addChannelMockProtocol,
		Config:   nil,
	}
	if err := cm.AddChannel(ch); err != nil {
		t.Fatalf("AddChannel with nil config: %v", err)
	}
}

func TestChannelManager_AddChannel_WithDevices(t *testing.T) {
	var saved []model.Channel
	saveCh := make(chan struct{}, 1)
	cm := NewChannelManager(nil, func(channels []model.Channel) error {
		saved = channels
		saveCh <- struct{}{}
		return nil
	})
	defer cm.cancel()

	ch := &model.Channel{
		ID:       "ch-with-devices",
		Name:     "With Devices",
		Protocol: addChannelMockProtocol,
		Enable:   false,
		Config:   map[string]any{},
		Devices: []model.Device{
			{
				ID:     "dev-1",
				Name:   "Device 1",
				Enable: true,
				Points: []model.Point{
					{ID: "pt-1", Name: "Point 1", Address: "0", DataType: "int16"},
				},
			},
		},
	}
	if err := cm.AddChannel(ch); err != nil {
		t.Fatalf("AddChannel: %v", err)
	}
	<-saveCh // wait for async save
	if len(saved) != 1 || len(saved[0].Devices) != 1 || len(saved[0].Devices[0].Points) != 1 {
		t.Fatalf("unexpected saved state: %+v", saved)
	}
	if _, ok := cm.drivers["ch-with-devices"]; !ok {
		t.Fatal("driver not registered")
	}
	if cm.driverMus["ch-with-devices"] == nil {
		t.Fatal("driver mutex not registered")
	}
}

func TestChannelManager_AddChannel_UsesNameAsID(t *testing.T) {
	cm := NewChannelManager(nil, nil)
	defer cm.cancel()

	ch := &model.Channel{
		Name:     "named-channel",
		Protocol: addChannelMockProtocol,
	}
	if err := cm.AddChannel(ch); err != nil {
		t.Fatalf("AddChannel: %v", err)
	}
	if ch.ID != "named-channel" {
		t.Fatalf("expected ID from name, got %q", ch.ID)
	}
}

func TestChannelManager_AddChannel_ConcurrentDuplicate(t *testing.T) {
	cm := NewChannelManager(nil, nil)
	defer cm.cancel()

	ch := &model.Channel{
		ID:       "ch-race",
		Name:     "Race",
		Protocol: addChannelMockProtocol,
	}

	var wg sync.WaitGroup
	errCh := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errCh <- cm.AddChannel(ch)
		}()
	}
	wg.Wait()
	close(errCh)

	var okCount, failCount int
	for err := range errCh {
		if err == nil {
			okCount++
		} else {
			failCount++
		}
	}
	if okCount != 1 || failCount != 1 {
		t.Fatalf("expected exactly one success and one failure, got ok=%d fail=%d", okCount, failCount)
	}
}

// ============================================================================
// 新增测试：设备 ID 冲突 / 去重 / Fail-Fast / UpdateChannel
// ============================================================================

// callTrackerDriver 追踪 Init/Disconnect 调用次数，用于验证 Fail-Fast。
type callTrackerDriver struct {
	initCount       int
	disconnectCount int
	initErr         error
	mu              sync.Mutex
}

func (m *callTrackerDriver) Init(_ model.DriverConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.initCount++
	return m.initErr
}
func (m *callTrackerDriver) Connect(_ context.Context) error { return nil }
func (m *callTrackerDriver) Disconnect() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.disconnectCount++
	return nil
}
func (m *callTrackerDriver) ReadPoints(_ context.Context, _ []model.Point) (map[string]model.Value, error) {
	return nil, nil
}
func (m *callTrackerDriver) WritePoint(_ context.Context, _ model.Point, _ any) error { return nil }
func (m *callTrackerDriver) Health() driver.HealthStatus                              { return driver.HealthStatusGood }
func (m *callTrackerDriver) SetSlaveID(_ uint8) error                                 { return nil }
func (m *callTrackerDriver) SetDeviceConfig(_ map[string]any) error                   { return nil }
func (m *callTrackerDriver) GetConnectionMetrics() (int64, int64, string, string, time.Time) {
	return 0, 0, "", "", time.Time{}
}

// 注册追踪 driver 作为一个共享实例，方便测试断言调用次数。
var (
	trackerDriverInstance *callTrackerDriver
	trackerDriverOnce     sync.Once
)

func ensureTrackerDriverProtocol() string {
	const proto = "add-channel-tracker"
	trackerDriverOnce.Do(func() {
		trackerDriverInstance = &callTrackerDriver{}
		driver.RegisterDriver(proto, func() driver.Driver {
			// 每次 AddChannel 拿到同一份 tracker，计数能跨调用累积
			return trackerDriverInstance
		})
	})
	trackerDriverInstance.mu.Lock()
	trackerDriverInstance.initCount = 0
	trackerDriverInstance.disconnectCount = 0
	trackerDriverInstance.mu.Unlock()
	return proto
}

// Test_AddChannel_RejectsDeviceIDGlobalConflict 跨通道设备 ID 冲突
func TestChannelManager_AddChannel_RejectsDeviceIDGlobalConflict(t *testing.T) {
	proto := ensureTrackerDriverProtocol()
	cm := NewChannelManager(nil, nil)
	defer cm.cancel()

	// 先加一个通道，设备 dev-shared
	if err := cm.AddChannel(&model.Channel{
		ID:       "ch-a",
		Name:     "Channel A",
		Protocol: proto,
		Config:   map[string]any{},
		Devices: []model.Device{
			{ID: "dev-shared", Name: "Shared Device"},
		},
	}); err != nil {
		t.Fatalf("first AddChannel: %v", err)
	}

	// 第二个通道尝试用同一个 dev-shared → 应被拒绝
	err := cm.AddChannel(&model.Channel{
		ID:       "ch-b",
		Name:     "Channel B",
		Protocol: proto,
		Config:   map[string]any{},
		Devices: []model.Device{
			{ID: "dev-shared", Name: "Duplicate Device"},
		},
	})
	if err == nil {
		t.Fatal("expected global device ID conflict error")
	}

	// 第二个通道不应被加入
	cm.mu.RLock()
	_, exists := cm.channels["ch-b"]
	cm.mu.RUnlock()
	if exists {
		t.Fatal("channel ch-b should NOT exist after conflict")
	}
}

// Test_AddChannel_RejectsIntraChannelDuplicate 同批次设备 ID 重复
func TestChannelManager_AddChannel_RejectsIntraChannelDuplicate(t *testing.T) {
	proto := ensureTrackerDriverProtocol()
	cm := NewChannelManager(nil, nil)
	defer cm.cancel()

	err := cm.AddChannel(&model.Channel{
		ID:       "ch-dup-dev",
		Name:     "Dup Channel",
		Protocol: proto,
		Config:   map[string]any{},
		Devices: []model.Device{
			{ID: "dev-same", Name: "First"},
			{ID: "dev-same", Name: "Second"},
		},
	})
	if err == nil {
		t.Fatal("expected intra-channel duplicate device ID error")
	}

	cm.mu.RLock()
	_, exists := cm.channels["ch-dup-dev"]
	cm.mu.RUnlock()
	if exists {
		t.Fatal("channel should NOT exist after duplicate-device rejection")
	}
}

// Test_AddChannel_FailFast_NoInitOnConflict 验证 Fail-Fast：冲突时 driver Init 从未被调用
func TestChannelManager_AddChannel_FailFast_NoInitOnConflict(t *testing.T) {
	proto := ensureTrackerDriverProtocol()
	cm := NewChannelManager(nil, nil)
	defer cm.cancel()

	// 先加一个通道
	if err := cm.AddChannel(&model.Channel{
		ID:       "ch-base",
		Name:     "Base",
		Protocol: proto,
		Config:   map[string]any{},
		Devices:  []model.Device{{ID: "dev-base", Name: "Base Dev"}},
	}); err != nil {
		t.Fatalf("base AddChannel: %v", err)
	}

	// Init 次数应 = 1，Disconnect 次数应 = 0
	trackerDriverInstance.mu.Lock()
	initBefore := trackerDriverInstance.initCount
	trackerDriverInstance.mu.Unlock()
	if initBefore != 1 {
		t.Fatalf("expected 1 Init after base channel, got %d", initBefore)
	}

	// 第二个通道触发设备 ID 冲突
	_ = cm.AddChannel(&model.Channel{
		ID:       "ch-conflict",
		Name:     "Conflict",
		Protocol: proto,
		Config:   map[string]any{},
		Devices:  []model.Device{{ID: "dev-base", Name: "Clash"}},
	})

	// Init 次数仍应 = 1（冲突在 Init 前被拦截）
	trackerDriverInstance.mu.Lock()
	initAfter := trackerDriverInstance.initCount
	disconnectAfter := trackerDriverInstance.disconnectCount
	trackerDriverInstance.mu.Unlock()

	if initAfter != 1 {
		t.Fatalf("Fail-Fast broken: Init called after device ID conflict (init=%d)", initAfter)
	}
	if disconnectAfter != 0 {
		t.Fatalf("Disconnect should not be called when Init was skipped (disconnect=%d)", disconnectAfter)
	}
}

// Test_AddChannel_DeviceIDFromName 设备 ID 为空时从 Name 补齐
func TestChannelManager_AddChannel_DeviceIDFromName(t *testing.T) {
	proto := ensureTrackerDriverProtocol()
	cm := NewChannelManager(nil, nil)
	defer cm.cancel()

	ch := &model.Channel{
		ID:       "ch-dev-id-from-name",
		Name:     "Device ID From Name",
		Protocol: proto,
		Config:   map[string]any{},
		Devices: []model.Device{
			{Name: "Sensor A"},
			{Name: "Sensor B"},
		},
	}
	if err := cm.AddChannel(ch); err != nil {
		t.Fatalf("AddChannel: %v", err)
	}

	if ch.Devices[0].ID != "Sensor-A" {
		t.Fatalf("expected device ID 'Sensor-A', got %q", ch.Devices[0].ID)
	}
	if ch.Devices[1].ID != "Sensor-B" {
		t.Fatalf("expected device ID 'Sensor-B', got %q", ch.Devices[1].ID)
	}
}

// Test_UpdateChannel_ExcludesSelfFromGlobalCheck 更新时允许保留通道自身已有设备 ID
func TestChannelManager_UpdateChannel_ExcludesSelfFromGlobalCheck(t *testing.T) {
	proto := ensureTrackerDriverProtocol()
	cm := NewChannelManager(nil, nil)
	defer cm.cancel()

	// 先加一个通道，有设备 dev-s1
	if err := cm.AddChannel(&model.Channel{
		ID:       "ch-update",
		Name:     "Original",
		Protocol: proto,
		Config:   map[string]any{},
		Devices:  []model.Device{{ID: "dev-s1", Name: "Keep"}},
	}); err != nil {
		t.Fatalf("AddChannel: %v", err)
	}

	// UpdateChannel 保留 dev-s1 并新增 dev-s2 — dev-s1 是"自身"应豁免，允许通过
	if err := cm.UpdateChannel(&model.Channel{
		ID:       "ch-update",
		Name:     "Updated",
		Protocol: proto,
		Config:   map[string]any{},
		Devices: []model.Device{
			{ID: "dev-s1", Name: "Keep Renamed"},
			{ID: "dev-s2", Name: "New Device"},
		},
	}); err != nil {
		t.Fatalf("UpdateChannel should allow self-ID exemption, got: %v", err)
	}
}

// Test_UpdateChannel_RejectsGlobalConflict 用外部已占用的设备 ID 更新应被拒
func TestChannelManager_UpdateChannel_RejectsGlobalConflict(t *testing.T) {
	proto := ensureTrackerDriverProtocol()
	cm := NewChannelManager(nil, nil)
	defer cm.cancel()

	// ch-other 先占 dev-external
	if err := cm.AddChannel(&model.Channel{
		ID:       "ch-other",
		Name:     "Other",
		Protocol: proto,
		Config:   map[string]any{},
		Devices:  []model.Device{{ID: "dev-external", Name: "External"}},
	}); err != nil {
		t.Fatalf("AddChannel ch-other: %v", err)
	}

	// ch-update 原有的设备
	if err := cm.AddChannel(&model.Channel{
		ID:       "ch-update-reject",
		Name:     "Update Reject",
		Protocol: proto,
		Config:   map[string]any{},
		Devices:  []model.Device{{ID: "dev-local", Name: "Local"}},
	}); err != nil {
		t.Fatalf("AddChannel ch-update-reject: %v", err)
	}

	// Update 把 dev-local 改成 dev-external → 全局冲突应拒绝
	err := cm.UpdateChannel(&model.Channel{
		ID:       "ch-update-reject",
		Name:     "Attempt Rename",
		Protocol: proto,
		Config:   map[string]any{},
		Devices:  []model.Device{{ID: "dev-external", Name: "Clash"}},
	})
	if err == nil {
		t.Fatal("expected UpdateChannel global device ID conflict error")
	}

	// devIDs map 中仍应包含 dev-local，不应被 dev-external 覆盖
	cm.mu.RLock()
	_, hasLocal := cm.deviceIDs["dev-local"]
	_, hasExternal := cm.deviceIDs["dev-external"]
	cm.mu.RUnlock()
	if !hasLocal {
		t.Fatal("dev-local should remain after failed update")
	}
	if !hasExternal {
		t.Fatal("dev-external should still be tracked (owned by ch-other)")
	}
}
