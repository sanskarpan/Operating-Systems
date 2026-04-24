/*
Advanced CPU Scheduling
=======================

Real-time and modern scheduling algorithms:
  - EDF  (Earliest Deadline First)  — SCHED-001
  - Rate Monotonic                  — SCHED-002
  - CFS  (Completely Fair Scheduler)— SCHED-003
  - Work-Stealing Scheduler         — SCHED-004
*/

package scheduling

import (
	"container/heap"
	"errors"
	"fmt"
	"math"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// =============================================================================
// SCHED-001: EDF — Earliest Deadline First
// =============================================================================

// RTTask is a real-time periodic task.
type RTTask struct {
	ID          int
	Period      int // time units between successive releases
	Execution   int // worst-case execution time
	Deadline    int // relative deadline (≤ Period for constrained tasks)
	absDeadline int // absolute deadline of the current job
	remaining   int // execution units left in current job
	released    int // time when the current job was released
}

// edfEntry is used by the EDF priority queue (min-heap on absDeadline).
type edfEntry struct {
	task  *RTTask
	index int
}

type edfHeap []*edfEntry

func (h edfHeap) Len() int            { return len(h) }
func (h edfHeap) Less(i, j int) bool  { return h[i].task.absDeadline < h[j].task.absDeadline }
func (h edfHeap) Swap(i, j int)       { h[i], h[j] = h[j], h[i]; h[i].index = i; h[j].index = j }
func (h *edfHeap) Push(x interface{}) { e := x.(*edfEntry); e.index = len(*h); *h = append(*h, e) }
func (h *edfHeap) Pop() interface{}   { old := *h; n := len(old); e := old[n-1]; *h = old[:n-1]; return e }

// EDFScheduler implements the Earliest Deadline First real-time scheduler.
// It operates in discrete time units (ticks).
type EDFScheduler struct {
	tasks        []*RTTask
	readyQueue   edfHeap
	currentTime  int
	DeadlineMiss int      // total deadline misses observed
	Schedule     []string // log of scheduling decisions
	mu           sync.Mutex
}

// NewEDFScheduler creates an EDF scheduler with the given tasks.
func NewEDFScheduler(tasks []*RTTask) *EDFScheduler {
	return &EDFScheduler{tasks: tasks}
}

// UtilizationCheck returns the total CPU utilisation U = Σ(Ci/Ti).
// EDF is schedulable on a single processor iff U ≤ 1.0.
func (e *EDFScheduler) UtilizationCheck() float64 {
	u := 0.0
	for _, t := range e.tasks {
		u += float64(t.Execution) / float64(t.Period)
	}
	return u
}

// IsSchedulable reports whether the task set is schedulable under EDF.
func (e *EDFScheduler) IsSchedulable() bool {
	return e.UtilizationCheck() <= 1.0
}

// Run simulates the EDF scheduler for `duration` time units.
// Returns the scheduling trace (one entry per tick).
func (e *EDFScheduler) Run(duration int) []string {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Reset state.
	for _, t := range e.tasks {
		t.remaining = t.Execution
		t.absDeadline = t.Deadline
		t.released = 0
	}
	e.readyQueue = edfHeap{}
	e.currentTime = 0
	e.DeadlineMiss = 0
	e.Schedule = nil

	// Initial job releases at t=0.
	for _, t := range e.tasks {
		heap.Push(&e.readyQueue, &edfEntry{task: t})
	}

	for tick := 0; tick < duration; tick++ {
		// Release new jobs whose period boundary has been reached.
		for _, t := range e.tasks {
			if tick > 0 && tick%t.Period == 0 {
				// Check if previous job missed deadline.
				if t.remaining > 0 {
					e.DeadlineMiss++
					e.Schedule = append(e.Schedule, fmt.Sprintf("t=%d: DEADLINE MISS task %d", tick, t.ID))
				}
				t.remaining = t.Execution
				t.released = tick
				t.absDeadline = tick + t.Deadline
				heap.Push(&e.readyQueue, &edfEntry{task: t})
			}
		}

		// Check for deadline misses at current tick (before running).
		for _, t := range e.tasks {
			if t.remaining > 0 && t.absDeadline == tick {
				e.DeadlineMiss++
				e.Schedule = append(e.Schedule, fmt.Sprintf("t=%d: DEADLINE MISS task %d", tick, t.ID))
			}
		}

		if len(e.readyQueue) == 0 {
			e.Schedule = append(e.Schedule, fmt.Sprintf("t=%d: idle", tick))
			continue
		}

		// Run the task with the earliest deadline.
		top := e.readyQueue[0]
		t := top.task
		t.remaining--
		e.Schedule = append(e.Schedule, fmt.Sprintf("t=%d: task %d (deadline=%d, remaining=%d)", tick, t.ID, t.absDeadline, t.remaining))

		if t.remaining == 0 {
			heap.Pop(&e.readyQueue)
		}
	}
	e.currentTime = duration
	return e.Schedule
}

// =============================================================================
// SCHED-002: Rate Monotonic Scheduler
// =============================================================================

// RMTask is a periodic real-time task for Rate Monotonic scheduling.
type RMTask struct {
	ID        int
	Period    int // release period
	Execution int // worst-case execution time
	Priority  int // assigned by RM: shorter period → higher priority (lower number)
	remaining int
}

// RMScheduler implements Rate Monotonic fixed-priority scheduling.
type RMScheduler struct {
	tasks       []*RMTask
	DeadlineMiss int
	Schedule    []string
	mu          sync.Mutex
}

// NewRMScheduler creates an RM scheduler and assigns RM priorities.
// RM rule: shorter period → higher priority (priority 1 = highest).
func NewRMScheduler(tasks []*RMTask) *RMScheduler {
	// Sort by period ascending to assign priorities.
	sorted := make([]*RMTask, len(tasks))
	copy(sorted, tasks)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Period < sorted[j].Period })
	for i, t := range sorted {
		t.Priority = i + 1
	}
	return &RMScheduler{tasks: tasks}
}

// UtilizationBound returns Liu & Layland's bound: n(2^(1/n) − 1).
func (r *RMScheduler) UtilizationBound() float64 {
	n := float64(len(r.tasks))
	return n * (math.Pow(2, 1/n) - 1)
}

// Utilization returns the actual CPU utilisation U = Σ(Ci/Ti).
func (r *RMScheduler) Utilization() float64 {
	u := 0.0
	for _, t := range r.tasks {
		u += float64(t.Execution) / float64(t.Period)
	}
	return u
}

// IsSchedulable reports whether U ≤ UtilizationBound (sufficient condition).
func (r *RMScheduler) IsSchedulable() bool {
	return r.Utilization() <= r.UtilizationBound()
}

// Run simulates the RM scheduler for `duration` time units.
func (r *RMScheduler) Run(duration int) []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, t := range r.tasks {
		t.remaining = t.Execution
	}
	r.DeadlineMiss = 0
	r.Schedule = nil

	for tick := 0; tick < duration; tick++ {
		// Release new jobs at period boundaries.
		for _, t := range r.tasks {
			if tick > 0 && tick%t.Period == 0 {
				if t.remaining > 0 {
					r.DeadlineMiss++
					r.Schedule = append(r.Schedule, fmt.Sprintf("t=%d: DEADLINE MISS task %d", tick, t.ID))
				}
				t.remaining = t.Execution
			}
		}

		// Pick highest-priority ready task (lowest Priority number).
		var selected *RMTask
		for _, t := range r.tasks {
			if t.remaining > 0 {
				if selected == nil || t.Priority < selected.Priority {
					selected = t
				}
			}
		}

		if selected == nil {
			r.Schedule = append(r.Schedule, fmt.Sprintf("t=%d: idle", tick))
			continue
		}

		selected.remaining--
		r.Schedule = append(r.Schedule, fmt.Sprintf("t=%d: task %d (priority=%d, remaining=%d)",
			tick, selected.ID, selected.Priority, selected.remaining))
	}
	return r.Schedule
}

// =============================================================================
// SCHED-003: CFS — Completely Fair Scheduler
// =============================================================================

// CFSTask represents a task in the CFS scheduler.
type CFSTask struct {
	ID       int
	Nice     int     // -20..19, default 0
	Weight   int     // derived from Nice via the Linux weight table
	VRuntime float64 // virtual runtime (ns equivalent)
	Name     string
}

// niceToWeight maps nice values (-20..19) to CFS weights (Linux kernel values).
var niceToWeight = [40]int{
	88761, 71755, 56483, 46273, 36291,
	29154, 23254, 18705, 14949, 11916,
	9548, 7620, 6100, 4904, 3906,
	3121, 2501, 1991, 1586, 1277,
	1024, 820, 655, 526, 423,
	335, 272, 215, 172, 137,
	110, 87, 70, 56, 45,
	36, 29, 23, 18, 15,
}

func niceWeight(nice int) int {
	idx := nice + 20
	if idx < 0 {
		idx = 0
	}
	if idx >= 40 {
		idx = 39
	}
	return niceToWeight[idx]
}

// cfsHeap is a min-heap ordered by VRuntime.
type cfsHeap []*CFSTask

func (h cfsHeap) Len() int            { return len(h) }
func (h cfsHeap) Less(i, j int) bool  { return h[i].VRuntime < h[j].VRuntime }
func (h cfsHeap) Swap(i, j int)       { h[i], h[j] = h[j], h[i] }
func (h *cfsHeap) Push(x interface{}) { *h = append(*h, x.(*CFSTask)) }
func (h *cfsHeap) Pop() interface{}   { old := *h; n := len(old); t := old[n-1]; *h = old[:n-1]; return t }

// CFSScheduler implements a simplified CFS scheduler.
// Each scheduling period (timeslice) the task with minimum vruntime runs.
// Its vruntime is incremented by: delta * (weightRef / taskWeight)
// where weightRef = weight of nice-0 task (1024).
type CFSScheduler struct {
	mu          sync.Mutex
	rq          cfsHeap     // run queue (min-heap)
	minVRuntime float64     // global min vruntime
	timeslice   float64     // target scheduling timeslice (wall-clock units)
	Schedule    []CFSSlice  // trace
}

// CFSSlice records one scheduling decision.
type CFSSlice struct {
	TaskID    int
	VRuntime  float64
	WallClock float64
}

const cfsWeightRef = 1024 // weight of nice-0 task

// NewCFSScheduler creates a CFS scheduler.
// timeslice is the target scheduling period in wall-clock units (e.g. ms).
func NewCFSScheduler(timeslice float64) *CFSScheduler {
	s := &CFSScheduler{timeslice: timeslice}
	heap.Init(&s.rq)
	return s
}

// AddTask adds a task to the scheduler's run queue.
// Its initial vruntime is set to max(0, minVRuntime) so it doesn't get starved.
func (s *CFSScheduler) AddTask(t *CFSTask) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t.Weight = niceWeight(t.Nice)
	if t.VRuntime < s.minVRuntime {
		t.VRuntime = s.minVRuntime
	}
	heap.Push(&s.rq, t)
}

// RemoveTask removes a task from the run queue (e.g. it blocked).
func (s *CFSScheduler) RemoveTask(id int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, t := range s.rq {
		if t.ID == id {
			heap.Remove(&s.rq, i)
			return true
		}
	}
	return false
}

// Tick advances the scheduler by one timeslice, running the min-vruntime task.
// Returns the task that ran, or nil if the run queue is empty.
func (s *CFSScheduler) Tick(wallClock float64) *CFSTask {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.rq) == 0 {
		return nil
	}

	// Pop the task with min vruntime.
	t := heap.Pop(&s.rq).(*CFSTask)

	// Advance vruntime: delta_vruntime = timeslice * (refWeight / taskWeight)
	deltaVR := s.timeslice * float64(cfsWeightRef) / float64(t.Weight)
	t.VRuntime += deltaVR

	// Update global min_vruntime.
	if len(s.rq) > 0 {
		s.minVRuntime = s.rq[0].VRuntime
	}

	s.Schedule = append(s.Schedule, CFSSlice{TaskID: t.ID, VRuntime: t.VRuntime, WallClock: wallClock})

	// Re-insert the task.
	heap.Push(&s.rq, t)
	return t
}

// RunFor simulates CFS for `ticks` scheduling periods.
// wallClockStart is the starting wall-clock value; each tick advances it by timeslice.
func (s *CFSScheduler) RunFor(ticks int, wallClockStart float64) []CFSSlice {
	for i := 0; i < ticks; i++ {
		s.Tick(wallClockStart + float64(i)*s.timeslice)
	}
	return s.Schedule
}

// Fairness returns the coefficient of variation of vruntime across tasks.
// A value close to 0 means very fair scheduling.
func (s *CFSScheduler) Fairness() float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.rq) == 0 {
		return 0
	}
	sum := 0.0
	for _, t := range s.rq {
		sum += t.VRuntime
	}
	mean := sum / float64(len(s.rq))
	variance := 0.0
	for _, t := range s.rq {
		d := t.VRuntime - mean
		variance += d * d
	}
	variance /= float64(len(s.rq))
	if mean == 0 {
		return 0
	}
	return math.Sqrt(variance) / mean
}

// =============================================================================
// SCHED-004: Work-Stealing Scheduler
// =============================================================================

// WSTask is a unit of work in the work-stealing scheduler.
type WSTask struct {
	ID   int64
	Work func()
}

// wsDeque is a double-ended queue for work-stealing.
// The owner pushes/pops from the bottom; thieves steal from the top.
type wsDeque struct {
	mu    sync.Mutex
	items []*WSTask
}

func (d *wsDeque) pushBottom(t *WSTask) {
	d.mu.Lock()
	d.items = append(d.items, t)
	d.mu.Unlock()
}

func (d *wsDeque) popBottom() (*WSTask, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.items) == 0 {
		return nil, false
	}
	n := len(d.items)
	t := d.items[n-1]
	d.items = d.items[:n-1]
	return t, true
}

func (d *wsDeque) stealTop() (*WSTask, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.items) == 0 {
		return nil, false
	}
	t := d.items[0]
	d.items = d.items[1:]
	return t, true
}

func (d *wsDeque) size() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.items)
}

// WSWorker is a single worker in the work-stealing pool.
type WSWorker struct {
	id        int
	deque     *wsDeque
	scheduler *WSScheduler
	Executed  int64 // tasks executed by this worker
	Stolen    int64 // tasks stolen by this worker from others
}

// WSScheduler implements a work-stealing task scheduler.
type WSScheduler struct {
	workers       []*WSWorker
	stealThreshold int // min items in victim queue to attempt steal
	done          chan struct{}
	wg            sync.WaitGroup
	taskCounter   int64
	TotalExecuted int64
	TotalSteals   int64
}

// NewWSScheduler creates a work-stealing scheduler with numWorkers workers.
// stealThreshold is the minimum queue depth before a steal is attempted.
func NewWSScheduler(numWorkers, stealThreshold int) *WSScheduler {
	s := &WSScheduler{
		stealThreshold: stealThreshold,
		done:           make(chan struct{}),
	}
	s.workers = make([]*WSWorker, numWorkers)
	for i := range s.workers {
		s.workers[i] = &WSWorker{id: i, deque: &wsDeque{}, scheduler: s}
	}
	return s
}

// Submit adds a task to the run queue of the worker with the fewest tasks.
func (s *WSScheduler) Submit(work func()) int64 {
	id := atomic.AddInt64(&s.taskCounter, 1)
	task := &WSTask{ID: id, Work: work}

	// Find least-loaded worker.
	minWorker := s.workers[0]
	minSize := minWorker.deque.size()
	for _, w := range s.workers[1:] {
		if sz := w.deque.size(); sz < minSize {
			minSize = sz
			minWorker = w
		}
	}
	minWorker.deque.pushBottom(task)
	return id
}

// Start launches all worker goroutines. Call Stop() to shut down.
func (s *WSScheduler) Start() {
	for _, w := range s.workers {
		w := w
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.runWorker(w)
		}()
	}
}

// Stop signals workers to exit after draining and waits for them.
func (s *WSScheduler) Stop() {
	close(s.done)
	s.wg.Wait()
}

// WorkerStats returns per-worker executed and stolen task counts.
func (s *WSScheduler) WorkerStats() []struct{ Executed, Stolen int64 } {
	stats := make([]struct{ Executed, Stolen int64 }, len(s.workers))
	for i, w := range s.workers {
		stats[i] = struct{ Executed, Stolen int64 }{
			atomic.LoadInt64(&w.Executed),
			atomic.LoadInt64(&w.Stolen),
		}
	}
	return stats
}

func (s *WSScheduler) runWorker(w *WSWorker) {
	for {
		// Try own deque first.
		if task, ok := w.deque.popBottom(); ok {
			task.Work()
			atomic.AddInt64(&w.Executed, 1)
			atomic.AddInt64(&s.TotalExecuted, 1)
			continue
		}

		// Try to steal from another worker.
		stolen := false
		for _, victim := range s.workers {
			if victim == w {
				continue
			}
			if victim.deque.size() < s.stealThreshold {
				continue
			}
			if task, ok := victim.deque.stealTop(); ok {
				task.Work()
				atomic.AddInt64(&w.Executed, 1)
				atomic.AddInt64(&w.Stolen, 1)
				atomic.AddInt64(&s.TotalExecuted, 1)
				atomic.AddInt64(&s.TotalSteals, 1)
				stolen = true
				break
			}
		}
		if stolen {
			continue
		}

		// Check for shutdown.
		select {
		case <-s.done:
			// Drain remaining tasks.
			for {
				if task, ok := w.deque.popBottom(); ok {
					task.Work()
					atomic.AddInt64(&w.Executed, 1)
					atomic.AddInt64(&s.TotalExecuted, 1)
				} else {
					return
				}
			}
		default:
			// Brief yield to avoid busy-spinning.
			time.Sleep(time.Microsecond)
		}
	}
}

// ErrNoTasks is returned when the scheduler has no tasks.
var ErrNoTasks = errors.New("no tasks available")
