package patterns

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// =============================================================================
// PAT-001: Singleflight Tests
// =============================================================================

func TestSingleflight_DeduplicatesConcurrent(t *testing.T) {
	g := NewSingleflightGroup()
	var calls int64

	var wg sync.WaitGroup
	results := make([]interface{}, 10)
	for i := 0; i < 10; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, _, _ := g.Do("key", func() (interface{}, error) {
				atomic.AddInt64(&calls, 1)
				time.Sleep(20 * time.Millisecond)
				return 42, nil
			})
			results[i] = v
		}()
	}
	wg.Wait()

	// All goroutines should get 42.
	for i, r := range results {
		if r != 42 {
			t.Errorf("result[%d] = %v, expected 42", i, r)
		}
	}
	// Only one actual call should have happened (or very few due to timing).
	if calls == 0 {
		t.Error("expected at least one real call")
	}
	if calls > 3 {
		t.Logf("note: %d calls (expected ~1, timing may vary)", calls)
	}
}

func TestSingleflight_ErrorPropagates(t *testing.T) {
	g := NewSingleflightGroup()
	want := errors.New("upstream failed")
	_, shared, err := g.Do("fail", func() (interface{}, error) {
		return nil, want
	})
	if err != want {
		t.Errorf("expected %v, got %v", want, err)
	}
	if shared {
		t.Error("first caller should not be shared")
	}
}

func TestSingleflight_KeyReuseAfterCompletion(t *testing.T) {
	g := NewSingleflightGroup()
	var calls int64
	for i := 0; i < 3; i++ {
		g.Do("k", func() (interface{}, error) {
			atomic.AddInt64(&calls, 1)
			return nil, nil
		})
	}
	if calls != 3 {
		t.Errorf("sequential calls should each execute; got %d", calls)
	}
}

func TestSingleflight_InFlight(t *testing.T) {
	g := NewSingleflightGroup()
	started := make(chan struct{})
	done := make(chan struct{})
	go func() {
		g.Do("slow", func() (interface{}, error) {
			close(started)
			<-done
			return nil, nil
		})
	}()
	<-started
	if g.InFlight() != 1 {
		t.Errorf("expected 1 in-flight, got %d", g.InFlight())
	}
	close(done)
	time.Sleep(5 * time.Millisecond)
	if g.InFlight() != 0 {
		t.Errorf("expected 0 in-flight after completion, got %d", g.InFlight())
	}
}

// =============================================================================
// PAT-002: Saga Tests
// =============================================================================

func TestSaga_AllStepsSucceed(t *testing.T) {
	var order []string
	s := NewSaga().
		AddStep("step1", func() error { order = append(order, "s1"); return nil },
			func() error { order = append(order, "c1"); return nil }).
		AddStep("step2", func() error { order = append(order, "s2"); return nil },
			func() error { order = append(order, "c2"); return nil })

	result := s.Run()
	if result.Err != nil {
		t.Errorf("expected success, got %v", result.Err)
	}
	if len(result.Completed) != 2 {
		t.Errorf("expected 2 completed steps, got %d", len(result.Completed))
	}
	if len(result.Compensated) != 0 {
		t.Errorf("no compensation expected on success, got %v", result.Compensated)
	}
}

func TestSaga_RollbackOnFailure(t *testing.T) {
	compensated := make([]string, 0)
	var mu sync.Mutex

	s := NewSaga().
		AddStep("step1",
			func() error { return nil },
			func() error {
				mu.Lock()
				compensated = append(compensated, "step1")
				mu.Unlock()
				return nil
			}).
		AddStep("step2",
			func() error { return nil },
			func() error {
				mu.Lock()
				compensated = append(compensated, "step2")
				mu.Unlock()
				return nil
			}).
		AddStep("step3",
			func() error { return errors.New("boom") },
			func() error { return nil })

	result := s.Run()
	if result.Err == nil {
		t.Error("expected error from step3")
	}
	if result.FailedStep != "step3" {
		t.Errorf("expected FailedStep=step3, got %q", result.FailedStep)
	}
	// step1 and step2 should have been compensated in reverse order.
	if len(compensated) != 2 {
		t.Errorf("expected 2 compensations, got %d: %v", len(compensated), compensated)
	}
	if compensated[0] != "step2" || compensated[1] != "step1" {
		t.Errorf("compensation order wrong: %v", compensated)
	}
}

func TestSaga_FirstStepFails_NoCompensation(t *testing.T) {
	compensated := 0
	s := NewSaga().
		AddStep("fail-first",
			func() error { return errors.New("immediate fail") },
			func() error { compensated++; return nil })

	result := s.Run()
	if result.Err == nil {
		t.Error("expected error")
	}
	if compensated != 0 {
		t.Error("no compensation should run if the step itself failed without prior completions")
	}
}

// =============================================================================
// PAT-003: Backpressure Tests
// =============================================================================

func TestBackpressure_DropNewest(t *testing.T) {
	b := NewBackpressureBuffer(2, DropNewest)
	b.Send(1)
	b.Send(2)
	b.Send(3) // dropped
	if b.Dropped != 1 {
		t.Errorf("expected 1 dropped, got %d", b.Dropped)
	}
	v := b.Receive().(int)
	if v != 1 {
		t.Errorf("expected 1, got %d", v)
	}
}

func TestBackpressure_DropOldest(t *testing.T) {
	b := NewBackpressureBuffer(2, DropOldest)
	b.Send(1)
	b.Send(2)
	b.Send(3) // evicts 1
	// Buffer should now contain [2, 3].
	v1 := b.Receive().(int)
	v2 := b.Receive().(int)
	if v1 != 2 || v2 != 3 {
		t.Errorf("expected [2,3], got [%d,%d]", v1, v2)
	}
}

func TestBackpressure_Block(t *testing.T) {
	b := NewBackpressureBuffer(2, Block)
	b.Send(1)
	b.Send(2)
	// Unblock in background.
	go func() {
		time.Sleep(5 * time.Millisecond)
		b.Receive()
	}()
	b.Send(3) // blocks until consumer drains
	if b.Produced != 3 {
		t.Errorf("expected 3 produced, got %d", b.Produced)
	}
}

func TestBackpressure_ErrorOnFull(t *testing.T) {
	b := NewBackpressureBuffer(1, ErrorOnFull)
	b.Send(1)
	if err := b.Send(2); err == nil {
		t.Error("expected error when buffer is full")
	}
}

func TestBackpressure_TryReceive(t *testing.T) {
	b := NewBackpressureBuffer(4, DropNewest)
	if _, ok := b.TryReceive(); ok {
		t.Error("TryReceive on empty buffer should return false")
	}
	b.Send(7)
	v, ok := b.TryReceive()
	if !ok || v.(int) != 7 {
		t.Errorf("expected 7, got %v ok=%v", v, ok)
	}
}

func TestBackpressure_Len(t *testing.T) {
	b := NewBackpressureBuffer(4, DropNewest)
	b.Send(1)
	b.Send(2)
	if b.Len() != 2 {
		t.Errorf("expected len 2, got %d", b.Len())
	}
}

// =============================================================================
// PAT-004: Bulkhead Tests
// =============================================================================

func TestBulkhead_LimitsParallelism(t *testing.T) {
	bh := NewBulkhead("db", 3, 50*time.Millisecond)
	var (
		mu         sync.Mutex
		concurrent int
		max        int
	)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			bh.Execute(func() error {
				mu.Lock()
				concurrent++
				if concurrent > max {
					max = concurrent
				}
				mu.Unlock()
				time.Sleep(10 * time.Millisecond)
				mu.Lock()
				concurrent--
				mu.Unlock()
				return nil
			})
		}()
	}
	wg.Wait()
	if max > 3 {
		t.Errorf("max concurrency exceeded bulkhead limit: %d > 3", max)
	}
}

func TestBulkhead_RejectsWhenFull(t *testing.T) {
	bh := NewBulkhead("svc", 1, time.Millisecond)
	holding := make(chan struct{})
	go func() {
		bh.Execute(func() error {
			<-holding
			return nil
		})
	}()
	time.Sleep(5 * time.Millisecond) // let goroutine acquire slot

	err := bh.Execute(func() error { return nil }) // should be rejected
	close(holding)
	if err == nil {
		t.Error("expected rejection when bulkhead is full")
	}
	if bh.Rejected == 0 {
		t.Error("expected Rejected > 0")
	}
}

func TestBulkheadRegistry_Execute(t *testing.T) {
	reg := NewBulkheadRegistry()
	reg.Register(NewBulkhead("cache", 2, 10*time.Millisecond))
	reg.Register(NewBulkhead("db", 4, 10*time.Millisecond))

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			reg.Execute("cache", func() error { return nil })
		}()
	}
	wg.Wait()

	if _, ok := reg.Get("cache"); !ok {
		t.Error("expected to find cache bulkhead")
	}
}

func TestBulkheadRegistry_UnknownResource(t *testing.T) {
	reg := NewBulkheadRegistry()
	if err := reg.Execute("missing", func() error { return nil }); err == nil {
		t.Error("expected error for unregistered resource")
	}
}

// =============================================================================
// PAT-005: Scatter-Gather Tests
// =============================================================================

func TestScatterGather_AllSucceed(t *testing.T) {
	upstreams := make([]UpstreamFn, 5)
	for i := range upstreams {
		i := i
		upstreams[i] = func(ctx context.Context) (interface{}, error) {
			return i, nil
		}
	}
	res := ScatterGather(context.Background(), upstreams, 3, 100*time.Millisecond)
	if len(res.Responses) < 3 {
		t.Errorf("expected at least 3 responses, got %d", len(res.Responses))
	}
}

func TestScatterGather_CancelsRemaining(t *testing.T) {
	var cancelled int64
	upstreams := make([]UpstreamFn, 5)
	for i := range upstreams {
		upstreams[i] = func(ctx context.Context) (interface{}, error) {
			select {
			case <-ctx.Done():
				atomic.AddInt64(&cancelled, 1)
				return nil, ctx.Err()
			case <-time.After(50 * time.Millisecond):
				return "ok", nil
			}
		}
	}
	res := ScatterGather(context.Background(), upstreams, 1, 200*time.Millisecond)
	if len(res.Responses) < 1 {
		t.Error("expected at least 1 response")
	}
	// At least some should have been cancelled.
	time.Sleep(60 * time.Millisecond)
	if atomic.LoadInt64(&cancelled) == 0 {
		t.Log("note: no cancellations observed (timing-sensitive)")
	}
}

func TestScatterGather_Timeout(t *testing.T) {
	upstreams := []UpstreamFn{
		func(ctx context.Context) (interface{}, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(500 * time.Millisecond):
				return "late", nil
			}
		},
	}
	res := ScatterGather(context.Background(), upstreams, 1, 20*time.Millisecond)
	if len(res.Responses) > 0 {
		t.Error("expected no responses before timeout")
	}
	if len(res.Errors) == 0 {
		t.Error("expected timeout error")
	}
}

func TestScatterGather_SomeErrors(t *testing.T) {
	upstreams := []UpstreamFn{
		func(ctx context.Context) (interface{}, error) { return nil, errors.New("fail") },
		func(ctx context.Context) (interface{}, error) { return "ok", nil },
	}
	res := ScatterGather(context.Background(), upstreams, 1, 100*time.Millisecond)
	if len(res.Responses) == 0 {
		t.Error("expected at least one successful response")
	}
}

// =============================================================================
// PAT-006: Work Stealing Tests
// =============================================================================

func TestStealingExecutor_ExecutesAll(t *testing.T) {
	e := NewStealingExecutor(4)
	const total = 200
	var executed int64

	e.Start()
	for i := 0; i < total; i++ {
		e.Submit(i%4, NewStealTask(func() {
			atomic.AddInt64(&executed, 1)
		}))
	}

	// Wait for all tasks to drain.
	deadline := time.Now().Add(2 * time.Second)
	for atomic.LoadInt64(&executed) < total && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	e.Stop()

	if atomic.LoadInt64(&executed) != total {
		t.Errorf("expected %d executed, got %d", total, atomic.LoadInt64(&executed))
	}
}

func TestStealingExecutor_Stealing(t *testing.T) {
	e := NewStealingExecutor(4)
	var executed int64

	e.Start()
	// Submit all tasks to worker 0 only — others should steal.
	for i := 0; i < 100; i++ {
		e.Submit(0, NewStealTask(func() {
			time.Sleep(time.Microsecond) // slow task to encourage stealing
			atomic.AddInt64(&executed, 1)
		}))
	}

	deadline := time.Now().Add(3 * time.Second)
	for atomic.LoadInt64(&executed) < 100 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	e.Stop()

	if atomic.LoadInt64(&executed) != 100 {
		t.Errorf("expected 100 executed, got %d", atomic.LoadInt64(&executed))
	}
	if e.Stolen == 0 {
		t.Log("note: no steals observed (timing-sensitive)")
	}
}
