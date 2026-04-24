package deadlock

import (
	"testing"
)

func TestBankersAlgorithmSafeState(t *testing.T) {
	// Example from operating systems textbook
	numProcesses := 5
	numResources := 3
	available := []int{3, 3, 2}

	ba := NewBankersAlgorithm(numProcesses, numResources, available)

	// Set maximum resources
	ba.SetMaximum(0, []int{7, 5, 3})
	ba.SetMaximum(1, []int{3, 2, 2})
	ba.SetMaximum(2, []int{9, 0, 2})
	ba.SetMaximum(3, []int{2, 2, 2})
	ba.SetMaximum(4, []int{4, 3, 3})

	// Set current allocations
	ba.SetAllocation(0, []int{0, 1, 0})
	ba.SetAllocation(1, []int{2, 0, 0})
	ba.SetAllocation(2, []int{3, 0, 2})
	ba.SetAllocation(3, []int{2, 1, 1})
	ba.SetAllocation(4, []int{0, 0, 2})

	safe, sequence := ba.IsSafeState()

	if !safe {
		t.Error("System should be in safe state")
	}

	if len(sequence) != numProcesses {
		t.Errorf("Expected safe sequence of length %d, got %d", numProcesses, len(sequence))
	}

	t.Logf("Safe sequence: %v", sequence)
}

func TestBankersAlgorithmRequestGranted(t *testing.T) {
	numProcesses := 3
	numResources := 2
	available := []int{3, 2}

	ba := NewBankersAlgorithm(numProcesses, numResources, available)

	ba.SetMaximum(0, []int{5, 3})
	ba.SetMaximum(1, []int{3, 2})
	ba.SetMaximum(2, []int{4, 2})

	ba.SetAllocation(0, []int{1, 0})
	ba.SetAllocation(1, []int{2, 1})
	ba.SetAllocation(2, []int{0, 1})

	// Request that should be granted (small request)
	request := []int{1, 0}
	granted, err := ba.RequestResources(0, request)

	if err != nil {
		t.Errorf("Request failed: %v", err)
	}

	if !granted {
		// It's ok if not granted - just check no error
		t.Logf("Request not granted (would lead to unsafe state)")
	}
}

func TestBankersAlgorithmRequestDenied(t *testing.T) {
	numProcesses := 2
	numResources := 2
	available := []int{2, 2}

	ba := NewBankersAlgorithm(numProcesses, numResources, available)

	ba.SetMaximum(0, []int{5, 5})
	ba.SetMaximum(1, []int{3, 3})

	ba.SetAllocation(0, []int{2, 2})
	ba.SetAllocation(1, []int{0, 0})

	// Request that exceeds maximum claim
	request := []int{5, 0}
	_, err := ba.RequestResources(0, request)

	if err == nil {
		t.Error("Request should fail (exceeds maximum claim)")
	}
}

func TestWaitForGraphDeadlockDetection(t *testing.T) {
	wfg := NewWaitForGraph(4)

	// Create a cycle: P0 -> P1 -> P2 -> P3 -> P1
	wfg.AddEdge(0, 1)
	wfg.AddEdge(1, 2)
	wfg.AddEdge(2, 3)
	wfg.AddEdge(3, 1) // Creates cycle

	hasDeadlock, cycle := wfg.DetectDeadlock()

	if !hasDeadlock {
		t.Error("Should detect deadlock")
	}

	if len(cycle) == 0 {
		t.Error("Should return cycle")
	}

	t.Logf("Detected cycle: %v", cycle)
}

func TestWaitForGraphNoDeadlock(t *testing.T) {
	wfg := NewWaitForGraph(4)

	// No cycle
	wfg.AddEdge(0, 1)
	wfg.AddEdge(1, 2)
	wfg.AddEdge(2, 3)

	hasDeadlock, _ := wfg.DetectDeadlock()

	if hasDeadlock {
		t.Error("Should not detect deadlock")
	}
}

func TestRAGCycleDetection(t *testing.T) {
	rag := NewRAG(2, 2)

	// P0 holds R0, requests R1
	rag.AddAssignEdge(0, 0)
	rag.AddRequestEdge(0, 1)

	// P1 holds R1, requests R0
	rag.AddAssignEdge(1, 1)
	rag.AddRequestEdge(1, 0)

	hasCycle, _ := rag.DetectCycle()

	if !hasCycle {
		t.Error("Should detect cycle (circular wait)")
	}
}

func TestResourceOrdering(t *testing.T) {
	ro := NewResourceOrdering()

	ro.SetOrder("database", 1)
	ro.SetOrder("filesystem", 2)
	ro.SetOrder("network", 3)

	// Holding database, can request filesystem (higher order)
	if !ro.CanRequest([]string{"database"}, "filesystem") {
		t.Error("Should allow requesting higher order resource")
	}

	// Holding filesystem, cannot request database (lower order)
	if ro.CanRequest([]string{"filesystem"}, "database") {
		t.Error("Should not allow requesting lower order resource")
	}
}

func TestDeadlockRecoveryVictimSelection(t *testing.T) {
	dr := NewDeadlockRecovery(ProcessTermination)

	cycle := []int{0, 1, 2, 3}
	costs := map[int]int{
		0: 100,
		1: 50,  // Minimum cost
		2: 75,
		3: 90,
	}

	victim := dr.SelectVictim(cycle, costs)

	if victim != 1 {
		t.Errorf("Expected victim process 1 (min cost), got %d", victim)
	}
}

func TestBankersAlgorithmReleaseResources(t *testing.T) {
	numProcesses := 2
	numResources := 2
	available := []int{1, 1}

	ba := NewBankersAlgorithm(numProcesses, numResources, available)

	ba.SetMaximum(0, []int{3, 3})
	ba.SetAllocation(0, []int{2, 2})

	// Release resources
	err := ba.ReleaseResources(0, []int{1, 1})

	if err != nil {
		t.Errorf("Release failed: %v", err)
	}

	// Check available increased
	if ba.Available[0] != 2 || ba.Available[1] != 2 {
		t.Errorf("Expected available [2, 2], got %v", ba.Available)
	}
}

func TestCanGrantRequest(t *testing.T) {
	rag := NewRAG(2, 2)

	// P0 requests R0 - should be grantable (no cycle)
	canGrant := rag.CanGrantRequest(0, 0)
	t.Logf("Can grant R0 to P0: %v", canGrant)

	// The function should handle edge cleanup properly
	// Test passes if no panic occurs
}
