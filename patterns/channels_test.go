package patterns

import (
	"sync"
	"testing"
	"time"
)

func TestGenerator(t *testing.T) {
	done := make(chan struct{})
	defer close(done)

	values := []int{1, 2, 3, 4, 5}
	out := Generator(done, values...)

	received := make([]int, 0)
	for v := range out {
		received = append(received, v)
	}

	if len(received) != len(values) {
		t.Errorf("Expected %d values, got %d", len(values), len(received))
	}

	for i, v := range received {
		if v != values[i] {
			t.Errorf("Expected %d at index %d, got %d", values[i], i, v)
		}
	}
}

func TestPipeline(t *testing.T) {
	done := make(chan struct{})
	defer close(done)

	in := Generator(done, 1, 2, 3, 4, 5)
	double := func(n int) int { return n * 2 }
	out := Pipeline(done, in, double)

	expected := []int{2, 4, 6, 8, 10}
	i := 0
	for v := range out {
		if v != expected[i] {
			t.Errorf("Expected %d, got %d", expected[i], v)
		}
		i++
	}
}

func TestFanOutFanIn(t *testing.T) {
	done := make(chan struct{})
	defer close(done)

	in := Generator(done, 1, 2, 3, 4, 5)
	square := func(n int) int { return n * n }

	channels := FanOut(done, in, 3, square)
	out := FanIn(done, channels...)

	results := make(map[int]bool)
	for v := range out {
		results[v] = true
	}

	expected := []int{1, 4, 9, 16, 25}
	for _, exp := range expected {
		if !results[exp] {
			t.Errorf("Expected to find %d in results", exp)
		}
	}
}

func TestOrDone(t *testing.T) {
	done := make(chan struct{})
	in := make(chan int)

	go func() {
		for i := 0; i < 100; i++ {
			select {
			case <-done:
				close(in)
				return
			case in <- i:
				time.Sleep(time.Millisecond)
			}
		}
		close(in)
	}()

	// Close done early
	go func() {
		time.Sleep(10 * time.Millisecond)
		close(done)
	}()

	count := 0
	for range OrDone(done, in) {
		count++
	}

	// Should exit early due to done signal
	if count >= 100 {
		t.Error("Expected to exit before processing all values")
	}
}

func TestTee(t *testing.T) {
	done := make(chan struct{})
	defer close(done)

	in := Generator(done, 1, 2, 3)
	out1, out2 := Tee(done, in)

	results1 := make([]int, 0)
	results2 := make([]int, 0)

	// Use WaitGroup so the main goroutine waits for the collector before
	// reading results1, eliminating the data race.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for v := range out1 {
			results1 = append(results1, v)
		}
	}()

	for v := range out2 {
		results2 = append(results2, v)
	}

	wg.Wait()

	if len(results1) != 3 || len(results2) != 3 {
		t.Errorf("Expected 3 values in each output, got %d and %d", len(results1), len(results2))
	}
}

func TestBridge(t *testing.T) {
	done := make(chan struct{})
	defer close(done)

	chanStream := make(chan (<-chan int))

	go func() {
		defer close(chanStream)
		for i := 0; i < 3; i++ {
			ch := make(chan int)
			chanStream <- ch
			go func(ch chan int) {
				defer close(ch)
				for j := 0; j < 3; j++ {
					ch <- j
				}
			}(ch)
		}
	}()

	out := Bridge(done, chanStream)
	count := 0
	for range out {
		count++
	}

	if count != 9 {
		t.Errorf("Expected 9 values, got %d", count)
	}
}

func TestOr(t *testing.T) {
	sig1 := make(chan struct{})
	sig2 := make(chan struct{})
	sig3 := make(chan struct{})

	orDone := Or(sig1, sig2, sig3)

	// Close one signal
	close(sig2)

	select {
	case <-orDone:
		// Success - orDone closed
	case <-time.After(100 * time.Millisecond):
		t.Error("Expected orDone to close")
	}
}

func TestBatch(t *testing.T) {
	done := make(chan struct{})
	defer close(done)

	in := Generator(done, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10)
	batches := Batch(done, in, 3)

	batchCount := 0
	for batch := range batches {
		batchCount++
		if batchCount < 4 && len(batch) != 3 {
			t.Errorf("Expected batch size 3, got %d", len(batch))
		}
	}

	if batchCount != 4 {
		t.Errorf("Expected 4 batches, got %d", batchCount)
	}
}

func TestDebounce(t *testing.T) {
	done := make(chan struct{})
	defer close(done)

	in := make(chan int)
	out := Debounce(done, in, 50*time.Millisecond)

	go func() {
		in <- 1
		time.Sleep(10 * time.Millisecond)
		in <- 2
		time.Sleep(10 * time.Millisecond)
		in <- 3
		time.Sleep(100 * time.Millisecond)
		close(in)
	}()

	results := make([]int, 0)
	for v := range out {
		results = append(results, v)
	}

	// Should only emit last value after silence
	if len(results) != 1 || results[0] != 3 {
		t.Errorf("Expected [3], got %v", results)
	}
}

func TestThrottle(t *testing.T) {
	done := make(chan struct{})
	defer close(done)

	in := make(chan int, 10)
	for i := 0; i < 10; i++ {
		in <- i
	}
	close(in)

	out := Throttle(done, in, 20*time.Millisecond)

	start := time.Now()
	count := 0
	for range out {
		count++
	}
	elapsed := time.Since(start)

	// Should throttle to ~50/sec, so 10 items should take ~200ms
	if elapsed < 100*time.Millisecond {
		t.Errorf("Throttle didn't limit rate, took only %v", elapsed)
	}
}

func TestTake(t *testing.T) {
	done := make(chan struct{})
	defer close(done)

	in := Generator(done, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10)
	out := Take(done, in, 5)

	count := 0
	for range out {
		count++
	}

	if count != 5 {
		t.Errorf("Expected 5 values, got %d", count)
	}
}

func TestRepeat(t *testing.T) {
	done := make(chan struct{})

	out := Repeat(done, 1, 2, 3)

	count := 0
	for v := range out {
		count++
		if count == 10 {
			close(done)
			break
		}
		expected := ((count - 1) % 3) + 1
		if v != expected {
			t.Errorf("Expected %d, got %d", expected, v)
		}
	}
}

func TestRepeatFn(t *testing.T) {
	done := make(chan struct{})

	counter := 0
	fn := func() int {
		counter++
		return counter
	}

	out := RepeatFn(done, fn)

	count := 0
	for v := range out {
		count++
		if count == 5 {
			close(done)
			break
		}
		if v != count {
			t.Errorf("Expected %d, got %d", count, v)
		}
	}
}

func TestBufferedGenerator(t *testing.T) {
	out := BufferedGenerator(5, 1, 2, 3, 4, 5)

	received := make([]int, 0)
	for v := range out {
		received = append(received, v)
	}

	if len(received) != 5 {
		t.Errorf("Expected 5 values, got %d", len(received))
	}
}

func TestTimeoutSelect(t *testing.T) {
	ch := make(chan int)

	// Test timeout
	_, ok := TimeoutSelect(ch, 10*time.Millisecond)
	if ok {
		t.Error("Expected timeout")
	}

	// Test success
	go func() {
		time.Sleep(5 * time.Millisecond)
		ch <- 42
	}()

	v, ok := TimeoutSelect(ch, 50*time.Millisecond)
	if !ok || v != 42 {
		t.Errorf("Expected 42, got %d (ok=%v)", v, ok)
	}
}

func TestMerge(t *testing.T) {
	done := make(chan struct{})
	defer close(done)

	ch1 := Generator(done, 1, 2, 3)
	ch2 := Generator(done, 4, 5, 6)
	ch3 := Generator(done, 7, 8, 9)

	out := Merge(done, ch1, ch2, ch3)

	results := make(map[int]bool)
	for v := range out {
		results[v] = true
	}

	if len(results) != 9 {
		t.Errorf("Expected 9 unique values, got %d", len(results))
	}
}

func TestNewDone(t *testing.T) {
	start := time.Now()
	done := NewDone(50 * time.Millisecond)
	<-done
	elapsed := time.Since(start)

	if elapsed < 50*time.Millisecond {
		t.Errorf("Done channel closed too early: %v", elapsed)
	}
}
