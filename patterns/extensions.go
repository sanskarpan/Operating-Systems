package patterns

// =============================================================================
// PAT-001: Singleflight
// =============================================================================
//
// Singleflight deduplicates concurrent identical in-flight requests.
// The first caller to invoke Do() for a given key executes the function;
// all subsequent callers with the same key block until it completes and
// then receive the same result.

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// SingleflightCall tracks an in-progress or completed request.
type SingleflightCall struct {
	wg  sync.WaitGroup
	val interface{}
	err error
	// Number of callers sharing this call.
	dups int64
}

// SingleflightGroup manages a set of in-flight (or recently completed) calls.
type SingleflightGroup struct {
	mu sync.Mutex
	m  map[string]*SingleflightCall
}

// NewSingleflightGroup creates a new group.
func NewSingleflightGroup() *SingleflightGroup {
	return &SingleflightGroup{m: make(map[string]*SingleflightCall)}
}

// Do executes fn if no call with key is already in flight, otherwise waits for
// the in-flight call to complete and returns its result.
// Returns (value, shared, error) where shared is true for callers that piggybacked.
func (g *SingleflightGroup) Do(key string, fn func() (interface{}, error)) (interface{}, bool, error) {
	g.mu.Lock()
	if call, ok := g.m[key]; ok {
		atomic.AddInt64(&call.dups, 1)
		g.mu.Unlock()
		call.wg.Wait()
		return call.val, true, call.err
	}
	call := &SingleflightCall{}
	call.wg.Add(1)
	g.m[key] = call
	g.mu.Unlock()

	call.val, call.err = fn()
	call.wg.Done()

	g.mu.Lock()
	delete(g.m, key)
	g.mu.Unlock()

	return call.val, false, call.err
}

// InFlight returns the number of keys currently in flight.
func (g *SingleflightGroup) InFlight() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.m)
}

// =============================================================================
// PAT-002: Saga Pattern
// =============================================================================
//
// A Saga is a sequence of steps, each with a compensating action for rollback.
// Steps execute in order; if any step fails, previously completed steps are
// compensated in reverse order (LIFO rollback).

// SagaStep is one unit of work with a compensating undo.
type SagaStep struct {
	Name       string
	Execute    func() error
	Compensate func() error
}

// SagaResult records execution outcome.
type SagaResult struct {
	Completed    []string // names of successfully completed steps
	FailedStep   string   // name of the step that failed (empty if success)
	Compensated  []string // names of steps that were compensated
	CompErrors   []error  // any errors from compensation actions
	Err          error    // the error that triggered rollback
}

// Saga orchestrates transactional steps with compensating rollbacks.
type Saga struct {
	steps []SagaStep
}

// NewSaga creates an empty saga.
func NewSaga() *Saga { return &Saga{} }

// AddStep appends a step (execute + compensate pair).
func (s *Saga) AddStep(name string, execute, compensate func() error) *Saga {
	s.steps = append(s.steps, SagaStep{Name: name, Execute: execute, Compensate: compensate})
	return s
}

// Run executes all steps sequentially. On failure it rolls back in reverse order.
func (s *Saga) Run() SagaResult {
	var result SagaResult
	completed := make([]*SagaStep, 0, len(s.steps))

	for i := range s.steps {
		step := &s.steps[i]
		if err := step.Execute(); err != nil {
			result.FailedStep = step.Name
			result.Err = err
			// Roll back completed steps in reverse.
			for j := len(completed) - 1; j >= 0; j-- {
				cs := completed[j]
				if cerr := cs.Compensate(); cerr != nil {
					result.CompErrors = append(result.CompErrors, cerr)
				} else {
					result.Compensated = append(result.Compensated, cs.Name)
				}
			}
			return result
		}
		result.Completed = append(result.Completed, step.Name)
		completed = append(completed, step)
	}
	return result
}

// =============================================================================
// PAT-003: Backpressure
// =============================================================================
//
// BackpressureStrategy controls what happens when the consumer falls behind.

// BackpressureStrategy selects the overflow behaviour.
type BackpressureStrategy int

const (
	DropOldest  BackpressureStrategy = iota // evict the oldest buffered item
	DropNewest                              // discard the incoming item
	Block                                   // block the producer until space is free
	ErrorOnFull                             // return an error to the producer
)

// BackpressureBuffer is a bounded channel with configurable overflow strategy.
type BackpressureBuffer struct {
	ch       chan interface{}
	strategy BackpressureStrategy
	mu       sync.Mutex

	Dropped  int64 // atomic — items dropped
	Produced int64 // atomic — items accepted
}

// NewBackpressureBuffer creates a buffer of the given capacity and strategy.
func NewBackpressureBuffer(capacity int, strategy BackpressureStrategy) *BackpressureBuffer {
	return &BackpressureBuffer{
		ch:       make(chan interface{}, capacity),
		strategy: strategy,
	}
}

// Send attempts to put item into the buffer according to the strategy.
// Returns an error only for ErrorOnFull; other strategies handle silently.
func (b *BackpressureBuffer) Send(item interface{}) error {
	switch b.strategy {
	case DropOldest:
		for {
			select {
			case b.ch <- item:
				atomic.AddInt64(&b.Produced, 1)
				return nil
			default:
				// Drain oldest.
				select {
				case <-b.ch:
					atomic.AddInt64(&b.Dropped, 1)
				default:
				}
			}
		}
	case DropNewest:
		select {
		case b.ch <- item:
			atomic.AddInt64(&b.Produced, 1)
		default:
			atomic.AddInt64(&b.Dropped, 1)
		}
		return nil
	case Block:
		b.ch <- item
		atomic.AddInt64(&b.Produced, 1)
		return nil
	case ErrorOnFull:
		select {
		case b.ch <- item:
			atomic.AddInt64(&b.Produced, 1)
			return nil
		default:
			atomic.AddInt64(&b.Dropped, 1)
			return errors.New("backpressure: buffer full")
		}
	}
	return nil
}

// Receive returns the next item from the buffer (blocks until available).
func (b *BackpressureBuffer) Receive() interface{} { return <-b.ch }

// TryReceive returns an item and true if one is available, else false.
func (b *BackpressureBuffer) TryReceive() (interface{}, bool) {
	select {
	case v := <-b.ch:
		return v, true
	default:
		return nil, false
	}
}

// Len returns the number of items currently in the buffer.
func (b *BackpressureBuffer) Len() int { return len(b.ch) }

// Cap returns the buffer capacity.
func (b *BackpressureBuffer) Cap() int { return cap(b.ch) }

// =============================================================================
// PAT-004: Bulkhead
// =============================================================================
//
// A Bulkhead isolates a semaphore-bounded goroutine pool per resource type.
// If the pool for a resource is exhausted, callers are rejected rather than
// blocking indefinitely, preventing one resource's load from starving others.

// Bulkhead limits concurrency for a named resource.
type Bulkhead struct {
	name    string
	sem     chan struct{}
	timeout time.Duration

	Accepted int64 // atomic
	Rejected int64 // atomic
}

// NewBulkhead creates a bulkhead for resource name with maxConcurrent slots
// and a wait timeout for acquiring a slot.
func NewBulkhead(name string, maxConcurrent int, waitTimeout time.Duration) *Bulkhead {
	return &Bulkhead{
		name:    name,
		sem:     make(chan struct{}, maxConcurrent),
		timeout: waitTimeout,
	}
}

// Execute runs fn if a slot is available within the timeout, otherwise rejects.
func (b *Bulkhead) Execute(fn func() error) error {
	select {
	case b.sem <- struct{}{}:
		defer func() { <-b.sem }()
		atomic.AddInt64(&b.Accepted, 1)
		return fn()
	case <-time.After(b.timeout):
		atomic.AddInt64(&b.Rejected, 1)
		return fmt.Errorf("bulkhead[%s]: rejected (pool exhausted)", b.name)
	}
}

// Available returns the number of free slots.
func (b *Bulkhead) Available() int { return cap(b.sem) - len(b.sem) }

// BulkheadRegistry manages multiple named bulkheads.
type BulkheadRegistry struct {
	mu       sync.RWMutex
	bulkheads map[string]*Bulkhead
}

// NewBulkheadRegistry creates an empty registry.
func NewBulkheadRegistry() *BulkheadRegistry {
	return &BulkheadRegistry{bulkheads: make(map[string]*Bulkhead)}
}

// Register adds or replaces a bulkhead for the named resource.
func (r *BulkheadRegistry) Register(b *Bulkhead) {
	r.mu.Lock()
	r.bulkheads[b.name] = b
	r.mu.Unlock()
}

// Get returns the bulkhead for name.
func (r *BulkheadRegistry) Get(name string) (*Bulkhead, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	b, ok := r.bulkheads[name]
	return b, ok
}

// Execute runs fn on the named resource's bulkhead.
func (r *BulkheadRegistry) Execute(name string, fn func() error) error {
	r.mu.RLock()
	b, ok := r.bulkheads[name]
	r.mu.RUnlock()
	if !ok {
		return fmt.Errorf("bulkhead: resource %q not registered", name)
	}
	return b.Execute(fn)
}

// =============================================================================
// PAT-005: Scatter-Gather
// =============================================================================
//
// Scatter-gather fans a request out to N upstream functions concurrently,
// collects the first K successful responses, cancels all remaining calls,
// and returns with an optional overall timeout.

// UpstreamFn is a function that performs one upstream call.
// It must respect context cancellation.
type UpstreamFn func(ctx context.Context) (interface{}, error)

// ScatterGatherResult holds the outcome of a scatter-gather call.
type ScatterGatherResult struct {
	Responses []interface{} // up to K successful responses
	Errors    []error       // errors from failed upstreams
}

// ScatterGather fans out to all upstreams, collects the first k successes,
// and cancels the rest. A zero timeout means no additional deadline.
func ScatterGather(
	ctx context.Context,
	upstreams []UpstreamFn,
	k int,
	timeout time.Duration,
) ScatterGatherResult {
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	type outcome struct {
		val interface{}
		err error
	}

	results := make(chan outcome, len(upstreams))
	for _, fn := range upstreams {
		fn := fn
		go func() {
			v, err := fn(ctx)
			results <- outcome{v, err}
		}()
	}

	var res ScatterGatherResult
	remaining := len(upstreams)
	for remaining > 0 && len(res.Responses) < k {
		select {
		case out := <-results:
			remaining--
			if out.err != nil {
				res.Errors = append(res.Errors, out.err)
			} else {
				res.Responses = append(res.Responses, out.val)
				if len(res.Responses) == k {
					cancel() // cancel remaining upstreams
				}
			}
		case <-ctx.Done():
			res.Errors = append(res.Errors, ctx.Err())
			return res
		}
	}
	return res
}

// =============================================================================
// PAT-006: Work Stealing (deque-based)
// =============================================================================
//
// Each worker owns a double-ended queue. It pushes/pops from the bottom.
// Idle workers steal from the top (front) of another worker's deque.
// This is a standalone patterns-layer implementation distinct from the
// scheduling package's WSScheduler.

// StealTask is the unit of work in the stealing scheduler.
type StealTask struct {
	fn func()
}

// NewStealTask wraps fn in a StealTask.
func NewStealTask(fn func()) *StealTask { return &StealTask{fn: fn} }

// stealDeque is a mutex-protected double-ended queue.
type stealDeque struct {
	mu    sync.Mutex
	items []*StealTask
}

func (d *stealDeque) pushBottom(t *StealTask) {
	d.mu.Lock()
	d.items = append(d.items, t)
	d.mu.Unlock()
}

func (d *stealDeque) popBottom() (*StealTask, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.items) == 0 {
		return nil, false
	}
	last := len(d.items) - 1
	t := d.items[last]
	d.items = d.items[:last]
	return t, true
}

func (d *stealDeque) stealTop() (*StealTask, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.items) == 0 {
		return nil, false
	}
	t := d.items[0]
	d.items = d.items[1:]
	return t, true
}

func (d *stealDeque) len() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.items)
}

// StealingExecutor is a pool of workers that steal from each other's deques.
type StealingExecutor struct {
	workers  []*stealDeque
	stop     chan struct{}
	wg       sync.WaitGroup
	Executed int64 // atomic — total tasks completed
	Stolen   int64 // atomic — tasks acquired by stealing
}

// NewStealingExecutor creates a pool of n workers.
func NewStealingExecutor(n int) *StealingExecutor {
	e := &StealingExecutor{
		workers: make([]*stealDeque, n),
		stop:    make(chan struct{}),
	}
	for i := range e.workers {
		e.workers[i] = &stealDeque{}
	}
	return e
}

// Submit places a task on worker workerID's deque.
func (e *StealingExecutor) Submit(workerID int, t *StealTask) {
	e.workers[workerID%len(e.workers)].pushBottom(t)
}

// Start launches all worker goroutines.
func (e *StealingExecutor) Start() {
	for id := range e.workers {
		id := id
		e.wg.Add(1)
		go func() {
			defer e.wg.Done()
			e.runWorker(id)
		}()
	}
}

// Stop signals all workers to drain and exit.
func (e *StealingExecutor) Stop() {
	close(e.stop)
	e.wg.Wait()
}

func (e *StealingExecutor) runWorker(id int) {
	own := e.workers[id]
	for {
		// Try own deque first.
		if t, ok := own.popBottom(); ok {
			t.fn()
			atomic.AddInt64(&e.Executed, 1)
			continue
		}
		// Try to steal from another worker.
		stolen := false
		for off := 1; off < len(e.workers); off++ {
			victim := e.workers[(id+off)%len(e.workers)]
			if t, ok := victim.stealTop(); ok {
				t.fn()
				atomic.AddInt64(&e.Executed, 1)
				atomic.AddInt64(&e.Stolen, 1)
				stolen = true
				break
			}
		}
		if !stolen {
			// Check for stop signal before spinning.
			select {
			case <-e.stop:
				return
			default:
				// Yield briefly before retrying.
				time.Sleep(time.Microsecond)
			}
		}
	}
}
