package concurrency

// =============================================================================
// CONC-001: Santa Claus Problem
// =============================================================================
//
// Santa sleeps until either all 9 reindeer return from vacation, or a group of
// 3 elves needs help. When woken by reindeer, Santa hitches up the sleigh and
// delivers toys; when woken by elves, he helps with a problem in the shop.
// Reindeer have priority: if both conditions are simultaneously met, reindeer
// win. Elves are served in groups of 3; remaining elves are not starved because
// only one elf group can consult Santa at a time.

import (
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

const (
	santaReindeerNeeded = 9
	santaElfGroupSize   = 3
)

// SantaEvent records what Santa did.
type SantaEvent struct {
	Kind  string // "deliver" or "help-elves"
	Round int
}

// SantaSimulation runs the Santa Claus problem for a fixed number of rounds.
type SantaSimulation struct {
	mu     sync.Mutex
	events []SantaEvent

	reindeerReady chan struct{} // one send per returning reindeer
	elfReady      chan struct{} // one send per elf wanting help
	elfDone       chan struct{} // Santa signals elf group consultation is over
	sleighReady   chan struct{} // Santa signals delivery is starting

	ReindeerTrips int64 // total deliveries
	ElfHelps      int64 // total elf consultations
}

// NewSantaSimulation creates the Santa apparatus (9 reindeer slots, elf queue).
func NewSantaSimulation() *SantaSimulation {
	return &SantaSimulation{
		reindeerReady: make(chan struct{}, santaReindeerNeeded),
		elfReady:      make(chan struct{}, 100),
		elfDone:       make(chan struct{}, santaElfGroupSize),
		sleighReady:   make(chan struct{}, 1),
	}
}

// RunReindeer simulates one reindeer for numRounds trips. Each trip: vacation
// (sleep), then signal Santa, then wait for delivery, then repeat.
func (s *SantaSimulation) RunReindeer(id, numRounds int, vacationMax time.Duration) {
	for i := 0; i < numRounds; i++ {
		// On vacation.
		time.Sleep(time.Duration(rand.Int63n(int64(vacationMax) + 1)))
		// Announce return.
		s.reindeerReady <- struct{}{}
		// Wait for Santa to hitch sleigh.
		<-s.sleighReady
	}
}

// RunElf simulates one elf trying to get help numTimes times.
func (s *SantaSimulation) RunElf(id, numTimes int, thinkMax time.Duration) {
	for i := 0; i < numTimes; i++ {
		// Work independently.
		time.Sleep(time.Duration(rand.Int63n(int64(thinkMax) + 1)))
		// Join the queue.
		s.elfReady <- struct{}{}
		// Wait for Santa to consult group.
		<-s.elfDone
	}
}

// RunSanta runs Santa until stopCh is closed.
// Returns events log.
func (s *SantaSimulation) RunSanta(stopCh <-chan struct{}) {
	round := 0
	for {
		// Check stop.
		select {
		case <-stopCh:
			return
		default:
		}
		// Count ready reindeer.
		if len(s.reindeerReady) >= santaReindeerNeeded {
			// Drain exactly 9.
			for i := 0; i < santaReindeerNeeded; i++ {
				<-s.reindeerReady
			}
			round++
			s.mu.Lock()
			s.events = append(s.events, SantaEvent{Kind: "deliver", Round: round})
			s.mu.Unlock()
			atomic.AddInt64(&s.ReindeerTrips, 1)
			// Signal reindeer to go.
			for i := 0; i < santaReindeerNeeded; i++ {
				s.sleighReady <- struct{}{}
			}
			continue
		}
		// Check for elf group.
		if len(s.elfReady) >= santaElfGroupSize {
			// Serve exactly 3 elves.
			for i := 0; i < santaElfGroupSize; i++ {
				<-s.elfReady
			}
			round++
			s.mu.Lock()
			s.events = append(s.events, SantaEvent{Kind: "help-elves", Round: round})
			s.mu.Unlock()
			atomic.AddInt64(&s.ElfHelps, 1)
			// Signal elf group.
			for i := 0; i < santaElfGroupSize; i++ {
				s.elfDone <- struct{}{}
			}
			continue
		}
		// Nothing to do — sleep briefly.
		time.Sleep(time.Millisecond)
	}
}

// Events returns a snapshot of Santa's activity log.
func (s *SantaSimulation) Events() []SantaEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]SantaEvent, len(s.events))
	copy(out, s.events)
	return out
}

// =============================================================================
// CONC-002: Livelock Detector
// =============================================================================
//
// A LivelockDetector tracks goroutines that increment a "retry" counter.
// If a goroutine's retry count keeps growing but its "progress" counter stays
// flat within a detection window, it is reported as livelocked.

// LivelockMonitor tracks one goroutine.
type LivelockMonitor struct {
	Name     string
	retries  int64 // atomic — incremented on each retry
	progress int64 // atomic — incremented on actual forward progress

	lastRetries  int64
	lastProgress int64
}

// NewLivelock creates a monitor for a goroutine named name.
func NewLivelock(name string) *LivelockMonitor {
	return &LivelockMonitor{Name: name}
}

// Retry signals that the goroutine retried (no progress).
func (m *LivelockMonitor) Retry() {
	atomic.AddInt64(&m.retries, 1)
}

// Progress signals that the goroutine made forward progress.
func (m *LivelockMonitor) Progress() {
	atomic.AddInt64(&m.progress, 1)
}

// Retries returns the total retry count.
func (m *LivelockMonitor) Retries() int64 { return atomic.LoadInt64(&m.retries) }

// Progresses returns the total progress count.
func (m *LivelockMonitor) Progresses() int64 { return atomic.LoadInt64(&m.progress) }

// LivelockDetector manages a set of monitors and periodically checks for livelock.
type LivelockDetector struct {
	mu       sync.Mutex
	monitors []*LivelockMonitor
	detected []string // names of livelocked goroutines

	window       time.Duration
	minRetries   int64 // must have retried at least this many times to be considered
	stopCh       chan struct{}
	wg           sync.WaitGroup
}

// NewLivelockDetector creates a detector that samples every window.
// minRetries is the minimum retry count needed to trigger detection.
func NewLivelockDetector(window time.Duration, minRetries int64) *LivelockDetector {
	return &LivelockDetector{
		window:     window,
		minRetries: minRetries,
		stopCh:     make(chan struct{}),
	}
}

// Register adds a monitor to the detector.
func (d *LivelockDetector) Register(m *LivelockMonitor) {
	d.mu.Lock()
	d.monitors = append(d.monitors, m)
	d.mu.Unlock()
}

// Start launches the background detection loop.
func (d *LivelockDetector) Start() {
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		ticker := time.NewTicker(d.window)
		defer ticker.Stop()
		for {
			select {
			case <-d.stopCh:
				return
			case <-ticker.C:
				d.sample()
			}
		}
	}()
}

// Stop halts the detection loop and waits for it to exit.
func (d *LivelockDetector) Stop() {
	close(d.stopCh)
	d.wg.Wait()
}

// sample checks all monitors for livelock.
func (d *LivelockDetector) sample() {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, m := range d.monitors {
		r := atomic.LoadInt64(&m.retries)
		p := atomic.LoadInt64(&m.progress)
		deltaR := r - m.lastRetries
		deltaP := p - m.lastProgress
		m.lastRetries = r
		m.lastProgress = p
		// Livelocked: retries growing but no progress AND total retries significant.
		if deltaR >= d.minRetries && deltaP == 0 && r >= d.minRetries {
			alreadyDetected := false
			for _, name := range d.detected {
				if name == m.Name {
					alreadyDetected = true
					break
				}
			}
			if !alreadyDetected {
				d.detected = append(d.detected, m.Name)
			}
		}
	}
}

// Detected returns the names of goroutines diagnosed as livelocked.
func (d *LivelockDetector) Detected() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]string, len(d.detected))
	copy(out, d.detected)
	return out
}

// ForceCheck runs one immediate sample (useful in tests).
func (d *LivelockDetector) ForceCheck() {
	d.sample()
}

// =============================================================================
// CONC-003: Priority Inversion Demo + Priority Inheritance Fix
// =============================================================================
//
// Priority inversion: a low-priority task holds a mutex; a high-priority task
// blocks on it; a medium-priority task runs freely (unbounded inversion).
//
// Priority inheritance: when H blocks waiting for a mutex held by L, L
// temporarily inherits H's priority so M cannot preempt it.
//
// Priority ceiling: each mutex has a ceiling = highest priority of any task
// that may ever lock it. A task can only acquire the mutex if its priority is
// higher than the ceiling of every mutex currently held by other tasks.
//
// We simulate these on a single-threaded round-robin scheduler driven by ticks.

// TaskPriority represents priority (lower number = higher priority, like Linux).
type TaskPriority int

const (
	PriorityHigh   TaskPriority = 1
	PriorityMedium TaskPriority = 2
	PriorityLow    TaskPriority = 3
)

// PITask is a simulated task in the priority-inversion demo.
type PITask struct {
	Name     string
	Priority TaskPriority
	// effectivePriority is modified by inheritance.
	effectivePriority TaskPriority

	ticks    int   // ticks consumed (work done)
	maxTicks int   // ticks needed to complete
	done     bool
	blocked  bool // waiting for a mutex

	mu sync.Mutex
}

func (t *PITask) effectivePrio() TaskPriority {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.effectivePriority
}

func (t *PITask) setEffectivePrio(p TaskPriority) {
	t.mu.Lock()
	t.effectivePriority = p
	t.mu.Unlock()
}

// PILog records one simulation event.
type PILog struct {
	Tick   int
	Task   string
	Action string // "run", "block", "inherit", "release", "complete"
	Detail string
}

// PIMutex is a mutex that supports priority inheritance.
type PIMutex struct {
	name    string
	ceiling TaskPriority // for priority ceiling protocol

	mu     sync.Mutex
	holder *PITask   // current holder (nil = free)
	waiters []*PITask // tasks waiting
}

// NewPIMutex creates a mutex with the given ceiling priority.
func NewPIMutex(name string, ceiling TaskPriority) *PIMutex {
	return &PIMutex{name: name, ceiling: ceiling}
}

// PriorityInversionSimulator runs a tick-based simulation.
type PriorityInversionSimulator struct {
	tasks  []*PITask
	mu     sync.Mutex
	log    []PILog
	tick   int

	// inheritance: when H waits for L's mutex, L gets H's priority.
	inheritance bool
}

// NewPriorityInversionSimulator creates a simulator.
// If inheritance=true, uses priority inheritance; otherwise bare priority.
func NewPriorityInversionSimulator(inheritance bool) *PriorityInversionSimulator {
	return &PriorityInversionSimulator{inheritance: inheritance}
}

// AddTask registers a task. Tasks must be added before Run.
func (sim *PriorityInversionSimulator) AddTask(name string, prio TaskPriority, ticks int) *PITask {
	t := &PITask{
		Name:              name,
		Priority:          prio,
		effectivePriority: prio,
		maxTicks:          ticks,
	}
	sim.tasks = append(sim.tasks, t)
	return t
}

// record appends an event to the log (caller holds no lock).
func (sim *PriorityInversionSimulator) record(task, action, detail string) {
	sim.mu.Lock()
	sim.log = append(sim.log, PILog{Tick: sim.tick, Task: task, Action: action, Detail: detail})
	sim.mu.Unlock()
}

// pickRunnable selects the runnable task with the lowest effective priority number (= highest priority).
func (sim *PriorityInversionSimulator) pickRunnable() *PITask {
	var best *PITask
	for _, t := range sim.tasks {
		if t.done || t.blocked {
			continue
		}
		if best == nil || t.effectivePrio() < best.effectivePrio() {
			best = t
		}
	}
	return best
}

// TryLock attempts to acquire m for task t (used during simulation step).
// Returns true if acquired. If inheritance is on, boosts holder's priority.
func (sim *PriorityInversionSimulator) TryLock(t *PITask, m *PIMutex) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.holder == nil {
		m.holder = t
		sim.record(t.Name, "lock", fmt.Sprintf("acquired %s", m.name))
		return true
	}
	// Mutex is held; block caller.
	t.blocked = true
	m.waiters = append(m.waiters, t)
	sim.record(t.Name, "block", fmt.Sprintf("waiting for %s held by %s", m.name, m.holder.Name))
	if sim.inheritance && t.Priority < m.holder.Priority {
		// Boost holder's priority.
		old := m.holder.effectivePriority
		m.holder.setEffectivePrio(t.Priority)
		sim.record(m.holder.Name, "inherit",
			fmt.Sprintf("priority %d→%d (inheriting from %s)", old, t.Priority, t.Name))
	}
	return false
}

// Unlock releases m for task t, unblocks highest-priority waiter.
func (sim *PriorityInversionSimulator) Unlock(t *PITask, m *PIMutex) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.holder != t {
		return
	}
	// Restore holder's priority.
	t.setEffectivePrio(t.Priority)
	sim.record(t.Name, "release", fmt.Sprintf("released %s", m.name))

	if len(m.waiters) == 0 {
		m.holder = nil
		return
	}
	// Unblock highest-priority waiter.
	best := 0
	for i, w := range m.waiters {
		if w.Priority < m.waiters[best].Priority {
			best = i
		}
	}
	next := m.waiters[best]
	m.waiters = append(m.waiters[:best], m.waiters[best+1:]...)
	next.blocked = false
	m.holder = next
	sim.record(next.Name, "lock", fmt.Sprintf("acquired %s after unblock", m.name))
}

// RunScenario runs the simulation for maxTicks ticks or until all tasks complete.
// scriptFn is called each tick with (tick, task) and may call TryLock/Unlock.
// It returns (task, true) when the task makes progress, (task, false) to skip.
func (sim *PriorityInversionSimulator) RunScenario(maxTicks int, scriptFn func(tick int, task *PITask)) []PILog {
	for sim.tick = 0; sim.tick < maxTicks; sim.tick++ {
		t := sim.pickRunnable()
		if t == nil {
			// Check if all done.
			allDone := true
			for _, task := range sim.tasks {
				if !task.done {
					allDone = false
					break
				}
			}
			if allDone {
				break
			}
			continue
		}
		scriptFn(sim.tick, t)
		if !t.blocked {
			t.ticks++
			sim.record(t.Name, "run", fmt.Sprintf("tick %d (%d/%d)", sim.tick, t.ticks, t.maxTicks))
			if t.ticks >= t.maxTicks {
				t.done = true
				sim.record(t.Name, "complete", "")
			}
		}
	}
	sim.mu.Lock()
	out := make([]PILog, len(sim.log))
	copy(out, sim.log)
	sim.mu.Unlock()
	return out
}

// Log returns the full event log.
func (sim *PriorityInversionSimulator) Log() []PILog {
	sim.mu.Lock()
	defer sim.mu.Unlock()
	out := make([]PILog, len(sim.log))
	copy(out, sim.log)
	return out
}

// =============================================================================
// Priority Ceiling Protocol
// =============================================================================
//
// Under PCP a task may acquire a mutex only if its priority is strictly higher
// (lower number) than the ceiling of all mutexes currently locked by other tasks.
// This prevents unbounded priority inversion without runtime boosting.

// PCPMutex is a mutex with a ceiling priority for the PCP protocol.
type PCPMutex struct {
	Name    string
	Ceiling TaskPriority // highest priority (lowest number) that may ever lock this

	holder *PITask
	mu     sync.Mutex
}

// NewPCPMutex creates a PCP mutex.
func NewPCPMutex(name string, ceiling TaskPriority) *PCPMutex {
	return &PCPMutex{Name: name, Ceiling: ceiling}
}

// PCPSystem manages a set of PCP mutexes and tasks.
type PCPSystem struct {
	mutexes []*PCPMutex
	mu      sync.Mutex
	log     []string
}

// NewPCPSystem creates an empty PCP system.
func NewPCPSystem() *PCPSystem { return &PCPSystem{} }

// AddMutex registers a PCP mutex.
func (s *PCPSystem) AddMutex(m *PCPMutex) { s.mutexes = append(s.mutexes, m) }

// highestCeilingHeldByOthers returns the ceiling (lowest number = highest) of
// all mutexes currently held by tasks other than t.
func (s *PCPSystem) highestCeilingHeldByOthers(t *PITask) (TaskPriority, bool) {
	best := TaskPriority(999)
	found := false
	for _, m := range s.mutexes {
		m.mu.Lock()
		if m.holder != nil && m.holder != t {
			if m.Ceiling < best {
				best = m.Ceiling
				found = true
			}
		}
		m.mu.Unlock()
	}
	return best, found
}

// TryLockPCP attempts to lock m under PCP rules.
// Returns true if acquired, false if blocked by ceiling or contention.
func (s *PCPSystem) TryLockPCP(t *PITask, m *PCPMutex) (bool, error) {
	// Check ceiling condition.
	if ceiling, found := s.highestCeilingHeldByOthers(t); found {
		if int(t.Priority) >= int(ceiling) {
			return false, fmt.Errorf("PCP: %s blocked by ceiling %d (priority %d)",
				t.Name, ceiling, t.Priority)
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.holder != nil {
		return false, fmt.Errorf("PCP: %s blocked, %s held by %s", m.Name, m.Name, m.holder.Name)
	}
	m.holder = t
	s.mu.Lock()
	s.log = append(s.log, fmt.Sprintf("%s locked %s", t.Name, m.Name))
	s.mu.Unlock()
	return true, nil
}

// UnlockPCP releases m.
func (s *PCPSystem) UnlockPCP(t *PITask, m *PCPMutex) {
	m.mu.Lock()
	if m.holder == t {
		m.holder = nil
	}
	m.mu.Unlock()
	s.mu.Lock()
	s.log = append(s.log, fmt.Sprintf("%s unlocked %s", t.Name, m.Name))
	s.mu.Unlock()
}

// Log returns the PCP event log.
func (s *PCPSystem) Log() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.log))
	copy(out, s.log)
	return out
}

// suppress unused import warnings
var _ = rand.Int63n
var _ = time.Sleep
