package patterns

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCancellableWork(t *testing.T) {
	// Test successful completion
	ctx := context.Background()
	work := func() error {
		time.Sleep(10 * time.Millisecond)
		return nil
	}

	err := CancellableWork(ctx, work)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Test cancellation
	ctx, cancel := context.WithCancel(context.Background())
	slowWork := func() error {
		time.Sleep(100 * time.Millisecond)
		return nil
	}

	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	err = CancellableWork(ctx, slowWork)
	if err != context.Canceled {
		t.Errorf("Expected context.Canceled, got %v", err)
	}
}

func TestWithTimeout(t *testing.T) {
	// Test successful completion
	err := WithTimeout(100*time.Millisecond, func(ctx context.Context) error {
		time.Sleep(10 * time.Millisecond)
		return nil
	})

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Test timeout
	err = WithTimeout(10*time.Millisecond, func(ctx context.Context) error {
		time.Sleep(50 * time.Millisecond)
		return nil
	})

	if err != context.DeadlineExceeded {
		t.Errorf("Expected timeout error, got %v", err)
	}
}

func TestTimeoutOperation(t *testing.T) {
	// Test successful operation
	result, err := TimeoutOperation(100*time.Millisecond, func() (interface{}, error) {
		time.Sleep(10 * time.Millisecond)
		return 42, nil
	})

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if result != 42 {
		t.Errorf("Expected 42, got %v", result)
	}

	// Test timeout
	_, err = TimeoutOperation(10*time.Millisecond, func() (interface{}, error) {
		time.Sleep(50 * time.Millisecond)
		return nil, nil
	})

	if err != context.DeadlineExceeded {
		t.Errorf("Expected timeout, got %v", err)
	}
}

func TestWithDeadline(t *testing.T) {
	deadline := time.Now().Add(50 * time.Millisecond)

	err := WithDeadline(deadline, func(ctx context.Context) error {
		time.Sleep(10 * time.Millisecond)
		return nil
	})

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Test past deadline
	pastDeadline := time.Now().Add(10 * time.Millisecond)
	err = WithDeadline(pastDeadline, func(ctx context.Context) error {
		time.Sleep(50 * time.Millisecond)
		return nil
	})

	if err == nil {
		t.Error("Expected deadline exceeded error")
	}
}

func TestDeadlineMonitor(t *testing.T) {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(100*time.Millisecond))
	defer cancel()

	warnings := DeadlineMonitor(ctx, 50*time.Millisecond)

	select {
	case warning := <-warnings:
		if warning < 0 || warning > 60*time.Millisecond {
			t.Errorf("Unexpected warning duration: %v", warning)
		}
	case <-time.After(150 * time.Millisecond):
		t.Error("Expected deadline warning")
	}
}

func TestContextValues(t *testing.T) {
	ctx := context.Background()

	// Test RequestID
	ctx = WithRequestID(ctx, "req-123")
	requestID, ok := GetRequestID(ctx)
	if !ok || requestID != "req-123" {
		t.Errorf("Expected req-123, got %s (ok=%v)", requestID, ok)
	}

	// Test UserID
	ctx = WithUserID(ctx, "user-456")
	userID, ok := GetUserID(ctx)
	if !ok || userID != "user-456" {
		t.Errorf("Expected user-456, got %s (ok=%v)", userID, ok)
	}

	// Test missing value
	emptyCtx := context.Background()
	_, ok = GetRequestID(emptyCtx)
	if ok {
		t.Error("Expected missing value")
	}
}

func TestCascadingContext(t *testing.T) {
	parent, parentCancel := context.WithCancel(context.Background())
	child, _ := CascadingContext(parent)

	// Cancel parent
	parentCancel()

	// Child should also be cancelled
	select {
	case <-child.Done():
		// Success
	case <-time.After(100 * time.Millisecond):
		t.Error("Child context should be cancelled when parent is cancelled")
	}
}

func TestContextAwarePipeline(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	in := make(chan int, 5)
	for i := 1; i <= 5; i++ {
		in <- i
	}
	close(in)

	double := func(n int) int { return n * 2 }
	out := ContextAwarePipeline(ctx, in, double)

	results := make([]int, 0)
	for v := range out {
		results = append(results, v)
	}

	expected := []int{2, 4, 6, 8, 10}
	if len(results) != len(expected) {
		t.Errorf("Expected %d results, got %d", len(expected), len(results))
	}

	for i, v := range results {
		if v != expected[i] {
			t.Errorf("Expected %d at index %d, got %d", expected[i], i, v)
		}
	}
}

func TestRetryWithTimeout(t *testing.T) {
	ctx := context.Background()

	// Test successful retry
	attempts := 0
	err := RetryWithTimeout(ctx, 3, 50*time.Millisecond, func() error {
		attempts++
		if attempts < 2 {
			return errors.New("temporary error")
		}
		return nil
	})

	if err != nil {
		t.Errorf("Expected success after retry, got %v", err)
	}

	if attempts != 2 {
		t.Errorf("Expected 2 attempts, got %d", attempts)
	}

	// Test max retries exceeded
	attempts = 0
	err = RetryWithTimeout(ctx, 3, 50*time.Millisecond, func() error {
		attempts++
		return errors.New("persistent error")
	})

	if err == nil {
		t.Error("Expected error after max retries")
	}

	if attempts != 3 {
		t.Errorf("Expected 3 attempts, got %d", attempts)
	}
}

func TestContextWorkerPool(t *testing.T) {
	ctx := context.Background()
	pool := NewContextWorkerPool(ctx, 3)

	processFunc := func(ctx context.Context, job Job) Result {
		select {
		case <-ctx.Done():
			return Result{JobID: job.ID, Error: ctx.Err()}
		default:
			return Result{JobID: job.ID, Value: job.Data.(int) * 2}
		}
	}

	pool.StartContext(processFunc)

	// Submit jobs
	go func() {
		for i := 0; i < 10; i++ {
			pool.SubmitContext(Job{ID: i, Data: i})
		}
		time.Sleep(100 * time.Millisecond)
		pool.Cancel()
	}()

	// Collect results
	count := 0
	for range pool.ResultsContext() {
		count++
	}

	if count != 10 {
		t.Logf("Processed %d jobs (expected 10)", count)
	}
}

func TestGracefulShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	cleanupDone := false
	cleanup := func() error {
		time.Sleep(20 * time.Millisecond)
		cleanupDone = true
		return nil
	}

	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	err := GracefulShutdown(ctx, cleanup, 100*time.Millisecond)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if !cleanupDone {
		t.Error("Cleanup should have been called")
	}
}

func TestGracefulShutdownTimeout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	cleanup := func() error {
		time.Sleep(200 * time.Millisecond)
		return nil
	}

	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	err := GracefulShutdown(ctx, cleanup, 50*time.Millisecond)

	if err == nil {
		t.Error("Expected timeout error")
	}
}

func TestMergeContexts(t *testing.T) {
	ctx1, cancel1 := context.WithCancel(context.Background())
	ctx2, cancel2 := context.WithCancel(context.Background())
	ctx3, cancel3 := context.WithCancel(context.Background())
	defer cancel2()
	defer cancel3()

	merged, mergedCancel := MergeContexts(ctx1, ctx2, ctx3)
	defer mergedCancel()

	// Cancel one context
	cancel1()

	// Merged should be cancelled
	select {
	case <-merged.Done():
		// Success
	case <-time.After(100 * time.Millisecond):
		t.Error("Merged context should be cancelled")
	}
}

func TestBackgroundTask(t *testing.T) {
	ctx := context.Background()
	task := NewBackgroundTask(ctx)

	taskCompleted := false
	task.Run(func(ctx context.Context) {
		select {
		case <-ctx.Done():
			return
		case <-time.After(50 * time.Millisecond):
			taskCompleted = true
		}
	})

	task.Stop()

	if taskCompleted {
		t.Error("Task should have been cancelled before completion")
	}
}

func TestBackgroundTaskWait(t *testing.T) {
	ctx := context.Background()
	task := NewBackgroundTask(ctx)

	taskCompleted := false
	task.Run(func(ctx context.Context) {
		time.Sleep(20 * time.Millisecond)
		taskCompleted = true
	})

	task.Wait()

	if !taskCompleted {
		t.Error("Task should have completed")
	}
}

func TestTimeoutChain(t *testing.T) {
	operations := []func(context.Context) error{
		func(ctx context.Context) error {
			time.Sleep(10 * time.Millisecond)
			return nil
		},
		func(ctx context.Context) error {
			time.Sleep(10 * time.Millisecond)
			return nil
		},
		func(ctx context.Context) error {
			time.Sleep(10 * time.Millisecond)
			return nil
		},
	}

	// Test successful chain
	err := TimeoutChain(200*time.Millisecond, operations, 50*time.Millisecond)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Test parent timeout
	err = TimeoutChain(20*time.Millisecond, operations, 50*time.Millisecond)
	if err == nil {
		t.Error("Expected parent timeout error")
	}
}

func TestLogContext(t *testing.T) {
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()

	ctx = WithRequestID(ctx, "req-789")
	ctx = WithUserID(ctx, "user-123")

	// Just verify it doesn't panic
	LogContext(ctx)
}
