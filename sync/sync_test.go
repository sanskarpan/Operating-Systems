package ossync

import (
	"sync"
	"testing"
	"time"
)

func TestSemaphore(t *testing.T) {
	sem := NewSemaphore(2)

	// Acquire permits
	sem.Acquire()
	sem.Acquire()

	// Should have 0 permits now
	if sem.AvailablePermits() != 0 {
		t.Errorf("Expected 0 permits, got %d", sem.AvailablePermits())
	}

	// Release and check
	sem.Release()
	if sem.AvailablePermits() != 1 {
		t.Errorf("Expected 1 permit after release, got %d", sem.AvailablePermits())
	}
}

func TestSemaphoreAcquireTimeout(t *testing.T) {
	sem := NewSemaphore(1)

	// Exhaust the only permit so the next acquire must wait.
	sem.Acquire()

	// AcquireTimeout must return false within a reasonable multiple of the
	// requested timeout — never block indefinitely (BUG-001).
	timeout := 50 * time.Millisecond
	start := time.Now()
	got := sem.AcquireTimeout(timeout)
	elapsed := time.Since(start)

	if got {
		t.Error("AcquireTimeout should return false when no permits are available")
	}

	// Allow up to 10× the timeout for scheduling jitter, but it must not
	// block indefinitely as the old implementation could.
	if elapsed > 10*timeout {
		t.Errorf("AcquireTimeout blocked for %v, expected to return within ~%v", elapsed, timeout)
	}

	// Release the permit and verify AcquireTimeout now succeeds.
	sem.Release()
	if !sem.AcquireTimeout(timeout) {
		t.Error("AcquireTimeout should succeed when a permit is available")
	}
}

func TestSemaphoreTryAcquire(t *testing.T) {
	sem := NewSemaphore(1)

	// First acquire should succeed
	if !sem.TryAcquire() {
		t.Error("TryAcquire should succeed when permits available")
	}

	// Second should fail
	if sem.TryAcquire() {
		t.Error("TryAcquire should fail when no permits available")
	}

	sem.Release()

	// Should succeed again
	if !sem.TryAcquire() {
		t.Error("TryAcquire should succeed after release")
	}
}

func TestBarrier(t *testing.T) {
	barrier := NewBarrier(3)
	arrivals := 0
	var mu sync.Mutex

	var wg sync.WaitGroup
	wg.Add(3)

	for i := 0; i < 3; i++ {
		go func(id int) {
			defer wg.Done()

			mu.Lock()
			arrivals++
			mu.Unlock()

			barrier.Wait()

			mu.Lock()
			// All should have arrived before any proceed
			if arrivals != 3 {
				t.Errorf("Goroutine %d: expected 3 arrivals, got %d", id, arrivals)
			}
			mu.Unlock()
		}(i)
	}

	wg.Wait()
}

func TestRWLock(t *testing.T) {
	rw := NewRWLock(false) // Reader preference

	// Multiple readers should be allowed
	rw.RLock()
	rw.RLock()

	// Unlock readers
	rw.RUnlock()
	rw.RUnlock()

	// Writer lock
	rw.WLock()
	rw.WUnlock()
}

func TestLatch(t *testing.T) {
	latch := NewLatch(3)

	if latch.GetCount() != 3 {
		t.Errorf("Expected count 3, got %d", latch.GetCount())
	}

	latch.CountDown()
	latch.CountDown()

	if latch.GetCount() != 1 {
		t.Errorf("Expected count 1, got %d", latch.GetCount())
	}

	// Last countdown should trigger
	done := make(chan bool)
	go func() {
		latch.Wait()
		done <- true
	}()

	latch.CountDown()

	select {
	case <-done:
		// Success
	case <-time.After(time.Second):
		t.Error("Latch.Wait() timed out")
	}
}

func TestBoundedBuffer(t *testing.T) {
	bb := NewBoundedBuffer(3)

	// Put items
	bb.Put(1)
	bb.Put(2)
	bb.Put(3)

	if !bb.IsFull() {
		t.Error("Buffer should be full")
	}

	// Get items
	item := bb.Get()
	if item.(int) != 1 {
		t.Errorf("Expected 1, got %v", item)
	}

	if bb.IsFull() {
		t.Error("Buffer should not be full after get")
	}

	if bb.Size() != 2 {
		t.Errorf("Expected size 2, got %d", bb.Size())
	}
}

func TestBoundedBufferBlocking(t *testing.T) {
	bb := NewBoundedBuffer(2)

	// Fill buffer
	bb.Put(1)
	bb.Put(2)

	// Try to put when full (should block)
	done := make(chan bool)
	go func() {
		bb.Put(3) // This will block
		done <- true
	}()

	// Give it time to block
	time.Sleep(100 * time.Millisecond)

	// Now get an item to unblock
	bb.Get()

	select {
	case <-done:
		// Success - put unblocked
	case <-time.After(time.Second):
		t.Error("Put did not unblock")
	}
}

func TestSpinlock(t *testing.T) {
	sl := NewSpinlock()

	// Lock
	sl.Lock()

	// Try lock should fail
	if sl.TryLock() {
		t.Error("TryLock should fail when locked")
	}

	// Unlock
	sl.Unlock()

	// Try lock should succeed
	if !sl.TryLock() {
		t.Error("TryLock should succeed when unlocked")
	}

	sl.Unlock()
}

func TestMonitor(t *testing.T) {
	monitor := NewMonitor()

	monitor.Enter()

	// Signal should not panic even if no waiters
	monitor.Signal("test")

	monitor.Exit()
}

func TestCountdownLatch(t *testing.T) {
	latch := NewCountdownLatch(5)

	for i := 0; i < 5; i++ {
		go func() {
			time.Sleep(50 * time.Millisecond)
			latch.Done()
		}()
	}

	latch.Await()

	if latch.GetCount() != 0 {
		t.Errorf("Expected count 0, got %d", latch.GetCount())
	}
}

func TestConcurrentBoundedBuffer(t *testing.T) {
	bb := NewBoundedBuffer(10)
	numProducers := 3
	numConsumers := 3
	itemsPerProducer := 10

	var wg sync.WaitGroup
	wg.Add(numProducers + numConsumers)

	// Producers
	for i := 0; i < numProducers; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < itemsPerProducer; j++ {
				bb.Put(id*100 + j)
			}
		}(i)
	}

	// Consumers
	consumed := make(map[int]bool)
	var consumeMu sync.Mutex

	for i := 0; i < numConsumers; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < itemsPerProducer; j++ {
				item := bb.Get()
				consumeMu.Lock()
				consumed[item.(int)] = true
				consumeMu.Unlock()
			}
		}()
	}

	wg.Wait()

	itemsIn, itemsOut, _ := bb.GetStats()
	expectedTotal := numProducers * itemsPerProducer

	if itemsIn != expectedTotal {
		t.Errorf("Expected %d items in, got %d", expectedTotal, itemsIn)
	}

	if itemsOut != expectedTotal {
		t.Errorf("Expected %d items out, got %d", expectedTotal, itemsOut)
	}
}
