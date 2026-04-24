/*
Lock-Free Data Structures
==========================

Non-blocking concurrent data structures using atomic CAS operations:
  - Treiber Stack with ABA fix      (LF-001)
  - Michael & Scott Queue            (LF-002)
  - SPSC Ring Buffer                 (LF-003)
  - MPMC Ring Buffer                 (LF-004)
  - Futex simulation                 (LF-005)
*/

package lockfree

import (
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

// =============================================================================
// LF-001: Treiber Stack (CAS-based, with ABA fix via tagged pointers)
// =============================================================================

// stackNode is a singly-linked node in the Treiber stack.
type stackNode struct {
	value interface{}
	next  *stackNode
}

// taggedPointer packs a pointer and a tag into a single uint64.
// On 64-bit systems, pointer addresses fit in 48 bits leaving room for tags,
// but for portability we use a struct with two fields and operate on it via
// a mutex-less compare-and-swap on the pointer + tag pair stored in a struct
// that is accessed atomically via unsafe.Pointer.
type taggedPtr struct {
	ptr *stackNode
	tag uint64
}

// TreiberStack is a lock-free stack using compare-and-swap.
// The tagged pointer technique prevents the ABA problem: each push increments
// a version tag so a stale pointer from before a pop-push cycle is rejected.
type TreiberStack struct {
	head unsafe.Pointer // *taggedPtr, accessed via atomic load/store/CAS
	size int64
}

// NewTreiberStack creates an empty Treiber stack.
func NewTreiberStack() *TreiberStack {
	s := &TreiberStack{}
	// Initialize with a sentinel tagged pointer.
	initial := &taggedPtr{ptr: nil, tag: 0}
	atomic.StorePointer(&s.head, unsafe.Pointer(initial))
	return s
}

// Push adds a value to the top of the stack.
func (s *TreiberStack) Push(val interface{}) {
	node := &stackNode{value: val}
	for {
		oldHead := (*taggedPtr)(atomic.LoadPointer(&s.head))
		node.next = oldHead.ptr
		newHead := &taggedPtr{ptr: node, tag: oldHead.tag + 1}
		if atomic.CompareAndSwapPointer(&s.head,
			unsafe.Pointer(oldHead),
			unsafe.Pointer(newHead)) {
			atomic.AddInt64(&s.size, 1)
			return
		}
		runtime.Gosched()
	}
}

// Pop removes and returns the top value.
// Returns (value, true) or (nil, false) if the stack is empty.
func (s *TreiberStack) Pop() (interface{}, bool) {
	for {
		oldHead := (*taggedPtr)(atomic.LoadPointer(&s.head))
		if oldHead.ptr == nil {
			return nil, false
		}
		next := oldHead.ptr.next
		newHead := &taggedPtr{ptr: next, tag: oldHead.tag + 1}
		if atomic.CompareAndSwapPointer(&s.head,
			unsafe.Pointer(oldHead),
			unsafe.Pointer(newHead)) {
			atomic.AddInt64(&s.size, -1)
			return oldHead.ptr.value, true
		}
		runtime.Gosched()
	}
}

// Peek returns the top value without removing it.
func (s *TreiberStack) Peek() (interface{}, bool) {
	head := (*taggedPtr)(atomic.LoadPointer(&s.head))
	if head.ptr == nil {
		return nil, false
	}
	return head.ptr.value, true
}

// Len returns the approximate number of elements.
func (s *TreiberStack) Len() int64 {
	return atomic.LoadInt64(&s.size)
}

// IsEmpty reports whether the stack has no elements.
func (s *TreiberStack) IsEmpty() bool {
	head := (*taggedPtr)(atomic.LoadPointer(&s.head))
	return head.ptr == nil
}

// =============================================================================
// LF-002: Michael & Scott Non-Blocking Queue
// =============================================================================

// msNode is a node in the Michael & Scott queue.
type msNode struct {
	value interface{}
	next  unsafe.Pointer // *msNode
}

// MSQueue is a lock-free FIFO queue using the Michael & Scott algorithm.
// It maintains separate head (dequeue) and tail (enqueue) pointers,
// allowing concurrent enqueue and dequeue without locking.
type MSQueue struct {
	head unsafe.Pointer // *msNode (sentinel)
	tail unsafe.Pointer // *msNode
	size int64
}

// NewMSQueue creates an empty Michael & Scott queue with a sentinel node.
func NewMSQueue() *MSQueue {
	sentinel := &msNode{}
	q := &MSQueue{}
	atomic.StorePointer(&q.head, unsafe.Pointer(sentinel))
	atomic.StorePointer(&q.tail, unsafe.Pointer(sentinel))
	return q
}

// Enqueue adds a value to the tail of the queue.
func (q *MSQueue) Enqueue(val interface{}) {
	node := &msNode{value: val}
	for {
		tail := (*msNode)(atomic.LoadPointer(&q.tail))
		next := (*msNode)(atomic.LoadPointer(&tail.next))
		// Is tail still the actual tail?
		if tail == (*msNode)(atomic.LoadPointer(&q.tail)) {
			if next == nil {
				// Tail is pointing to the last node; try to link in new node.
				if atomic.CompareAndSwapPointer(&tail.next, nil, unsafe.Pointer(node)) {
					// Advance tail (best-effort; another goroutine may do it).
					atomic.CompareAndSwapPointer(&q.tail,
						unsafe.Pointer(tail), unsafe.Pointer(node))
					atomic.AddInt64(&q.size, 1)
					return
				}
			} else {
				// Tail is not pointing to the last node; advance it.
				atomic.CompareAndSwapPointer(&q.tail,
					unsafe.Pointer(tail), unsafe.Pointer(next))
			}
		}
		runtime.Gosched()
	}
}

// Dequeue removes and returns the front value.
// Returns (value, true) or (nil, false) if the queue is empty.
func (q *MSQueue) Dequeue() (interface{}, bool) {
	for {
		head := (*msNode)(atomic.LoadPointer(&q.head))
		tail := (*msNode)(atomic.LoadPointer(&q.tail))
		next := (*msNode)(atomic.LoadPointer(&head.next))

		if head == (*msNode)(atomic.LoadPointer(&q.head)) {
			if head == tail {
				if next == nil {
					return nil, false // empty
				}
				// Tail is falling behind; advance it.
				atomic.CompareAndSwapPointer(&q.tail,
					unsafe.Pointer(tail), unsafe.Pointer(next))
			} else {
				val := next.value
				if atomic.CompareAndSwapPointer(&q.head,
					unsafe.Pointer(head), unsafe.Pointer(next)) {
					atomic.AddInt64(&q.size, -1)
					return val, true
				}
			}
		}
		runtime.Gosched()
	}
}

// Len returns the approximate number of elements.
func (q *MSQueue) Len() int64 {
	return atomic.LoadInt64(&q.size)
}

// IsEmpty reports whether the queue has no elements.
func (q *MSQueue) IsEmpty() bool {
	head := (*msNode)(atomic.LoadPointer(&q.head))
	tail := (*msNode)(atomic.LoadPointer(&q.tail))
	next := (*msNode)(atomic.LoadPointer(&head.next))
	return head == tail && next == nil
}

// =============================================================================
// LF-003: SPSC Ring Buffer (Single-Producer Single-Consumer)
// =============================================================================

// cacheLine is the size of a CPU cache line in bytes.
const cacheLine = 64

// spscSlot is a padded slot to prevent false sharing.
type spscSlot struct {
	val interface{}
	_   [cacheLine - 8]byte // pad to cache line (subtract pointer size)
}

// SPSCRingBuffer is a wait-free ring buffer for exactly one producer and
// one consumer goroutine. The size must be a power of 2.
//
// The producer owns `head` (write pointer) and the consumer owns `tail`
// (read pointer). Each is padded to its own cache line.
type SPSCRingBuffer struct {
	_      [cacheLine]byte     // pad before head
	head   uint64              // write index (producer)
	_      [cacheLine - 8]byte // pad after head
	tail   uint64              // read index (consumer)
	_      [cacheLine - 8]byte // pad after tail
	mask   uint64
	slots  []spscSlot
}

// NewSPSCRingBuffer creates a single-producer single-consumer ring buffer.
// size must be a power of 2 and > 0.
func NewSPSCRingBuffer(size int) (*SPSCRingBuffer, error) {
	if size <= 0 || (size&(size-1)) != 0 {
		return nil, errors.New("size must be a positive power of 2")
	}
	return &SPSCRingBuffer{
		mask:  uint64(size - 1),
		slots: make([]spscSlot, size),
	}, nil
}

// TryPush attempts to enqueue a value. Returns false if the buffer is full.
// Must only be called from the producer goroutine.
func (r *SPSCRingBuffer) TryPush(val interface{}) bool {
	head := atomic.LoadUint64(&r.head)
	tail := atomic.LoadUint64(&r.tail)
	if head-tail > r.mask {
		return false // full
	}
	r.slots[head&r.mask].val = val
	atomic.StoreUint64(&r.head, head+1)
	return true
}

// TryPop attempts to dequeue a value. Returns (nil, false) if empty.
// Must only be called from the consumer goroutine.
func (r *SPSCRingBuffer) TryPop() (interface{}, bool) {
	tail := atomic.LoadUint64(&r.tail)
	head := atomic.LoadUint64(&r.head)
	if tail == head {
		return nil, false // empty
	}
	val := r.slots[tail&r.mask].val
	r.slots[tail&r.mask].val = nil // release reference
	atomic.StoreUint64(&r.tail, tail+1)
	return val, true
}

// Len returns an approximate count of elements currently in the buffer.
func (r *SPSCRingBuffer) Len() int {
	head := atomic.LoadUint64(&r.head)
	tail := atomic.LoadUint64(&r.tail)
	if head >= tail {
		return int(head - tail)
	}
	return 0
}

// Cap returns the capacity of the ring buffer.
func (r *SPSCRingBuffer) Cap() int {
	return int(r.mask + 1)
}

// =============================================================================
// LF-004: MPMC Ring Buffer (Dmitry Vyukov's algorithm)
// =============================================================================

// mpmcSlot is one cell in the MPMC ring buffer.
type mpmcSlot struct {
	sequence uint64 // sequence number
	data     interface{}
}

// MPMCRingBuffer is a multi-producer multi-consumer bounded queue based on
// Dmitry Vyukov's "non-intrusive MPMC queue" algorithm using sequence numbers.
// The capacity must be a power of 2.
type MPMCRingBuffer struct {
	_        [cacheLine]byte
	enqPos   uint64
	_        [cacheLine - 8]byte
	deqPos   uint64
	_        [cacheLine - 8]byte
	mask     uint64
	capacity uint64
	slots    []mpmcSlot
}

// NewMPMCRingBuffer creates a multi-producer multi-consumer ring buffer.
// capacity must be a positive power of 2.
func NewMPMCRingBuffer(capacity int) (*MPMCRingBuffer, error) {
	if capacity <= 0 || (capacity&(capacity-1)) != 0 {
		return nil, errors.New("capacity must be a positive power of 2")
	}
	q := &MPMCRingBuffer{
		capacity: uint64(capacity),
		mask:     uint64(capacity - 1),
		slots:    make([]mpmcSlot, capacity),
	}
	for i := range q.slots {
		q.slots[i].sequence = uint64(i)
	}
	return q, nil
}

// TryEnqueue attempts to enqueue a value. Returns false if full.
func (q *MPMCRingBuffer) TryEnqueue(data interface{}) bool {
	var pos uint64
	for {
		pos = atomic.LoadUint64(&q.enqPos)
		slot := &q.slots[pos&q.mask]
		seq := atomic.LoadUint64(&slot.sequence)
		diff := int64(seq) - int64(pos)
		if diff == 0 {
			// Slot is ready for writing; try to claim it.
			if atomic.CompareAndSwapUint64(&q.enqPos, pos, pos+1) {
				break
			}
		} else if diff < 0 {
			return false // full
		}
		runtime.Gosched()
	}
	slot := &q.slots[pos&q.mask]
	slot.data = data
	atomic.StoreUint64(&slot.sequence, pos+1)
	return true
}

// TryDequeue attempts to dequeue a value. Returns (nil, false) if empty.
func (q *MPMCRingBuffer) TryDequeue() (interface{}, bool) {
	var pos uint64
	for {
		pos = atomic.LoadUint64(&q.deqPos)
		slot := &q.slots[pos&q.mask]
		seq := atomic.LoadUint64(&slot.sequence)
		diff := int64(seq) - int64(pos+1)
		if diff == 0 {
			if atomic.CompareAndSwapUint64(&q.deqPos, pos, pos+1) {
				break
			}
		} else if diff < 0 {
			return nil, false // empty
		}
		runtime.Gosched()
	}
	slot := &q.slots[pos&q.mask]
	data := slot.data
	slot.data = nil
	atomic.StoreUint64(&slot.sequence, pos+q.mask+1)
	return data, true
}

// Len returns an approximate number of elements in the queue.
func (q *MPMCRingBuffer) Len() int {
	enq := atomic.LoadUint64(&q.enqPos)
	deq := atomic.LoadUint64(&q.deqPos)
	if enq >= deq {
		return int(enq - deq)
	}
	return 0
}

// Cap returns the capacity.
func (q *MPMCRingBuffer) Cap() int {
	return int(q.capacity)
}

// =============================================================================
// LF-005: Futex Simulation
// =============================================================================

// Futex simulates a Linux-style futex (fast userspace mutex).
//
// Fast path: if the lock word is 0 (unlocked), a CAS atomically sets it to 1
// (locked) without any syscall/channel overhead.
//
// Slow path: if the CAS fails (lock is contended), the goroutine registers
// itself on a wait channel and blocks until woken by the unlock path.
//
// This matches the Linux futex(2) contract:
//   - FUTEX_WAIT: atomically check lock==expected, then sleep
//   - FUTEX_WAKE: wake up to N waiters
type Futex struct {
	state   int32         // 0=unlocked, 1=locked-no-waiters, 2=locked-with-waiters
	mu      sync.Mutex    // protects waiters list
	waiters []chan struct{}

	// Metrics
	FastAcquires int64 // acquired without blocking
	SlowAcquires int64 // acquired after blocking
}

// NewFutex creates an unlocked futex.
func NewFutex() *Futex {
	return &Futex{}
}

// Lock acquires the futex, blocking if necessary.
// Fast path: CAS 0→1 without touching the mutex.
// Slow path: register as waiter under mutex, then block.
// Double-check after taking mutex prevents the "missed wake" race:
// if the holder releases between our failed CAS and our mu.Lock,
// we catch it and acquire directly.
func (f *Futex) Lock() {
	// Fast path: uncontended acquire.
	if atomic.CompareAndSwapInt32(&f.state, 0, 1) {
		atomic.AddInt64(&f.FastAcquires, 1)
		return
	}

	// Slow path.
	ch := make(chan struct{}, 1)
	f.mu.Lock()
	// Double-check: did the lock become free while we were waiting for mu?
	if atomic.CompareAndSwapInt32(&f.state, 0, 1) {
		f.mu.Unlock()
		atomic.AddInt64(&f.FastAcquires, 1)
		return
	}
	// State is 1 or 2; mark that there are waiters and register.
	atomic.StoreInt32(&f.state, 2)
	f.waiters = append(f.waiters, ch)
	f.mu.Unlock()

	// Block until woken by Unlock.
	<-ch
	atomic.AddInt64(&f.SlowAcquires, 1)
}

// TryLock attempts to acquire the futex without blocking.
// Returns true if acquired.
func (f *Futex) TryLock() bool {
	return atomic.CompareAndSwapInt32(&f.state, 0, 1)
}

// Unlock releases the futex.
// If waiters are present, ownership is transferred to the first waiter
// rather than setting state to 0 (which would allow a racing goroutine
// to steal the lock from the woken waiter).
func (f *Futex) Unlock() {
	f.mu.Lock()
	defer f.mu.Unlock()

	if len(f.waiters) == 0 {
		// No contention — simply mark unlocked.
		atomic.StoreInt32(&f.state, 0)
		return
	}
	// Transfer ownership to the first waiter.
	ch := f.waiters[0]
	f.waiters = f.waiters[1:]
	// State reflects how many waiters remain.
	if len(f.waiters) == 0 {
		atomic.StoreInt32(&f.state, 1) // woken goroutine owns it, no more waiters
	} else {
		atomic.StoreInt32(&f.state, 2) // woken goroutine owns it, waiters remain
	}
	ch <- struct{}{} // wake the waiter (lock ownership transfers)
}

// Wake wakes up to n waiters (FUTEX_WAKE equivalent) without releasing
// the logical lock — used for condition-variable style notifications.
func (f *Futex) Wake(n int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	woken := 0
	for woken < n && len(f.waiters) > 0 {
		ch := f.waiters[0]
		f.waiters = f.waiters[1:]
		ch <- struct{}{}
		woken++
	}
}

// WaitTimeout is a time-bounded lock acquisition.
// Returns true if the lock was acquired within the timeout.
func (f *Futex) WaitTimeout(timeout time.Duration) bool {
	// Fast path.
	if atomic.CompareAndSwapInt32(&f.state, 0, 1) {
		atomic.AddInt64(&f.FastAcquires, 1)
		return true
	}

	ch := make(chan struct{}, 1)
	f.mu.Lock()
	if atomic.CompareAndSwapInt32(&f.state, 0, 1) {
		f.mu.Unlock()
		atomic.AddInt64(&f.FastAcquires, 1)
		return true
	}
	atomic.StoreInt32(&f.state, 2)
	f.waiters = append(f.waiters, ch)
	f.mu.Unlock()

	select {
	case <-ch:
		atomic.AddInt64(&f.SlowAcquires, 1)
		return true
	case <-time.After(timeout):
		// Remove from waiters list on timeout.
		f.mu.Lock()
		for i, w := range f.waiters {
			if w == ch {
				f.waiters = append(f.waiters[:i], f.waiters[i+1:]...)
				break
			}
		}
		if len(f.waiters) == 0 && atomic.LoadInt32(&f.state) == 2 {
			// Nobody else is waiting; if lock is still held just leave state as 1.
			// We can't know if we're the holder or not here, so leave it.
		}
		f.mu.Unlock()
		return false
	}
}

// State returns the current futex state (0=unlocked, 1=locked, 2=locked+waiters).
func (f *Futex) State() int32 {
	return atomic.LoadInt32(&f.state)
}
