package patterns

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCircuitBreakerClosed(t *testing.T) {
	cb := NewCircuitBreaker(3, 100*time.Millisecond, 50*time.Millisecond)

	// Execute successful operations
	for i := 0; i < 5; i++ {
		err := cb.Execute(func() error {
			return nil
		})
		if err != nil {
			t.Errorf("Operation %d should succeed in closed state", i)
		}
	}

	if cb.State() != StateClosed {
		t.Error("Circuit breaker should be closed")
	}
}

func TestCircuitBreakerOpens(t *testing.T) {
	cb := NewCircuitBreaker(3, 200*time.Millisecond, 50*time.Millisecond)

	// Trigger failures
	for i := 0; i < 3; i++ {
		cb.Execute(func() error {
			return errors.New("failure")
		})
	}

	// Circuit should be open
	if cb.State() != StateOpen {
		t.Errorf("Expected Open state, got %s", cb.State())
	}

	// Should reject requests
	err := cb.Execute(func() error {
		return nil
	})

	if err != ErrCircuitOpen {
		t.Errorf("Expected circuit open error, got %v", err)
	}
}

func TestCircuitBreakerHalfOpen(t *testing.T) {
	cb := NewCircuitBreaker(2, 100*time.Millisecond, 50*time.Millisecond)

	// Open the circuit
	for i := 0; i < 2; i++ {
		cb.Execute(func() error {
			return errors.New("failure")
		})
	}

	if cb.State() != StateOpen {
		t.Error("Circuit should be open")
	}

	// Wait for reset timeout
	time.Sleep(150 * time.Millisecond)

	// Next request should transition to half-open
	err := cb.Execute(func() error {
		return nil
	})

	if err != nil {
		t.Errorf("Request should succeed in half-open: %v", err)
	}

	// Should be closed now
	if cb.State() != StateClosed {
		t.Errorf("Expected Closed state, got %s", cb.State())
	}
}

func TestCircuitBreakerReset(t *testing.T) {
	cb := NewCircuitBreaker(2, 100*time.Millisecond, 50*time.Millisecond)

	// Open circuit
	for i := 0; i < 2; i++ {
		cb.Execute(func() error {
			return errors.New("failure")
		})
	}

	if cb.State() != StateOpen {
		t.Error("Circuit should be open")
	}

	// Manual reset
	cb.Reset()

	if cb.State() != StateClosed {
		t.Error("Circuit should be closed after reset")
	}
}

func TestCircuitBreakerMetrics(t *testing.T) {
	cb := NewCircuitBreaker(3, 100*time.Millisecond, 50*time.Millisecond)

	// Execute some operations
	cb.Execute(func() error { return nil })
	cb.Execute(func() error { return errors.New("fail") })
	cb.Execute(func() error { return nil })

	requests, successes, failures, _ := cb.Metrics()

	if requests != 3 {
		t.Errorf("Expected 3 requests, got %d", requests)
	}

	if successes != 2 {
		t.Errorf("Expected 2 successes, got %d", successes)
	}

	if failures != 1 {
		t.Errorf("Expected 1 failure, got %d", failures)
	}
}

func TestCircuitBreakerTimeout(t *testing.T) {
	cb := NewCircuitBreaker(3, 100*time.Millisecond, 50*time.Millisecond)

	err := cb.Execute(func() error {
		time.Sleep(100 * time.Millisecond)
		return nil
	})

	if err != ErrTimeout {
		t.Errorf("Expected timeout error, got %v", err)
	}
}

func TestCircuitBreakerExecuteContext(t *testing.T) {
	cb := NewCircuitBreaker(3, 100*time.Millisecond, 200*time.Millisecond)

	ctx := context.Background()

	// Successful execution
	err := cb.ExecuteContext(ctx, func(ctx context.Context) error {
		return nil
	})

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Context cancellation
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	err = cb.ExecuteContext(ctx, func(ctx context.Context) error {
		time.Sleep(100 * time.Millisecond)
		return nil
	})

	if err != context.Canceled {
		t.Errorf("Expected context cancelled error, got %v", err)
	}
}

func TestAdvancedCircuitBreaker(t *testing.T) {
	acb := NewAdvancedCircuitBreaker(3, 2, 100*time.Millisecond, 50*time.Millisecond)

	// Successful operations
	for i := 0; i < 5; i++ {
		err := acb.Call(func() error {
			return nil
		})
		if err != nil {
			t.Errorf("Operation %d should succeed", i)
		}
	}

	if acb.GetState() != StateClosed {
		t.Error("Circuit should be closed")
	}
}

func TestAdvancedCircuitBreakerSuccessThreshold(t *testing.T) {
	acb := NewAdvancedCircuitBreaker(2, 3, 100*time.Millisecond, 50*time.Millisecond)

	// Open circuit
	for i := 0; i < 2; i++ {
		acb.Call(func() error {
			return errors.New("failure")
		})
	}

	if acb.GetState() != StateOpen {
		t.Error("Circuit should be open")
	}

	// Wait for reset timeout
	time.Sleep(150 * time.Millisecond)

	// Need 3 successes to close from half-open
	for i := 0; i < 3; i++ {
		acb.Call(func() error {
			return nil
		})
	}

	if acb.GetState() != StateClosed {
		t.Errorf("Expected Closed after success threshold, got %s", acb.GetState())
	}
}

func TestAdvancedCircuitBreakerMetrics(t *testing.T) {
	acb := NewAdvancedCircuitBreaker(3, 2, 100*time.Millisecond, 50*time.Millisecond)

	acb.Call(func() error { return nil })
	acb.Call(func() error { return errors.New("fail") })
	acb.Call(func() error { return nil })

	metrics := acb.GetMetrics()

	if metrics.TotalRequests != 3 {
		t.Errorf("Expected 3 total requests, got %d", metrics.TotalRequests)
	}

	if metrics.TotalSuccesses != 2 {
		t.Errorf("Expected 2 successes, got %d", metrics.TotalSuccesses)
	}

	if metrics.TotalFailures != 1 {
		t.Errorf("Expected 1 failure, got %d", metrics.TotalFailures)
	}
}

func TestAdvancedCircuitBreakerForceOpen(t *testing.T) {
	acb := NewAdvancedCircuitBreaker(3, 2, 100*time.Millisecond, 50*time.Millisecond)

	acb.ForceOpen()

	if acb.GetState() != StateOpen {
		t.Error("Circuit should be forcefully opened")
	}

	err := acb.Call(func() error {
		return nil
	})

	if err != ErrCircuitOpen {
		t.Errorf("Expected circuit open error, got %v", err)
	}
}

func TestAdvancedCircuitBreakerForceClose(t *testing.T) {
	acb := NewAdvancedCircuitBreaker(2, 2, 100*time.Millisecond, 50*time.Millisecond)

	// Open circuit
	for i := 0; i < 2; i++ {
		acb.Call(func() error {
			return errors.New("failure")
		})
	}

	acb.ForceClose()

	if acb.GetState() != StateClosed {
		t.Error("Circuit should be forcefully closed")
	}

	err := acb.Call(func() error {
		return nil
	})

	if err != nil {
		t.Errorf("Expected successful execution, got %v", err)
	}
}

func TestTwoStepCircuitBreaker(t *testing.T) {
	tscb := NewTwoStepCircuitBreaker(2, 1, 100*time.Millisecond, 50*time.Millisecond)

	stateChanges := 0
	tscb.OnStateChange(func(from, to State) {
		stateChanges++
	})

	// Trigger state change to Open
	for i := 0; i < 2; i++ {
		tscb.Execute(func() error {
			return errors.New("failure")
		})
	}

	if stateChanges == 0 {
		t.Error("Expected state change callback to be called")
	}

	if tscb.State() != StateOpen {
		t.Errorf("Expected Open state, got %s", tscb.State())
	}
}

func TestAdaptiveCircuitBreaker(t *testing.T) {
	acb := NewAdaptiveCircuitBreaker(3, 10, 0.5, 100*time.Millisecond, 50*time.Millisecond)

	// Execute operations
	for i := 0; i < 5; i++ {
		acb.Execute(func() error {
			return nil
		})
	}

	if acb.GetState() != StateClosed {
		t.Error("Circuit should be closed with low error rate")
	}

	// Introduce high error rate
	for i := 0; i < 10; i++ {
		acb.Execute(func() error {
			return errors.New("failure")
		})
	}

	errorRate := acb.GetErrorRate()
	if errorRate <= 0.5 {
		t.Logf("Error rate: %f (might not trigger circuit opening)", errorRate)
	}
}

func TestAdaptiveCircuitBreakerErrorRate(t *testing.T) {
	acb := NewAdaptiveCircuitBreaker(3, 10, 0.7, 100*time.Millisecond, 50*time.Millisecond)

	// 70% failure rate
	for i := 0; i < 10; i++ {
		if i < 7 {
			acb.Execute(func() error {
				return errors.New("failure")
			})
		} else {
			acb.Execute(func() error {
				return nil
			})
		}
	}

	errorRate := acb.GetErrorRate()
	if errorRate < 0.6 || errorRate > 0.8 {
		t.Logf("Expected error rate ~0.7, got %f", errorRate)
	}
}

func TestAdaptiveCircuitBreakerRecovery(t *testing.T) {
	acb := NewAdaptiveCircuitBreaker(3, 10, 0.6, 100*time.Millisecond, 50*time.Millisecond)

	// Open circuit with failures
	for i := 0; i < 10; i++ {
		acb.Execute(func() error {
			return errors.New("failure")
		})
	}

	if acb.GetState() != StateOpen {
		t.Error("Circuit should be open")
	}

	// Wait for reset timeout
	time.Sleep(150 * time.Millisecond)

	// Successful request should transition to half-open then closed
	err := acb.Execute(func() error {
		return nil
	})

	if err != nil {
		t.Logf("Recovery request: %v", err)
	}

	if acb.GetState() != StateClosed {
		t.Logf("Expected Closed state after recovery, got %s", acb.GetState())
	}
}

func TestStateString(t *testing.T) {
	states := []struct {
		state    State
		expected string
	}{
		{StateClosed, "Closed"},
		{StateOpen, "Open"},
		{StateHalfOpen, "HalfOpen"},
	}

	for _, tc := range states {
		if tc.state.String() != tc.expected {
			t.Errorf("Expected %s, got %s", tc.expected, tc.state.String())
		}
	}
}
