/*
Go Channel Patterns
===================

Channel-based concurrency patterns for communication and synchronization.

Applications:
- Goroutine communication
- Event-driven systems
- Stream processing
- Message passing systems
*/

package patterns

import (
	"fmt"
	"sync"
	"time"
)

// =============================================================================
// Basic Channel Patterns
// =============================================================================

// Generator creates a channel and sends values to it
func Generator(done <-chan struct{}, values ...int) <-chan int {
	out := make(chan int)
	go func() {
		defer close(out)
		for _, v := range values {
			select {
			case out <- v:
			case <-done:
				return
			}
		}
	}()
	return out
}

// Pipeline creates a pipeline stage that processes values
func Pipeline(done <-chan struct{}, in <-chan int, fn func(int) int) <-chan int {
	out := make(chan int)
	go func() {
		defer close(out)
		for v := range in {
			select {
			case out <- fn(v):
			case <-done:
				return
			}
		}
	}()
	return out
}

// =============================================================================
// Fan-Out / Fan-In Pattern
// =============================================================================

// FanOut spawns multiple goroutines to handle input from a channel
func FanOut(done <-chan struct{}, in <-chan int, numWorkers int, fn func(int) int) []<-chan int {
	channels := make([]<-chan int, numWorkers)
	for i := 0; i < numWorkers; i++ {
		channels[i] = worker(done, in, fn)
	}
	return channels
}

func worker(done <-chan struct{}, in <-chan int, fn func(int) int) <-chan int {
	out := make(chan int)
	go func() {
		defer close(out)
		for v := range in {
			select {
			case out <- fn(v):
			case <-done:
				return
			}
		}
	}()
	return out
}

// FanIn multiplexes multiple input channels into single output channel
func FanIn(done <-chan struct{}, channels ...<-chan int) <-chan int {
	out := make(chan int)
	var wg sync.WaitGroup

	multiplex := func(c <-chan int) {
		defer wg.Done()
		for v := range c {
			select {
			case out <- v:
			case <-done:
				return
			}
		}
	}

	wg.Add(len(channels))
	for _, c := range channels {
		go multiplex(c)
	}

	// Close output only after every multiplex goroutine has finished sending,
	// eliminating the race between senders and the channel close.
	go func() {
		wg.Wait()
		close(out)
	}()

	return out
}

// =============================================================================
// Or-Done Pattern
// =============================================================================

// OrDone wraps a channel to handle both data and done signal
func OrDone(done <-chan struct{}, c <-chan int) <-chan int {
	out := make(chan int)
	go func() {
		defer close(out)
		for {
			select {
			case <-done:
				return
			case v, ok := <-c:
				if !ok {
					return
				}
				select {
				case out <- v:
				case <-done:
					return
				}
			}
		}
	}()
	return out
}

// =============================================================================
// Tee Channel Pattern
// =============================================================================

// Tee splits a channel into two channels with same values
func Tee(done <-chan struct{}, in <-chan int) (<-chan int, <-chan int) {
	out1 := make(chan int)
	out2 := make(chan int)

	go func() {
		defer close(out1)
		defer close(out2)

		for v := range OrDone(done, in) {
			// Use local variables for sending to both channels
			var out1, out2 = out1, out2
			for i := 0; i < 2; i++ {
				select {
				case <-done:
					return
				case out1 <- v:
					out1 = nil // Disable this case
				case out2 <- v:
					out2 = nil // Disable this case
				}
			}
		}
	}()

	return out1, out2
}

// =============================================================================
// Bridge Pattern
// =============================================================================

// Bridge takes a channel of channels and creates a single channel
func Bridge(done <-chan struct{}, chanStream <-chan <-chan int) <-chan int {
	out := make(chan int)

	go func() {
		defer close(out)
		for {
			var stream <-chan int
			select {
			case <-done:
				return
			case maybeStream, ok := <-chanStream:
				if !ok {
					return
				}
				stream = maybeStream
			}

			for v := range OrDone(done, stream) {
				select {
				case out <- v:
				case <-done:
					return
				}
			}
		}
	}()

	return out
}

// =============================================================================
// Or-Channel Pattern (for multiple done signals)
// =============================================================================

// Or combines multiple done channels into one
func Or(channels ...<-chan struct{}) <-chan struct{} {
	switch len(channels) {
	case 0:
		return nil
	case 1:
		return channels[0]
	}

	orDone := make(chan struct{})
	go func() {
		defer close(orDone)

		switch len(channels) {
		case 2:
			select {
			case <-channels[0]:
			case <-channels[1]:
			}
		default:
			select {
			case <-channels[0]:
			case <-channels[1]:
			case <-channels[2]:
			case <-Or(append(channels[3:], orDone)...):
			}
		}
	}()

	return orDone
}

// =============================================================================
// Buffered Channel Patterns
// =============================================================================

// BufferedGenerator creates values with buffering
func BufferedGenerator(bufferSize int, values ...int) <-chan int {
	out := make(chan int, bufferSize)
	go func() {
		defer close(out)
		for _, v := range values {
			out <- v
		}
	}()
	return out
}

// =============================================================================
// Select Patterns
// =============================================================================

// SelectFirst returns value from first channel that has data
func SelectFirst(channels ...<-chan int) (int, bool) {
	cases := make([]interface{}, len(channels))
	for i, ch := range channels {
		cases[i] = ch
	}

	// Simple implementation - just try first available
	for _, ch := range channels {
		select {
		case v, ok := <-ch:
			return v, ok
		default:
		}
	}

	// If no immediate value, block on all
	for _, ch := range channels {
		select {
		case v, ok := <-ch:
			return v, ok
		}
	}

	return 0, false
}

// TimeoutSelect reads from channel with timeout
func TimeoutSelect(c <-chan int, timeout time.Duration) (int, bool) {
	select {
	case v := <-c:
		return v, true
	case <-time.After(timeout):
		return 0, false
	}
}

// =============================================================================
// Channel Utilities
// =============================================================================

// Repeat repeats values on a channel until done
func Repeat(done <-chan struct{}, values ...int) <-chan int {
	out := make(chan int)
	go func() {
		defer close(out)
		for {
			for _, v := range values {
				select {
				case <-done:
					return
				case out <- v:
				}
			}
		}
	}()
	return out
}

// Take takes first n values from channel
func Take(done <-chan struct{}, in <-chan int, n int) <-chan int {
	out := make(chan int)
	go func() {
		defer close(out)
		for i := 0; i < n; i++ {
			select {
			case <-done:
				return
			case v := <-in:
				select {
				case out <- v:
				case <-done:
					return
				}
			}
		}
	}()
	return out
}

// RepeatFn repeats function calls until done
func RepeatFn(done <-chan struct{}, fn func() int) <-chan int {
	out := make(chan int)
	go func() {
		defer close(out)
		for {
			select {
			case <-done:
				return
			case out <- fn():
			}
		}
	}()
	return out
}

// =============================================================================
// Batch Processing
// =============================================================================

// Batch groups items into batches of specified size
func Batch(done <-chan struct{}, in <-chan int, batchSize int) <-chan []int {
	out := make(chan []int)
	go func() {
		defer close(out)
		batch := make([]int, 0, batchSize)

		for v := range OrDone(done, in) {
			batch = append(batch, v)
			if len(batch) == batchSize {
				select {
				case out <- batch:
					batch = make([]int, 0, batchSize)
				case <-done:
					return
				}
			}
		}

		// Send remaining items
		if len(batch) > 0 {
			select {
			case out <- batch:
			case <-done:
			}
		}
	}()
	return out
}

// =============================================================================
// Debounce and Throttle
// =============================================================================

// Debounce returns a channel that emits only after silence period
func Debounce(done <-chan struct{}, in <-chan int, duration time.Duration) <-chan int {
	out := make(chan int)
	go func() {
		defer close(out)
		var lastValue int
		var hasValue bool
		timer := time.NewTimer(duration)
		timer.Stop()

		for {
			select {
			case <-done:
				return
			case v, ok := <-in:
				if !ok {
					// Channel closed, emit last value if any and return
					if hasValue {
						select {
						case out <- lastValue:
						case <-done:
						}
					}
					return
				}
				lastValue = v
				hasValue = true
				timer.Reset(duration)
			case <-timer.C:
				if hasValue {
					select {
					case out <- lastValue:
					case <-done:
						return
					}
					hasValue = false
				}
			}
		}
	}()
	return out
}

// Throttle limits emission rate
func Throttle(done <-chan struct{}, in <-chan int, duration time.Duration) <-chan int {
	out := make(chan int)
	go func() {
		defer close(out)
		ticker := time.NewTicker(duration)
		defer ticker.Stop()

		for {
			select {
			case <-done:
				return
			case v, ok := <-in:
				if !ok {
					return
				}
				select {
				case <-ticker.C:
					select {
					case out <- v:
					case <-done:
						return
					}
				case <-done:
					return
				}
			}
		}
	}()
	return out
}

// =============================================================================
// Merge Pattern (Fan-In with ordered merging)
// =============================================================================

// Merge combines multiple channels maintaining rough ordering
func Merge(done <-chan struct{}, channels ...<-chan int) <-chan int {
	return FanIn(done, channels...)
}

// =============================================================================
// Channel Direction Examples
// =============================================================================

// SendOnly demonstrates send-only channel
func SendOnly(ch chan<- int, value int) {
	ch <- value
}

// ReceiveOnly demonstrates receive-only channel
func ReceiveOnly(ch <-chan int) int {
	return <-ch
}

// =============================================================================
// Done Channel Factory
// =============================================================================

// NewDone creates a done channel that closes after duration
func NewDone(duration time.Duration) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		time.Sleep(duration)
		close(done)
	}()
	return done
}

// =============================================================================
// Logging/Debugging Utilities
// =============================================================================

// Debug logs values passing through channel
func Debug(done <-chan struct{}, in <-chan int, prefix string) <-chan int {
	out := make(chan int)
	go func() {
		defer close(out)
		for v := range OrDone(done, in) {
			fmt.Printf("%s: %d\n", prefix, v)
			select {
			case out <- v:
			case <-done:
				return
			}
		}
	}()
	return out
}
