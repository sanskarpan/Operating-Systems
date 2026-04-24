/*
Process Management
==================

Core process management concepts including process states, context switching,
and CPU scheduling algorithms.

Applications:
- Operating system process scheduling
- Task scheduling in distributed systems
- Job scheduling in cloud computing
- Real-time systems scheduling
*/

package process

import (
	"container/heap"
	"fmt"
	"sort"
	"time"
)

// ProcessState represents the state of a process in the system
type ProcessState int

const (
	Ready ProcessState = iota
	Running
	Blocked
	Terminated
)

func (s ProcessState) String() string {
	return [...]string{"Ready", "Running", "Blocked", "Terminated"}[s]
}

// Process represents a process in the operating system
type Process struct {
	PID          int           // Process ID
	ArrivalTime  int           // When process arrives in ready queue
	BurstTime    int           // CPU time required
	RemainingTime int          // Remaining CPU time
	Priority     int           // Priority (lower number = higher priority)
	WaitingTime  int           // Time spent waiting
	TurnaroundTime int         // Total time from arrival to completion
	ResponseTime int           // Time from arrival to first execution
	State        ProcessState  // Current state
	FirstRun     bool          // Whether process has started execution
}

// NewProcess creates a new process
func NewProcess(pid, arrivalTime, burstTime, priority int) *Process {
	return &Process{
		PID:           pid,
		ArrivalTime:   arrivalTime,
		BurstTime:     burstTime,
		RemainingTime: burstTime,
		Priority:      priority,
		State:         Ready,
		WaitingTime:   0,
		ResponseTime:  -1,
		FirstRun:      false,
	}
}

// SchedulerStats holds statistics about scheduling performance
type SchedulerStats struct {
	AverageWaitingTime    float64
	AverageTurnaroundTime float64
	AverageResponseTime   float64
	CPUUtilization        float64
	Throughput            float64
}

// Scheduler interface for different scheduling algorithms
type Scheduler interface {
	Schedule(processes []*Process) []*Process
	Name() string
}

// =============================================================================
// First-Come, First-Served (FCFS) Scheduler
// =============================================================================

// FCFS implements First-Come, First-Served scheduling
type FCFS struct{}

func (f *FCFS) Name() string {
	return "First-Come, First-Served (FCFS)"
}

func (f *FCFS) Schedule(processes []*Process) []*Process {
	// Sort by arrival time
	sort.Slice(processes, func(i, j int) bool {
		return processes[i].ArrivalTime < processes[j].ArrivalTime
	})

	currentTime := 0
	var schedule []*Process

	for _, p := range processes {
		// Wait for process to arrive
		if currentTime < p.ArrivalTime {
			currentTime = p.ArrivalTime
		}

		// Record response time (first time process runs)
		if p.ResponseTime == -1 {
			p.ResponseTime = currentTime - p.ArrivalTime
		}

		// Execute process
		p.WaitingTime = currentTime - p.ArrivalTime
		currentTime += p.BurstTime
		p.TurnaroundTime = currentTime - p.ArrivalTime
		p.State = Terminated

		schedule = append(schedule, p)
	}

	return schedule
}

// =============================================================================
// Shortest Job First (SJF) Scheduler - Non-preemptive
// =============================================================================

// SJF implements Shortest Job First scheduling
type SJF struct{}

func (s *SJF) Name() string {
	return "Shortest Job First (SJF)"
}

func (s *SJF) Schedule(processes []*Process) []*Process {
	currentTime := 0
	var schedule []*Process
	remaining := make([]*Process, len(processes))
	copy(remaining, processes)

	for len(remaining) > 0 {
		// Find all processes that have arrived
		var available []*Process
		for _, p := range remaining {
			if p.ArrivalTime <= currentTime {
				available = append(available, p)
			}
		}

		// If no process available, advance time to next arrival
		if len(available) == 0 {
			minArrival := remaining[0].ArrivalTime
			for _, p := range remaining {
				if p.ArrivalTime < minArrival {
					minArrival = p.ArrivalTime
				}
			}
			currentTime = minArrival
			continue
		}

		// Select process with shortest burst time
		shortestIdx := 0
		for i, p := range available {
			if p.BurstTime < available[shortestIdx].BurstTime {
				shortestIdx = i
			}
		}
		selected := available[shortestIdx]

		// Record response time
		if selected.ResponseTime == -1 {
			selected.ResponseTime = currentTime - selected.ArrivalTime
		}

		// Execute process
		selected.WaitingTime = currentTime - selected.ArrivalTime
		currentTime += selected.BurstTime
		selected.TurnaroundTime = currentTime - selected.ArrivalTime
		selected.State = Terminated

		schedule = append(schedule, selected)

		// Remove from remaining
		for i, p := range remaining {
			if p.PID == selected.PID {
				remaining = append(remaining[:i], remaining[i+1:]...)
				break
			}
		}
	}

	return schedule
}

// =============================================================================
// Shortest Remaining Time First (SRTF) - Preemptive SJF
// =============================================================================

// SRTF implements Shortest Remaining Time First scheduling (preemptive SJF)
type SRTF struct{}

func (s *SRTF) Name() string {
	return "Shortest Remaining Time First (SRTF)"
}

func (s *SRTF) Schedule(processes []*Process) []*Process {
	currentTime := 0
	completed := 0
	n := len(processes)

	// Track which processes are completed
	isCompleted := make([]bool, n)

	for completed < n {
		// Find process with shortest remaining time among arrived processes
		minRemaining := int(^uint(0) >> 1) // Max int
		selectedIdx := -1

		for i, p := range processes {
			if !isCompleted[i] && p.ArrivalTime <= currentTime && p.RemainingTime < minRemaining {
				minRemaining = p.RemainingTime
				selectedIdx = i
			}
		}

		// If no process found, advance time
		if selectedIdx == -1 {
			currentTime++
			continue
		}

		selected := processes[selectedIdx]

		// Record response time on first run
		if !selected.FirstRun {
			selected.ResponseTime = currentTime - selected.ArrivalTime
			selected.FirstRun = true
		}

		// Execute for 1 time unit
		selected.RemainingTime--
		currentTime++

		// If process completed
		if selected.RemainingTime == 0 {
			completed++
			selected.TurnaroundTime = currentTime - selected.ArrivalTime
			selected.WaitingTime = selected.TurnaroundTime - selected.BurstTime
			selected.State = Terminated
			isCompleted[selectedIdx] = true
		}
	}

	return processes
}

// =============================================================================
// Round Robin Scheduler
// =============================================================================

// RoundRobin implements Round Robin scheduling with time quantum
type RoundRobin struct {
	TimeQuantum int
}

func (r *RoundRobin) Name() string {
	return fmt.Sprintf("Round Robin (quantum=%d)", r.TimeQuantum)
}

func (r *RoundRobin) Schedule(processes []*Process) []*Process {
	currentTime := 0
	queue := make([]*Process, 0)
	remaining := make([]*Process, len(processes))
	copy(remaining, processes)

	// Sort by arrival time
	sort.Slice(remaining, func(i, j int) bool {
		return remaining[i].ArrivalTime < remaining[j].ArrivalTime
	})

	nextArrivalIdx := 0

	for len(queue) > 0 || nextArrivalIdx < len(remaining) {
		// Add newly arrived processes to queue
		for nextArrivalIdx < len(remaining) && remaining[nextArrivalIdx].ArrivalTime <= currentTime {
			queue = append(queue, remaining[nextArrivalIdx])
			nextArrivalIdx++
		}

		// If no process in queue, advance time
		if len(queue) == 0 {
			if nextArrivalIdx < len(remaining) {
				currentTime = remaining[nextArrivalIdx].ArrivalTime
			}
			continue
		}

		// Get next process from queue
		p := queue[0]
		queue = queue[1:]

		// Record response time on first run
		if !p.FirstRun {
			p.ResponseTime = currentTime - p.ArrivalTime
			p.FirstRun = true
		}

		// Execute for time quantum or remaining time
		execTime := r.TimeQuantum
		if p.RemainingTime < execTime {
			execTime = p.RemainingTime
		}

		p.RemainingTime -= execTime
		currentTime += execTime

		// Add newly arrived processes before re-adding current process
		newArrivals := make([]*Process, 0)
		for nextArrivalIdx < len(remaining) && remaining[nextArrivalIdx].ArrivalTime <= currentTime {
			newArrivals = append(newArrivals, remaining[nextArrivalIdx])
			nextArrivalIdx++
		}

		// If process not finished, add back to queue
		if p.RemainingTime > 0 {
			queue = append(queue, newArrivals...)
			queue = append(queue, p)
		} else {
			// Process completed
			p.TurnaroundTime = currentTime - p.ArrivalTime
			p.WaitingTime = p.TurnaroundTime - p.BurstTime
			p.State = Terminated
			queue = append(queue, newArrivals...)
		}
	}

	return processes
}

// =============================================================================
// Priority Scheduling
// =============================================================================

// PriorityScheduler implements priority-based scheduling
type PriorityScheduler struct {
	Preemptive bool
}

func (ps *PriorityScheduler) Name() string {
	if ps.Preemptive {
		return "Priority Scheduling (Preemptive)"
	}
	return "Priority Scheduling (Non-preemptive)"
}

func (ps *PriorityScheduler) Schedule(processes []*Process) []*Process {
	if ps.Preemptive {
		return ps.schedulePreemptive(processes)
	}
	return ps.scheduleNonPreemptive(processes)
}

func (ps *PriorityScheduler) scheduleNonPreemptive(processes []*Process) []*Process {
	currentTime := 0
	var schedule []*Process
	remaining := make([]*Process, len(processes))
	copy(remaining, processes)

	for len(remaining) > 0 {
		// Find available processes
		var available []*Process
		for _, p := range remaining {
			if p.ArrivalTime <= currentTime {
				available = append(available, p)
			}
		}

		if len(available) == 0 {
			// Advance time to next arrival
			minArrival := remaining[0].ArrivalTime
			for _, p := range remaining {
				if p.ArrivalTime < minArrival {
					minArrival = p.ArrivalTime
				}
			}
			currentTime = minArrival
			continue
		}

		// Select highest priority process (lowest priority number)
		highestPriorityIdx := 0
		for i, p := range available {
			if p.Priority < available[highestPriorityIdx].Priority {
				highestPriorityIdx = i
			}
		}
		selected := available[highestPriorityIdx]

		// Execute process
		if selected.ResponseTime == -1 {
			selected.ResponseTime = currentTime - selected.ArrivalTime
		}
		selected.WaitingTime = currentTime - selected.ArrivalTime
		currentTime += selected.BurstTime
		selected.TurnaroundTime = currentTime - selected.ArrivalTime
		selected.State = Terminated

		schedule = append(schedule, selected)

		// Remove from remaining
		for i, p := range remaining {
			if p.PID == selected.PID {
				remaining = append(remaining[:i], remaining[i+1:]...)
				break
			}
		}
	}

	return schedule
}

func (ps *PriorityScheduler) schedulePreemptive(processes []*Process) []*Process {
	currentTime := 0
	completed := 0
	n := len(processes)
	isCompleted := make([]bool, n)

	for completed < n {
		// Find highest priority process among arrived processes
		highestPriority := int(^uint(0) >> 1) // Max int
		selectedIdx := -1

		for i, p := range processes {
			if !isCompleted[i] && p.ArrivalTime <= currentTime && p.Priority < highestPriority {
				highestPriority = p.Priority
				selectedIdx = i
			}
		}

		if selectedIdx == -1 {
			currentTime++
			continue
		}

		selected := processes[selectedIdx]

		if !selected.FirstRun {
			selected.ResponseTime = currentTime - selected.ArrivalTime
			selected.FirstRun = true
		}

		selected.RemainingTime--
		currentTime++

		if selected.RemainingTime == 0 {
			completed++
			selected.TurnaroundTime = currentTime - selected.ArrivalTime
			selected.WaitingTime = selected.TurnaroundTime - selected.BurstTime
			selected.State = Terminated
			isCompleted[selectedIdx] = true
		}
	}

	return processes
}

// =============================================================================
// Multilevel Feedback Queue (MLFQ)
// =============================================================================

// MLFQ implements Multilevel Feedback Queue scheduling
type MLFQ struct {
	NumQueues    int
	TimeQuantums []int // Time quantum for each queue
	BoostPeriod  int   // Period to boost all processes to highest priority
}

// ProcessMLFQ extends Process with MLFQ-specific fields
type ProcessMLFQ struct {
	*Process
	CurrentQueue int
}

func (m *MLFQ) Name() string {
	return fmt.Sprintf("Multilevel Feedback Queue (queues=%d)", m.NumQueues)
}

func (m *MLFQ) Schedule(processes []*Process) []*Process {
	// Convert to MLFQ processes
	mlfqProcs := make([]*ProcessMLFQ, len(processes))
	for i, p := range processes {
		mlfqProcs[i] = &ProcessMLFQ{
			Process:      p,
			CurrentQueue: 0, // Start in highest priority queue
		}
	}

	currentTime := 0
	lastBoost := 0
	remaining := make([]*ProcessMLFQ, len(mlfqProcs))
	copy(remaining, mlfqProcs)

	for len(remaining) > 0 {
		// Periodic priority boost
		if m.BoostPeriod > 0 && currentTime-lastBoost >= m.BoostPeriod {
			for _, p := range remaining {
				p.CurrentQueue = 0
			}
			lastBoost = currentTime
		}

		// Find highest priority queue with available process
		var selected *ProcessMLFQ
		for queue := 0; queue < m.NumQueues; queue++ {
			for _, p := range remaining {
				if p.CurrentQueue == queue && p.ArrivalTime <= currentTime {
					selected = p
					break
				}
			}
			if selected != nil {
				break
			}
		}

		// If no process available, advance time
		if selected == nil {
			currentTime++
			continue
		}

		// Record response time
		if !selected.FirstRun {
			selected.ResponseTime = currentTime - selected.ArrivalTime
			selected.FirstRun = true
		}

		// Get time quantum for current queue
		quantum := m.TimeQuantums[selected.CurrentQueue]
		execTime := quantum
		if selected.RemainingTime < execTime {
			execTime = selected.RemainingTime
		}

		// Execute process
		selected.RemainingTime -= execTime
		currentTime += execTime

		if selected.RemainingTime == 0 {
			// Process completed
			selected.TurnaroundTime = currentTime - selected.ArrivalTime
			selected.WaitingTime = selected.TurnaroundTime - selected.BurstTime
			selected.State = Terminated

			// Remove from remaining
			for i, p := range remaining {
				if p.PID == selected.PID {
					remaining = append(remaining[:i], remaining[i+1:]...)
					break
				}
			}
		} else {
			// Move to lower priority queue if not at lowest
			if selected.CurrentQueue < m.NumQueues-1 {
				selected.CurrentQueue++
			}
		}
	}

	return processes
}

// =============================================================================
// Helper Functions
// =============================================================================

// CalculateStats computes scheduling performance statistics
func CalculateStats(processes []*Process, totalTime int) SchedulerStats {
	if len(processes) == 0 {
		return SchedulerStats{}
	}

	totalWaiting := 0
	totalTurnaround := 0
	totalResponse := 0
	totalBurst := 0

	for _, p := range processes {
		totalWaiting += p.WaitingTime
		totalTurnaround += p.TurnaroundTime
		totalResponse += p.ResponseTime
		totalBurst += p.BurstTime
	}

	n := float64(len(processes))

	return SchedulerStats{
		AverageWaitingTime:    float64(totalWaiting) / n,
		AverageTurnaroundTime: float64(totalTurnaround) / n,
		AverageResponseTime:   float64(totalResponse) / n,
		CPUUtilization:        float64(totalBurst) / float64(totalTime) * 100,
		Throughput:            n / float64(totalTime),
	}
}

// CloneProcesses creates deep copies of processes for testing different schedulers
func CloneProcesses(processes []*Process) []*Process {
	clones := make([]*Process, len(processes))
	for i, p := range processes {
		clone := *p
		clones[i] = &clone
	}
	return clones
}

// =============================================================================
// Priority Queue for advanced scheduling
// =============================================================================

// ProcessQueue implements heap.Interface for priority queue
type ProcessQueue []*Process

func (pq ProcessQueue) Len() int { return len(pq) }

func (pq ProcessQueue) Less(i, j int) bool {
	// Lower priority number = higher priority
	return pq[i].Priority < pq[j].Priority
}

func (pq ProcessQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
}

func (pq *ProcessQueue) Push(x interface{}) {
	*pq = append(*pq, x.(*Process))
}

func (pq *ProcessQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	*pq = old[0 : n-1]
	return item
}

// GetNextProcess returns highest priority process from queue
func GetNextProcess(pq *ProcessQueue) *Process {
	if pq.Len() == 0 {
		return nil
	}
	return heap.Pop(pq).(*Process)
}

// AddProcess adds a process to the priority queue
func AddProcess(pq *ProcessQueue, p *Process) {
	heap.Push(pq, p)
}

// =============================================================================
// Context Switching
// =============================================================================

// ContextSwitch simulates a context switch between processes
type ContextSwitch struct {
	FromPID int
	ToPID   int
	Time    time.Time
	Cost    int // Context switch overhead in time units
}

// PerformContextSwitch simulates the overhead of switching between processes
func PerformContextSwitch(from, to *Process, overhead int) ContextSwitch {
	return ContextSwitch{
		FromPID: from.PID,
		ToPID:   to.PID,
		Time:    time.Now(),
		Cost:    overhead,
	}
}
