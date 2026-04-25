/*
Operating Systems Concepts Tutorial
====================================

Comprehensive demonstration of OS concepts implemented in Go.
*/

package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/sanskarpan/Operating-Systems/concurrency"
	"github.com/sanskarpan/Operating-Systems/deadlock"
	"github.com/sanskarpan/Operating-Systems/io"
	"github.com/sanskarpan/Operating-Systems/memory"
	"github.com/sanskarpan/Operating-Systems/process"
)

func main() {
	fmt.Println(strings.Repeat("=", 70))
	fmt.Println("OPERATING SYSTEMS TUTORIAL")
	fmt.Println(strings.Repeat("=", 70))

	processSchedulingDemo()
	memoryManagementDemo()
	diskSchedulingDemo()
	deadlockDemo()
	concurrencyDemo()

	fmt.Println("\n" + strings.Repeat("=", 70))
	fmt.Println("Tutorial Complete!")
	fmt.Println(strings.Repeat("=", 70))
}

func processSchedulingDemo() {
	fmt.Println("\n" + strings.Repeat("=", 70))
	fmt.Println("1. PROCESS SCHEDULING ALGORITHMS")
	fmt.Println(strings.Repeat("=", 70))

	// Create sample processes
	processes := []*process.Process{
		process.NewProcess(1, 0, 24, 3),
		process.NewProcess(2, 1, 3, 1),
		process.NewProcess(3, 2, 3, 2),
		process.NewProcess(4, 3, 5, 1),
	}

	schedulers := []process.Scheduler{
		&process.FCFS{},
		&process.SJF{},
		&process.RoundRobin{TimeQuantum: 4},
		&process.PriorityScheduler{Preemptive: false},
	}

	fmt.Println("\nProcess Details:")
	fmt.Printf("%-8s %-12s %-12s %-10s\n", "PID", "Arrival", "Burst", "Priority")
	fmt.Println(strings.Repeat("-", 50))
	for _, p := range processes {
		fmt.Printf("%-8d %-12d %-12d %-10d\n", p.PID, p.ArrivalTime, p.BurstTime, p.Priority)
	}

	fmt.Println("\n\nScheduling Results:")
	fmt.Println(strings.Repeat("-", 70))
	fmt.Printf("%-30s %-15s %-15s %-15s\n", "Algorithm", "Avg Wait", "Avg Turnaround", "Avg Response")
	fmt.Println(strings.Repeat("-", 70))

	for _, scheduler := range schedulers {
		// Clone processes for each scheduler
		procs := process.CloneProcesses(processes)

		// Schedule
		scheduler.Schedule(procs)

		// Calculate stats
		totalTime := 0
		for _, p := range procs {
			if p.TurnaroundTime > totalTime {
				totalTime = p.ArrivalTime + p.TurnaroundTime
			}
		}

		stats := process.CalculateStats(procs, totalTime)

		fmt.Printf("%-30s %-15.2f %-15.2f %-15.2f\n",
			scheduler.Name(),
			stats.AverageWaitingTime,
			stats.AverageTurnaroundTime,
			stats.AverageResponseTime)
	}

	fmt.Println("\n→ FCFS is simple but can cause convoy effect")
	fmt.Println("→ SJF minimizes average waiting time")
	fmt.Println("→ Round Robin provides fair CPU time sharing")
	fmt.Println("→ Priority scheduling allows important tasks to run first")
}

func memoryManagementDemo() {
	fmt.Println("\n" + strings.Repeat("=", 70))
	fmt.Println("2. MEMORY MANAGEMENT - Page Replacement Algorithms")
	fmt.Println(strings.Repeat("=", 70))

	references := []int{7, 0, 1, 2, 0, 3, 0, 4, 2, 3, 0, 3, 2, 1, 2, 0, 1, 7, 0, 1}
	capacity := 3

	fmt.Printf("\nPage Reference String: %v\n", references)
	fmt.Printf("Frame Capacity: %d\n", capacity)

	fifo := memory.NewFIFO(capacity)
	lru := memory.NewLRU(capacity)
	clock := memory.NewClock(capacity)
	optimal := memory.NewOptimal(capacity, references)

	// Run simulations
	for i, page := range references {
		fifo.Access(page, i)
		lru.Access(page, i)
		clock.Access(page, i)
		optimal.Access(page, i)
	}

	fmt.Println("\nPage Replacement Results:")
	fmt.Println(strings.Repeat("-", 50))
	fmt.Printf("%-30s %-15s %-15s\n", "Algorithm", "Page Faults", "Hit Rate %")
	fmt.Println(strings.Repeat("-", 50))

	algorithms := []struct {
		name   string
		faults int
	}{
		{"FIFO", fifo.GetPageFaults()},
		{"LRU", lru.GetPageFaults()},
		{"Clock (Second Chance)", clock.GetPageFaults()},
		{"Optimal", optimal.GetPageFaults()},
	}

	for _, alg := range algorithms {
		hitRate := (float64(len(references)-alg.faults) / float64(len(references))) * 100
		fmt.Printf("%-30s %-15d %-15.2f\n", alg.name, alg.faults, hitRate)
	}

	fmt.Println("\n→ Optimal algorithm has minimum page faults (theoretical)")
	fmt.Println("→ LRU performs well in practice")
	fmt.Println("→ FIFO is simple but may suffer from Belady's anomaly")
	fmt.Println("→ Clock algorithm is efficient approximation of LRU")

	// Memory Allocation Demo
	fmt.Println("\n" + strings.Repeat("-", 70))
	fmt.Println("Memory Allocation Strategies")
	fmt.Println(strings.Repeat("-", 70))

	totalMem := 1000
	ff := memory.NewFirstFit(totalMem)
	_ = memory.NewBestFit(totalMem)
	_ = memory.NewWorstFit(totalMem)

	requests := []struct {
		pid  int
		size int
	}{
		{1, 100},
		{2, 200},
		{3, 150},
	}

	fmt.Println("\nMemory Allocation Requests:")
	for _, req := range requests {
		fmt.Printf("  Process %d: %d bytes\n", req.pid, req.size)
	}

	// Test First-Fit
	fmt.Println("\nFirst-Fit Allocation:")
	for _, req := range requests {
		addr, err := ff.Allocate(req.pid, req.size)
		if err != nil {
			fmt.Printf("  Process %d: Allocation FAILED\n", req.pid)
		} else {
			fmt.Printf("  Process %d: Allocated at address %d\n", req.pid, addr)
		}
	}

	fmt.Println("\n→ First-Fit: Fast, uses first available block")
	fmt.Println("→ Best-Fit: Minimizes wasted space, slower search")
	fmt.Println("→ Worst-Fit: Leaves larger free blocks, but more fragmentation")
}

func diskSchedulingDemo() {
	fmt.Println("\n" + strings.Repeat("=", 70))
	fmt.Println("3. DISK I/O SCHEDULING")
	fmt.Println(strings.Repeat("=", 70))

	// Create disk requests
	requests := []*io.DiskRequest{
		{ID: 1, Track: 98, ArrivalTime: 0},
		{ID: 2, Track: 183, ArrivalTime: 1},
		{ID: 3, Track: 37, ArrivalTime: 2},
		{ID: 4, Track: 122, ArrivalTime: 3},
		{ID: 5, Track: 14, ArrivalTime: 4},
		{ID: 6, Track: 124, ArrivalTime: 5},
		{ID: 7, Track: 65, ArrivalTime: 6},
		{ID: 8, Track: 67, ArrivalTime: 7},
	}

	currentTrack := 53

	fmt.Printf("\nInitial Head Position: %d\n", currentTrack)
	fmt.Println("Pending Requests:")
	for _, req := range requests {
		fmt.Printf("  Track %d\n", req.Track)
	}

	schedulers := []io.DiskScheduler{
		io.NewFCFS(),
		io.NewSSTF(),
		io.NewSCAN(200, "up"),
		io.NewLOOK("up"),
	}

	fmt.Println("\n\nDisk Scheduling Results:")
	fmt.Println(strings.Repeat("-", 70))
	fmt.Printf("%-25s %-15s %-15s %-15s\n", "Algorithm", "Total Seek", "Avg Seek", "Max Seek")
	fmt.Println(strings.Repeat("-", 70))

	for _, scheduler := range schedulers {
		sequence := scheduler.Schedule(requests, currentTrack)
		metrics := io.CalculateMetrics(sequence, currentTrack)

		fmt.Printf("%-25s %-15d %-15.2f %-15d\n",
			scheduler.Name(),
			metrics.TotalSeekDistance,
			metrics.AverageSeekDistance,
			metrics.MaxSeekDistance)
	}

	fmt.Println("\n→ FCFS is fair but has high seek time")
	fmt.Println("→ SSTF minimizes seek time but may cause starvation")
	fmt.Println("→ SCAN eliminates starvation with elevator algorithm")
	fmt.Println("→ LOOK is more efficient variant of SCAN")
}

func deadlockDemo() {
	fmt.Println("\n" + strings.Repeat("=", 70))
	fmt.Println("4. DEADLOCK DETECTION AND PREVENTION")
	fmt.Println(strings.Repeat("=", 70))

	// Banker's Algorithm Demo
	fmt.Println("\nBanker's Algorithm (Deadlock Avoidance):")
	fmt.Println(strings.Repeat("-", 50))

	numProcesses := 5
	numResources := 3
	available := []int{3, 3, 2}

	ba := deadlock.NewBankersAlgorithm(numProcesses, numResources, available)

	// Set maximum resources for each process
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

	fmt.Printf("Available Resources: %v\n", available)
	fmt.Printf("\nSystem State: ")
	if safe {
		fmt.Printf("SAFE\n")
		fmt.Printf("Safe Sequence: %v\n", sequence)
	} else {
		fmt.Printf("UNSAFE (Potential Deadlock)\n")
	}

	// Test resource request
	fmt.Println("\nTesting Resource Request:")
	request := []int{0, 2, 0}
	fmt.Printf("Process 1 requests: %v\n", request)

	granted, err := ba.RequestResources(1, request)
	if err != nil {
		fmt.Printf("Request rejected: %v\n", err)
	} else if granted {
		fmt.Println("Request GRANTED - System remains in safe state")
	} else {
		fmt.Println("Request DENIED - Would lead to unsafe state")
	}

	fmt.Println("\n→ Banker's algorithm prevents deadlock by ensuring safe states")
	fmt.Println("→ Resource allocation is granted only if system stays safe")

	// Wait-For Graph Demo
	fmt.Println("\n" + strings.Repeat("-", 70))
	fmt.Println("Wait-For Graph (Deadlock Detection):")
	fmt.Println(strings.Repeat("-", 50))

	wfg := deadlock.NewWaitForGraph(4)
	wfg.AddEdge(0, 1) // P0 waiting for P1
	wfg.AddEdge(1, 2) // P1 waiting for P2
	wfg.AddEdge(2, 3) // P2 waiting for P3
	wfg.AddEdge(3, 1) // P3 waiting for P1 (creates cycle!)

	hasDeadlock, cycle := wfg.DetectDeadlock()

	if hasDeadlock {
		fmt.Printf("DEADLOCK DETECTED!\n")
		fmt.Printf("Cycle: %v\n", cycle)
		fmt.Println("Recovery required: Terminate or preempt one process in cycle")
	} else {
		fmt.Println("No deadlock detected")
	}

	fmt.Println("\n→ Wait-for graphs detect circular wait condition")
	fmt.Println("→ Deadlock recovery requires breaking the cycle")
}

func concurrencyDemo() {
	fmt.Println("\n" + strings.Repeat("=", 70))
	fmt.Println("5. CLASSIC CONCURRENCY PROBLEMS")
	fmt.Println(strings.Repeat("=", 70))

	// Producer-Consumer Demo
	fmt.Println("\nProducer-Consumer Problem:")
	fmt.Println(strings.Repeat("-", 50))

	pc := concurrency.NewProducerConsumer(5)

	fmt.Println("Starting 2 producers and 2 consumers...")
	fmt.Println("(Showing first few operations)")

	producerWg := pc.RunProducers(2, 3)
	consumerWg := pc.RunConsumers(2, 3)

	// Wait a bit to see some output
	time.Sleep(time.Second * 2)

	producerWg.Wait()
	consumerWg.Wait()

	produced, consumed := pc.GetStats()
	fmt.Printf("\nTotal Produced: %d\n", produced)
	fmt.Printf("Total Consumed: %d\n", consumed)

	fmt.Println("\n→ Bounded buffer coordinates producers and consumers")
	fmt.Println("→ Prevents buffer overflow and underflow")

	// Dining Philosophers Demo
	fmt.Println("\n" + strings.Repeat("-", 70))
	fmt.Println("Dining Philosophers Problem:")
	fmt.Println(strings.Repeat("-", 50))

	dp := concurrency.NewDiningPhilosophers(5)

	fmt.Println("5 philosophers, 5 forks, each philosopher eats 2 times")
	fmt.Println("Using deadlock prevention strategy...")

	wg := dp.Run(2)
	wg.Wait()

	totalMeals := dp.GetMealsEaten()
	fmt.Printf("\nTotal meals eaten: %d (expected 10)\n", totalMeals)

	fmt.Println("\n→ Resource ordering prevents deadlock")
	fmt.Println("→ Limiting concurrent eaters ensures progress")

	// Readers-Writers Demo
	fmt.Println("\n" + strings.Repeat("-", 70))
	fmt.Println("Readers-Writers Problem:")
	fmt.Println(strings.Repeat("-", 50))

	rw := concurrency.NewReadersWriters(false)

	fmt.Println("Starting 3 readers and 2 writers...")

	readerWg := rw.RunReaders(3, 2)
	writerWg := rw.RunWriters(2, 2)

	time.Sleep(time.Second * 2)

	readerWg.Wait()
	writerWg.Wait()

	reads, writes := rw.GetStats()
	fmt.Printf("\nTotal Reads: %d\n", reads)
	fmt.Printf("Total Writes: %d\n", writes)

	fmt.Println("\n→ Multiple readers can access simultaneously")
	fmt.Println("→ Writers have exclusive access")
	fmt.Println("→ Prevents race conditions while maximizing concurrency")
}
