/*
Concurrency Patterns Examples
==============================

Comprehensive examples demonstrating all Go concurrency patterns.
*/

package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/sanskarpan/Operating-Systems/patterns"
)

func main() {
	fmt.Println(strings.Repeat("=", 70))
	fmt.Println("Go Concurrency Patterns - Comprehensive Examples")
	fmt.Println(strings.Repeat("=", 70))

	// Channel Patterns
	demoChannelPatterns()

	// Worker Pool Patterns
	demoWorkerPoolPatterns()

	// Context Patterns
	demoContextPatterns()

	// Rate Limiting Patterns
	demoRateLimitingPatterns()

	// Circuit Breaker Patterns
	demoCircuitBreakerPatterns()

	// Error Group Patterns
	demoErrorGroupPatterns()

	fmt.Println(strings.Repeat("=", 70))
	fmt.Println("All demonstrations completed successfully!")
	fmt.Println(strings.Repeat("=", 70))
}

// =============================================================================
// Channel Patterns Demonstration
// =============================================================================

func demoChannelPatterns() {
	fmt.Println("\n" + strings.Repeat("-", 70))
	fmt.Println("CHANNEL PATTERNS")
	fmt.Println(strings.Repeat("-", 70))

	// Generator Pattern
	fmt.Println("\n1. Generator Pattern:")
	done := make(chan struct{})
	values := patterns.Generator(done, 1, 2, 3, 4, 5)
	count := 0
	for v := range values {
		fmt.Printf("  Generated: %d\n", v)
		count++
		if count == 3 {
			close(done)
			break
		}
	}

	// Pipeline Pattern
	fmt.Println("\n2. Pipeline Pattern (double numbers):")
	done2 := make(chan struct{})
	defer close(done2)
	in := patterns.Generator(done2, 1, 2, 3, 4, 5)
	out := patterns.Pipeline(done2, in, func(n int) int { return n * 2 })
	for v := range out {
		fmt.Printf("  Processed: %d\n", v)
	}

	// Fan-Out/Fan-In Pattern
	fmt.Println("\n3. Fan-Out/Fan-In Pattern (3 workers):")
	done3 := make(chan struct{})
	defer close(done3)
	in3 := patterns.Generator(done3, 1, 2, 3, 4, 5, 6)
	workers := patterns.FanOut(done3, in3, 3, func(n int) int { return n * n })
	results := patterns.FanIn(done3, workers...)
	fmt.Print("  Results: ")
	for v := range results {
		fmt.Printf("%d ", v)
	}
	fmt.Println()

	// Batch Pattern
	fmt.Println("\n4. Batch Pattern (batch size 3):")
	done4 := make(chan struct{})
	defer close(done4)
	in4 := patterns.Generator(done4, 1, 2, 3, 4, 5, 6, 7, 8)
	batches := patterns.Batch(done4, in4, 3)
	batchNum := 1
	for batch := range batches {
		fmt.Printf("  Batch %d: %v\n", batchNum, batch)
		batchNum++
	}
}

// =============================================================================
// Worker Pool Patterns Demonstration
// =============================================================================

func demoWorkerPoolPatterns() {
	fmt.Println("\n" + strings.Repeat("-", 70))
	fmt.Println("WORKER POOL PATTERNS")
	fmt.Println(strings.Repeat("-", 70))

	// Fixed Worker Pool
	fmt.Println("\n1. Fixed Worker Pool (3 workers, 10 jobs):")
	pool := patterns.NewWorkerPool(3)
	processFunc := func(job patterns.Job) patterns.Result {
		time.Sleep(10 * time.Millisecond)
		return patterns.Result{
			JobID: job.ID,
			Value: job.Data.(int) * 2,
		}
	}
	pool.Start(processFunc)

	go func() {
		for i := 0; i < 10; i++ {
			pool.Submit(patterns.Job{ID: i, Data: i})
		}
		pool.Close()
	}()

	fmt.Print("  Results: ")
	for result := range pool.Results() {
		fmt.Printf("[%d:%v] ", result.JobID, result.Value)
	}
	fmt.Println()

	// Bounded Worker Pool
	fmt.Println("\n2. Bounded Worker Pool (max 2 concurrent):")
	bounded := patterns.NewBoundedWorkerPool(2)
	for i := 0; i < 5; i++ {
		i := i
		bounded.Execute(func() {
			fmt.Printf("  Task %d executing\n", i)
			time.Sleep(50 * time.Millisecond)
		})
	}
	bounded.Wait()
	fmt.Println("  All tasks completed")

	// Task Queue
	fmt.Println("\n3. Task Queue (3 workers):")
	tq := patterns.NewTaskQueue(3, 10)
	for i := 0; i < 5; i++ {
		i := i
		tq.Submit(func() {
			fmt.Printf("  Executing task %d\n", i)
		})
	}
	tq.Stop()
	fmt.Println("  Task queue stopped")
}

// =============================================================================
// Context Patterns Demonstration
// =============================================================================

func demoContextPatterns() {
	fmt.Println("\n" + strings.Repeat("-", 70))
	fmt.Println("CONTEXT PATTERNS")
	fmt.Println(strings.Repeat("-", 70))

	// Cancellable Work
	fmt.Println("\n1. Cancellable Work:")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := patterns.CancellableWork(ctx, func() error {
		time.Sleep(10 * time.Millisecond)
		fmt.Println("  Work completed successfully")
		return nil
	})
	if err != nil {
		fmt.Printf("  Error: %v\n", err)
	}

	// WithTimeout
	fmt.Println("\n2. Timeout Pattern:")
	err = patterns.WithTimeout(100*time.Millisecond, func(ctx context.Context) error {
		time.Sleep(10 * time.Millisecond)
		fmt.Println("  Operation completed within timeout")
		return nil
	})
	if err != nil {
		fmt.Printf("  Error: %v\n", err)
	}

	// Context Values
	fmt.Println("\n3. Context Value Propagation:")
	ctx2 := context.Background()
	ctx2 = patterns.WithRequestID(ctx2, "req-12345")
	ctx2 = patterns.WithUserID(ctx2, "user-789")

	if reqID, ok := patterns.GetRequestID(ctx2); ok {
		fmt.Printf("  Request ID: %s\n", reqID)
	}
	if userID, ok := patterns.GetUserID(ctx2); ok {
		fmt.Printf("  User ID: %s\n", userID)
	}

	// Retry with Timeout
	fmt.Println("\n4. Retry with Timeout:")
	attempts := 0
	err = patterns.RetryWithTimeout(context.Background(), 3, 50*time.Millisecond, func() error {
		attempts++
		if attempts < 2 {
			return errors.New("temporary error")
		}
		return nil
	})
	fmt.Printf("  Succeeded after %d attempts\n", attempts)

	// Graceful Shutdown
	fmt.Println("\n5. Graceful Shutdown:")
	ctx3, cancel3 := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel3()
	}()

	err = patterns.GracefulShutdown(ctx3, func() error {
		fmt.Println("  Cleanup completed")
		return nil
	}, 100*time.Millisecond)
	if err != nil {
		fmt.Printf("  Shutdown error: %v\n", err)
	}
}

// =============================================================================
// Rate Limiting Patterns Demonstration
// =============================================================================

func demoRateLimitingPatterns() {
	fmt.Println("\n" + strings.Repeat("-", 70))
	fmt.Println("RATE LIMITING PATTERNS")
	fmt.Println(strings.Repeat("-", 70))

	// Token Bucket
	fmt.Println("\n1. Token Bucket (5 tokens, 10/sec refill):")
	tb := patterns.NewTokenBucket(5, 10)
	allowed := 0
	denied := 0
	for i := 0; i < 10; i++ {
		if tb.Allow() {
			allowed++
		} else {
			denied++
		}
	}
	fmt.Printf("  Allowed: %d, Denied: %d\n", allowed, denied)

	// Fixed Window Counter
	fmt.Println("\n2. Fixed Window Counter (5 req/100ms):")
	fwc := patterns.NewFixedWindowCounter(5, 100*time.Millisecond)
	allowed = 0
	for i := 0; i < 8; i++ {
		if fwc.Allow() {
			allowed++
		}
	}
	fmt.Printf("  Allowed in window: %d\n", allowed)

	// Concurrent Limiter
	fmt.Println("\n3. Concurrent Request Limiter (max 3):")
	cl := patterns.NewConcurrentLimiter(3)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		err := cl.Acquire(ctx)
		if err != nil {
			fmt.Printf("  Acquire %d failed\n", i)
		} else {
			fmt.Printf("  Acquired slot %d\n", i+1)
		}
	}

	if !cl.TryAcquire() {
		fmt.Println("  Fourth request denied (limit reached)")
	}

	cl.Release()
	fmt.Printf("  Released slot, available: %d\n", cl.Available())

	// Per-User Rate Limiter
	fmt.Println("\n4. Per-User Rate Limiter:")
	purl := patterns.NewPerUserRateLimiter(3, 10)

	user1Allowed := 0
	for i := 0; i < 5; i++ {
		if purl.Allow("user1") {
			user1Allowed++
		}
	}
	fmt.Printf("  User1 allowed: %d/5 requests\n", user1Allowed)

	if purl.Allow("user2") {
		fmt.Println("  User2: First request allowed (separate limit)")
	}
}

// =============================================================================
// Circuit Breaker Patterns Demonstration
// =============================================================================

func demoCircuitBreakerPatterns() {
	fmt.Println("\n" + strings.Repeat("-", 70))
	fmt.Println("CIRCUIT BREAKER PATTERNS")
	fmt.Println(strings.Repeat("-", 70))

	// Basic Circuit Breaker
	fmt.Println("\n1. Basic Circuit Breaker:")
	cb := patterns.NewCircuitBreaker(3, 200*time.Millisecond, 100*time.Millisecond)

	// Successful operations
	fmt.Println("  Executing successful operations:")
	for i := 0; i < 3; i++ {
		err := cb.Execute(func() error {
			return nil
		})
		if err != nil {
			fmt.Printf("    Operation %d failed: %v\n", i, err)
		} else {
			fmt.Printf("    Operation %d succeeded\n", i)
		}
	}
	fmt.Printf("  Circuit state: %s\n", cb.State())

	// Failed operations
	fmt.Println("\n  Executing failed operations:")
	for i := 0; i < 3; i++ {
		cb.Execute(func() error {
			return errors.New("service unavailable")
		})
	}
	fmt.Printf("  Circuit state after failures: %s\n", cb.State())

	// Try to execute when circuit is open
	err := cb.Execute(func() error {
		return nil
	})
	if err == patterns.ErrCircuitOpen {
		fmt.Println("  Request denied: Circuit is OPEN")
	}

	// Advanced Circuit Breaker
	fmt.Println("\n2. Advanced Circuit Breaker with Metrics:")
	acb := patterns.NewAdvancedCircuitBreaker(2, 2, 100*time.Millisecond, 50*time.Millisecond)

	acb.Call(func() error { return nil })
	acb.Call(func() error { return errors.New("error") })
	acb.Call(func() error { return nil })

	metrics := acb.GetMetrics()
	fmt.Printf("  Requests: %d, Successes: %d, Failures: %d\n",
		metrics.TotalRequests, metrics.TotalSuccesses, metrics.TotalFailures)
	fmt.Printf("  State: %s\n", metrics.State)
}

// =============================================================================
// Error Group Patterns Demonstration
// =============================================================================

func demoErrorGroupPatterns() {
	fmt.Println("\n" + strings.Repeat("-", 70))
	fmt.Println("ERROR GROUP PATTERNS")
	fmt.Println(strings.Repeat("-", 70))

	// Basic Error Group
	fmt.Println("\n1. Basic Error Group:")
	eg := patterns.NewErrorGroup()

	eg.Go(func() error {
		fmt.Println("  Task 1 executing")
		return nil
	})

	eg.Go(func() error {
		fmt.Println("  Task 2 executing")
		return nil
	})

	err := eg.Wait()
	if err == nil {
		fmt.Println("  All tasks completed successfully")
	}

	// Context Error Group
	fmt.Println("\n2. Context Error Group (with cancellation):")
	ctx := context.Background()
	ceg, egCtx := patterns.WithContext(ctx)

	ceg.Go(func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			fmt.Println("  Task cancelled")
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
			fmt.Println("  Task completed")
			return nil
		}
	})

	err = ceg.Wait()
	fmt.Printf("  Error group completed: %v\n", err)
	fmt.Printf("  Context done: %v\n", egCtx.Err())

	// Multi-Error Group
	fmt.Println("\n3. Multi-Error Group (collects all errors):")
	meg := patterns.NewMultiErrorGroup()

	meg.Go(func() error {
		return errors.New("error 1")
	})

	meg.Go(func() error {
		return nil
	})

	meg.Go(func() error {
		return errors.New("error 2")
	})

	errs := meg.Wait()
	fmt.Printf("  Collected %d errors\n", len(errs))
	for i, err := range errs {
		fmt.Printf("    %d. %v\n", i+1, err)
	}

	// Limited Error Group
	fmt.Println("\n4. Limited Error Group (max 2 concurrent):")
	leg := patterns.NewLimitedErrorGroup(2)

	for i := 0; i < 5; i++ {
		i := i
		leg.Go(func() error {
			fmt.Printf("  Task %d executing\n", i)
			time.Sleep(20 * time.Millisecond)
			return nil
		})
	}

	err = leg.Wait()
	if err == nil {
		fmt.Println("  All tasks completed with concurrency limit")
	}

	// Result Error Group
	fmt.Println("\n5. Result Error Group:")
	reg := patterns.NewResultErrorGroup()

	for i := 0; i < 3; i++ {
		i := i
		reg.Go(i, func() (interface{}, error) {
			return i * 2, nil
		})
	}

	results := reg.Wait()
	fmt.Printf("  Collected %d results:\n", len(results))
	for _, result := range results {
		fmt.Printf("    Index %d: %v\n", result.Index, result.Value)
	}

	// Parallel Helper Functions
	fmt.Println("\n6. Parallel Execute Helper:")
	err = patterns.ParallelExecute(
		func() error {
			fmt.Println("  Function 1 executed")
			return nil
		},
		func() error {
			fmt.Println("  Function 2 executed")
			return nil
		},
		func() error {
			fmt.Println("  Function 3 executed")
			return nil
		},
	)
	if err == nil {
		fmt.Println("  All functions executed successfully")
	}
}
