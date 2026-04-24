/*
Circuit Breaker Patterns
========================

Circuit breaker implementations for fault tolerance and preventing cascading failures.

Applications:
- Microservice resilience
- API fault tolerance
- Preventing cascading failures
- Service degradation handling
*/

package patterns

import (
	"context"
	"errors"
	"sync"
	"time"
)

// =============================================================================
// Circuit Breaker States
// =============================================================================

// State represents circuit breaker state
type State int

const (
	StateClosed State = iota // Normal operation
	StateOpen                // Failing, reject requests
	StateHalfOpen            // Testing if service recovered
)

func (s State) String() string {
	switch s {
	case StateClosed:
		return "Closed"
	case StateOpen:
		return "Open"
	case StateHalfOpen:
		return "HalfOpen"
	default:
		return "Unknown"
	}
}

// =============================================================================
// Circuit Breaker Errors
// =============================================================================

var (
	ErrCircuitOpen     = errors.New("circuit breaker is open")
	ErrTooManyRequests = errors.New("too many requests")
	ErrTimeout         = errors.New("operation timeout")
)

// =============================================================================
// Basic Circuit Breaker
// =============================================================================

// CircuitBreaker implements basic circuit breaker pattern
type CircuitBreaker struct {
	maxFailures  int           // Max failures before opening
	resetTimeout time.Duration // Time before attempting reset
	timeout      time.Duration // Operation timeout

	state        State
	failures     int
	successes    int
	lastFailTime time.Time
	mu           sync.RWMutex

	// Metrics
	totalRequests  int64
	totalSuccesses int64
	totalFailures  int64
	stateChanges   int64
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(maxFailures int, resetTimeout, timeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		maxFailures:  maxFailures,
		resetTimeout: resetTimeout,
		timeout:      timeout,
		state:        StateClosed,
	}
}

// Execute executes a function with circuit breaker protection
func (cb *CircuitBreaker) Execute(fn func() error) error {
	if err := cb.beforeRequest(); err != nil {
		return err
	}

	// Execute with timeout
	done := make(chan error, 1)
	go func() {
		done <- fn()
	}()

	select {
	case err := <-done:
		cb.afterRequest(err)
		return err
	case <-time.After(cb.timeout):
		cb.afterRequest(ErrTimeout)
		return ErrTimeout
	}
}

// ExecuteContext executes a function with context and circuit breaker protection
func (cb *CircuitBreaker) ExecuteContext(ctx context.Context, fn func(context.Context) error) error {
	if err := cb.beforeRequest(); err != nil {
		return err
	}

	done := make(chan error, 1)
	go func() {
		done <- fn(ctx)
	}()

	select {
	case err := <-done:
		cb.afterRequest(err)
		return err
	case <-ctx.Done():
		cb.afterRequest(ctx.Err())
		return ctx.Err()
	case <-time.After(cb.timeout):
		cb.afterRequest(ErrTimeout)
		return ErrTimeout
	}
}

func (cb *CircuitBreaker) beforeRequest() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.totalRequests++

	switch cb.state {
	case StateOpen:
		// Check if enough time has passed to try again
		if time.Since(cb.lastFailTime) > cb.resetTimeout {
			cb.setState(StateHalfOpen)
			return nil
		}
		return ErrCircuitOpen

	case StateHalfOpen:
		// Allow only one request in half-open state
		if cb.successes > 0 {
			return ErrTooManyRequests
		}
		return nil

	default: // StateClosed
		return nil
	}
}

func (cb *CircuitBreaker) afterRequest(err error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if err != nil {
		cb.totalFailures++
		cb.onFailure()
	} else {
		cb.totalSuccesses++
		cb.onSuccess()
	}
}

func (cb *CircuitBreaker) onSuccess() {
	switch cb.state {
	case StateClosed:
		cb.failures = 0

	case StateHalfOpen:
		cb.successes++
		if cb.successes >= 1 { // Could require multiple successes
			cb.setState(StateClosed)
			cb.failures = 0
			cb.successes = 0
		}
	}
}

func (cb *CircuitBreaker) onFailure() {
	cb.failures++
	cb.lastFailTime = time.Now()

	switch cb.state {
	case StateClosed:
		if cb.failures >= cb.maxFailures {
			cb.setState(StateOpen)
		}

	case StateHalfOpen:
		cb.setState(StateOpen)
		cb.successes = 0
	}
}

func (cb *CircuitBreaker) setState(state State) {
	if cb.state != state {
		cb.state = state
		cb.stateChanges++
	}
}

// State returns current state
func (cb *CircuitBreaker) State() State {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// Metrics returns circuit breaker metrics
func (cb *CircuitBreaker) Metrics() (requests, successes, failures, stateChanges int64) {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.totalRequests, cb.totalSuccesses, cb.totalFailures, cb.stateChanges
}

// Reset resets the circuit breaker to closed state
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.state = StateClosed
	cb.failures = 0
	cb.successes = 0
}

// =============================================================================
// Advanced Circuit Breaker with Success Threshold
// =============================================================================

// AdvancedCircuitBreaker includes success threshold for half-open state
type AdvancedCircuitBreaker struct {
	maxFailures      int           // Max consecutive failures
	successThreshold int           // Successes needed to close from half-open
	resetTimeout     time.Duration // Time before attempting reset
	timeout          time.Duration // Operation timeout

	state            State
	consecutiveFails int
	consecutiveSucc  int
	lastStateChange  time.Time
	mu               sync.RWMutex

	// Metrics
	metrics CircuitBreakerMetrics
}

// CircuitBreakerMetrics holds circuit breaker statistics
type CircuitBreakerMetrics struct {
	TotalRequests    int64
	TotalSuccesses   int64
	TotalFailures    int64
	ConsecutiveFails int
	ConsecutiveSucc  int
	State            State
	StateChanges     int64
	LastStateChange  time.Time
}

// NewAdvancedCircuitBreaker creates an advanced circuit breaker
func NewAdvancedCircuitBreaker(maxFailures, successThreshold int, resetTimeout, timeout time.Duration) *AdvancedCircuitBreaker {
	return &AdvancedCircuitBreaker{
		maxFailures:      maxFailures,
		successThreshold: successThreshold,
		resetTimeout:     resetTimeout,
		timeout:          timeout,
		state:            StateClosed,
		lastStateChange:  time.Now(),
	}
}

// Call executes a function with circuit breaker protection
func (acb *AdvancedCircuitBreaker) Call(fn func() error) error {
	if !acb.allowRequest() {
		acb.metrics.TotalRequests++
		return ErrCircuitOpen
	}

	acb.mu.Lock()
	acb.metrics.TotalRequests++
	acb.mu.Unlock()

	err := acb.executeWithTimeout(fn)
	acb.recordResult(err)
	return err
}

func (acb *AdvancedCircuitBreaker) allowRequest() bool {
	acb.mu.RLock()
	defer acb.mu.RUnlock()

	if acb.state == StateClosed {
		return true
	}

	if acb.state == StateOpen {
		// Check if we should transition to half-open
		if time.Since(acb.lastStateChange) > acb.resetTimeout {
			return true // Will transition in recordResult
		}
		return false
	}

	// StateHalfOpen - allow limited requests
	return true
}

func (acb *AdvancedCircuitBreaker) executeWithTimeout(fn func() error) error {
	done := make(chan error, 1)
	go func() {
		done <- fn()
	}()

	select {
	case err := <-done:
		return err
	case <-time.After(acb.timeout):
		return ErrTimeout
	}
}

func (acb *AdvancedCircuitBreaker) recordResult(err error) {
	acb.mu.Lock()
	defer acb.mu.Unlock()

	// Transition from Open to HalfOpen if timeout passed
	if acb.state == StateOpen && time.Since(acb.lastStateChange) > acb.resetTimeout {
		acb.transitionTo(StateHalfOpen)
	}

	if err != nil {
		acb.metrics.TotalFailures++
		acb.onCallFailure()
	} else {
		acb.metrics.TotalSuccesses++
		acb.onCallSuccess()
	}
}

func (acb *AdvancedCircuitBreaker) onCallSuccess() {
	acb.consecutiveFails = 0
	acb.consecutiveSucc++
	acb.metrics.ConsecutiveSucc = acb.consecutiveSucc

	if acb.state == StateHalfOpen && acb.consecutiveSucc >= acb.successThreshold {
		acb.transitionTo(StateClosed)
		acb.consecutiveSucc = 0
	}
}

func (acb *AdvancedCircuitBreaker) onCallFailure() {
	acb.consecutiveSucc = 0
	acb.consecutiveFails++
	acb.metrics.ConsecutiveFails = acb.consecutiveFails

	if acb.state == StateClosed && acb.consecutiveFails >= acb.maxFailures {
		acb.transitionTo(StateOpen)
	} else if acb.state == StateHalfOpen {
		acb.transitionTo(StateOpen)
	}
}

func (acb *AdvancedCircuitBreaker) transitionTo(newState State) {
	if acb.state != newState {
		acb.state = newState
		acb.lastStateChange = time.Now()
		acb.metrics.StateChanges++
		acb.metrics.State = newState
		acb.metrics.LastStateChange = acb.lastStateChange
	}
}

// GetMetrics returns current metrics
func (acb *AdvancedCircuitBreaker) GetMetrics() CircuitBreakerMetrics {
	acb.mu.RLock()
	defer acb.mu.RUnlock()
	return acb.metrics
}

// GetState returns current state
func (acb *AdvancedCircuitBreaker) GetState() State {
	acb.mu.RLock()
	defer acb.mu.RUnlock()
	return acb.state
}

// ForceOpen forces circuit breaker to open state
func (acb *AdvancedCircuitBreaker) ForceOpen() {
	acb.mu.Lock()
	defer acb.mu.Unlock()
	acb.transitionTo(StateOpen)
}

// ForceClose forces circuit breaker to closed state
func (acb *AdvancedCircuitBreaker) ForceClose() {
	acb.mu.Lock()
	defer acb.mu.Unlock()
	acb.transitionTo(StateClosed)
	acb.consecutiveFails = 0
	acb.consecutiveSucc = 0
}

// =============================================================================
// Two-Step Circuit Breaker
// =============================================================================

// TwoStepCircuitBreaker provides callbacks for state changes
type TwoStepCircuitBreaker struct {
	cb         *AdvancedCircuitBreaker
	onStateChange func(from, to State)
}

// NewTwoStepCircuitBreaker creates a two-step circuit breaker
func NewTwoStepCircuitBreaker(maxFailures, successThreshold int, resetTimeout, timeout time.Duration) *TwoStepCircuitBreaker {
	return &TwoStepCircuitBreaker{
		cb: NewAdvancedCircuitBreaker(maxFailures, successThreshold, resetTimeout, timeout),
	}
}

// OnStateChange sets callback for state changes
func (tscb *TwoStepCircuitBreaker) OnStateChange(callback func(from, to State)) {
	tscb.onStateChange = callback
}

// Execute executes function with circuit breaker
func (tscb *TwoStepCircuitBreaker) Execute(fn func() error) error {
	oldState := tscb.cb.GetState()
	err := tscb.cb.Call(fn)
	newState := tscb.cb.GetState()

	if oldState != newState && tscb.onStateChange != nil {
		tscb.onStateChange(oldState, newState)
	}

	return err
}

// State returns current state
func (tscb *TwoStepCircuitBreaker) State() State {
	return tscb.cb.GetState()
}

// =============================================================================
// Adaptive Circuit Breaker
// =============================================================================

// AdaptiveCircuitBreaker adjusts thresholds based on error rate
type AdaptiveCircuitBreaker struct {
	baseFailureThreshold int
	windowSize           int
	errorRateThreshold   float64
	timeout              time.Duration
	resetTimeout         time.Duration

	state       State
	window      []bool // true = success, false = failure
	windowIndex int
	mu          sync.RWMutex
	lastOpen    time.Time
}

// NewAdaptiveCircuitBreaker creates an adaptive circuit breaker
func NewAdaptiveCircuitBreaker(baseFailureThreshold, windowSize int, errorRateThreshold float64, resetTimeout, timeout time.Duration) *AdaptiveCircuitBreaker {
	// Initialise the sliding window to all successes so the error rate starts
	// at 0 and the circuit begins closed.  A zero-value bool slice would be
	// all false (= failures), causing the circuit to open on the very first call.
	window := make([]bool, windowSize)
	for i := range window {
		window[i] = true
	}

	return &AdaptiveCircuitBreaker{
		baseFailureThreshold: baseFailureThreshold,
		windowSize:           windowSize,
		errorRateThreshold:   errorRateThreshold,
		timeout:              timeout,
		resetTimeout:         resetTimeout,
		state:                StateClosed,
		window:               window,
	}
}

// Execute executes function with adaptive circuit breaker
func (acb *AdaptiveCircuitBreaker) Execute(fn func() error) error {
	if !acb.canExecute() {
		return ErrCircuitOpen
	}

	done := make(chan error, 1)
	go func() {
		done <- fn()
	}()

	var err error
	select {
	case err = <-done:
	case <-time.After(acb.timeout):
		err = ErrTimeout
	}

	acb.recordExecution(err == nil)
	return err
}

func (acb *AdaptiveCircuitBreaker) canExecute() bool {
	acb.mu.RLock()
	defer acb.mu.RUnlock()

	if acb.state == StateClosed {
		return true
	}

	if acb.state == StateOpen {
		if time.Since(acb.lastOpen) > acb.resetTimeout {
			return true
		}
		return false
	}

	return true // HalfOpen
}

func (acb *AdaptiveCircuitBreaker) recordExecution(success bool) {
	acb.mu.Lock()
	defer acb.mu.Unlock()

	acb.window[acb.windowIndex] = success
	acb.windowIndex = (acb.windowIndex + 1) % acb.windowSize

	errorRate := acb.calculateErrorRate()

	switch acb.state {
	case StateClosed:
		if errorRate > acb.errorRateThreshold {
			acb.state = StateOpen
			acb.lastOpen = time.Now()
		}
	case StateHalfOpen:
		if success {
			acb.state = StateClosed
		} else {
			acb.state = StateOpen
			acb.lastOpen = time.Now()
		}
	case StateOpen:
		if time.Since(acb.lastOpen) > acb.resetTimeout {
			acb.state = StateHalfOpen
		}
	}
}

func (acb *AdaptiveCircuitBreaker) calculateErrorRate() float64 {
	failures := 0
	total := 0

	for _, success := range acb.window {
		total++
		if !success {
			failures++
		}
	}

	if total == 0 {
		return 0
	}
	return float64(failures) / float64(total)
}

// GetState returns current state
func (acb *AdaptiveCircuitBreaker) GetState() State {
	acb.mu.RLock()
	defer acb.mu.RUnlock()
	return acb.state
}

// GetErrorRate returns current error rate
func (acb *AdaptiveCircuitBreaker) GetErrorRate() float64 {
	acb.mu.RLock()
	defer acb.mu.RUnlock()
	return acb.calculateErrorRate()
}
