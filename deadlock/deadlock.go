/*
Deadlock Detection and Prevention
==================================

Implementation of deadlock detection algorithms and prevention strategies.

Applications:
- Database transaction management
- Distributed systems resource allocation
- Operating system resource management
- Multi-threaded application safety
*/

package deadlock

import (
	"errors"
	"fmt"
	"sync"
)

// =============================================================================
// Resource Allocation Graph
// =============================================================================

// EdgeType represents type of edge in resource allocation graph
type EdgeType int

const (
	Request EdgeType = iota // Process -> Resource
	Assign                  // Resource -> Process
)

// Edge represents an edge in the resource allocation graph
type Edge struct {
	From int
	To   int
	Type EdgeType
}

// RAG represents a Resource Allocation Graph
type RAG struct {
	NumProcesses  int
	NumResources  int
	Edges         []Edge
	AdjList       map[int][]int
	ProcessOffset int // Offset to distinguish processes from resources
	mu            sync.Mutex
}

// NewRAG creates a new resource allocation graph
func NewRAG(numProcesses, numResources int) *RAG {
	return &RAG{
		NumProcesses:  numProcesses,
		NumResources:  numResources,
		Edges:         make([]Edge, 0),
		AdjList:       make(map[int][]int),
		ProcessOffset: numResources, // Resources: 0..n-1, Processes: n..n+m-1
	}
}

// AddRequestEdge adds a request edge (process requesting resource)
func (rag *RAG) AddRequestEdge(process, resource int) {
	rag.mu.Lock()
	defer rag.mu.Unlock()
	pid := rag.ProcessOffset + process
	rag.Edges = append(rag.Edges, Edge{From: pid, To: resource, Type: Request})
	rag.AdjList[pid] = append(rag.AdjList[pid], resource)
}

// AddAssignEdge adds an assignment edge (resource assigned to process)
func (rag *RAG) AddAssignEdge(resource, process int) {
	rag.mu.Lock()
	defer rag.mu.Unlock()
	pid := rag.ProcessOffset + process
	rag.Edges = append(rag.Edges, Edge{From: resource, To: pid, Type: Assign})
	rag.AdjList[resource] = append(rag.AdjList[resource], pid)
}

// DetectCycle detects if there's a cycle in the graph (deadlock)
func (rag *RAG) DetectCycle() (bool, []int) {
	totalNodes := rag.NumProcesses + rag.NumResources
	visited := make([]bool, totalNodes)
	recStack := make([]bool, totalNodes)
	parent := make([]int, totalNodes)
	for i := range parent {
		parent[i] = -1
	}

	var dfs func(int) (bool, []int)
	dfs = func(node int) (bool, []int) {
		visited[node] = true
		recStack[node] = true

		for _, neighbor := range rag.AdjList[node] {
			if !visited[neighbor] {
				parent[neighbor] = node
				if hasCycle, cycle := dfs(neighbor); hasCycle {
					return true, cycle
				}
			} else if recStack[neighbor] {
				// Found cycle - reconstruct it
				cycle := []int{neighbor}
				curr := node
				for curr != neighbor {
					cycle = append(cycle, curr)
					curr = parent[curr]
					if curr == -1 {
						break
					}
				}
				return true, cycle
			}
		}

		recStack[node] = false
		return false, nil
	}

	for node := 0; node < totalNodes; node++ {
		if !visited[node] {
			if hasCycle, cycle := dfs(node); hasCycle {
				return true, cycle
			}
		}
	}

	return false, nil
}

// =============================================================================
// Banker's Algorithm
// =============================================================================

// BankersAlgorithm implements the Banker's deadlock avoidance algorithm
type BankersAlgorithm struct {
	NumProcesses int
	NumResources int
	Available    []int   // Available[j] = available instances of resource j
	Maximum      [][]int // Maximum[i][j] = max instances of resource j needed by process i
	Allocation   [][]int // Allocation[i][j] = instances of resource j allocated to process i
	Need         [][]int // Need[i][j] = remaining instances of resource j needed by process i
}

// NewBankersAlgorithm creates a new Banker's algorithm instance
func NewBankersAlgorithm(numProcesses, numResources int, available []int) *BankersAlgorithm {
	ba := &BankersAlgorithm{
		NumProcesses: numProcesses,
		NumResources: numResources,
		Available:    make([]int, numResources),
		Maximum:      make([][]int, numProcesses),
		Allocation:   make([][]int, numProcesses),
		Need:         make([][]int, numProcesses),
	}

	copy(ba.Available, available)

	for i := 0; i < numProcesses; i++ {
		ba.Maximum[i] = make([]int, numResources)
		ba.Allocation[i] = make([]int, numResources)
		ba.Need[i] = make([]int, numResources)
	}

	return ba
}

// SetMaximum sets the maximum resource requirement for a process
func (ba *BankersAlgorithm) SetMaximum(process int, maximum []int) error {
	if process < 0 || process >= ba.NumProcesses {
		return errors.New("invalid process ID")
	}
	if len(maximum) != ba.NumResources {
		return errors.New("invalid maximum array size")
	}

	copy(ba.Maximum[process], maximum)
	ba.calculateNeed(process)
	return nil
}

// SetAllocation sets current allocation for a process
func (ba *BankersAlgorithm) SetAllocation(process int, allocation []int) error {
	if process < 0 || process >= ba.NumProcesses {
		return errors.New("invalid process ID")
	}
	if len(allocation) != ba.NumResources {
		return errors.New("invalid allocation array size")
	}

	copy(ba.Allocation[process], allocation)
	ba.calculateNeed(process)
	return nil
}

// calculateNeed computes Need[i] = Maximum[i] - Allocation[i]
func (ba *BankersAlgorithm) calculateNeed(process int) {
	for j := 0; j < ba.NumResources; j++ {
		ba.Need[process][j] = ba.Maximum[process][j] - ba.Allocation[process][j]
	}
}

// IsSafeState checks if the system is in a safe state
func (ba *BankersAlgorithm) IsSafeState() (bool, []int) {
	// Work = Available
	work := make([]int, ba.NumResources)
	copy(work, ba.Available)

	// Finish[i] = false for all i
	finish := make([]bool, ba.NumProcesses)
	safeSequence := make([]int, 0, ba.NumProcesses)

	// Find process that can finish
	for len(safeSequence) < ba.NumProcesses {
		found := false

		for i := 0; i < ba.NumProcesses; i++ {
			if finish[i] {
				continue
			}

			// Check if Need[i] <= Work
			canFinish := true
			for j := 0; j < ba.NumResources; j++ {
				if ba.Need[i][j] > work[j] {
					canFinish = false
					break
				}
			}

			if canFinish {
				// Process can finish - release its resources
				for j := 0; j < ba.NumResources; j++ {
					work[j] += ba.Allocation[i][j]
				}
				finish[i] = true
				safeSequence = append(safeSequence, i)
				found = true
				break
			}
		}

		if !found {
			// No process can finish - unsafe state
			return false, nil
		}
	}

	return true, safeSequence
}

// RequestResources handles a resource request from a process
func (ba *BankersAlgorithm) RequestResources(process int, request []int) (bool, error) {
	if process < 0 || process >= ba.NumProcesses {
		return false, errors.New("invalid process ID")
	}
	if len(request) != ba.NumResources {
		return false, errors.New("invalid request size")
	}

	// Check if request <= Need[process]
	for j := 0; j < ba.NumResources; j++ {
		if request[j] > ba.Need[process][j] {
			return false, fmt.Errorf("process has exceeded maximum claim for resource %d", j)
		}
	}

	// Check if request <= Available
	for j := 0; j < ba.NumResources; j++ {
		if request[j] > ba.Available[j] {
			return false, nil // Request cannot be granted now, process must wait
		}
	}

	// Pretend to allocate resources
	for j := 0; j < ba.NumResources; j++ {
		ba.Available[j] -= request[j]
		ba.Allocation[process][j] += request[j]
		ba.Need[process][j] -= request[j]
	}

	// Check if state is safe
	safe, _ := ba.IsSafeState()

	if !safe {
		// Rollback allocation
		for j := 0; j < ba.NumResources; j++ {
			ba.Available[j] += request[j]
			ba.Allocation[process][j] -= request[j]
			ba.Need[process][j] += request[j]
		}
		return false, nil
	}

	// Request granted
	return true, nil
}

// ReleaseResources releases resources from a process
func (ba *BankersAlgorithm) ReleaseResources(process int, release []int) error {
	if process < 0 || process >= ba.NumProcesses {
		return errors.New("invalid process ID")
	}
	if len(release) != ba.NumResources {
		return errors.New("invalid release size")
	}

	// Check if release <= Allocation[process]
	for j := 0; j < ba.NumResources; j++ {
		if release[j] > ba.Allocation[process][j] {
			return fmt.Errorf("process trying to release more than allocated for resource %d", j)
		}
	}

	// Release resources
	for j := 0; j < ba.NumResources; j++ {
		ba.Available[j] += release[j]
		ba.Allocation[process][j] -= release[j]
		ba.Need[process][j] += release[j]
	}

	return nil
}

// =============================================================================
// Wait-For Graph (for single instance resources)
// =============================================================================

// WaitForGraph represents a wait-for graph for deadlock detection
type WaitForGraph struct {
	NumProcesses int
	AdjList      map[int][]int // process -> list of processes it's waiting for
}

// NewWaitForGraph creates a new wait-for graph
func NewWaitForGraph(numProcesses int) *WaitForGraph {
	return &WaitForGraph{
		NumProcesses: numProcesses,
		AdjList:      make(map[int][]int),
	}
}

// AddEdge adds an edge indicating process i is waiting for process j
func (wfg *WaitForGraph) AddEdge(from, to int) {
	wfg.AdjList[from] = append(wfg.AdjList[from], to)
}

// RemoveEdge removes a wait-for edge
func (wfg *WaitForGraph) RemoveEdge(from, to int) {
	if neighbors, exists := wfg.AdjList[from]; exists {
		for i, neighbor := range neighbors {
			if neighbor == to {
				wfg.AdjList[from] = append(neighbors[:i], neighbors[i+1:]...)
				break
			}
		}
	}
}

// DetectDeadlock detects if there's a cycle (deadlock) in the wait-for graph
func (wfg *WaitForGraph) DetectDeadlock() (bool, []int) {
	visited := make([]bool, wfg.NumProcesses)
	recStack := make([]bool, wfg.NumProcesses)
	parent := make([]int, wfg.NumProcesses)
	for i := range parent {
		parent[i] = -1
	}

	var dfs func(int) (bool, []int)
	dfs = func(node int) (bool, []int) {
		visited[node] = true
		recStack[node] = true

		for _, neighbor := range wfg.AdjList[node] {
			if !visited[neighbor] {
				parent[neighbor] = node
				if hasDeadlock, cycle := dfs(neighbor); hasDeadlock {
					return true, cycle
				}
			} else if recStack[neighbor] {
				// Found cycle - deadlock detected
				cycle := []int{neighbor}
				curr := node
				for curr != neighbor && curr != -1 {
					cycle = append(cycle, curr)
					curr = parent[curr]
				}
				return true, cycle
			}
		}

		recStack[node] = false
		return false, nil
	}

	for node := 0; node < wfg.NumProcesses; node++ {
		if !visited[node] {
			if hasDeadlock, cycle := dfs(node); hasDeadlock {
				return true, cycle
			}
		}
	}

	return false, nil
}

// =============================================================================
// Deadlock Prevention Strategies
// =============================================================================

// ResourceOrdering enforces resource ordering to prevent circular wait
type ResourceOrdering struct {
	ResourceOrder map[string]int // Resource name -> order number
}

// NewResourceOrdering creates a resource ordering strategy
func NewResourceOrdering() *ResourceOrdering {
	return &ResourceOrdering{
		ResourceOrder: make(map[string]int),
	}
}

// SetOrder assigns an order number to a resource
func (ro *ResourceOrdering) SetOrder(resource string, order int) {
	ro.ResourceOrder[resource] = order
}

// CanRequest checks if a process can request a resource based on current holdings
func (ro *ResourceOrdering) CanRequest(held []string, requested string) bool {
	requestedOrder := ro.ResourceOrder[requested]

	// Can only request resources with higher order numbers
	for _, heldResource := range held {
		if ro.ResourceOrder[heldResource] >= requestedOrder {
			return false
		}
	}

	return true
}

// =============================================================================
// Deadlock Recovery
// =============================================================================

// RecoveryStrategy defines how to recover from deadlock
type RecoveryStrategy int

const (
	ProcessTermination RecoveryStrategy = iota // Terminate processes
	ResourcePreemption                         // Preempt resources
	Rollback                                   // Rollback to safe state
)

// DeadlockRecovery handles deadlock recovery
type DeadlockRecovery struct {
	Strategy RecoveryStrategy
}

// NewDeadlockRecovery creates a deadlock recovery handler
func NewDeadlockRecovery(strategy RecoveryStrategy) *DeadlockRecovery {
	return &DeadlockRecovery{
		Strategy: strategy,
	}
}

// SelectVictim selects a victim process for termination/preemption
// Uses simple heuristic: process with minimum cost
func (dr *DeadlockRecovery) SelectVictim(cycle []int, costs map[int]int) int {
	if len(cycle) == 0 {
		return -1
	}

	minCost := costs[cycle[0]]
	victim := cycle[0]

	for _, pid := range cycle {
		if cost, exists := costs[pid]; exists && cost < minCost {
			minCost = cost
			victim = pid
		}
	}

	return victim
}

// RecoverFromDeadlock applies recovery strategy
func (dr *DeadlockRecovery) RecoverFromDeadlock(cycle []int) string {
	switch dr.Strategy {
	case ProcessTermination:
		return fmt.Sprintf("Terminate process %d to break deadlock", cycle[0])
	case ResourcePreemption:
		return fmt.Sprintf("Preempt resources from process %d", cycle[0])
	case Rollback:
		return fmt.Sprintf("Rollback process %d to safe checkpoint", cycle[0])
	default:
		return "Unknown recovery strategy"
	}
}

// =============================================================================
// Deadlock Avoidance - Resource Allocation Graph Algorithm
// =============================================================================

// CanGrantRequest checks if granting a resource request would create a cycle.
// The entire add/detect/rollback sequence is performed under the RAG mutex so
// concurrent callers cannot observe the temporary edge or corrupt the graph.
func (rag *RAG) CanGrantRequest(process, resource int) bool {
	rag.mu.Lock()
	defer rag.mu.Unlock()

	pid := rag.ProcessOffset + process

	// Temporarily add the assignment edge without re-acquiring the lock.
	rag.Edges = append(rag.Edges, Edge{From: resource, To: pid, Type: Assign})
	rag.AdjList[resource] = append(rag.AdjList[resource], pid)

	// Check for cycle
	hasCycle, _ := rag.DetectCycle()

	// Roll back: remove the temporary edge from AdjList
	if neighbors, exists := rag.AdjList[resource]; exists {
		for i, neighbor := range neighbors {
			if neighbor == pid {
				rag.AdjList[resource] = append(neighbors[:i], neighbors[i+1:]...)
				break
			}
		}
	}

	// Roll back: remove the temporary edge from Edges
	for i := len(rag.Edges) - 1; i >= 0; i-- {
		edge := rag.Edges[i]
		if edge.From == resource && edge.To == pid && edge.Type == Assign {
			rag.Edges = append(rag.Edges[:i], rag.Edges[i+1:]...)
			break
		}
	}

	return !hasCycle
}
