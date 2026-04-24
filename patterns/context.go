/*
Context Patterns
================

Context-based cancellation, timeouts, and value propagation patterns.

Applications:
- Request cancellation
- Deadline enforcement
- Timeout handling
- Request-scoped values
*/

package patterns

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// =============================================================================
// Context Cancellation Patterns
// =============================================================================

// CancellableWork demonstrates context cancellation
func CancellableWork(ctx context.Context, work func() error) error {
	done := make(chan error, 1)

	go func() {
		done <- work()
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-done:
		return err
	}
}

// =============================================================================
// Timeout Patterns
// =============================================================================

// WithTimeout executes function with timeout.
// The timeout is enforced externally — fn need not check ctx.Done() itself.
func WithTimeout(timeout time.Duration, fn func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- fn(ctx) }()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-done:
		return err
	}
}

// TimeoutOperation demonstrates timeout handling
func TimeoutOperation(timeout time.Duration, operation func() (interface{}, error)) (interface{}, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	resultChan := make(chan interface{}, 1)
	errChan := make(chan error, 1)

	go func() {
		result, err := operation()
		if err != nil {
			errChan <- err
			return
		}
		resultChan <- result
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case err := <-errChan:
		return nil, err
	case result := <-resultChan:
		return result, nil
	}
}

// =============================================================================
// Deadline Patterns
// =============================================================================

// WithDeadline executes function with deadline.
// The deadline is enforced externally — fn need not check ctx.Done() itself.
func WithDeadline(deadline time.Time, fn func(context.Context) error) error {
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- fn(ctx) }()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-done:
		return err
	}
}

// DeadlineMonitor monitors if deadline is approaching
func DeadlineMonitor(ctx context.Context, warningDuration time.Duration) <-chan time.Duration {
	warnings := make(chan time.Duration)

	go func() {
		defer close(warnings)

		deadline, ok := ctx.Deadline()
		if !ok {
			return
		}

		timeUntilDeadline := time.Until(deadline)
		if timeUntilDeadline <= warningDuration {
			select {
			case warnings <- timeUntilDeadline:
			case <-ctx.Done():
			}
			return
		}

		warningTime := timeUntilDeadline - warningDuration
		select {
		case <-time.After(warningTime):
			select {
			case warnings <- warningDuration:
			case <-ctx.Done():
			}
		case <-ctx.Done():
		}
	}()

	return warnings
}

// =============================================================================
// Value Propagation Patterns
// =============================================================================

// ContextKey type for context values
type ContextKey string

const (
	// RequestIDKey for request ID
	RequestIDKey ContextKey = "request-id"
	// UserIDKey for user ID
	UserIDKey ContextKey = "user-id"
	// TraceIDKey for trace ID
	TraceIDKey ContextKey = "trace-id"
)

// WithRequestID adds request ID to context
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, RequestIDKey, requestID)
}

// GetRequestID retrieves request ID from context
func GetRequestID(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(RequestIDKey).(string)
	return id, ok
}

// WithUserID adds user ID to context
func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, UserIDKey, userID)
}

// GetUserID retrieves user ID from context
func GetUserID(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(UserIDKey).(string)
	return id, ok
}

// =============================================================================
// Cascading Cancellation
// =============================================================================

// CascadingContext creates child contexts that cancel together
func CascadingContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithCancel(parent)
}

// MultiLevelCancellation demonstrates nested context cancellation
func MultiLevelCancellation() {
	// Root context
	root, rootCancel := context.WithCancel(context.Background())
	defer rootCancel()

	// Level 1 context
	level1, level1Cancel := context.WithCancel(root)
	defer level1Cancel()

	// Level 2 context
	level2, level2Cancel := context.WithCancel(level1)
	defer level2Cancel()

	// Cancelling root cancels all children
	_ = level2
}

// =============================================================================
// Context Propagation in Pipelines
// =============================================================================

// ContextAwarePipeline creates pipeline that respects context
func ContextAwarePipeline(ctx context.Context, in <-chan int, fn func(int) int) <-chan int {
	out := make(chan int)

	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case v, ok := <-in:
				if !ok {
					return
				}
				result := fn(v)
				select {
				case <-ctx.Done():
					return
				case out <- result:
				}
			}
		}
	}()

	return out
}

// =============================================================================
// Timeout with Retry
// =============================================================================

// RetryWithTimeout retries operation with timeout
func RetryWithTimeout(ctx context.Context, maxRetries int, timeout time.Duration, operation func() error) error {
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		// Create timeout context for this attempt
		attemptCtx, cancel := context.WithTimeout(ctx, timeout)

		// Try operation
		errChan := make(chan error, 1)
		go func() {
			errChan <- operation()
		}()

		select {
		case <-attemptCtx.Done():
			cancel()
			lastErr = attemptCtx.Err()
			if ctx.Err() != nil {
				// Parent context cancelled
				return ctx.Err()
			}
			// Timeout on this attempt, retry
			continue
		case err := <-errChan:
			cancel()
			if err == nil {
				return nil
			}
			lastErr = err
		}
	}

	return fmt.Errorf("operation failed after %d retries: %w", maxRetries, lastErr)
}

// =============================================================================
// Context-Aware Worker Pool
// =============================================================================

// ContextWorkerPool is a worker pool that respects context
type ContextWorkerPool struct {
	ctx        context.Context
	numWorkers int
	jobs       chan Job
	results    chan Result
	cancel     context.CancelFunc
}

// NewContextWorkerPool creates context-aware worker pool
func NewContextWorkerPool(ctx context.Context, numWorkers int) *ContextWorkerPool {
	childCtx, cancel := context.WithCancel(ctx)

	return &ContextWorkerPool{
		ctx:        childCtx,
		numWorkers: numWorkers,
		jobs:       make(chan Job, numWorkers*2),
		results:    make(chan Result, numWorkers*2),
		cancel:     cancel,
	}
}

// StartContext starts workers with context awareness.
// The results channel is closed automatically once all workers have exited,
// so callers can range over ResultsContext() safely.
func (cwp *ContextWorkerPool) StartContext(processFunc func(context.Context, Job) Result) {
	var wg sync.WaitGroup
	for i := 0; i < cwp.numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cwp.contextWorker(processFunc)
		}()
	}
	go func() {
		wg.Wait()
		close(cwp.results)
	}()
}

func (cwp *ContextWorkerPool) contextWorker(processFunc func(context.Context, Job) Result) {
	for {
		select {
		case <-cwp.ctx.Done():
			return
		case job, ok := <-cwp.jobs:
			if !ok {
				return
			}
			result := processFunc(cwp.ctx, job)
			select {
			case <-cwp.ctx.Done():
				return
			case cwp.results <- result:
			}
		}
	}
}

// SubmitContext submits job with context check
func (cwp *ContextWorkerPool) SubmitContext(job Job) error {
	select {
	case <-cwp.ctx.Done():
		return cwp.ctx.Err()
	case cwp.jobs <- job:
		return nil
	}
}

// ResultsContext returns results channel
func (cwp *ContextWorkerPool) ResultsContext() <-chan Result {
	return cwp.results
}

// Cancel cancels all workers
func (cwp *ContextWorkerPool) Cancel() {
	cwp.cancel()
}

// =============================================================================
// Context Merge Pattern
// =============================================================================

// MergeContexts merges multiple contexts - cancels when any cancels
func MergeContexts(ctxs ...context.Context) (context.Context, context.CancelFunc) {
	merged, cancel := context.WithCancel(context.Background())

	for _, c := range ctxs {
		go func(c context.Context) {
			select {
			case <-c.Done():
				cancel()
			case <-merged.Done():
				return
			}
		}(c)
	}

	return merged, cancel
}

// =============================================================================
// Graceful Shutdown Pattern
// =============================================================================

// GracefulShutdown handles graceful shutdown with context
func GracefulShutdown(ctx context.Context, cleanup func() error, shutdownTimeout time.Duration) error {
	<-ctx.Done()

	// Create timeout for cleanup
	cleanupCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	cleanupDone := make(chan error, 1)
	go func() {
		cleanupDone <- cleanup()
	}()

	select {
	case err := <-cleanupDone:
		return err
	case <-cleanupCtx.Done():
		return errors.New("cleanup timeout exceeded")
	}
}

// =============================================================================
// Context Logging
// =============================================================================

// LogContext logs context state
func LogContext(ctx context.Context) {
	deadline, hasDeadline := ctx.Deadline()
	if hasDeadline {
		fmt.Printf("Context deadline: %v (in %v)\n", deadline, time.Until(deadline))
	} else {
		fmt.Println("Context has no deadline")
	}

	if requestID, ok := GetRequestID(ctx); ok {
		fmt.Printf("Request ID: %s\n", requestID)
	}

	if userID, ok := GetUserID(ctx); ok {
		fmt.Printf("User ID: %s\n", userID)
	}
}

// =============================================================================
// Context-Aware Select
// =============================================================================

// ContextSelect performs select with context awareness
func ContextSelect(ctx context.Context, channels []<-chan int) (int, error) {
	for _, ch := range channels {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case v, ok := <-ch:
			if ok {
				return v, nil
			}
		}
	}
	return 0, errors.New("no channels have data")
}

// =============================================================================
// Background Task with Context
// =============================================================================

// BackgroundTask runs a task in background with context
type BackgroundTask struct {
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
}

// NewBackgroundTask creates a background task
func NewBackgroundTask(ctx context.Context) *BackgroundTask {
	childCtx, cancel := context.WithCancel(ctx)
	return &BackgroundTask{
		ctx:    childCtx,
		cancel: cancel,
		done:   make(chan struct{}),
	}
}

// Run runs the task
func (bt *BackgroundTask) Run(task func(context.Context)) {
	go func() {
		defer close(bt.done)
		task(bt.ctx)
	}()
}

// Stop stops the task
func (bt *BackgroundTask) Stop() {
	bt.cancel()
	<-bt.done
}

// Wait waits for task completion
func (bt *BackgroundTask) Wait() {
	<-bt.done
}

// =============================================================================
// Context Timeout Chain
// =============================================================================

// TimeoutChain creates a chain of operations with individual timeouts
func TimeoutChain(parentTimeout time.Duration, operations []func(context.Context) error, individualTimeout time.Duration) error {
	parentCtx, cancel := context.WithTimeout(context.Background(), parentTimeout)
	defer cancel()

	for i, op := range operations {
		opCtx, opCancel := context.WithTimeout(parentCtx, individualTimeout)

		err := op(opCtx)
		opCancel()

		if err != nil {
			return fmt.Errorf("operation %d failed: %w", i, err)
		}

		// Check if parent context is cancelled
		select {
		case <-parentCtx.Done():
			return fmt.Errorf("parent timeout exceeded")
		default:
		}
	}

	return nil
}
