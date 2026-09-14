package integration_test

import (
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/core"
)

// TestS7Protocol_SessionFramework is a skeleton for long-running S7 session stability (D-04).
func TestS7Protocol_SessionFramework(t *testing.T) {
	if testing.Short() {
		t.Skip("S7 long-session test skipped in short mode")
	}

	se := core.NewScanEngine(core.ScanEngineConfig{
		TickInterval: 5 * time.Millisecond,
	})
	se.RegisterProtocol("s7", core.ProtocolTypeLimited)

	// Framework mirrors modbus_protocol_test.go:
	// - shared connection manager session lock
	// - subscription/read pacing under interval scheduler
	// Wire PLCSIM or hardware when available.

	_ = se
	t.Log("S7 session framework ready; connect snap7/plcsim to enable full soak")
}

func TestS7Protocol_TaskScheduled(t *testing.T) {
	se := core.NewScanEngine(core.ScanEngineConfig{})
	task := se.AddTask("s7-plc-1", "s7", 200*time.Millisecond, 5, []string{"db1.w0"}, nil)

	got := se.GetTask(task.ID)
	if got == nil {
		t.Fatal("task not found")
	}
	if got.NextRun.IsZero() {
		t.Fatal("expected a scheduled NextRun for S7 task")
	}
	if got.Interval != 200*time.Millisecond {
		t.Fatalf("interval = %v, want 200ms", got.Interval)
	}
}
