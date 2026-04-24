package process

import (
	"testing"
)

func TestFCFS(t *testing.T) {
	processes := []*Process{
		NewProcess(1, 0, 24, 0),
		NewProcess(2, 1, 3, 0),
		NewProcess(3, 2, 3, 0),
	}

	fcfs := &FCFS{}
	scheduled := fcfs.Schedule(processes)

	if len(scheduled) != 3 {
		t.Errorf("Expected 3 processes, got %d", len(scheduled))
	}

	// FCFS should maintain arrival order
	if scheduled[0].PID != 1 {
		t.Errorf("Expected first process to be PID 1, got %d", scheduled[0].PID)
	}
}

func TestSJF(t *testing.T) {
	processes := []*Process{
		NewProcess(1, 0, 6, 0),
		NewProcess(2, 1, 8, 0),
		NewProcess(3, 2, 7, 0),
		NewProcess(4, 3, 3, 0),
	}

	sjf := &SJF{}
	scheduled := sjf.Schedule(processes)

	if len(scheduled) != 4 {
		t.Errorf("Expected 4 processes, got %d", len(scheduled))
	}

	// Verify scheduling occurred
	totalWaitTime := 0
	for _, p := range scheduled {
		totalWaitTime += p.WaitingTime
	}

	if totalWaitTime < 0 {
		t.Errorf("Total wait time should be non-negative, got %d", totalWaitTime)
	}
}

func TestRoundRobin(t *testing.T) {
	processes := []*Process{
		NewProcess(1, 0, 10, 0),
		NewProcess(2, 1, 5, 0),
		NewProcess(3, 2, 8, 0),
	}

	rr := &RoundRobin{TimeQuantum: 2}
	scheduled := rr.Schedule(processes)

	if len(scheduled) != 3 {
		t.Errorf("Expected 3 processes, got %d", len(scheduled))
	}

	// Check that all processes completed
	for _, p := range scheduled {
		if p.State != Terminated {
			t.Errorf("Process %d not terminated", p.PID)
		}
		if p.RemainingTime != 0 {
			t.Errorf("Process %d has remaining time %d", p.PID, p.RemainingTime)
		}
	}
}

func TestPriorityScheduler(t *testing.T) {
	processes := []*Process{
		NewProcess(1, 0, 10, 3),
		NewProcess(2, 1, 5, 1),
		NewProcess(3, 2, 8, 2),
	}

	ps := &PriorityScheduler{Preemptive: false}
	scheduled := ps.Schedule(processes)

	if len(scheduled) != 3 {
		t.Errorf("Expected 3 processes, got %d", len(scheduled))
	}
}

func TestMLFQ(t *testing.T) {
	processes := []*Process{
		NewProcess(1, 0, 10, 0),
		NewProcess(2, 1, 5, 0),
	}

	mlfq := &MLFQ{
		NumQueues:    3,
		TimeQuantums: []int{2, 4, 8},
		BoostPeriod:  20,
	}

	scheduled := mlfq.Schedule(processes)

	if len(scheduled) != 2 {
		t.Errorf("Expected 2 processes, got %d", len(scheduled))
	}
}

func TestCalculateStats(t *testing.T) {
	processes := []*Process{
		NewProcess(1, 0, 10, 0),
		NewProcess(2, 0, 5, 0),
	}

	// Set some stats manually for testing
	processes[0].WaitingTime = 5
	processes[0].TurnaroundTime = 15
	processes[0].ResponseTime = 0

	processes[1].WaitingTime = 10
	processes[1].TurnaroundTime = 15
	processes[1].ResponseTime = 10

	stats := CalculateStats(processes, 30)

	if stats.AverageWaitingTime != 7.5 {
		t.Errorf("Expected avg waiting time 7.5, got %f", stats.AverageWaitingTime)
	}

	if stats.AverageTurnaroundTime != 15.0 {
		t.Errorf("Expected avg turnaround time 15.0, got %f", stats.AverageTurnaroundTime)
	}
}

func TestSRTF(t *testing.T) {
	processes := []*Process{
		NewProcess(1, 0, 7, 0),
		NewProcess(2, 2, 4, 0),
		NewProcess(3, 4, 1, 0),
		NewProcess(4, 5, 4, 0),
	}

	srtf := &SRTF{}
	scheduled := srtf.Schedule(processes)

	// Verify all processes completed
	for _, p := range scheduled {
		if p.State != Terminated {
			t.Errorf("Process %d not terminated", p.PID)
		}
	}
}

func TestCloneProcesses(t *testing.T) {
	original := []*Process{
		NewProcess(1, 0, 10, 2),
		NewProcess(2, 1, 5, 1),
	}

	cloned := CloneProcesses(original)

	if len(cloned) != len(original) {
		t.Errorf("Expected %d clones, got %d", len(original), len(cloned))
	}

	// Modify clone and ensure original unchanged
	cloned[0].BurstTime = 999

	if original[0].BurstTime == 999 {
		t.Error("Cloning created shallow copy, expected deep copy")
	}
}
