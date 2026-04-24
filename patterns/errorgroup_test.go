package patterns

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestErrorGroup(t *testing.T) {
	eg := NewErrorGroup()

	// All successful
	for i := 0; i < 5; i++ {
		i := i
		eg.Go(func() error {
			time.Sleep(time.Duration(i) * 10 * time.Millisecond)
			return nil
		})
	}

	err := eg.Wait()
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
}

func TestErrorGroupWithError(t *testing.T) {
	eg := NewErrorGroup()

	expectedErr := errors.New("task failed")

	eg.Go(func() error {
		return nil
	})

	eg.Go(func() error {
		return expectedErr
	})

	eg.Go(func() error {
		time.Sleep(50 * time.Millisecond)
		return nil
	})

	err := eg.Wait()
	if err == nil {
		t.Error("Expected error")
	}
}

func TestContextErrorGroup(t *testing.T) {
	ctx := context.Background()
	ceg, _ := WithContext(ctx)

	for i := 0; i < 5; i++ {
		ceg.Go(func(ctx context.Context) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(10 * time.Millisecond):
				return nil
			}
		})
	}

	err := ceg.Wait()
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
}

func TestContextErrorGroupCancellation(t *testing.T) {
	ctx := context.Background()
	ceg, egCtx := WithContext(ctx)

	ceg.Go(func(ctx context.Context) error {
		return errors.New("first error")
	})

	ceg.Go(func(ctx context.Context) error {
		time.Sleep(50 * time.Millisecond)
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	})

	err := ceg.Wait()
	if err == nil {
		t.Error("Expected error")
	}

	// Context should be cancelled
	select {
	case <-egCtx.Done():
		// Success
	default:
		t.Error("Context should be cancelled")
	}
}

func TestMultiErrorGroup(t *testing.T) {
	meg := NewMultiErrorGroup()

	meg.Go(func() error {
		return errors.New("error 1")
	})

	meg.Go(func() error {
		return nil
	})

	meg.Go(func() error {
		return errors.New("error 2")
	})

	errors := meg.Wait()

	if len(errors) != 2 {
		t.Errorf("Expected 2 errors, got %d", len(errors))
	}
}

func TestMultiErrorGroupError(t *testing.T) {
	meg := NewMultiErrorGroup()

	meg.Go(func() error {
		return errors.New("error 1")
	})

	meg.Go(func() error {
		return errors.New("error 2")
	})

	err := meg.Error()
	if err == nil {
		t.Error("Expected combined error")
	}
}

func TestLimitedErrorGroup(t *testing.T) {
	leg := NewLimitedErrorGroup(3)

	var mu sync.Mutex
	running := 0
	maxRunning := 0

	for i := 0; i < 10; i++ {
		leg.Go(func() error {
			mu.Lock()
			running++
			if running > maxRunning {
				maxRunning = running
			}
			mu.Unlock()
			time.Sleep(20 * time.Millisecond)
			mu.Lock()
			running--
			mu.Unlock()
			return nil
		})
	}

	leg.Wait()

	if maxRunning > 3 {
		t.Errorf("Expected max 3 concurrent, got %d", maxRunning)
	}
}

func TestLimitedErrorGroupWithError(t *testing.T) {
	leg := NewLimitedErrorGroup(2)

	leg.Go(func() error {
		return errors.New("task error")
	})

	leg.Go(func() error {
		time.Sleep(30 * time.Millisecond)
		return nil
	})

	err := leg.Wait()
	if err == nil {
		t.Error("Expected error")
	}
}

func TestTypedErrorGroup(t *testing.T) {
	teg := NewTypedErrorGroup()

	teg.Go("network", func() error {
		return errors.New("network error")
	})

	teg.Go("database", func() error {
		return errors.New("database error")
	})

	teg.Go("network", func() error {
		return errors.New("another network error")
	})

	errorMap := teg.Wait()

	if len(errorMap["network"]) != 2 {
		t.Errorf("Expected 2 network errors, got %d", len(errorMap["network"]))
	}

	if len(errorMap["database"]) != 1 {
		t.Errorf("Expected 1 database error, got %d", len(errorMap["database"]))
	}
}

func TestTypedErrorGroupHasErrors(t *testing.T) {
	teg := NewTypedErrorGroup()

	if teg.HasErrors() {
		t.Error("Should not have errors initially")
	}

	teg.Go("test", func() error {
		return errors.New("error")
	})

	teg.Wait()

	if !teg.HasErrors() {
		t.Error("Should have errors")
	}
}

func TestPipelineErrorGroup(t *testing.T) {
	peg := NewPipelineErrorGroup(context.Background())

	executed := make([]int, 0)

	peg.AddStage("stage1", func(ctx context.Context) error {
		executed = append(executed, 1)
		return nil
	})

	peg.AddStage("stage2", func(ctx context.Context) error {
		executed = append(executed, 2)
		return nil
	})

	peg.AddStage("stage3", func(ctx context.Context) error {
		executed = append(executed, 3)
		return nil
	})

	err := peg.Execute()
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if len(executed) != 3 {
		t.Errorf("Expected 3 stages executed, got %d", len(executed))
	}
}

func TestPipelineErrorGroupFailure(t *testing.T) {
	peg := NewPipelineErrorGroup(context.Background())

	executed := make([]int, 0)

	peg.AddStage("stage1", func(ctx context.Context) error {
		executed = append(executed, 1)
		return nil
	})

	peg.AddStage("stage2", func(ctx context.Context) error {
		return errors.New("stage2 failed")
	})

	peg.AddStage("stage3", func(ctx context.Context) error {
		executed = append(executed, 3)
		return nil
	})

	err := peg.Execute()
	if err == nil {
		t.Error("Expected error from stage2")
	}

	// Stage 3 should not execute
	if len(executed) != 1 {
		t.Errorf("Expected only 1 stage executed, got %d", len(executed))
	}
}

func TestResultErrorGroup(t *testing.T) {
	reg := NewResultErrorGroup()

	for i := 0; i < 5; i++ {
		i := i
		reg.Go(i, func() (interface{}, error) {
			return i * 2, nil
		})
	}

	results := reg.Wait()

	if len(results) != 5 {
		t.Errorf("Expected 5 results, got %d", len(results))
	}

	for _, result := range results {
		if result.Error != nil {
			t.Errorf("Unexpected error: %v", result.Error)
		}
	}
}

func TestResultErrorGroupGetSuccessful(t *testing.T) {
	reg := NewResultErrorGroup()

	reg.Go(0, func() (interface{}, error) {
		return 1, nil
	})

	reg.Go(1, func() (interface{}, error) {
		return nil, errors.New("error")
	})

	reg.Go(2, func() (interface{}, error) {
		return 3, nil
	})

	successful := reg.GetSuccessful()

	if len(successful) != 2 {
		t.Errorf("Expected 2 successful results, got %d", len(successful))
	}
}

func TestResultErrorGroupGetErrors(t *testing.T) {
	reg := NewResultErrorGroup()

	reg.Go(0, func() (interface{}, error) {
		return 1, nil
	})

	reg.Go(1, func() (interface{}, error) {
		return nil, errors.New("error 1")
	})

	reg.Go(2, func() (interface{}, error) {
		return nil, errors.New("error 2")
	})

	errorResults := reg.GetErrors()

	if len(errorResults) != 2 {
		t.Errorf("Expected 2 error results, got %d", len(errorResults))
	}
}

func TestRetryErrorGroup(t *testing.T) {
	reg := NewRetryErrorGroup(2)

	attempts := 0
	reg.Go(func() error {
		attempts++
		if attempts < 2 {
			return errors.New("temporary error")
		}
		return nil
	})

	err := reg.Wait()
	if err != nil {
		t.Errorf("Expected success after retry, got %v", err)
	}

	if attempts != 2 {
		t.Errorf("Expected 2 attempts, got %d", attempts)
	}
}

func TestRetryErrorGroupMaxRetries(t *testing.T) {
	reg := NewRetryErrorGroup(3)

	attempts := 0
	reg.Go(func() error {
		attempts++
		return errors.New("persistent error")
	})

	err := reg.Wait()
	if err == nil {
		t.Error("Expected error after max retries")
	}

	if attempts != 4 { // Initial + 3 retries
		t.Errorf("Expected 4 attempts, got %d", attempts)
	}
}

func TestBatchErrorGroup(t *testing.T) {
	beg := NewBatchErrorGroup(3)

	items := make([]interface{}, 10)
	for i := 0; i < 10; i++ {
		items[i] = i
	}

	var mu sync.Mutex
	processed := make([]int, 0)
	errs := beg.Process(items, func(item interface{}) error {
		mu.Lock()
		processed = append(processed, item.(int))
		mu.Unlock()
		return nil
	})

	if len(errs) > 0 {
		t.Errorf("Expected no errors, got %d", len(errs))
	}

	if len(processed) != 10 {
		t.Errorf("Expected 10 items processed, got %d", len(processed))
	}
}

func TestHierarchicalErrorGroup(t *testing.T) {
	root := NewHierarchicalErrorGroup("root")

	child1 := root.CreateChild("child1")
	child2 := root.CreateChild("child2")

	var mu sync.Mutex
	executed := make([]string, 0)

	root.Go(func() error {
		mu.Lock()
		executed = append(executed, "root")
		mu.Unlock()
		return nil
	})

	child1.Go(func() error {
		mu.Lock()
		executed = append(executed, "child1")
		mu.Unlock()
		return nil
	})

	child2.Go(func() error {
		mu.Lock()
		executed = append(executed, "child2")
		mu.Unlock()
		return nil
	})

	err := root.Wait()
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	mu.Lock()
	n := len(executed)
	mu.Unlock()
	if n != 3 {
		t.Errorf("Expected 3 tasks executed, got %d", n)
	}
}

func TestHierarchicalErrorGroupChildFailure(t *testing.T) {
	root := NewHierarchicalErrorGroup("root")
	child := root.CreateChild("child")

	child.Go(func() error {
		return errors.New("child error")
	})

	err := root.Wait()
	if err == nil {
		t.Error("Expected error from child")
	}
}

func TestParallelExecute(t *testing.T) {
	var mu sync.Mutex
	executed := make([]int, 0)

	err := ParallelExecute(
		func() error {
			mu.Lock()
			executed = append(executed, 1)
			mu.Unlock()
			return nil
		},
		func() error {
			mu.Lock()
			executed = append(executed, 2)
			mu.Unlock()
			return nil
		},
		func() error {
			mu.Lock()
			executed = append(executed, 3)
			mu.Unlock()
			return nil
		},
	)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if len(executed) != 3 {
		t.Errorf("Expected 3 functions executed, got %d", len(executed))
	}
}

func TestParallelExecuteWithContext(t *testing.T) {
	ctx := context.Background()

	err := ParallelExecuteWithContext(ctx,
		func(ctx context.Context) error {
			return nil
		},
		func(ctx context.Context) error {
			return nil
		},
	)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
}

func TestParallelMap(t *testing.T) {
	items := []interface{}{1, 2, 3, 4, 5}
	var mu sync.Mutex
	results := make([]int, 0)

	err := ParallelMap(items, func(item interface{}) error {
		mu.Lock()
		results = append(results, item.(int)*2)
		mu.Unlock()
		return nil
	})

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if len(results) != 5 {
		t.Errorf("Expected 5 results, got %d", len(results))
	}
}

func TestParallelMapWithLimit(t *testing.T) {
	items := make([]interface{}, 20)
	for i := 0; i < 20; i++ {
		items[i] = i
	}

	var mu sync.Mutex
	running := 0
	maxRunning := 0

	err := ParallelMapWithLimit(items, 3, func(item interface{}) error {
		mu.Lock()
		running++
		if running > maxRunning {
			maxRunning = running
		}
		mu.Unlock()
		time.Sleep(10 * time.Millisecond)
		mu.Lock()
		running--
		mu.Unlock()
		return nil
	})

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if maxRunning > 3 {
		t.Errorf("Expected max 3 concurrent, got %d", maxRunning)
	}
}
