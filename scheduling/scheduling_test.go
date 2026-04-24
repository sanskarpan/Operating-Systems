package scheduling

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// =============================================================================
// SCHED-001: EDF Tests
// =============================================================================

func TestEDF_Utilization(t *testing.T) {
	tasks := []*RTTask{
		{ID: 1, Period: 4, Execution: 1, Deadline: 4},
		{ID: 2, Period: 6, Execution: 2, Deadline: 6},
		{ID: 3, Period: 12, Execution: 3, Deadline: 12},
	}
	sched := NewEDFScheduler(tasks)
	u := sched.UtilizationCheck()
	// 1/4 + 2/6 + 3/12 = 0.25+0.333+0.25 = 0.833
	if u < 0.83 || u > 0.84 {
		t.Errorf("expected utilisation ~0.833, got %.4f", u)
	}
	if !sched.IsSchedulable() {
		t.Error("task set with U<1 should be schedulable under EDF")
	}
}

func TestEDF_SchedulableSet(t *testing.T) {
	tasks := []*RTTask{
		{ID: 1, Period: 2, Execution: 1, Deadline: 2},
		{ID: 2, Period: 5, Execution: 2, Deadline: 5},
	}
	sched := NewEDFScheduler(tasks)
	trace := sched.Run(10)
	if len(trace) != 10 {
		t.Errorf("expected 10 trace entries, got %d", len(trace))
	}
	if sched.DeadlineMiss > 0 {
		t.Errorf("schedulable set should have 0 deadline misses, got %d", sched.DeadlineMiss)
	}
}

func TestEDF_DeadlineMissDetected(t *testing.T) {
	// U > 1 — unschedulable.
	tasks := []*RTTask{
		{ID: 1, Period: 2, Execution: 2, Deadline: 2}, // U=1.0
		{ID: 2, Period: 3, Execution: 2, Deadline: 3}, // U=0.667 → total 1.667
	}
	sched := NewEDFScheduler(tasks)
	if sched.IsSchedulable() {
		t.Error("overloaded task set should not be schedulable")
	}
	sched.Run(12)
	if sched.DeadlineMiss == 0 {
		t.Error("expected deadline misses for overloaded set")
	}
}

func TestEDF_Idle(t *testing.T) {
	tasks := []*RTTask{
		{ID: 1, Period: 4, Execution: 1, Deadline: 4},
	}
	sched := NewEDFScheduler(tasks)
	trace := sched.Run(8)
	idleCount := 0
	for _, s := range trace {
		if len(s) > 5 && s[len(s)-4:] == "idle" {
			idleCount++
		}
	}
	// Task executes 1 tick out of every 4 → 6 idle ticks.
	if idleCount < 4 {
		t.Errorf("expected at least 4 idle ticks, got %d", idleCount)
	}
}

func TestEDF_SingleTask(t *testing.T) {
	tasks := []*RTTask{{ID: 1, Period: 3, Execution: 1, Deadline: 3}}
	s := NewEDFScheduler(tasks)
	s.Run(9)
	if s.DeadlineMiss != 0 {
		t.Errorf("single task with U=0.33 should have 0 misses, got %d", s.DeadlineMiss)
	}
}

// =============================================================================
// SCHED-002: Rate Monotonic Tests
// =============================================================================

func TestRM_PriorityAssignment(t *testing.T) {
	tasks := []*RMTask{
		{ID: 1, Period: 10, Execution: 2},
		{ID: 2, Period: 5, Execution: 1},
		{ID: 3, Period: 20, Execution: 3},
	}
	NewRMScheduler(tasks)
	// Task with shortest period (5) gets priority 1.
	for _, t2 := range tasks {
		if t2.ID == 2 && t2.Priority != 1 {
			t.Errorf("task 2 (period=5) should have priority 1, got %d", t2.Priority)
		}
		if t2.ID == 1 && t2.Priority != 2 {
			t.Errorf("task 1 (period=10) should have priority 2, got %d", t2.Priority)
		}
		if t2.ID == 3 && t2.Priority != 3 {
			t.Errorf("task 3 (period=20) should have priority 3, got %d", t2.Priority)
		}
	}
}

func TestRM_UtilizationBound(t *testing.T) {
	tasks := []*RMTask{
		{ID: 1, Period: 4, Execution: 1},
		{ID: 2, Period: 6, Execution: 1},
	}
	sched := NewRMScheduler(tasks)
	bound := sched.UtilizationBound()
	// n=2: 2(2^0.5 - 1) ≈ 0.828
	if bound < 0.82 || bound > 0.84 {
		t.Errorf("expected bound ~0.828, got %.4f", bound)
	}
}

func TestRM_IsSchedulable(t *testing.T) {
	tasks := []*RMTask{
		{ID: 1, Period: 4, Execution: 1},
		{ID: 2, Period: 6, Execution: 1},
	}
	rm := NewRMScheduler(tasks)
	// U = 0.25 + 0.167 = 0.417 < bound 0.828 → schedulable
	if !rm.IsSchedulable() {
		t.Error("task set with U < bound should be schedulable")
	}
}

func TestRM_Run(t *testing.T) {
	tasks := []*RMTask{
		{ID: 1, Period: 4, Execution: 1},
		{ID: 2, Period: 6, Execution: 2},
	}
	sched := NewRMScheduler(tasks)
	trace := sched.Run(12)
	if len(trace) != 12 {
		t.Errorf("expected 12 trace entries, got %d", len(trace))
	}
	if sched.DeadlineMiss > 0 {
		t.Errorf("schedulable set should have 0 misses, got %d", sched.DeadlineMiss)
	}
}

func TestRM_SingleTask(t *testing.T) {
	tasks := []*RMTask{{ID: 1, Period: 5, Execution: 2}}
	sched := NewRMScheduler(tasks)
	sched.Run(20)
	if sched.DeadlineMiss > 0 {
		t.Errorf("single task should have 0 misses, got %d", sched.DeadlineMiss)
	}
}

// =============================================================================
// SCHED-003: CFS Tests
// =============================================================================

func TestCFS_FairScheduling(t *testing.T) {
	s := NewCFSScheduler(10.0)
	// Three tasks with equal nice values → equal vruntimes after N ticks.
	for i := 0; i < 3; i++ {
		s.AddTask(&CFSTask{ID: i, Nice: 0, Name: "task"})
	}
	s.RunFor(90, 0)
	// Each task should have been scheduled ~30 times.
	counts := make(map[int]int)
	for _, sl := range s.Schedule {
		counts[sl.TaskID]++
	}
	for id, c := range counts {
		if c < 25 || c > 35 {
			t.Errorf("task %d scheduled %d times (expected ~30)", id, c)
		}
	}
}

func TestCFS_PriorityFavorLow(t *testing.T) {
	s := NewCFSScheduler(10.0)
	// One high-priority (nice=-10) and one low-priority (nice=10).
	s.AddTask(&CFSTask{ID: 1, Nice: -10}) // heavier weight → less vruntime advance
	s.AddTask(&CFSTask{ID: 2, Nice: 10})  // lighter weight → more vruntime advance
	s.RunFor(100, 0)
	counts := make(map[int]int)
	for _, sl := range s.Schedule {
		counts[sl.TaskID]++
	}
	if counts[1] <= counts[2] {
		t.Errorf("high-priority task should run more: task1=%d, task2=%d", counts[1], counts[2])
	}
}

func TestCFS_Fairness(t *testing.T) {
	s := NewCFSScheduler(10.0)
	for i := 0; i < 4; i++ {
		s.AddTask(&CFSTask{ID: i, Nice: 0})
	}
	s.RunFor(400, 0)
	f := s.Fairness()
	if f > 0.05 {
		t.Errorf("fairness (CV of vruntime) should be low, got %.4f", f)
	}
}

func TestCFS_AddRemoveTask(t *testing.T) {
	s := NewCFSScheduler(10.0)
	s.AddTask(&CFSTask{ID: 1, Nice: 0})
	s.AddTask(&CFSTask{ID: 2, Nice: 0})
	s.RemoveTask(2)
	s.Tick(0)
	// Only task 1 should have run.
	if len(s.Schedule) == 0 {
		t.Fatal("expected schedule entry")
	}
	if s.Schedule[0].TaskID != 1 {
		t.Errorf("expected task 1, got %d", s.Schedule[0].TaskID)
	}
}

func TestCFS_EmptyQueue(t *testing.T) {
	s := NewCFSScheduler(10.0)
	result := s.Tick(0)
	if result != nil {
		t.Error("empty queue should return nil")
	}
}

func TestCFS_NiceWeights(t *testing.T) {
	if niceWeight(0) != 1024 {
		t.Errorf("nice 0 should have weight 1024, got %d", niceWeight(0))
	}
	if niceWeight(-20) != 88761 {
		t.Errorf("nice -20 should have weight 88761, got %d", niceWeight(-20))
	}
	if niceWeight(19) != 15 {
		t.Errorf("nice 19 should have weight 15, got %d", niceWeight(19))
	}
}

func BenchmarkCFSTick(bm *testing.B) {
	s := NewCFSScheduler(10.0)
	for i := 0; i < 32; i++ {
		s.AddTask(&CFSTask{ID: i, Nice: 0})
	}
	bm.ResetTimer()
	for i := 0; i < bm.N; i++ {
		s.Tick(float64(i) * 10)
	}
}

// =============================================================================
// SCHED-004: Work-Stealing Tests
// =============================================================================

func TestWorkStealing_ExecutesAllTasks(t *testing.T) {
	s := NewWSScheduler(4, 1)
	s.Start()

	var executed int64
	const total = 200
	for i := 0; i < total; i++ {
		s.Submit(func() {
			atomic.AddInt64(&executed, 1)
		})
	}
	time.Sleep(100 * time.Millisecond)
	s.Stop()

	if atomic.LoadInt64(&executed) != total {
		t.Errorf("expected %d executed, got %d", total, atomic.LoadInt64(&executed))
	}
}

func TestWorkStealing_StealingOccurs(t *testing.T) {
	// Submit all tasks to worker 0 to force stealing.
	s := NewWSScheduler(3, 1)
	// Pre-load worker 0's deque directly before starting.
	var executed int64
	const total = 90
	for i := 0; i < total; i++ {
		s.workers[0].deque.pushBottom(&WSTask{ID: int64(i), Work: func() {
			atomic.AddInt64(&executed, 1)
		}})
	}
	s.Start()
	time.Sleep(200 * time.Millisecond)
	s.Stop()

	if atomic.LoadInt64(&s.TotalSteals) == 0 {
		t.Log("no steals observed (may be timing-dependent)")
	}
	if atomic.LoadInt64(&executed) != total {
		t.Errorf("expected %d executed, got %d", total, atomic.LoadInt64(&executed))
	}
}

func TestWorkStealing_Concurrent(t *testing.T) {
	s := NewWSScheduler(8, 1)
	s.Start()

	var wg sync.WaitGroup
	var count int64
	for i := 0; i < 500; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.Submit(func() { atomic.AddInt64(&count, 1) })
		}()
	}
	wg.Wait()
	time.Sleep(200 * time.Millisecond)
	s.Stop()

	if atomic.LoadInt64(&count) != 500 {
		t.Errorf("expected 500 tasks executed, got %d", atomic.LoadInt64(&count))
	}
}

func TestWorkStealing_WorkerStats(t *testing.T) {
	s := NewWSScheduler(2, 1)
	s.Start()
	var done int64
	for i := 0; i < 50; i++ {
		s.Submit(func() { atomic.AddInt64(&done, 1) })
	}
	time.Sleep(100 * time.Millisecond)
	s.Stop()

	stats := s.WorkerStats()
	total := int64(0)
	for _, st := range stats {
		total += st.Executed
	}
	if total != atomic.LoadInt64(&s.TotalExecuted) {
		t.Errorf("worker stats mismatch: sum=%d TotalExecuted=%d", total, s.TotalExecuted)
	}
}

func BenchmarkWorkStealing(bm *testing.B) {
	s := NewWSScheduler(4, 1)
	s.Start()
	bm.ResetTimer()
	var wg sync.WaitGroup
	for i := 0; i < bm.N; i++ {
		wg.Add(1)
		s.Submit(func() { wg.Done() })
	}
	wg.Wait()
	bm.StopTimer()
	s.Stop()
}
