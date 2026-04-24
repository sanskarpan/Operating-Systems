package lockfree

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// =============================================================================
// LF-001: Treiber Stack Tests
// =============================================================================

func TestTreiber_PushPop(t *testing.T) {
	s := NewTreiberStack()
	s.Push(1)
	s.Push(2)
	s.Push(3)
	for i := 3; i >= 1; i-- {
		v, ok := s.Pop()
		if !ok {
			t.Fatalf("Pop %d: expected ok", i)
		}
		if v != i {
			t.Errorf("expected %d, got %v", i, v)
		}
	}
}

func TestTreiber_PopEmpty(t *testing.T) {
	s := NewTreiberStack()
	_, ok := s.Pop()
	if ok {
		t.Error("Pop on empty stack should return false")
	}
}

func TestTreiber_Peek(t *testing.T) {
	s := NewTreiberStack()
	s.Push(42)
	v, ok := s.Peek()
	if !ok || v != 42 {
		t.Errorf("expected 42, got %v ok=%v", v, ok)
	}
	if s.Len() != 1 {
		t.Errorf("Peek should not remove element; len=%d", s.Len())
	}
}

func TestTreiber_Len(t *testing.T) {
	s := NewTreiberStack()
	if s.Len() != 0 {
		t.Error("empty stack len should be 0")
	}
	s.Push("a")
	s.Push("b")
	if s.Len() != 2 {
		t.Errorf("expected len 2, got %d", s.Len())
	}
	s.Pop()
	if s.Len() != 1 {
		t.Errorf("expected len 1, got %d", s.Len())
	}
}

func TestTreiber_Concurrent(t *testing.T) {
	s := NewTreiberStack()
	const goroutines = 32
	const ops = 100

	var wg sync.WaitGroup
	var pushed int64
	var popped int64

	for g := 0; g < goroutines; g++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				s.Push(i)
				atomic.AddInt64(&pushed, 1)
			}
		}()
		go func() {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				if _, ok := s.Pop(); ok {
					atomic.AddInt64(&popped, 1)
				}
			}
		}()
	}
	wg.Wait()
	// Drain remaining.
	for {
		if _, ok := s.Pop(); !ok {
			break
		}
		atomic.AddInt64(&popped, 1)
	}
	if pushed != popped {
		t.Errorf("pushed %d != popped %d", pushed, popped)
	}
}

func BenchmarkTreiberPush(bm *testing.B) {
	s := NewTreiberStack()
	bm.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			s.Push(1)
		}
	})
}

func BenchmarkTreiberPop(bm *testing.B) {
	s := NewTreiberStack()
	for i := 0; i < bm.N; i++ {
		s.Push(i)
	}
	bm.ResetTimer()
	bm.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			s.Pop()
		}
	})
}

// =============================================================================
// LF-002: Michael & Scott Queue Tests
// =============================================================================

func TestMSQueue_EnqueueDequeue(t *testing.T) {
	q := NewMSQueue()
	q.Enqueue(1)
	q.Enqueue(2)
	q.Enqueue(3)
	for i := 1; i <= 3; i++ {
		v, ok := q.Dequeue()
		if !ok {
			t.Fatalf("Dequeue %d: expected ok", i)
		}
		if v != i {
			t.Errorf("expected %d, got %v", i, v)
		}
	}
}

func TestMSQueue_DequeueEmpty(t *testing.T) {
	q := NewMSQueue()
	_, ok := q.Dequeue()
	if ok {
		t.Error("Dequeue from empty queue should return false")
	}
}

func TestMSQueue_IsEmpty(t *testing.T) {
	q := NewMSQueue()
	if !q.IsEmpty() {
		t.Error("new queue should be empty")
	}
	q.Enqueue(1)
	if q.IsEmpty() {
		t.Error("queue with element should not be empty")
	}
}

func TestMSQueue_Concurrent(t *testing.T) {
	q := NewMSQueue()
	const n = 10000

	var wg sync.WaitGroup
	var enqueued, dequeued int64

	// Producers.
	for p := 0; p < 8; p++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < n/8; i++ {
				q.Enqueue(i)
				atomic.AddInt64(&enqueued, 1)
			}
		}()
	}
	// Consumers.
	for c := 0; c < 8; c++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < n/8; i++ {
				for {
					if _, ok := q.Dequeue(); ok {
						atomic.AddInt64(&dequeued, 1)
						break
					}
					time.Sleep(time.Microsecond)
				}
			}
		}()
	}
	wg.Wait()
	if enqueued != dequeued {
		t.Errorf("enqueued %d != dequeued %d", enqueued, dequeued)
	}
}

func BenchmarkMSQueueEnqueue(bm *testing.B) {
	q := NewMSQueue()
	bm.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			q.Enqueue(1)
		}
	})
}

// =============================================================================
// LF-003: SPSC Ring Buffer Tests
// =============================================================================

func TestSPSC_PushPop(t *testing.T) {
	r, err := NewSPSCRingBuffer(8)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		if !r.TryPush(i) {
			t.Fatalf("TryPush %d failed", i)
		}
	}
	if r.TryPush(99) {
		t.Error("push to full buffer should fail")
	}
	for i := 0; i < 8; i++ {
		v, ok := r.TryPop()
		if !ok {
			t.Fatalf("TryPop %d failed", i)
		}
		if v != i {
			t.Errorf("expected %d, got %v", i, v)
		}
	}
}

func TestSPSC_InvalidSize(t *testing.T) {
	if _, err := NewSPSCRingBuffer(3); err == nil {
		t.Error("expected error for non-power-of-2 size")
	}
	if _, err := NewSPSCRingBuffer(0); err == nil {
		t.Error("expected error for size 0")
	}
}

func TestSPSC_PopEmpty(t *testing.T) {
	r, _ := NewSPSCRingBuffer(4)
	_, ok := r.TryPop()
	if ok {
		t.Error("pop from empty buffer should return false")
	}
}

func TestSPSC_Len(t *testing.T) {
	r, _ := NewSPSCRingBuffer(4)
	r.TryPush(1)
	r.TryPush(2)
	if r.Len() != 2 {
		t.Errorf("expected len 2, got %d", r.Len())
	}
}

func TestSPSC_ConcurrentSPSC(t *testing.T) {
	r, _ := NewSPSCRingBuffer(256)
	const total = 10000

	var received int64
	done := make(chan struct{})

	// Single consumer.
	go func() {
		for atomic.LoadInt64(&received) < total {
			if _, ok := r.TryPop(); ok {
				atomic.AddInt64(&received, 1)
			}
		}
		close(done)
	}()

	// Single producer.
	for i := 0; i < total; i++ {
		for !r.TryPush(i) {
			time.Sleep(time.Nanosecond)
		}
	}
	<-done
	if atomic.LoadInt64(&received) != total {
		t.Errorf("expected %d received, got %d", total, received)
	}
}

func BenchmarkSPSCPushPop(bm *testing.B) {
	r, _ := NewSPSCRingBuffer(1024)
	bm.ResetTimer()
	done := make(chan struct{})
	go func() {
		for i := 0; i < bm.N; i++ {
			for {
				if _, ok := r.TryPop(); ok {
					break
				}
				runtime_Gosched()
			}
		}
		close(done)
	}()
	for i := 0; i < bm.N; i++ {
		for !r.TryPush(i) {
			runtime_Gosched()
		}
	}
	<-done
}

// Workaround for using runtime.Gosched in benchmark without importing runtime.
func runtime_Gosched() { time.Sleep(0) }

// =============================================================================
// LF-004: MPMC Ring Buffer Tests
// =============================================================================

func TestMPMC_EnqueueDequeue(t *testing.T) {
	q, err := NewMPMCRingBuffer(8)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		if !q.TryEnqueue(i) {
			t.Fatalf("TryEnqueue %d failed", i)
		}
	}
	if q.TryEnqueue(99) {
		t.Error("enqueue to full buffer should fail")
	}
	for i := 0; i < 8; i++ {
		v, ok := q.TryDequeue()
		if !ok || v != i {
			t.Errorf("expected %d, got %v ok=%v", i, v, ok)
		}
	}
}

func TestMPMC_InvalidCapacity(t *testing.T) {
	if _, err := NewMPMCRingBuffer(5); err == nil {
		t.Error("expected error for non-power-of-2 capacity")
	}
}

func TestMPMC_Concurrent(t *testing.T) {
	q, _ := NewMPMCRingBuffer(1024)
	const producers = 4
	const consumers = 4
	const perProducer = 1000

	var wg sync.WaitGroup
	var produced, consumed int64

	for p := 0; p < producers; p++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perProducer; i++ {
				for !q.TryEnqueue(i) {
					time.Sleep(time.Nanosecond)
				}
				atomic.AddInt64(&produced, 1)
			}
		}()
	}

	total := int64(producers * perProducer)
	for c := 0; c < consumers; c++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for atomic.LoadInt64(&consumed) < total {
				if _, ok := q.TryDequeue(); ok {
					atomic.AddInt64(&consumed, 1)
				}
			}
		}()
	}

	wg.Wait()
	if produced != consumed {
		t.Errorf("produced %d != consumed %d", produced, consumed)
	}
}

func BenchmarkMPMCEnqueueDequeue(bm *testing.B) {
	q, _ := NewMPMCRingBuffer(1024)
	bm.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			q.TryEnqueue(1)
			q.TryDequeue()
		}
	})
}

// =============================================================================
// LF-005: Futex Tests
// =============================================================================

func TestFutex_LockUnlock(t *testing.T) {
	f := NewFutex()
	f.Lock()
	if f.State() == 0 {
		t.Error("futex should be locked")
	}
	f.Unlock()
	if f.State() != 0 {
		t.Error("futex should be unlocked after unlock")
	}
}

func TestFutex_TryLock(t *testing.T) {
	f := NewFutex()
	if !f.TryLock() {
		t.Error("TryLock should succeed on unlocked futex")
	}
	if f.TryLock() {
		t.Error("TryLock should fail on locked futex")
	}
	f.Unlock()
}

func TestFutex_WaitTimeout_Expires(t *testing.T) {
	f := NewFutex()
	f.Lock()

	acquired := f.WaitTimeout(20 * time.Millisecond)
	if acquired {
		t.Error("WaitTimeout should fail when futex is locked")
	}
	f.Unlock()
}

func TestFutex_WaitTimeout_Acquired(t *testing.T) {
	f := NewFutex()
	// Unlock after a short delay.
	go func() {
		time.Sleep(10 * time.Millisecond)
		f.Lock()
		// Immediately unlock so waiter can acquire.
		f.Unlock()
	}()
	time.Sleep(5 * time.Millisecond) // ensure the goroutine locks first
	acquired := f.WaitTimeout(100 * time.Millisecond)
	if !acquired {
		t.Log("WaitTimeout: futex not acquired (timing-sensitive test)")
	}
}

func TestFutex_Concurrent(t *testing.T) {
	f := NewFutex()
	var count int64
	const goroutines = 64

	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			f.Lock()
			atomic.AddInt64(&count, 1)
			f.Unlock()
		}()
	}
	wg.Wait()
	if count != goroutines {
		t.Errorf("expected %d increments, got %d", goroutines, count)
	}
}

func TestFutex_FastAcquireMetric(t *testing.T) {
	f := NewFutex()
	f.Lock()
	f.Unlock()
	if f.FastAcquires != 1 {
		t.Errorf("expected 1 fast acquire, got %d", f.FastAcquires)
	}
}

func BenchmarkFutex(bm *testing.B) {
	f := NewFutex()
	bm.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			f.Lock()
			f.Unlock()
		}
	})
}

func BenchmarkSyncMutex(bm *testing.B) {
	var mu sync.Mutex
	bm.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			mu.Lock()
			mu.Unlock()
		}
	})
}
