/*
Synchronization Primitives
===========================

Implementation of classic synchronization primitives for concurrent programming.

Applications:
- Thread-safe data structures
- Critical section protection
- Producer-consumer problems
- Resource pooling
- Concurrent algorithm coordination
*/

package ossync

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// =============================================================================
// Semaphore
// =============================================================================

// Semaphore implements a counting semaphore
type Semaphore struct {
	permits int32
	cond    *sync.Cond
	mu      sync.Mutex
}

// NewSemaphore creates a new semaphore with given number of permits
func NewSemaphore(permits int) *Semaphore {
	s := &Semaphore{
		permits: int32(permits),
	}
	s.cond = sync.NewCond(&s.mu)
	return s
}

// Acquire acquires one permit, blocking until available
func (s *Semaphore) Acquire() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for s.permits <= 0 {
		s.cond.Wait()
	}
	s.permits--
}

// TryAcquire attempts to acquire a permit without blocking
func (s *Semaphore) TryAcquire() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.permits > 0 {
		s.permits--
		return true
	}
	return false
}

// AcquireTimeout attempts to acquire with timeout.
// A single goroutine is spawned for the duration of the call; it broadcasts
// on the cond when the deadline fires so that the blocked Wait() returns and
// the deadline check in the loop can detect expiry.  The goroutine is
// cancelled via the done channel as soon as AcquireTimeout returns (success
// or timeout), preventing any goroutine leak.
func (s *Semaphore) AcquireTimeout(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)

	done := make(chan struct{})
	defer close(done) // cancel the wakeup goroutine on return

	go func() {
		select {
		case <-time.After(timeout):
			// Wake up any goroutine blocked in s.cond.Wait() so the
			// deadline check in the loop below can fire.
			s.mu.Lock()
			s.cond.Broadcast()
			s.mu.Unlock()
		case <-done:
			// Semaphore acquired or caller returned — nothing to do.
		}
	}()

	s.mu.Lock()
	defer s.mu.Unlock()

	for s.permits <= 0 {
		if time.Until(deadline) <= 0 {
			return false
		}
		s.cond.Wait()
	}

	s.permits--
	return true
}

// Release releases one permit
func (s *Semaphore) Release() {
	s.mu.Lock()
	s.permits++
	s.mu.Unlock()
	s.cond.Signal()
}

// AvailablePermits returns the current number of available permits
func (s *Semaphore) AvailablePermits() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return int(s.permits)
}

// =============================================================================
// Barrier
// =============================================================================

// Barrier allows a set of goroutines to wait for each other
type Barrier struct {
	size     int
	count    int
	mu       sync.Mutex
	cond     *sync.Cond
	generation int
}

// NewBarrier creates a barrier for n goroutines
func NewBarrier(n int) *Barrier {
	b := &Barrier{
		size:  n,
		count: 0,
		generation: 0,
	}
	b.cond = sync.NewCond(&b.mu)
	return b
}

// Wait waits at the barrier until all goroutines arrive
func (b *Barrier) Wait() {
	b.mu.Lock()
	generation := b.generation
	b.count++

	if b.count == b.size {
		// Last one to arrive - release all
		b.count = 0
		b.generation++
		b.cond.Broadcast()
		b.mu.Unlock()
		return
	}

	// Wait for all to arrive
	for generation == b.generation {
		b.cond.Wait()
	}
	b.mu.Unlock()
}

// Reset resets the barrier to initial state
func (b *Barrier) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.count = 0
	b.generation++
	b.cond.Broadcast()
}

// =============================================================================
// Read-Write Lock (custom implementation)
// =============================================================================

// RWLock implements a reader-writer lock with preference options
type RWLock struct {
	readers      int
	writers      int
	writeWaiting int
	writerPref   bool // True = writer preference, False = reader preference
	mu           sync.Mutex
	readCond     *sync.Cond
	writeCond    *sync.Cond
}

// NewRWLock creates a new read-write lock
func NewRWLock(writerPreference bool) *RWLock {
	rw := &RWLock{
		writerPref: writerPreference,
	}
	rw.readCond = sync.NewCond(&rw.mu)
	rw.writeCond = sync.NewCond(&rw.mu)
	return rw
}

// RLock acquires a read lock
func (rw *RWLock) RLock() {
	rw.mu.Lock()
	defer rw.mu.Unlock()

	if rw.writerPref {
		// Wait if there are writers or waiting writers
		for rw.writers > 0 || rw.writeWaiting > 0 {
			rw.readCond.Wait()
		}
	} else {
		// Wait only if there are active writers
		for rw.writers > 0 {
			rw.readCond.Wait()
		}
	}

	rw.readers++
}

// RUnlock releases a read lock
func (rw *RWLock) RUnlock() {
	rw.mu.Lock()
	defer rw.mu.Unlock()

	rw.readers--
	if rw.readers == 0 {
		rw.writeCond.Signal()
	}
}

// WLock acquires a write lock
func (rw *RWLock) WLock() {
	rw.mu.Lock()
	defer rw.mu.Unlock()

	rw.writeWaiting++
	for rw.readers > 0 || rw.writers > 0 {
		rw.writeCond.Wait()
	}
	rw.writeWaiting--
	rw.writers++
}

// WUnlock releases a write lock
func (rw *RWLock) WUnlock() {
	rw.mu.Lock()
	defer rw.mu.Unlock()

	rw.writers--
	if rw.writerPref && rw.writeWaiting > 0 {
		rw.writeCond.Signal()
	} else {
		rw.readCond.Broadcast()
		rw.writeCond.Signal()
	}
}

// =============================================================================
// Latch - One-time barrier
// =============================================================================

// Latch allows goroutines to wait for a one-time event
type Latch struct {
	count int32
	done  chan struct{}
}

// NewLatch creates a latch with count
func NewLatch(count int) *Latch {
	return &Latch{
		count: int32(count),
		done:  make(chan struct{}),
	}
}

// CountDown decrements the latch count
func (l *Latch) CountDown() {
	if atomic.AddInt32(&l.count, -1) == 0 {
		close(l.done)
	}
}

// Wait waits until latch count reaches zero
func (l *Latch) Wait() {
	<-l.done
}

// WaitTimeout waits with timeout
func (l *Latch) WaitTimeout(timeout time.Duration) bool {
	select {
	case <-l.done:
		return true
	case <-time.After(timeout):
		return false
	}
}

// GetCount returns current count
func (l *Latch) GetCount() int {
	return int(atomic.LoadInt32(&l.count))
}

// =============================================================================
// Monitor - High-level synchronization
// =============================================================================

// Monitor implements a monitor with condition variables
type Monitor struct {
	mu    sync.Mutex
	conds map[string]*sync.Cond
}

// NewMonitor creates a new monitor
func NewMonitor() *Monitor {
	return &Monitor{
		conds: make(map[string]*sync.Cond),
	}
}

// Enter enters the monitor (acquires lock)
func (m *Monitor) Enter() {
	m.mu.Lock()
}

// Exit exits the monitor (releases lock)
func (m *Monitor) Exit() {
	m.mu.Unlock()
}

// Wait waits on a condition variable
func (m *Monitor) Wait(condName string) {
	if _, exists := m.conds[condName]; !exists {
		m.conds[condName] = sync.NewCond(&m.mu)
	}
	m.conds[condName].Wait()
}

// Signal signals one waiting goroutine on condition
func (m *Monitor) Signal(condName string) {
	if cond, exists := m.conds[condName]; exists {
		cond.Signal()
	}
}

// Broadcast signals all waiting goroutines on condition
func (m *Monitor) Broadcast(condName string) {
	if cond, exists := m.conds[condName]; exists {
		cond.Broadcast()
	}
}

// =============================================================================
// Spinlock - Busy-wait lock
// =============================================================================

// Spinlock implements a spinlock using atomic operations
type Spinlock struct {
	state int32
}

// NewSpinlock creates a new spinlock
func NewSpinlock() *Spinlock {
	return &Spinlock{state: 0}
}

// Lock acquires the spinlock
func (s *Spinlock) Lock() {
	for !atomic.CompareAndSwapInt32(&s.state, 0, 1) {
		// Busy wait (spin)
		for atomic.LoadInt32(&s.state) == 1 {
			// Hint to CPU that we're spinning
			// In production, use runtime.Gosched() or similar
		}
	}
}

// Unlock releases the spinlock
func (s *Spinlock) Unlock() {
	atomic.StoreInt32(&s.state, 0)
}

// TryLock attempts to acquire lock without blocking
func (s *Spinlock) TryLock() bool {
	return atomic.CompareAndSwapInt32(&s.state, 0, 1)
}

// =============================================================================
// Reentrant Lock
// =============================================================================

// ReentrantLock allows the same goroutine to acquire the lock multiple times
type ReentrantLock struct {
	mu       sync.Mutex
	owner    int64 // Goroutine ID (simplified with counter)
	count    int
	ownerSet bool
}

// NewReentrantLock creates a new reentrant lock
func NewReentrantLock() *ReentrantLock {
	return &ReentrantLock{}
}

// Lock acquires the lock
func (r *ReentrantLock) Lock(goroutineID int64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.ownerSet && r.owner == goroutineID {
		r.count++
		return
	}

	// Wait until lock is free
	for r.ownerSet && r.count > 0 {
		r.mu.Unlock()
		time.Sleep(time.Microsecond)
		r.mu.Lock()
	}

	r.owner = goroutineID
	r.ownerSet = true
	r.count = 1
}

// Unlock releases the lock
func (r *ReentrantLock) Unlock(goroutineID int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.ownerSet || r.owner != goroutineID {
		return errors.New("not lock owner")
	}

	r.count--
	if r.count == 0 {
		r.ownerSet = false
	}
	return nil
}

// =============================================================================
// Bounded Buffer (Thread-Safe Queue)
// =============================================================================

// BoundedBuffer implements a thread-safe bounded buffer
type BoundedBuffer struct {
	buffer   []interface{}
	capacity int
	size     int
	head     int
	tail     int
	mu       sync.Mutex
	notEmpty *sync.Cond
	notFull  *sync.Cond
	itemsIn  int
	itemsOut int
}

// NewBoundedBuffer creates a bounded buffer with given capacity
func NewBoundedBuffer(capacity int) *BoundedBuffer {
	bb := &BoundedBuffer{
		buffer:   make([]interface{}, capacity),
		capacity: capacity,
		size:     0,
		head:     0,
		tail:     0,
	}
	bb.notEmpty = sync.NewCond(&bb.mu)
	bb.notFull = sync.NewCond(&bb.mu)
	return bb
}

// Put adds item to buffer, blocking if full
func (bb *BoundedBuffer) Put(item interface{}) {
	bb.mu.Lock()
	defer bb.mu.Unlock()

	for bb.size == bb.capacity {
		bb.notFull.Wait()
	}

	bb.buffer[bb.tail] = item
	bb.tail = (bb.tail + 1) % bb.capacity
	bb.size++
	bb.itemsIn++

	bb.notEmpty.Signal()
}

// Get removes and returns item from buffer, blocking if empty
func (bb *BoundedBuffer) Get() interface{} {
	bb.mu.Lock()
	defer bb.mu.Unlock()

	for bb.size == 0 {
		bb.notEmpty.Wait()
	}

	item := bb.buffer[bb.head]
	bb.head = (bb.head + 1) % bb.capacity
	bb.size--
	bb.itemsOut++

	bb.notFull.Signal()
	return item
}

// TryPut attempts to add item without blocking
func (bb *BoundedBuffer) TryPut(item interface{}) bool {
	bb.mu.Lock()
	defer bb.mu.Unlock()

	if bb.size == bb.capacity {
		return false
	}

	bb.buffer[bb.tail] = item
	bb.tail = (bb.tail + 1) % bb.capacity
	bb.size++
	bb.itemsIn++

	bb.notEmpty.Signal()
	return true
}

// TryGet attempts to get item without blocking
func (bb *BoundedBuffer) TryGet() (interface{}, bool) {
	bb.mu.Lock()
	defer bb.mu.Unlock()

	if bb.size == 0 {
		return nil, false
	}

	item := bb.buffer[bb.head]
	bb.head = (bb.head + 1) % bb.capacity
	bb.size--
	bb.itemsOut++

	bb.notFull.Signal()
	return item, true
}

// Size returns current size
func (bb *BoundedBuffer) Size() int {
	bb.mu.Lock()
	defer bb.mu.Unlock()
	return bb.size
}

// IsFull returns whether buffer is full
func (bb *BoundedBuffer) IsFull() bool {
	bb.mu.Lock()
	defer bb.mu.Unlock()
	return bb.size == bb.capacity
}

// IsEmpty returns whether buffer is empty
func (bb *BoundedBuffer) IsEmpty() bool {
	bb.mu.Lock()
	defer bb.mu.Unlock()
	return bb.size == 0
}

// GetStats returns buffer statistics
func (bb *BoundedBuffer) GetStats() (in, out, current int) {
	bb.mu.Lock()
	defer bb.mu.Unlock()
	return bb.itemsIn, bb.itemsOut, bb.size
}

// =============================================================================
// Countdown Latch
// =============================================================================

// CountdownLatch waits for a set of operations to complete
type CountdownLatch struct {
	count uint64
	wait  chan struct{}
	once  sync.Once
}

// NewCountdownLatch creates a countdown latch
func NewCountdownLatch(count uint64) *CountdownLatch {
	return &CountdownLatch{
		count: count,
		wait:  make(chan struct{}),
	}
}

// Done decrements the counter
func (c *CountdownLatch) Done() {
	newCount := atomic.AddUint64(&c.count, ^uint64(0)) // Decrement
	if newCount == 0 {
		c.once.Do(func() {
			close(c.wait)
		})
	}
}

// Await waits for counter to reach zero
func (c *CountdownLatch) Await() {
	<-c.wait
}

// GetCount returns current count
func (c *CountdownLatch) GetCount() uint64 {
	return atomic.LoadUint64(&c.count)
}
