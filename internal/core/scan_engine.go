package core

import (
	"container/heap"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/anviod/edgeCore/internal/driver"
	"github.com/anviod/edgeCore/internal/model"
	"go.uber.org/zap"
)

// ScanEngineConfig 采集调度引擎配置。参照 Kepware：仅保留间隔节奏与并发限流，
// 去除截止期/抖动/防饿死等复杂调度参数，靠冷却降级与自愈保障稳定性。
type ScanEngineConfig struct {
	TickInterval    time.Duration // 调度主循环节拍
	WorkerCount     int           // 并发采集 worker 数
	MaxQueueSize    int           // 就绪队列最大长度（超限时资源排队）
	GoroutineLimit  int           // 协程上限
	ConnectionLimit int           // 并发连接上限
}

type ScanTaskStatus int

const (
	ScanTaskStatusIdle ScanTaskStatus = iota
	ScanTaskStatusRunning
	ScanTaskStatusDegraded
	ScanTaskStatusStopped
)

func (s ScanTaskStatus) String() string {
	switch s {
	case ScanTaskStatusIdle:
		return "Idle"
	case ScanTaskStatusRunning:
		return "Running"
	case ScanTaskStatusDegraded:
		return "Degraded"
	case ScanTaskStatusStopped:
		return "Stopped"
	default:
		return "Unknown"
	}
}

type ScanTask struct {
	ID                  string
	DeviceKey           string
	ScanClass           string
	Protocol            string
	Interval            time.Duration
	BaseInterval        time.Duration
	NextRun             time.Time
	LastScheduledAt     time.Time
	Priority            int
	FailRate            float64
	Status              ScanTaskStatus
	ConsecutiveFailures int
	ConsecutiveSuccess  int
	LastSuccess         time.Time
	LastFailure         time.Time
	PointIDs            []string
	Points              []model.Point
	pointsScratch       []model.Point
	Params              map[string]any
	// queued reports whether this task is currently present in
	// ScanEngine.priorityQueue. It is the single source of truth that
	// prevents double-enqueue (which would let the same task run
	// concurrently in two workers and corrupt task-scoped state).
	// Guarded by mu; only ever mutated while holding ScanEngine.mu so
	// the se.mu -> task.mu lock order is preserved.
	queued bool
	mu     sync.RWMutex
}

func (t *ScanTask) GetStatus() ScanTaskStatus {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.Status
}

func (t *ScanTask) SetStatus(status ScanTaskStatus) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.Status = status
}

// LastSuccessTime 返回最近一次成功采集时间（并发安全）。
func (t *ScanTask) LastSuccessTime() time.Time {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.LastSuccess
}

// isQueued reports whether the task is currently in the priority queue.
func (t *ScanTask) isQueued() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.queued
}

// setQueued marks queue membership. Must be called while holding
// ScanEngine.mu, immediately around the corresponding heap operation.
func (t *ScanTask) setQueued(v bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.queued = v
}

// paramsSnapshot returns the current immutable task parameters map.
//
// Contract: a published Params map is NEVER mutated in place. Writers call
// setParams with a freshly built map; readers only read. That makes the
// returned reference safe to use after the read lock is released, and
// eliminates the "concurrent map read and map write" fatal error that a
// live in-place update would otherwise cause against hot-path readers.
func (t *ScanTask) paramsSnapshot() map[string]any {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.Params
}

// setParams atomically publishes a new Params map (copy-on-write).
func (t *ScanTask) setParams(next map[string]any) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.Params = next
}

func (t *ScanTask) UpdateNextRun(interval time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	t.LastScheduledAt = now
	t.NextRun = now.Add(interval)
}

type PriorityQueue []*ScanTask

func (pq PriorityQueue) Len() int { return len(pq) }

func (pq PriorityQueue) Less(i, j int) bool {
	pq[i].mu.RLock()
	iNext, iPriority := pq[i].NextRun, pq[i].Priority
	pq[i].mu.RUnlock()
	pq[j].mu.RLock()
	jNext, jPriority := pq[j].NextRun, pq[j].Priority
	pq[j].mu.RUnlock()
	// 按最早就绪时刻出队（Kepware 式轮询节奏）；tie-break 用优先级保证确定性。
	if iNext.Before(jNext) {
		return true
	}
	if jNext.Before(iNext) {
		return false
	}
	return iPriority > jPriority
}

func (pq PriorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
}

func (pq *PriorityQueue) Push(x interface{}) {
	*pq = append(*pq, x.(*ScanTask))
}

func (pq *PriorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	*pq = old[0 : n-1]
	return item
}

func (pq *PriorityQueue) Peek() *ScanTask {
	if len(*pq) == 0 {
		return nil
	}
	return (*pq)[0]
}

// CollectFinalizeFunc 采集完成后回写设备通信状态。
type CollectFinalizeFunc func(deviceID string, result *ExecuteResult)

type ScanEngine struct {
	tasks           map[string]*ScanTask
	priorityQueue   *PriorityQueue
	executionLayer  *ExecutionLayer
	resourceCtrl    *ResourceController
	shadowCore      *ShadowCore
	shadowIngress   *ShadowIngress
	pointDegrade    *PointDegradationManager
	collectFinalize CollectFinalizeFunc
	metrics         *ScanEngineMetrics
	config          ScanEngineConfig
	ticker          *time.Ticker
	running         bool
	stopCh          chan struct{}
	wg              sync.WaitGroup
	mu              sync.RWMutex
	taskIDCounter   int
}

func NewScanEngine(config ScanEngineConfig) *ScanEngine {
	if config.TickInterval == 0 {
		config.TickInterval = 10 * time.Millisecond
	}
	if config.WorkerCount == 0 {
		config.WorkerCount = 4
	}
	if config.MaxQueueSize == 0 {
		config.MaxQueueSize = 10000
	}
	if config.GoroutineLimit == 0 {
		config.GoroutineLimit = 2048
	}
	if config.ConnectionLimit == 0 {
		config.ConnectionLimit = 500
	}

	se := &ScanEngine{
		tasks:          make(map[string]*ScanTask),
		priorityQueue:  &PriorityQueue{},
		executionLayer: NewExecutionLayer(),
		resourceCtrl: NewResourceController(ResourceLimits{
			GoroutineLimit:  config.GoroutineLimit,
			ConnectionLimit: config.ConnectionLimit,
			QueueLimit:      config.MaxQueueSize,
		}),
		shadowCore: nil,
		metrics: &ScanEngineMetrics{
			lagSamples: make([]int64, 0, scanLagSampleCap),
		},
		config: config,
		stopCh: make(chan struct{}),
	}

	heap.Init(se.priorityQueue)

	return se
}

func (se *ScanEngine) Run() {
	se.mu.Lock()
	if se.running {
		se.mu.Unlock()
		return
	}
	se.running = true
	se.mu.Unlock()

	se.wg.Add(1)
	go se.dispatchLoop()

	se.wg.Add(1)
	go se.resourceCtrl.Monitor(&se.wg)

	if se.executionLayer != nil {
		se.executionLayer.Start()
	}

	zap.L().Info("[ScanEngine] 调度引擎已启动",
		zap.String("tickInterval", se.config.TickInterval.String()),
		zap.Int("workerCount", se.config.WorkerCount),
		zap.Int("maxQueueSize", se.config.MaxQueueSize),
		zap.Int("goroutineLimit", se.config.GoroutineLimit),
		zap.Int("connectionLimit", se.config.ConnectionLimit),
	)
}

func (se *ScanEngine) Stop() {
	se.mu.Lock()
	if !se.running {
		se.mu.Unlock()
		return
	}
	se.running = false
	se.mu.Unlock()

	close(se.stopCh)

	if se.ticker != nil {
		se.ticker.Stop()
	}

	se.resourceCtrl.Stop()
	if se.executionLayer != nil {
		se.executionLayer.Stop()
	}

	se.wg.Wait()

	zap.L().Info("[ScanEngine] 调度引擎已停止")
}

func (se *ScanEngine) fallbackTickInterval() time.Duration {
	tick := se.config.TickInterval
	if se.config.MaxQueueSize <= 0 {
		return tick
	}
	loadRatio := float64(se.GetPendingTaskCount()) / float64(se.config.MaxQueueSize)
	if loadRatio > 0.7 {
		return 50 * time.Millisecond
	}
	return tick
}

func (se *ScanEngine) dispatchLoop() {
	defer se.wg.Done()

	fallbackTick := se.fallbackTickInterval()
	fallback := time.NewTicker(fallbackTick)
	defer fallback.Stop()

	var wakeTimer *time.Timer
	var wakeCh <-chan time.Time

	scheduleWake := func() {
		next := se.nextReadyTime()
		if next.IsZero() {
			if wakeTimer != nil {
				if !wakeTimer.Stop() {
					select {
					case <-wakeTimer.C:
					default:
					}
				}
				wakeTimer = nil
				wakeCh = nil
			}
			return
		}

		delay := time.Until(next)
		if delay < 0 {
			delay = 0
		}

		if wakeTimer == nil {
			wakeTimer = time.NewTimer(delay)
			wakeCh = wakeTimer.C
			return
		}

		if !wakeTimer.Stop() {
			select {
			case <-wakeTimer.C:
			default:
			}
		}
		wakeTimer.Reset(delay)
		wakeCh = wakeTimer.C
	}

	scheduleWake()

	for {
		select {
		case <-se.stopCh:
			if wakeTimer != nil {
				wakeTimer.Stop()
			}
			return
		case <-wakeCh:
			se.safeProcessReadyTasks()
			scheduleWake()
		case <-fallback.C:
			se.safeProcessReadyTasks()
			scheduleWake()
			if newTick := se.fallbackTickInterval(); newTick != fallbackTick {
				fallbackTick = newTick
				fallback.Reset(fallbackTick)
			}
		}
	}
}

// safeProcessReadyTasks wraps the dispatch tick with panic isolation and
// queue self-repair. A panic in the dispatch path would otherwise kill the
// only scheduling goroutine, permanently stalling EVERY device until the
// whole process restarts. On panic we rebuild the priority queue from the
// authoritative task map, restoring a consistent state (equivalent to a
// rollback of the corrupted in-memory queue).
func (se *ScanEngine) safeProcessReadyTasks() {
	defer func() {
		if r := recover(); r != nil {
			se.metrics.TaskPanicsTotal.Add(1)
			zap.L().Error("[ScanEngine] 调度循环 panic 已恢复，重建优先队列",
				zap.Any("panic", r),
				zap.Stack("stack"),
			)
			se.rebuildQueueAfterPanic()
			se.metrics.TaskRecoveriesTotal.Add(1)
		}
	}()
	se.processReadyTasks()
}

// rebuildQueueAfterPanic rebuilds the priority queue from the authoritative
// se.tasks map, dropping duplicates and stale pointers and re-deriving each
// task's queue-membership flag. Caller must not hold se.mu.
func (se *ScanEngine) rebuildQueueAfterPanic() {
	se.mu.Lock()
	defer se.mu.Unlock()

	pq := make(PriorityQueue, 0, len(se.tasks))
	for _, t := range se.tasks {
		if t.GetStatus() == ScanTaskStatusStopped {
			t.setQueued(false)
			continue
		}
		t.setQueued(true)
		pq = append(pq, t)
	}
	heap.Init(&pq)
	se.priorityQueue = &pq
}

func (se *ScanEngine) nextReadyTime() time.Time {
	se.mu.RLock()
	defer se.mu.RUnlock()
	if se.priorityQueue.Len() == 0 {
		return time.Time{}
	}
	task := (*se.priorityQueue)[0]
	if task == nil {
		return time.Time{}
	}
	task.mu.RLock()
	next := task.NextRun
	task.mu.RUnlock()
	return next
}

func (se *ScanEngine) processReadyTasks() {
	now := time.Now()

	for {
		task := se.popReadyTask(now)
		if task == nil {
			break
		}

		// A task stopped by RemoveTask/RemoveTasksByDeviceKey must never be
		// re-queued; check before the CanExecute push-back path.
		if task.GetStatus() == ScanTaskStatusStopped {
			continue
		}

		if !se.resourceCtrl.CanExecute() {
			se.mu.Lock()
			heap.Push(se.priorityQueue, task)
			task.setQueued(true)
			se.mu.Unlock()
			break
		}

		se.resourceCtrl.Acquire()
		go se.executeTaskAsync(task)
	}
}

// popReadyTask 从队列取出就绪（NextRun<=now）且 NextRun 最早的任务。
func (se *ScanEngine) popReadyTask(now time.Time) *ScanTask {
	se.mu.Lock()
	defer se.mu.Unlock()

	pq := se.priorityQueue
	if pq.Len() == 0 {
		return nil
	}

	bestIdx := -1
	for i, task := range *pq {
		task.mu.RLock()
		nextRun, priority := task.NextRun, task.Priority
		task.mu.RUnlock()
		if now.Before(nextRun) {
			continue
		}
		if bestIdx < 0 {
			bestIdx = i
			continue
		}
		best := (*pq)[bestIdx]
		best.mu.RLock()
		bestNext, bestPriority := best.NextRun, best.Priority
		best.mu.RUnlock()
		if nextRun.Before(bestNext) || (nextRun.Equal(bestNext) && priority > bestPriority) {
			bestIdx = i
		}
	}
	if bestIdx < 0 {
		return nil
	}
	removed := heap.Remove(pq, bestIdx).(*ScanTask)
	removed.setQueued(false)
	return removed
}

func (se *ScanEngine) executeTaskAsync(task *ScanTask) {
	defer se.resourceCtrl.Release()
	// Driver code runs arbitrary protocol parsing (including CGO for BACnet);
	// a single malformed frame must never take down the whole gateway. Recover
	// the collect goroutine, count it, and re-arm the task so its schedule
	// survives one bad I/O.
	defer func() {
		if r := recover(); r != nil {
			se.metrics.TaskPanicsTotal.Add(1)
			zap.L().Error("[ScanEngine] 采集任务 panic 已恢复",
				zap.Any("panic", r),
				zap.String("taskID", task.ID),
				zap.String("deviceKey", task.DeviceKey),
				zap.Stack("stack"),
			)
			se.rearmTaskAfterPanic(task)
		}
	}()

	task.SetStatus(ScanTaskStatusRunning)

	task.mu.RLock()
	scheduledAt := task.NextRun
	task.mu.RUnlock()
	start := time.Now()
	lagMicros := start.Sub(scheduledAt).Microseconds()
	if lagMicros < 0 {
		lagMicros = 0
	}

	var result *ExecuteResult
	if se.executionLayer != nil {
		result = se.executionLayer.Execute(task)
	} else {
		result = &ExecuteResult{Success: false, Error: ErrDriverNotFound}
	}

	if result.Success && se.shadowCore != nil {
		rttMicros := time.Since(start).Microseconds()
		se.shadowCore.UpdateDeviceRTT(task.DeviceKey, rttMicros)
	}
	se.metrics.RecordExecuteForChannel(taskShadowChannelID(task), result != nil && result.Success, lagMicros)

	se.applyCollectToShadow(task, result)

	// 所有结果（成功/失败/熔断半开）统一走同步状态更新：成功恢复间隔，失败冷却退避。
	if result != nil {
		se.updateTaskState(task, result)
	}

	if se.collectFinalize != nil {
		se.collectFinalize(task.DeviceKey, result)
	}

	// 检查执行期间是否已被 RemoveTasksByDeviceKey 停止，若是则不再重新入队。
	if task.GetStatus() == ScanTaskStatusStopped {
		return
	}

	task.SetStatus(ScanTaskStatusIdle)

	se.mu.RLock()
	running := se.running
	se.mu.RUnlock()

	if !running {
		return
	}

	se.rescheduleTask(task, time.Now())

	se.mu.Lock()
	if se.running && !task.isQueued() && task.GetStatus() != ScanTaskStatusStopped {
		heap.Push(se.priorityQueue, task)
		task.setQueued(true)
	}
	se.mu.Unlock()
}

// rearmTaskAfterPanic restores a task to the schedule after its collect
// goroutine panicked, so one bad frame cannot permanently silence a device.
// It only re-arms tasks that are still registered and whose engine is running;
// intentionally does NOT record a collect outcome (the panic is not a device
// reachability signal).
func (se *ScanEngine) rearmTaskAfterPanic(task *ScanTask) {
	if task == nil {
		return
	}
	if task.GetStatus() == ScanTaskStatusStopped {
		return
	}
	task.SetStatus(ScanTaskStatusIdle)

	se.mu.RLock()
	running := se.running
	registered := se.tasks[task.ID] == task
	se.mu.RUnlock()
	if !running || !registered {
		return
	}

	se.rescheduleTask(task, time.Now())

	se.mu.Lock()
	if se.running && !task.isQueued() && task.GetStatus() != ScanTaskStatusStopped {
		heap.Push(se.priorityQueue, task)
		task.setQueued(true)
		se.metrics.TaskRecoveriesTotal.Add(1)
	}
	se.mu.Unlock()
}

func (se *ScanEngine) rescheduleTask(task *ScanTask, completedAt time.Time) {
	task.mu.Lock()
	defer task.mu.Unlock()

	interval := task.Interval
	if interval <= 0 {
		interval = time.Millisecond
	}

	anchor := task.LastScheduledAt
	if anchor.IsZero() {
		anchor = task.NextRun
	}
	if anchor.IsZero() {
		anchor = completedAt
	}

	// 纯间隔节奏：轮耗时 ≤ 间隔 → 对齐到基准整点（周期≈间隔，不漂移累积）；
	// 轮耗时 > 间隔 → 完成后立即续跑（周期≈轮耗时），不空等。
	elapsed := completedAt.Sub(anchor)
	if elapsed < 0 {
		elapsed = 0
	}
	hold := interval - elapsed
	if hold < 0 {
		hold = 0
	}
	next := completedAt.Add(hold)

	task.LastScheduledAt = next
	task.NextRun = next
}

func taskCollectPointIDs(task *ScanTask) []string {
	if len(task.Points) > 0 {
		ids := make([]string, len(task.Points))
		for i, p := range task.Points {
			ids[i] = p.ID
		}
		return ids
	}
	return task.PointIDs
}

// taskShadowChannelIDLocked assumes the caller already holds task.mu
// (read or write). Needed because sync.RWMutex is NOT reentrant — taking
// RLock twice while a writer waits deadlocks.
func taskShadowChannelIDLocked(task *ScanTask) string {
	if task.Params != nil {
		if id, ok := task.Params["channelID"].(string); ok {
			return id
		}
	}
	return ""
}

func taskShadowChannelID(task *ScanTask) string {
	task.mu.RLock()
	defer task.mu.RUnlock()
	return taskShadowChannelIDLocked(task)
}

func resolveCollectQuality(v model.Value) string {
	if v.Quality != "" {
		return v.Quality
	}
	if v.Value == nil {
		return "Bad"
	}
	return "Good"
}

func preservedShadowValue(existing *model.ShadowDevice, pointID string, newVal any, quality string) any {
	if newVal != nil {
		return newVal
	}
	if quality == "Good" {
		return nil
	}
	if existing != nil {
		if sp, ok := existing.Points[pointID]; ok && sp.Value != nil {
			return sp.Value
		}
	}
	return nil
}

func (se *ScanEngine) writeShadowMessage(msg model.ShadowIngressMessage) {
	if se.shadowIngress != nil {
		se.shadowIngress.IngestDirect(msg)
	} else if se.shadowCore != nil {
		se.shadowCore.WriteShadowDevice(msg)
	}
}

// applyCollectToShadow writes scan results to shadow, including Bad quality on
// failed reads so stale Good values are not left behind when collection fails.
func (se *ScanEngine) applyCollectToShadow(task *ScanTask, result *ExecuteResult) {
	if se.shadowCore == nil && se.shadowIngress == nil {
		return
	}

	pointIDs := taskCollectPointIDs(task)
	if len(pointIDs) == 0 {
		return
	}

	now := time.Now()
	shadowID := fmt.Sprintf("shadow-%s", task.DeviceKey)
	existing, _ := se.shadowCore.GetShadowDevice(shadowID)

	resultValues := map[string]model.Value{}
	if result != nil && len(result.Values) > 0 {
		resultValues = result.Values
	}

	samplePeriodMs := int(task.Interval.Milliseconds())
	raw := borrowShadowIngressPointSlice(len(pointIDs))
	points := (*raw)[:0]

	for _, pointID := range pointIDs {
		value, inResult := resultValues[pointID]
		if !inResult {
			// Missing entries on both failed and successful collects must not
			// leave prior Good values untouched (e.g. SNMP parse skips, Modbus cooldown).
			value = model.Value{Quality: "Bad"}
		}

		quality := resolveCollectQuality(value)
		collectedAt := value.TS
		if collectedAt.IsZero() {
			collectedAt = now
		}
		val := preservedShadowValue(existing, pointID, value.Value, quality)

		degraded := false
		if se.pointDegrade != nil {
			degraded = se.pointDegrade.IsDegraded(task.DeviceKey, pointID)
		}
		points = append(points, model.ShadowIngressPoint{
			PointID:        pointID,
			Value:          val,
			Quality:        quality,
			SamplePeriodMs: samplePeriodMs,
			CollectedAt:    collectedAt,
			Degraded:       degraded,
		})
	}

	if len(points) == 0 {
		returnShadowIngressPointSlice(raw)
		return
	}

	msgPoints := make([]model.ShadowIngressPoint, len(points))
	copy(msgPoints, points)
	returnShadowIngressPointSlice(raw)

	se.writeShadowMessage(model.ShadowIngressMessage{
		DeviceID:  task.DeviceKey,
		ChannelID: taskShadowChannelID(task),
		Timestamp: now,
		Points:    msgPoints,
		Meta: model.ShadowIngressMeta{
			Source: "scan_engine",
		},
	})
}

// markDeviceShadowBad marks all known shadow points Bad when a device goes offline
// (e.g. channel connect failure) so stale Good values are not left behind.
func (se *ScanEngine) markDeviceShadowBad(deviceKey, channelID string) {
	if (se.shadowCore == nil && se.shadowIngress == nil) || deviceKey == "" {
		return
	}

	pointIDs := make(map[string]struct{})
	for _, task := range se.GetTasksByDeviceKey(deviceKey) {
		for _, pid := range taskCollectPointIDs(task) {
			pointIDs[pid] = struct{}{}
		}
	}

	shadowID := fmt.Sprintf("shadow-%s", deviceKey)
	existing, _ := se.shadowCore.GetShadowDevice(shadowID)
	if len(pointIDs) == 0 && existing != nil {
		for pid := range existing.Points {
			pointIDs[pid] = struct{}{}
		}
	}
	if len(pointIDs) == 0 {
		return
	}

	now := time.Now()
	ingress := make([]model.ShadowIngressPoint, 0, len(pointIDs))
	for pid := range pointIDs {
		ingress = append(ingress, model.ShadowIngressPoint{
			PointID:     pid,
			Value:       preservedShadowValue(existing, pid, nil, "Bad"),
			Quality:     "Bad",
			CollectedAt: now,
		})
	}

	se.writeShadowMessage(model.ShadowIngressMessage{
		DeviceID:  deviceKey,
		ChannelID: channelID,
		Timestamp: now,
		Points:    ingress,
		Meta: model.ShadowIngressMeta{
			Source: "channel_offline",
		},
	})
}

func (se *ScanEngine) updateTaskState(task *ScanTask, result *ExecuteResult) {
	task.mu.Lock()

	if result.Success {
		task.ConsecutiveSuccess++
		task.ConsecutiveFailures = 0
		task.LastSuccess = time.Now()
		task.FailRate = 0
		task.Status = ScanTaskStatusIdle
		// 成功：恢复基础间隔（自愈）。
		if task.BaseInterval > 0 && task.Interval != task.BaseInterval {
			task.Interval = task.BaseInterval
		}
	} else if result != nil && errors.Is(result.Error, ErrCircuitOpen) {
		// Fast-fail while CB open: keep scan cadence for HalfOpen probes.
		task.Status = ScanTaskStatusIdle
		if task.BaseInterval > 0 {
			task.Interval = task.BaseInterval
		}
	} else {
		task.ConsecutiveFailures++
		task.ConsecutiveSuccess = 0
		task.LastFailure = time.Now()
		task.FailRate = (task.FailRate*0.8 + 1.0*0.2)

		// 冷却降级：连续失败后按 2^n 递增间隔（第 3 次失败起倍增，封顶 64s）。
		if task.ConsecutiveFailures >= 3 && se.taskDegradeOnFailureLocked(task) {
			shift := task.ConsecutiveFailures - 2
			if shift > 6 {
				shift = 6
			}
			newInterval := task.Interval * (1 << shift)
			if newInterval > 64*time.Second {
				newInterval = 64 * time.Second
			}
			if task.BaseInterval > 0 && newInterval < task.BaseInterval {
				newInterval = task.BaseInterval
			}
			if newInterval < time.Millisecond {
				newInterval = time.Millisecond
			}
			if newInterval != task.Interval {
				zap.L().Warn("[降级] 任务失败率过高，调整采集间隔",
					zap.String("taskID", task.ID),
					zap.String("deviceKey", task.DeviceKey),
					zap.Int("failures", task.ConsecutiveFailures),
					zap.Duration("oldInterval", task.Interval),
					zap.Duration("newInterval", newInterval),
				)
				task.Interval = newInterval
				task.Status = ScanTaskStatusDegraded
			}
		}
	}
	task.mu.Unlock()
}

// taskDegradeOnFailureLocked assumes the caller already holds task.mu.
func (se *ScanEngine) taskDegradeOnFailureLocked(task *ScanTask) bool {
	if task.Params == nil {
		return true
	}
	if v, ok := task.Params["degradeOnFailure"].(bool); ok {
		return v
	}
	return true
}

func (se *ScanEngine) taskDegradeOnFailure(task *ScanTask) bool {
	task.mu.RLock()
	defer task.mu.RUnlock()
	return se.taskDegradeOnFailureLocked(task)
}

func (se *ScanEngine) AddTask(deviceKey, protocol string, interval time.Duration, priority int, pointIDs []string, params map[string]any) *ScanTask {
	return se.addTask(deviceKey, protocol, "", interval, priority, pointIDs, params)
}

func (se *ScanEngine) AddTaskWithScanClass(deviceKey, protocol, scanClass string, interval time.Duration, priority int, pointIDs []string, params map[string]any) *ScanTask {
	return se.addTask(deviceKey, protocol, scanClass, interval, priority, pointIDs, params)
}

func (se *ScanEngine) addTask(deviceKey, protocol, scanClass string, interval time.Duration, priority int, pointIDs []string, params map[string]any) *ScanTask {
	se.mu.Lock()
	defer se.mu.Unlock()

	taskID := fmt.Sprintf("task_%d_%s", se.taskIDCounter, deviceKey)
	if scanClass != "" {
		taskID = fmt.Sprintf("task_%d_%s_%s", se.taskIDCounter, deviceKey, scanClass)
	}
	se.taskIDCounter++

	var points []model.Point
	if params != nil {
		if pts, ok := params["points"].([]model.Point); ok {
			points = pts
		}
	}

	now := time.Now()
	base := now

	task := &ScanTask{
		ID:              taskID,
		DeviceKey:       deviceKey,
		ScanClass:       scanClass,
		Protocol:        protocol,
		Interval:        interval,
		BaseInterval:    interval,
		LastScheduledAt: base,
		NextRun:         base,
		Priority:        priority,
		FailRate:        0,
		Status:          ScanTaskStatusIdle,
		PointIDs:        pointIDs,
		Points:          points,
		Params:          params,
		LastSuccess:     time.Time{},
		LastFailure:     time.Time{},
	}

	se.tasks[taskID] = task
	task.queued = true
	heap.Push(se.priorityQueue, task)

	zap.L().Info("[ScanEngine] 添加任务",
		zap.String("taskID", taskID),
		zap.String("deviceKey", deviceKey),
		zap.String("scanClass", scanClass),
		zap.String("protocol", protocol),
		zap.Duration("interval", interval),
		zap.Int("priority", priority),
		zap.Int("pointsCount", len(pointIDs)),
	)

	return task
}

func (se *ScanEngine) RemoveTask(taskID string) {
	se.mu.Lock()
	defer se.mu.Unlock()

	if task, exists := se.tasks[taskID]; exists {
		task.SetStatus(ScanTaskStatusStopped)
		delete(se.tasks, taskID)
		se.removeFromQueueLocked(task)
		zap.L().Info("[ScanEngine] 移除任务",
			zap.String("taskID", taskID),
			zap.String("deviceKey", task.DeviceKey),
		)
	}
}

// removeFromQueueLocked drops a task's entry from the priority queue if present.
// Caller must hold se.mu. Idempotent: safe when the task is mid-flight (absent).
func (se *ScanEngine) removeFromQueueLocked(task *ScanTask) {
	pq := se.priorityQueue
	for i := 0; i < pq.Len(); {
		if (*pq)[i] == task {
			heap.Remove(pq, i)
		} else {
			i++
		}
	}
	task.setQueued(false)
}

func (se *ScanEngine) RemoveTasksByDeviceKey(deviceKey string) {
	se.mu.Lock()
	defer se.mu.Unlock()

	for taskID, task := range se.tasks {
		if task.DeviceKey == deviceKey {
			task.SetStatus(ScanTaskStatusStopped)
			delete(se.tasks, taskID)
			zap.L().Info("[ScanEngine] 移除任务",
				zap.String("taskID", taskID),
				zap.String("deviceKey", deviceKey),
			)
		}
	}

	// 同步从优先队列中清除残留任务指针，防止 processReadyTasks 再次弹出执行。
	pq := se.priorityQueue
	for i := 0; i < pq.Len(); {
		t := (*pq)[i]
		if t.DeviceKey == deviceKey {
			t.setQueued(false)
			heap.Remove(pq, i)
		} else {
			i++
		}
	}
}

func (se *ScanEngine) GetTask(taskID string) *ScanTask {
	se.mu.RLock()
	defer se.mu.RUnlock()
	return se.tasks[taskID]
}

func (se *ScanEngine) GetTaskByDeviceKey(deviceKey string) *ScanTask {
	se.mu.RLock()
	defer se.mu.RUnlock()
	for _, task := range se.tasks {
		if task.DeviceKey == deviceKey {
			return task
		}
	}
	return nil
}

func (se *ScanEngine) GetTasksByDeviceKey(deviceKey string) []*ScanTask {
	se.mu.RLock()
	defer se.mu.RUnlock()
	var tasks []*ScanTask
	for _, task := range se.tasks {
		if task.DeviceKey == deviceKey {
			tasks = append(tasks, task)
		}
	}
	return tasks
}

func (se *ScanEngine) GetTasks() []*ScanTask {
	se.mu.RLock()
	defer se.mu.RUnlock()
	tasks := make([]*ScanTask, 0, len(se.tasks))
	for _, task := range se.tasks {
		tasks = append(tasks, task)
	}
	return tasks
}

func (se *ScanEngine) UpdateTaskInterval(deviceKey string, interval time.Duration) {
	se.mu.Lock()
	defer se.mu.Unlock()

	task := se.findTaskLocked(deviceKey)
	if task != nil {
		task.mu.Lock()
		task.Interval = interval
		task.BaseInterval = interval
		task.mu.Unlock()
		se.rescheduleTask(task, time.Now())
		zap.L().Info("[ScanEngine] 更新任务间隔",
			zap.String("taskID", task.ID),
			zap.Duration("interval", interval),
		)
	}
}

func (se *ScanEngine) UpdateTaskDriverConfig(deviceKey string, updates map[string]any) {
	if len(updates) == 0 {
		return
	}
	se.mu.Lock()
	defer se.mu.Unlock()

	for _, task := range se.tasks {
		if task.DeviceKey != deviceKey {
			continue
		}
		params := task.paramsSnapshot()
		if params == nil {
			continue
		}
		base, ok := params["driverConfig"].(map[string]any)
		if !ok || base == nil {
			base = map[string]any{}
		}
		// Build a fresh driverConfig so readers holding the previous
		// reference never observe a mutation.
		nextDriverCfg := make(map[string]any, len(base)+len(updates))
		for k, v := range base {
			nextDriverCfg[k] = v
		}
		for k, v := range updates {
			nextDriverCfg[k] = v
		}
		// Build a fresh outer params map and publish it atomically.
		nextParams := make(map[string]any, len(params))
		for k, v := range params {
			nextParams[k] = v
		}
		nextParams["driverConfig"] = nextDriverCfg
		task.setParams(nextParams)
	}
}

func (se *ScanEngine) findTaskLocked(key string) *ScanTask {
	if task, exists := se.tasks[key]; exists {
		return task
	}
	for _, task := range se.tasks {
		if task.DeviceKey == key {
			return task
		}
	}
	return nil
}

func (se *ScanEngine) UpdateTaskPriority(deviceKey string, priority int) {
	se.mu.Lock()
	defer se.mu.Unlock()

	task := se.findTaskLocked(deviceKey)
	if task != nil {
		task.mu.Lock()
		task.Priority = priority
		task.mu.Unlock()
		zap.L().Info("[ScanEngine] 更新任务优先级",
			zap.String("taskID", task.ID),
			zap.Int("priority", priority),
		)
	}
}

func (se *ScanEngine) IsRunning() bool {
	se.mu.RLock()
	defer se.mu.RUnlock()
	return se.running
}

func (se *ScanEngine) GetActiveTaskCount() int {
	se.mu.RLock()
	defer se.mu.RUnlock()
	count := 0
	for _, task := range se.tasks {
		if task.GetStatus() == ScanTaskStatusRunning {
			count++
		}
	}
	return count
}

func (se *ScanEngine) GetPendingTaskCount() int {
	se.mu.RLock()
	defer se.mu.RUnlock()
	return len(*se.priorityQueue)
}

func (se *ScanEngine) RegisterProtocol(protocol string, pType ProtocolType) {
	se.executionLayer.RegisterProtocol(protocol, pType)
}

func (se *ScanEngine) RegisterDriver(deviceKey string, d driver.Driver) {
	se.executionLayer.RegisterDriver(deviceKey, d)
}

func (se *ScanEngine) UnregisterDriver(deviceKey string) {
	se.executionLayer.UnregisterDriver(deviceKey)
}

func (se *ScanEngine) SetShadowCore(sc *ShadowCore) {
	se.mu.Lock()
	defer se.mu.Unlock()
	se.shadowCore = sc
}

func (se *ScanEngine) SetShadowIngress(si *ShadowIngress) {
	se.mu.Lock()
	defer se.mu.Unlock()
	se.shadowIngress = si
	if si != nil {
		se.shadowCore = si.shadowCore
	}
}

func (se *ScanEngine) SetCollectFinalize(fn CollectFinalizeFunc) {
	se.mu.Lock()
	defer se.mu.Unlock()
	se.collectFinalize = fn
}

func (se *ScanEngine) GetShadowCore() *ShadowCore {
	return se.shadowCore
}

func (se *ScanEngine) SetPointDegradation(m *PointDegradationManager) {
	se.mu.Lock()
	se.pointDegrade = m
	se.mu.Unlock()
	if se.executionLayer != nil {
		se.executionLayer.SetPointDegradation(m)
	}
}

func (se *ScanEngine) SetIOProfileProvider(fn IOProfileProvider) {
	if se.executionLayer != nil {
		se.executionLayer.SetIOProfileProvider(fn)
	}
}

func (se *ScanEngine) GetMetrics() *ScanEngineMetrics {
	return se.metrics
}

func (se *ScanEngine) GetCircuitBreaker() *DriverCircuitBreaker {
	if se == nil || se.executionLayer == nil {
		return nil
	}
	return se.executionLayer.GetCircuitBreaker()
}

func (se *ScanEngine) SetCircuitBreakerEventHandler(fn CircuitBreakerEventHandler) {
	if se.executionLayer != nil {
		se.executionLayer.SetCircuitBreakerEventHandler(fn)
	}
}

func (se *ScanEngine) OperationalSnapshot() map[string]any {
	out := map[string]any{}
	if se == nil || se.executionLayer == nil {
		return out
	}
	out["serial_queue_depth"] = se.executionLayer.GetSerialQueueDepths()
	if bp := se.executionLayer.GetBackpressure(); bp != nil {
		out["backpressure_reject_total"] = bp.RejectTotal()
		out["throttle_reject_by_reason"] = bp.RejectByReason()
	}
	return out
}

func (se *ScanEngine) ExecuteTask(task *ScanTask) *ExecuteResult {
	if se == nil || se.executionLayer == nil || task == nil {
		return &ExecuteResult{Success: false, Error: ErrDriverNotFound}
	}
	return se.executionLayer.Execute(task)
}
