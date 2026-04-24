# Go Concurrency Patterns

Comprehensive implementation of Go concurrency patterns including channels, worker pools, context management, rate limiting, circuit breakers, and error groups.

## Patterns Implemented

### 1. Channel Patterns (`channels.go`)

**Basic Patterns:**
- **Generator** - Creates a channel and sends values to it
- **Pipeline** - Processes values through transformation functions
- **Fan-Out** - Distributes work across multiple workers
- **Fan-In** - Combines results from multiple channels

**Advanced Patterns:**
- **Or-Done** - Handles both data and done signals
- **Tee** - Splits a channel into two
- **Bridge** - Converts channel of channels into single channel
- **Or-Channel** - Combines multiple done channels

**Utility Patterns:**
- **Batch** - Groups items into batches
- **Debounce** - Emits only after silence period
- **Throttle** - Limits emission rate
- **Take** - Takes first n values
- **Repeat** - Repeats values infinitely

### 2. Worker Pool Patterns (`workerpool.go`)

- **Fixed Worker Pool** - Simple parallel processing with fixed workers
- **Dynamic Pool** - Adjusts worker count based on load
- **Bounded Worker Pool** - Limits concurrent executions
- **Task Queue** - Function-based task processing
- **Work Stealing Pool** - Load balancing between workers
- **Priority Worker Pool** - Processes jobs by priority

### 3. Context Patterns (`context.go`)

**Cancellation:**
- Cancellable work execution
- Cascading cancellation
- Merged contexts

**Timeouts:**
- Timeout operations
- Deadline monitoring
- Retry with timeout
- Timeout chains

**Value Propagation:**
- Request ID tracking
- User ID tracking
- Trace ID tracking

**Advanced:**
- Context-aware pipelines
- Context-aware worker pools
- Graceful shutdown
- Background tasks

### 4. Rate Limiting Patterns (`ratelimit.go`)

- **Token Bucket** - Classic token bucket algorithm
- **Leaky Bucket** - Request queue with fixed leak rate
- **Fixed Window Counter** - Simple window-based limiting
- **Sliding Window Log** - Precise sliding window
- **Sliding Window Counter** - Efficient sliding window
- **Per-User Rate Limiter** - Independent limits per user
- **Concurrent Limiter** - Limits concurrent requests
- **Adaptive Rate Limiter** - Adjusts rate based on load

### 5. Circuit Breaker Patterns (`circuitbreaker.go`)

- **Basic Circuit Breaker** - Three-state (Closed/Open/Half-Open) breaker
- **Advanced Circuit Breaker** - With success threshold and metrics
- **Two-Step Circuit Breaker** - With state change callbacks
- **Adaptive Circuit Breaker** - Adjusts based on error rate

### 6. Error Group Patterns (`errorgroup.go`)

**Basic Groups:**
- **Error Group** - Returns first error
- **Context Error Group** - With cancellation on error
- **Multi-Error Group** - Collects all errors

**Advanced Groups:**
- **Limited Error Group** - With concurrency limits
- **Typed Error Group** - Groups errors by type
- **Result Error Group** - Collects results with errors
- **Retry Error Group** - Retries failed operations
- **Pipeline Error Group** - Sequential stage execution
- **Batch Error Group** - Processes items in batches
- **Hierarchical Error Group** - Nested error groups

**Helper Functions:**
- ParallelExecute
- ParallelExecuteWithContext
- ParallelMap
- ParallelMapWithLimit

## Usage Examples

See `examples/main.go` for comprehensive demonstrations of all patterns.

Run the examples:
```bash
cd patterns/examples
go run main.go
```

## Running Tests

Run all tests:
```bash
go test ./patterns/... -v
```

Run specific test file:
```bash
go test ./patterns/ -run TestGenerator -v
```

Run with race detector:
```bash
go test ./patterns/... -race
```

## Test Coverage

The module includes 100+ comprehensive tests covering:
- 20 channel pattern tests
- 10 worker pool pattern tests
- 22 context pattern tests
- 18 rate limiting pattern tests
- 19 circuit breaker pattern tests
- 19 error group pattern tests

## Performance Considerations

- **Buffered channels** are used where appropriate to prevent goroutine blocking
- **Worker pools** limit concurrent executions to prevent resource exhaustion
- **Rate limiters** use efficient algorithms (token bucket, sliding window)
- **Circuit breakers** prevent cascading failures
- **Error groups** coordinate parallel execution with proper cleanup

## Best Practices

1. **Always close done channels** to prevent goroutine leaks
2. **Use context for cancellation** in production code
3. **Set appropriate timeouts** to prevent infinite waiting
4. **Monitor circuit breaker metrics** for system health
5. **Choose the right rate limiting algorithm** for your use case
6. **Use error groups** for coordinated parallel execution

## Integration with Standard Library

These patterns work seamlessly with:
- `context` package for cancellation and deadlines
- `sync` package for synchronization primitives
- `time` package for timeouts and tickers
- `errors` package for error handling

## Production Readiness

All patterns include:
- Proper resource cleanup with `defer`
- Race condition protection with mutexes
- Context cancellation support
- Graceful shutdown handling
- Comprehensive error handling
- Thread-safe implementations

## License

Part of Phase0_Core/OperatingSystems educational project.
