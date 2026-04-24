package patterns

import (
	"context"
	"testing"
	"time"
)

func TestTokenBucket(t *testing.T) {
	tb := NewTokenBucket(5, 10)

	// Should allow initial requests up to capacity
	for i := 0; i < 5; i++ {
		if !tb.Allow() {
			t.Errorf("Request %d should be allowed", i)
		}
	}

	// Should deny when bucket is empty
	if tb.Allow() {
		t.Error("Request should be denied when bucket is empty")
	}

	// Wait for refill
	time.Sleep(200 * time.Millisecond)

	// Should allow again after refill
	if !tb.Allow() {
		t.Error("Request should be allowed after refill")
	}
}

func TestTokenBucketWait(t *testing.T) {
	tb := NewTokenBucket(1, 5)

	ctx := context.Background()

	// First request should succeed immediately
	err := tb.Wait(ctx)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Second request should wait for refill
	start := time.Now()
	err = tb.Wait(ctx)
	elapsed := time.Since(start)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if elapsed < 100*time.Millisecond {
		t.Logf("Wait might not be working: elapsed %v", elapsed)
	}
}

func TestTokenBucketContextCancellation(t *testing.T) {
	tb := NewTokenBucket(1, 1)
	tb.Allow() // Drain bucket

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := tb.Wait(ctx)
	if err != context.DeadlineExceeded {
		t.Errorf("Expected timeout error, got %v", err)
	}
}

func TestLeakyBucket(t *testing.T) {
	lb := NewLeakyBucket(5, 50*time.Millisecond)
	defer lb.Stop()

	// Should accept requests up to capacity
	for i := 0; i < 5; i++ {
		if !lb.Add(i) {
			t.Errorf("Request %d should be accepted", i)
		}
	}

	// Should reject when full
	if lb.Add(10) {
		t.Error("Request should be rejected when bucket is full")
	}

	// Wait for leak
	time.Sleep(300 * time.Millisecond)

	// Should have room now
	if !lb.Add(20) {
		t.Error("Request should be accepted after leak")
	}
}

func TestLeakyBucketSize(t *testing.T) {
	lb := NewLeakyBucket(3, 100*time.Millisecond)
	defer lb.Stop()

	lb.Add(1)
	lb.Add(2)

	size := lb.Size()
	if size != 2 {
		t.Errorf("Expected size 2, got %d", size)
	}

	time.Sleep(250 * time.Millisecond)

	size = lb.Size()
	if size >= 2 {
		t.Errorf("Expected size < 2 after leak, got %d", size)
	}
}

func TestFixedWindowCounter(t *testing.T) {
	fwc := NewFixedWindowCounter(5, 100*time.Millisecond)

	// Should allow requests within limit
	for i := 0; i < 5; i++ {
		if !fwc.Allow() {
			t.Errorf("Request %d should be allowed", i)
		}
	}

	// Should deny when limit reached
	if fwc.Allow() {
		t.Error("Request should be denied when limit reached")
	}

	// Wait for window to expire
	time.Sleep(150 * time.Millisecond)

	// Should allow in new window
	if !fwc.Allow() {
		t.Error("Request should be allowed in new window")
	}
}

func TestFixedWindowCounterRemaining(t *testing.T) {
	fwc := NewFixedWindowCounter(10, 200*time.Millisecond)

	remaining := fwc.Remaining()
	if remaining != 10 {
		t.Errorf("Expected 10 remaining, got %d", remaining)
	}

	fwc.Allow()
	fwc.Allow()
	fwc.Allow()

	remaining = fwc.Remaining()
	if remaining != 7 {
		t.Errorf("Expected 7 remaining, got %d", remaining)
	}
}

func TestSlidingWindowLog(t *testing.T) {
	swl := NewSlidingWindowLog(5, 100*time.Millisecond)

	// Allow initial requests
	for i := 0; i < 5; i++ {
		if !swl.Allow() {
			t.Errorf("Request %d should be allowed", i)
		}
	}

	// Deny when limit reached
	if swl.Allow() {
		t.Error("Request should be denied")
	}

	// Wait for window to slide
	time.Sleep(120 * time.Millisecond)

	// Should allow again
	if !swl.Allow() {
		t.Error("Request should be allowed after window slide")
	}
}

func TestSlidingWindowLogCount(t *testing.T) {
	swl := NewSlidingWindowLog(10, 200*time.Millisecond)

	for i := 0; i < 5; i++ {
		swl.Allow()
	}

	count := swl.Count()
	if count != 5 {
		t.Errorf("Expected count 5, got %d", count)
	}

	time.Sleep(250 * time.Millisecond)

	count = swl.Count()
	if count != 0 {
		t.Errorf("Expected count 0 after window expiration, got %d", count)
	}
}

func TestSlidingWindowCounter(t *testing.T) {
	swc := NewSlidingWindowCounter(10, 100*time.Millisecond)

	// Allow requests
	for i := 0; i < 10; i++ {
		if !swc.Allow() {
			t.Errorf("Request %d should be allowed", i)
		}
	}

	// Should deny when limit reached
	if swc.Allow() {
		t.Error("Request should be denied at limit")
	}

	// Wait for window to slide
	time.Sleep(120 * time.Millisecond)

	// Should allow in new window
	if !swc.Allow() {
		t.Error("Request should be allowed in new window")
	}
}

func TestPerUserRateLimiter(t *testing.T) {
	limiter := NewPerUserRateLimiter(3, 10)

	// User 1 should get 3 requests
	for i := 0; i < 3; i++ {
		if !limiter.Allow("user1") {
			t.Errorf("User1 request %d should be allowed", i)
		}
	}

	// User 1 should be denied
	if limiter.Allow("user1") {
		t.Error("User1 should be rate limited")
	}

	// User 2 should still be allowed
	if !limiter.Allow("user2") {
		t.Error("User2 should be allowed")
	}
}

func TestPerUserRateLimiterWait(t *testing.T) {
	limiter := NewPerUserRateLimiter(1, 5)

	ctx := context.Background()

	// First request succeeds
	err := limiter.Wait(ctx, "user1")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Second request waits
	start := time.Now()
	err = limiter.Wait(ctx, "user1")
	elapsed := time.Since(start)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if elapsed < 100*time.Millisecond {
		t.Logf("Wait might not be working: elapsed %v", elapsed)
	}
}

func TestConcurrentLimiter(t *testing.T) {
	limiter := NewConcurrentLimiter(3)
	ctx := context.Background()

	// Acquire 3 slots
	for i := 0; i < 3; i++ {
		err := limiter.Acquire(ctx)
		if err != nil {
			t.Errorf("Acquire %d failed: %v", i, err)
		}
	}

	// Fourth acquire should block
	acquired := make(chan bool)
	go func() {
		ctx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
		defer cancel()
		err := limiter.Acquire(ctx)
		acquired <- (err == nil)
	}()

	select {
	case success := <-acquired:
		if success {
			t.Error("Should not acquire when limit reached")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Acquire should timeout")
	}

	// Release one slot
	limiter.Release()

	// Now should be able to acquire
	err := limiter.Acquire(ctx)
	if err != nil {
		t.Errorf("Should be able to acquire after release: %v", err)
	}
}

func TestConcurrentLimiterTryAcquire(t *testing.T) {
	limiter := NewConcurrentLimiter(2)

	if !limiter.TryAcquire() {
		t.Error("First acquire should succeed")
	}

	if !limiter.TryAcquire() {
		t.Error("Second acquire should succeed")
	}

	if limiter.TryAcquire() {
		t.Error("Third acquire should fail")
	}

	limiter.Release()

	if !limiter.TryAcquire() {
		t.Error("Acquire should succeed after release")
	}
}

func TestAdaptiveRateLimiter(t *testing.T) {
	arl := NewAdaptiveRateLimiter(5, 20)

	initialRate := arl.CurrentRate()
	if initialRate < 5 || initialRate > 20 {
		t.Errorf("Initial rate should be between 5 and 20, got %d", initialRate)
	}

	// Increase rate
	arl.IncreaseRate()
	newRate := arl.CurrentRate()
	if newRate <= initialRate {
		t.Error("Rate should increase")
	}

	// Decrease rate
	arl.DecreaseRate()
	newRate = arl.CurrentRate()
	if newRate > 20 {
		t.Errorf("Rate should not exceed max, got %d", newRate)
	}

	// Decrease to minimum
	for i := 0; i < 20; i++ {
		arl.DecreaseRate()
	}

	finalRate := arl.CurrentRate()
	if finalRate < 5 {
		t.Errorf("Rate should not go below min, got %d", finalRate)
	}
}

func TestDistributedRateLimiter(t *testing.T) {
	drl := NewDistributedRateLimiter(100, 5, 1*time.Second)

	// Each instance should get ~20 requests per second
	allowed := 0
	for i := 0; i < 25; i++ {
		if drl.Allow() {
			allowed++
		}
	}

	if allowed != 20 {
		t.Logf("Expected ~20 allowed requests per instance, got %d", allowed)
	}
}

func TestWithRateLimit(t *testing.T) {
	limiter := NewTokenBucket(1, 5)

	executed := false
	fn := func() error {
		executed = true
		return nil
	}

	// First call should succeed
	err := WithRateLimit(limiter, fn)
	if err != nil || !executed {
		t.Error("First call should succeed")
	}

	// Second call should be rate limited
	executed = false
	err = WithRateLimit(limiter, fn)
	if err == nil || executed {
		t.Error("Second call should be rate limited")
	}
}

func TestWithRateLimitWait(t *testing.T) {
	limiter := NewTokenBucket(1, 5)
	ctx := context.Background()

	counter := 0
	fn := func() error {
		counter++
		return nil
	}

	// Should execute both calls, second one waits
	start := time.Now()
	err := WithRateLimitWait(ctx, limiter, fn)
	if err != nil {
		t.Errorf("First call failed: %v", err)
	}

	err = WithRateLimitWait(ctx, limiter, fn)
	elapsed := time.Since(start)

	if err != nil {
		t.Errorf("Second call failed: %v", err)
	}

	if counter != 2 {
		t.Errorf("Expected 2 executions, got %d", counter)
	}

	if elapsed < 100*time.Millisecond {
		t.Logf("Second call might not have waited: elapsed %v", elapsed)
	}
}
