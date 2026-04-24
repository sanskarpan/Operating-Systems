/*
Rate Limiting Patterns
======================

Rate limiting patterns for controlling request rates and preventing overload.

Applications:
- API rate limiting
- Request throttling
- Resource protection
- DoS prevention
*/

package patterns

import (
	"context"
	"sync"
	"time"
)

// =============================================================================
// Token Bucket Rate Limiter
// =============================================================================

// TokenBucket implements token bucket algorithm for rate limiting
type TokenBucket struct {
	capacity   int64         // Maximum tokens
	tokens     int64         // Current tokens
	refillRate int64         // Tokens per second
	lastRefill time.Time     // Last refill time
	mu         sync.Mutex
}

// NewTokenBucket creates a new token bucket rate limiter
func NewTokenBucket(capacity, refillRate int64) *TokenBucket {
	return &TokenBucket{
		capacity:   capacity,
		tokens:     capacity,
		refillRate: refillRate,
		lastRefill: time.Now(),
	}
}

// Allow checks if request is allowed and consumes a token
func (tb *TokenBucket) Allow() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	tb.refill()

	if tb.tokens > 0 {
		tb.tokens--
		return true
	}
	return false
}

// AllowN checks if n requests are allowed and consumes n tokens
func (tb *TokenBucket) AllowN(n int64) bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	tb.refill()

	if tb.tokens >= n {
		tb.tokens -= n
		return false
	}
	return false
}

// Wait blocks until a token is available
func (tb *TokenBucket) Wait(ctx context.Context) error {
	for {
		if tb.Allow() {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Millisecond * 10):
			// Retry after short delay
		}
	}
}

// refill adds tokens based on elapsed time
func (tb *TokenBucket) refill() {
	now := time.Now()
	elapsed := now.Sub(tb.lastRefill)

	tokensToAdd := int64(elapsed.Seconds() * float64(tb.refillRate))
	if tokensToAdd > 0 {
		tb.tokens += tokensToAdd
		if tb.tokens > tb.capacity {
			tb.tokens = tb.capacity
		}
		tb.lastRefill = now
	}
}

// AvailableTokens returns current number of available tokens
func (tb *TokenBucket) AvailableTokens() int64 {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	tb.refill()
	return tb.tokens
}

// =============================================================================
// Leaky Bucket Rate Limiter
// =============================================================================

// LeakyBucket implements leaky bucket algorithm for rate limiting
type LeakyBucket struct {
	capacity   int           // Maximum capacity
	queue      []interface{} // Request queue
	rate       time.Duration // Leak rate
	mu         sync.Mutex
	processing bool
	done       chan struct{}
}

// NewLeakyBucket creates a new leaky bucket rate limiter
func NewLeakyBucket(capacity int, rate time.Duration) *LeakyBucket {
	lb := &LeakyBucket{
		capacity: capacity,
		queue:    make([]interface{}, 0, capacity),
		rate:     rate,
		done:     make(chan struct{}),
	}
	go lb.leak()
	return lb
}

// Add adds a request to the bucket
func (lb *LeakyBucket) Add(req interface{}) bool {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	if len(lb.queue) >= lb.capacity {
		return false // Bucket full, drop request
	}

	lb.queue = append(lb.queue, req)
	return true
}

// leak processes requests at fixed rate
func (lb *LeakyBucket) leak() {
	ticker := time.NewTicker(lb.rate)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			lb.mu.Lock()
			if len(lb.queue) > 0 {
				// Remove first request (leak)
				lb.queue = lb.queue[1:]
			}
			lb.mu.Unlock()
		case <-lb.done:
			return
		}
	}
}

// Size returns current queue size
func (lb *LeakyBucket) Size() int {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	return len(lb.queue)
}

// Stop stops the leaky bucket
func (lb *LeakyBucket) Stop() {
	close(lb.done)
}

// =============================================================================
// Fixed Window Counter
// =============================================================================

// FixedWindowCounter implements fixed window rate limiting
type FixedWindowCounter struct {
	limit      int           // Max requests per window
	window     time.Duration // Window duration
	count      int           // Current count
	windowStart time.Time     // Window start time
	mu         sync.Mutex
}

// NewFixedWindowCounter creates a fixed window counter
func NewFixedWindowCounter(limit int, window time.Duration) *FixedWindowCounter {
	return &FixedWindowCounter{
		limit:      limit,
		window:     window,
		count:      0,
		windowStart: time.Now(),
	}
}

// Allow checks if request is allowed
func (fwc *FixedWindowCounter) Allow() bool {
	fwc.mu.Lock()
	defer fwc.mu.Unlock()

	now := time.Now()

	// Check if window has expired
	if now.Sub(fwc.windowStart) >= fwc.window {
		fwc.count = 0
		fwc.windowStart = now
	}

	if fwc.count < fwc.limit {
		fwc.count++
		return true
	}
	return false
}

// Remaining returns remaining requests in current window
func (fwc *FixedWindowCounter) Remaining() int {
	fwc.mu.Lock()
	defer fwc.mu.Unlock()

	now := time.Now()
	if now.Sub(fwc.windowStart) >= fwc.window {
		return fwc.limit
	}
	return fwc.limit - fwc.count
}

// =============================================================================
// Sliding Window Log
// =============================================================================

// SlidingWindowLog implements sliding window log algorithm
type SlidingWindowLog struct {
	limit      int           // Max requests per window
	window     time.Duration // Window duration
	requests   []time.Time   // Request timestamps
	mu         sync.Mutex
}

// NewSlidingWindowLog creates a sliding window log rate limiter
func NewSlidingWindowLog(limit int, window time.Duration) *SlidingWindowLog {
	return &SlidingWindowLog{
		limit:    limit,
		window:   window,
		requests: make([]time.Time, 0),
	}
}

// Allow checks if request is allowed
func (swl *SlidingWindowLog) Allow() bool {
	swl.mu.Lock()
	defer swl.mu.Unlock()

	now := time.Now()
	windowStart := now.Add(-swl.window)

	// Remove old requests outside window
	validRequests := make([]time.Time, 0)
	for _, reqTime := range swl.requests {
		if reqTime.After(windowStart) {
			validRequests = append(validRequests, reqTime)
		}
	}
	swl.requests = validRequests

	if len(swl.requests) < swl.limit {
		swl.requests = append(swl.requests, now)
		return true
	}
	return false
}

// Count returns current request count in window
func (swl *SlidingWindowLog) Count() int {
	swl.mu.Lock()
	defer swl.mu.Unlock()

	now := time.Now()
	windowStart := now.Add(-swl.window)

	count := 0
	for _, reqTime := range swl.requests {
		if reqTime.After(windowStart) {
			count++
		}
	}
	return count
}

// =============================================================================
// Sliding Window Counter
// =============================================================================

// SlidingWindowCounter implements sliding window counter algorithm
type SlidingWindowCounter struct {
	limit          int           // Max requests per window
	window         time.Duration // Window duration
	currentCount   int           // Current window count
	previousCount  int           // Previous window count
	currentStart   time.Time     // Current window start
	mu             sync.Mutex
}

// NewSlidingWindowCounter creates a sliding window counter
func NewSlidingWindowCounter(limit int, window time.Duration) *SlidingWindowCounter {
	return &SlidingWindowCounter{
		limit:        limit,
		window:       window,
		currentCount: 0,
		previousCount: 0,
		currentStart: time.Now(),
	}
}

// Allow checks if request is allowed
func (swc *SlidingWindowCounter) Allow() bool {
	swc.mu.Lock()
	defer swc.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(swc.currentStart)

	// Slide window if needed
	if elapsed >= swc.window {
		if elapsed >= 2*swc.window {
			// More than two full windows have passed — fully reset
			swc.previousCount = 0
			swc.currentCount = 0
			swc.currentStart = now
			elapsed = 0
		} else {
			// Advance exactly one window, keeping the correct elapsed offset
			swc.previousCount = swc.currentCount
			swc.currentCount = 0
			swc.currentStart = swc.currentStart.Add(swc.window)
			elapsed = elapsed - swc.window
		}
	}

	// Calculate weighted count
	percentageInCurrent := float64(elapsed) / float64(swc.window)
	weightedCount := float64(swc.previousCount)*(1-percentageInCurrent) + float64(swc.currentCount)

	if int(weightedCount) < swc.limit {
		swc.currentCount++
		return true
	}
	return false
}

// =============================================================================
// Per-User Rate Limiter
// =============================================================================

// PerUserRateLimiter implements per-user rate limiting
type PerUserRateLimiter struct {
	limiters map[string]*TokenBucket
	capacity int64
	rate     int64
	mu       sync.RWMutex
}

// NewPerUserRateLimiter creates a per-user rate limiter
func NewPerUserRateLimiter(capacity, rate int64) *PerUserRateLimiter {
	return &PerUserRateLimiter{
		limiters: make(map[string]*TokenBucket),
		capacity: capacity,
		rate:     rate,
	}
}

// Allow checks if request is allowed for user
func (purl *PerUserRateLimiter) Allow(userID string) bool {
	purl.mu.Lock()
	limiter, exists := purl.limiters[userID]
	if !exists {
		limiter = NewTokenBucket(purl.capacity, purl.rate)
		purl.limiters[userID] = limiter
	}
	purl.mu.Unlock()

	return limiter.Allow()
}

// Wait blocks until request is allowed for user
func (purl *PerUserRateLimiter) Wait(ctx context.Context, userID string) error {
	purl.mu.Lock()
	limiter, exists := purl.limiters[userID]
	if !exists {
		limiter = NewTokenBucket(purl.capacity, purl.rate)
		purl.limiters[userID] = limiter
	}
	purl.mu.Unlock()

	return limiter.Wait(ctx)
}

// CleanupInactive removes limiters for inactive users
func (purl *PerUserRateLimiter) CleanupInactive() {
	purl.mu.Lock()
	defer purl.mu.Unlock()

	// Remove limiters with full tokens (inactive users)
	for userID, limiter := range purl.limiters {
		if limiter.AvailableTokens() == purl.capacity {
			delete(purl.limiters, userID)
		}
	}
}

// =============================================================================
// Concurrent Request Limiter
// =============================================================================

// ConcurrentLimiter limits number of concurrent requests
type ConcurrentLimiter struct {
	limit     int
	semaphore chan struct{}
}

// NewConcurrentLimiter creates a concurrent request limiter
func NewConcurrentLimiter(limit int) *ConcurrentLimiter {
	return &ConcurrentLimiter{
		limit:     limit,
		semaphore: make(chan struct{}, limit),
	}
}

// Acquire acquires a slot
func (cl *ConcurrentLimiter) Acquire(ctx context.Context) error {
	select {
	case cl.semaphore <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Release releases a slot
func (cl *ConcurrentLimiter) Release() {
	<-cl.semaphore
}

// TryAcquire tries to acquire without blocking
func (cl *ConcurrentLimiter) TryAcquire() bool {
	select {
	case cl.semaphore <- struct{}{}:
		return true
	default:
		return false
	}
}

// Available returns available slots
func (cl *ConcurrentLimiter) Available() int {
	return cl.limit - len(cl.semaphore)
}

// =============================================================================
// Adaptive Rate Limiter
// =============================================================================

// AdaptiveRateLimiter adjusts rate based on system load
type AdaptiveRateLimiter struct {
	minRate    int64         // Minimum rate
	maxRate    int64         // Maximum rate
	currentRate int64         // Current rate
	bucket     *TokenBucket
	mu         sync.Mutex
}

// NewAdaptiveRateLimiter creates an adaptive rate limiter
func NewAdaptiveRateLimiter(minRate, maxRate int64) *AdaptiveRateLimiter {
	currentRate := (minRate + maxRate) / 2
	return &AdaptiveRateLimiter{
		minRate:    minRate,
		maxRate:    maxRate,
		currentRate: currentRate,
		bucket:     NewTokenBucket(currentRate, currentRate),
	}
}

// Allow checks if request is allowed
func (arl *AdaptiveRateLimiter) Allow() bool {
	return arl.bucket.Allow()
}

// IncreaseRate increases the rate limit
func (arl *AdaptiveRateLimiter) IncreaseRate() {
	arl.mu.Lock()
	defer arl.mu.Unlock()

	increment := (arl.maxRate - arl.currentRate) / 10
	if increment < 1 {
		increment = 1
	}
	newRate := arl.currentRate + increment
	if newRate > arl.maxRate {
		newRate = arl.maxRate
	}
	arl.currentRate = newRate
	arl.bucket = NewTokenBucket(newRate, newRate)
}

// DecreaseRate decreases the rate limit
func (arl *AdaptiveRateLimiter) DecreaseRate() {
	arl.mu.Lock()
	defer arl.mu.Unlock()

	newRate := arl.currentRate - (arl.currentRate-arl.minRate)/10
	if newRate < arl.minRate {
		newRate = arl.minRate
	}
	arl.currentRate = newRate
	arl.bucket = NewTokenBucket(newRate, newRate)
}

// CurrentRate returns current rate
func (arl *AdaptiveRateLimiter) CurrentRate() int64 {
	arl.mu.Lock()
	defer arl.mu.Unlock()
	return arl.currentRate
}

// =============================================================================
// Distributed Rate Limiter (simplified)
// =============================================================================

// DistributedRateLimiter coordinates rate limiting across multiple instances
type DistributedRateLimiter struct {
	globalLimit int
	instances   int
	localLimit  int
	counter     *FixedWindowCounter
}

// NewDistributedRateLimiter creates a distributed rate limiter
func NewDistributedRateLimiter(globalLimit, instances int, window time.Duration) *DistributedRateLimiter {
	localLimit := globalLimit / instances
	return &DistributedRateLimiter{
		globalLimit: globalLimit,
		instances:   instances,
		localLimit:  localLimit,
		counter:     NewFixedWindowCounter(localLimit, window),
	}
}

// Allow checks if request is allowed
func (drl *DistributedRateLimiter) Allow() bool {
	return drl.counter.Allow()
}

// =============================================================================
// Rate Limiter Middleware
// =============================================================================

// RateLimiterFunc is a function type for rate-limited operations
type RateLimiterFunc func() error

// WithRateLimit wraps a function with rate limiting
func WithRateLimit(limiter *TokenBucket, fn RateLimiterFunc) error {
	if !limiter.Allow() {
		return context.DeadlineExceeded
	}
	return fn()
}

// WithRateLimitWait wraps a function with rate limiting and waiting
func WithRateLimitWait(ctx context.Context, limiter *TokenBucket, fn RateLimiterFunc) error {
	if err := limiter.Wait(ctx); err != nil {
		return err
	}
	return fn()
}
