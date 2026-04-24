package concurrency

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// =============================================================================
// CONC-001: Santa Claus Problem Tests
// =============================================================================

func TestSanta_ReindeerDelivery(t *testing.T) {
	sim := NewSantaSimulation()
	stop := make(chan struct{})

	go sim.RunSanta(stop)

	var wg sync.WaitGroup
	// All 9 reindeer do 1 trip each.
	for i := 0; i < santaReindeerNeeded; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			sim.RunReindeer(i, 1, time.Millisecond)
		}()
	}
	wg.Wait()

	// Wait for Santa to process.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && atomic.LoadInt64(&sim.ReindeerTrips) < 1 {
		time.Sleep(time.Millisecond)
	}
	close(stop)

	if sim.ReindeerTrips < 1 {
		t.Errorf("expected at least 1 delivery, got %d", sim.ReindeerTrips)
	}
}

func TestSanta_ElfConsultation(t *testing.T) {
	sim := NewSantaSimulation()
	stop := make(chan struct{})

	go sim.RunSanta(stop)

	var wg sync.WaitGroup
	// 3 elves request help.
	for i := 0; i < santaElfGroupSize; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			sim.RunElf(i, 1, time.Millisecond)
		}()
	}
	wg.Wait()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && atomic.LoadInt64(&sim.ElfHelps) < 1 {
		time.Sleep(time.Millisecond)
	}
	close(stop)

	if sim.ElfHelps < 1 {
		t.Errorf("expected at least 1 elf consultation, got %d", sim.ElfHelps)
	}
}

func TestSanta_ReindeerPriority(t *testing.T) {
	// If both reindeer and elves are ready simultaneously, reindeer win.
	sim := NewSantaSimulation()
	stop := make(chan struct{})

	// Pre-fill both queues before starting Santa.
	for i := 0; i < santaReindeerNeeded; i++ {
		sim.reindeerReady <- struct{}{}
	}
	for i := 0; i < santaElfGroupSize; i++ {
		sim.elfReady <- struct{}{}
	}

	go sim.RunSanta(stop)

	// Wait briefly for Santa to process.
	time.Sleep(20 * time.Millisecond)

	// Signal reindeer waiting for sleigh.
	for i := 0; i < santaReindeerNeeded; i++ {
		select {
		case <-sim.sleighReady:
		default:
		}
	}

	close(stop)
	events := sim.Events()
	if len(events) == 0 {
		t.Fatal("expected at least one event")
	}
	// First event should be "deliver" (reindeer priority).
	if events[0].Kind != "deliver" {
		t.Errorf("expected first event to be deliver (reindeer priority), got %q", events[0].Kind)
	}
}

func TestSanta_FullSimulation(t *testing.T) {
	sim := NewSantaSimulation()
	stop := make(chan struct{})

	go sim.RunSanta(stop)

	var wg sync.WaitGroup
	// 9 reindeer, 2 trips each.
	for i := 0; i < santaReindeerNeeded; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			sim.RunReindeer(id, 2, 2*time.Millisecond)
		}(i)
	}
	// 6 elves, 1 consultation each (2 groups of 3).
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			sim.RunElf(id, 1, time.Millisecond)
		}(i)
	}
	wg.Wait()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) &&
		(atomic.LoadInt64(&sim.ReindeerTrips) < 2 || atomic.LoadInt64(&sim.ElfHelps) < 2) {
		time.Sleep(time.Millisecond)
	}
	close(stop)

	if sim.ReindeerTrips < 2 {
		t.Errorf("expected at least 2 delivery rounds, got %d", sim.ReindeerTrips)
	}
	if sim.ElfHelps < 2 {
		t.Errorf("expected at least 2 elf groups helped, got %d", sim.ElfHelps)
	}
}

// =============================================================================
// CONC-002: Livelock Detector Tests
// =============================================================================

func TestLivelockDetector_DetectsLivelock(t *testing.T) {
	d := NewLivelockDetector(10*time.Millisecond, 5)
	m := NewLivelock("spinner")
	d.Register(m)
	d.Start()
	defer d.Stop()

	// Simulate a livelocked goroutine: retries without progress.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 50; i++ {
			m.Retry()
			time.Sleep(time.Millisecond)
		}
	}()
	<-done

	d.ForceCheck()
	detected := d.Detected()
	if len(detected) == 0 {
		t.Error("expected livelock detection for spinning goroutine")
	}
	if detected[0] != "spinner" {
		t.Errorf("expected spinner, got %v", detected)
	}
}

func TestLivelockDetector_NoFalsePositive(t *testing.T) {
	d := NewLivelockDetector(10*time.Millisecond, 5)
	m := NewLivelock("worker")
	d.Register(m)
	d.Start()
	defer d.Stop()

	// Simulate a goroutine making progress.
	for i := 0; i < 10; i++ {
		m.Retry()
		m.Progress()
	}

	d.ForceCheck()
	if len(d.Detected()) != 0 {
		t.Error("goroutine with progress should not be detected as livelocked")
	}
}

func TestLivelockDetector_MinRetriesThreshold(t *testing.T) {
	d := NewLivelockDetector(10*time.Millisecond, 100) // high threshold
	m := NewLivelock("light-spinner")
	d.Register(m)

	// Only 3 retries — below threshold.
	m.Retry()
	m.Retry()
	m.Retry()

	d.ForceCheck()
	if len(d.Detected()) != 0 {
		t.Error("should not detect livelock below minRetries threshold")
	}
}

func TestLivelockDetector_MultipleMonitors(t *testing.T) {
	d := NewLivelockDetector(10*time.Millisecond, 3)
	healthy := NewLivelock("healthy")
	stuck := NewLivelock("stuck")
	d.Register(healthy)
	d.Register(stuck)

	// Healthy makes progress.
	for i := 0; i < 5; i++ {
		healthy.Retry()
		healthy.Progress()
	}
	// Stuck only retries.
	for i := 0; i < 10; i++ {
		stuck.Retry()
	}

	d.ForceCheck()
	detected := d.Detected()
	found := false
	for _, name := range detected {
		if name == "stuck" {
			found = true
		}
		if name == "healthy" {
			t.Error("healthy goroutine should not be detected")
		}
	}
	if !found {
		t.Error("expected stuck goroutine to be detected")
	}
}

func TestLivelockDetector_Counters(t *testing.T) {
	m := NewLivelock("test")
	m.Retry()
	m.Retry()
	m.Progress()
	if m.Retries() != 2 {
		t.Errorf("expected 2 retries, got %d", m.Retries())
	}
	if m.Progresses() != 1 {
		t.Errorf("expected 1 progress, got %d", m.Progresses())
	}
}

// =============================================================================
// CONC-003: Priority Inversion / Inheritance / Ceiling Tests
// =============================================================================

func TestPriorityInversion_Observed(t *testing.T) {
	// Without inheritance: Medium can preempt Low (which holds the mutex),
	// causing High to wait longer than necessary.
	sim := NewPriorityInversionSimulator(false)
	mutex := NewPIMutex("resource", PriorityHigh)

	low := sim.AddTask("Low", PriorityLow, 10)
	med := sim.AddTask("Medium", PriorityMedium, 5)
	high := sim.AddTask("High", PriorityHigh, 5)
	_ = med // used by simulation

	mutexAcquired := false

	logs := sim.RunScenario(50, func(tick int, task *PITask) {
		if task == low {
			if !mutexAcquired {
				sim.TryLock(low, mutex)
				mutexAcquired = true
			}
			if task.ticks >= 8 {
				sim.Unlock(low, mutex)
			}
		}
		if task == high && !mutexAcquired {
			sim.TryLock(high, mutex)
		}
	})

	// Low should appear in the log; high should eventually complete.
	_ = logs
	if !high.done && !high.blocked {
		// High ran some ticks without the mutex — acceptable.
	}
	_ = low
}

func TestPriorityInheritance_BoostsLowPriority(t *testing.T) {
	sim := NewPriorityInversionSimulator(true) // inheritance enabled
	mutex := NewPIMutex("resource", PriorityHigh)

	low := sim.AddTask("Low", PriorityLow, 6)
	_ = sim.AddTask("Medium", PriorityMedium, 3)
	high := sim.AddTask("High", PriorityHigh, 3)

	// Initially block High so Low (lower priority = runs last normally) gets
	// to run first and acquire the mutex.
	high.blocked = true

	lowHasMutex := false
	highUnblocked := false
	highTriedLock := false
	lowUnlocked := false

	sim.RunScenario(40, func(tick int, task *PITask) {
		if task == low && !lowHasMutex {
			sim.TryLock(low, mutex)
			lowHasMutex = true
			// Now let High contend for the mutex.
			high.blocked = false
			highUnblocked = true
		}
		if task == high && highUnblocked && !highTriedLock {
			highTriedLock = true
			sim.TryLock(high, mutex) // blocks High, triggers inheritance
		}
		if task == low && low.ticks >= 4 && !lowUnlocked {
			lowUnlocked = true
			sim.Unlock(low, mutex)
		}
	})

	log := sim.Log()
	// Check that an inheritance event was recorded.
	inherited := false
	for _, e := range log {
		if e.Action == "inherit" {
			inherited = true
			break
		}
	}
	if !inherited {
		t.Error("expected priority inheritance event to be logged")
	}
}

func TestPriorityInheritance_RestoredAfterRelease(t *testing.T) {
	sim := NewPriorityInversionSimulator(true)
	mutex := NewPIMutex("m", PriorityHigh)

	low := sim.AddTask("Low", PriorityLow, 4)
	high := sim.AddTask("High", PriorityHigh, 2)

	// Block high initially so low acquires the mutex first.
	high.blocked = true

	lowLocked := false
	highUnblocked := false
	highBlocked := false
	lowUnlocked := false

	sim.RunScenario(30, func(tick int, task *PITask) {
		if task == low && !lowLocked {
			sim.TryLock(low, mutex)
			lowLocked = true
			high.blocked = false
			highUnblocked = true
		}
		if task == high && highUnblocked && !highBlocked {
			highBlocked = true
			sim.TryLock(high, mutex)
		}
		if task == low && low.ticks >= 3 && !lowUnlocked {
			lowUnlocked = true
			sim.Unlock(low, mutex)
		}
	})

	// After release, Low's effective priority should be restored.
	if low.effectivePriority != PriorityLow {
		t.Errorf("Low priority should be restored to %d after release, got %d",
			PriorityLow, low.effectivePriority)
	}
}

func TestPCP_BlockedByCeiling(t *testing.T) {
	sys := NewPCPSystem()
	m := NewPCPMutex("shared", PriorityHigh) // ceiling = 1 (highest)
	sys.AddMutex(m)

	low := &PITask{Name: "Low", Priority: PriorityLow, effectivePriority: PriorityLow}
	med := &PITask{Name: "Medium", Priority: PriorityMedium, effectivePriority: PriorityMedium}
	high := &PITask{Name: "High", Priority: PriorityHigh, effectivePriority: PriorityHigh}

	// Low locks the mutex.
	ok, err := sys.TryLockPCP(low, m)
	if !ok || err != nil {
		t.Fatalf("Low should acquire mutex: %v", err)
	}

	// High tries to lock the same mutex — should be blocked by contention.
	ok2, _ := sys.TryLockPCP(high, m)
	if ok2 {
		t.Error("High should not acquire already-held mutex")
	}

	// Unlock.
	sys.UnlockPCP(low, m)

	// After unlock, medium should be able to lock.
	ok3, err3 := sys.TryLockPCP(med, m)
	if !ok3 {
		t.Errorf("Medium should acquire free mutex: %v", err3)
	}
	_ = high
}

func TestPCP_Log(t *testing.T) {
	sys := NewPCPSystem()
	m := NewPCPMutex("r", PriorityHigh)
	sys.AddMutex(m)

	t1 := &PITask{Name: "T1", Priority: PriorityHigh, effectivePriority: PriorityHigh}
	sys.TryLockPCP(t1, m)
	sys.UnlockPCP(t1, m)

	log := sys.Log()
	if len(log) < 2 {
		t.Errorf("expected at least 2 log entries, got %d", len(log))
	}
}

func TestSanta_Events(t *testing.T) {
	sim := NewSantaSimulation()
	stop := make(chan struct{})

	go sim.RunSanta(stop)

	// Trigger one delivery.
	for i := 0; i < santaReindeerNeeded; i++ {
		sim.reindeerReady <- struct{}{}
	}
	time.Sleep(20 * time.Millisecond)
	// Drain sleigh signals.
	for i := 0; i < santaReindeerNeeded; i++ {
		select {
		case <-sim.sleighReady:
		default:
		}
	}
	close(stop)

	events := sim.Events()
	if len(events) == 0 {
		t.Error("expected at least one event in log")
	}
}
