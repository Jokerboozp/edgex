package core

import (
	"container/heap"
	"testing"
	"time"
)

func TestPopReadyTask_PrefersEarliestNextRun(t *testing.T) {
	se := NewScanEngine(ScanEngineConfig{})
	now := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)

	early := &ScanTask{ID: "early", NextRun: now.Add(-time.Millisecond), Priority: 5, Status: ScanTaskStatusIdle}
	due := &ScanTask{ID: "due", NextRun: now, Priority: 3, Status: ScanTaskStatusIdle}
	late := &ScanTask{ID: "late", NextRun: now.Add(time.Millisecond), Priority: 8, Status: ScanTaskStatusIdle}

	se.mu.Lock()
	heap.Push(se.priorityQueue, late)
	heap.Push(se.priorityQueue, due)
	heap.Push(se.priorityQueue, early)
	se.mu.Unlock()

	got := se.popReadyTask(now)
	if got == nil || got.ID != early.ID {
		t.Fatalf("popReadyTask = %v, want early", got)
	}
}

func TestPopReadyTask_SkipsNotReady(t *testing.T) {
	se := NewScanEngine(ScanEngineConfig{})
	now := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)

	future := &ScanTask{ID: "future", NextRun: now.Add(time.Second), Priority: 5, Status: ScanTaskStatusIdle}
	se.mu.Lock()
	heap.Push(se.priorityQueue, future)
	se.mu.Unlock()

	if got := se.popReadyTask(now); got != nil {
		t.Fatalf("expected nil for not-ready task, got %v", got.ID)
	}
}

// TestRescheduleTask_ElapsedOverIntervalRunsImmediately 验证"超时立即续跑"：
// 轮耗时 > 间隔时，下一轮从完成时刻立即续跑，不空等到相位整点。
func TestRescheduleTask_ElapsedOverIntervalRunsImmediately(t *testing.T) {
	se := NewScanEngine(ScanEngineConfig{})
	base := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)
	task := &ScanTask{ID: "overtime", Interval: 100 * time.Millisecond, LastScheduledAt: base, NextRun: base}

	se.rescheduleTask(task, base.Add(250*time.Millisecond))

	task.mu.RLock()
	got := task.NextRun
	task.mu.RUnlock()
	if want := base.Add(250 * time.Millisecond); !got.Equal(want) {
		t.Fatalf("NextRun = %v, want immediate completedAt %v", got, want)
	}
}

// TestRescheduleTask_ElapsedWithinIntervalAlignsGrid 验证未超时对齐相位整点：
// 轮耗时 ≤ 间隔时，下一轮对齐到基准整点（周期≈间隔，不漂移累积）。
func TestRescheduleTask_ElapsedWithinIntervalAlignsGrid(t *testing.T) {
	se := NewScanEngine(ScanEngineConfig{})
	base := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)
	task := &ScanTask{ID: "undertime", Interval: 100 * time.Millisecond, LastScheduledAt: base, NextRun: base}

	se.rescheduleTask(task, base.Add(60*time.Millisecond))

	task.mu.RLock()
	got := task.NextRun
	task.mu.RUnlock()
	if want := base.Add(100 * time.Millisecond); !got.Equal(want) {
		t.Fatalf("NextRun = %v, want aligned %v", got, want)
	}
}