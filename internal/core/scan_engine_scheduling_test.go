package core

import (
	"testing"
	"time"
)

func TestRescheduleTask_OvertimeRunsImmediately(t *testing.T) {
	se := NewScanEngine(ScanEngineConfig{
		TickInterval: 10 * time.Millisecond,
	})

	interval := 100 * time.Millisecond
	base := time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC)
	task := &ScanTask{
		ID:              "task_drift",
		Interval:        interval,
		LastScheduledAt: base,
		NextRun:         base,
	}

	completedAt := base.Add(350 * time.Millisecond)
	se.rescheduleTask(task, completedAt)

	task.mu.RLock()
	defer task.mu.RUnlock()

	// 轮耗时(350ms)远超间隔(100ms)：完成后立即续跑，不再空等到下一整点。
	if !task.LastScheduledAt.Equal(completedAt) {
		t.Fatalf("LastScheduledAt = %v, want immediate completedAt %v", task.LastScheduledAt, completedAt)
	}
	if !task.NextRun.Equal(completedAt) {
		t.Fatalf("NextRun = %v, want %v", task.NextRun, completedAt)
	}
}

func TestRescheduleTask_AlignsWhenOnTime(t *testing.T) {
	se := NewScanEngine(ScanEngineConfig{})

	interval := 200 * time.Millisecond
	base := time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC)
	task := &ScanTask{
		ID:              "task_on_time",
		Interval:        interval,
		LastScheduledAt: base,
		NextRun:         base,
	}

	// 轮耗时(20ms) ≤ 间隔(200ms)：下一轮对齐到基准整点，不累积漂移。
	se.rescheduleTask(task, base.Add(20*time.Millisecond))

	task.mu.RLock()
	defer task.mu.RUnlock()
	want := base.Add(interval)
	if !task.NextRun.Equal(want) {
		t.Fatalf("NextRun = %v, want %v", task.NextRun, want)
	}
}

func TestUpdateTaskState_DegradesOnFailuresAndRecoversOnSuccess(t *testing.T) {
	se := NewScanEngine(ScanEngineConfig{})
	base := time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC)
	task := &ScanTask{
		ID:           "dev",
		Interval:     time.Second,
		BaseInterval: time.Second,
		NextRun:      base,
	}

	fail := &ExecuteResult{Success: false, Error: errTestFail}
	for i := 0; i < 3; i++ {
		se.updateTaskState(task, fail)
	}

	task.mu.RLock()
	if task.Interval != 2*time.Second {
		t.Fatalf("degraded interval = %v, want 2s after 3 consecutive failures", task.Interval)
	}
	if task.Status != ScanTaskStatusDegraded {
		t.Fatalf("status = %v, want Degraded", task.Status)
	}
	task.mu.RUnlock()

	se.updateTaskState(task, &ExecuteResult{Success: true})

	task.mu.RLock()
	defer task.mu.RUnlock()
	if task.Interval != time.Second {
		t.Fatalf("recovered interval = %v, want base 1s", task.Interval)
	}
	if task.Status != ScanTaskStatusIdle {
		t.Fatalf("status = %v, want Idle after success", task.Status)
	}
}

var errTestFail = &execErr{}

type execErr struct{}

func (*execErr) Error() string { return "test fail" }

func TestAddTask_ImmediateFirstRun(t *testing.T) {
	se := NewScanEngine(ScanEngineConfig{})
	interval := time.Second

	task := se.AddTask("device-alpha", "modbus-tcp", interval, 5, []string{"p1"}, nil)
	if task == nil {
		t.Fatal("AddTask returned nil")
	}
	task.mu.RLock()
	first := task.NextRun
	task.mu.RUnlock()
	if first.Before(time.Now().Add(-time.Second)) || first.After(time.Now().Add(time.Second)) {
		t.Fatalf("first run = %v, want ~now", first)
	}
}