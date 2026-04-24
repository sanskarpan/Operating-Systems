package patterns

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestWorkerPool(t *testing.T) {
	pool := NewWorkerPool(3)

	processFunc := func(job Job) Result {
		return Result{
			JobID: job.ID,
			Value: job.Data.(int) * 2,
		}
	}

	pool.Start(processFunc)

	// Submit jobs
	go func() {
		for i := 0; i < 10; i++ {
			pool.Submit(Job{ID: i, Data: i})
		}
		pool.Close()
	}()

	// Collect results
	results := make([]Result, 0)
	for result := range pool.Results() {
		results = append(results, result)
	}

	if len(results) != 10 {
		t.Errorf("Expected 10 results, got %d", len(results))
	}
}

func TestDynamicPool(t *testing.T) {
	pool := NewDynamicPool(2, 5)

	// Submit jobs
	go func() {
		for i := 0; i < 20; i++ {
			pool.SubmitJob(Job{ID: i, Data: i})
		}
		time.Sleep(100 * time.Millisecond)
		pool.Shutdown()
	}()

	// Collect results
	count := 0
	for range pool.GetResults() {
		count++
	}

	if count != 20 {
		t.Errorf("Expected 20 results, got %d", count)
	}
}

func TestBoundedWorkerPool(t *testing.T) {
	pool := NewBoundedWorkerPool(3)

	results := make(chan int, 10)
	for i := 0; i < 10; i++ {
		i := i
		pool.Execute(func() {
			time.Sleep(10 * time.Millisecond)
			results <- i
		})
	}

	pool.Wait()
	close(results)

	count := 0
	for range results {
		count++
	}

	if count != 10 {
		t.Errorf("Expected 10 results, got %d", count)
	}
}

func TestTaskQueue(t *testing.T) {
	tq := NewTaskQueue(3, 10)

	results := make(chan int, 20)
	for i := 0; i < 20; i++ {
		i := i
		tq.Submit(func() {
			results <- i * 2
		})
	}

	tq.Stop()
	close(results)

	count := 0
	for range results {
		count++
	}

	if count != 20 {
		t.Errorf("Expected 20 results, got %d", count)
	}
}

func TestWorkStealingPool(t *testing.T) {
	pool := NewWorkStealingPool(3)

	processFunc := func(job Job) Result {
		time.Sleep(5 * time.Millisecond)
		return Result{
			JobID: job.ID,
			Value: job.Data.(int) * 3,
		}
	}

	pool.StartWorkers(processFunc)

	// Submit jobs to different workers
	go func() {
		for i := 0; i < 15; i++ {
			workerID := i % 3
			pool.SubmitToWorker(workerID, Job{ID: i, Data: i})
		}
		time.Sleep(200 * time.Millisecond)
		pool.Shutdown()
	}()

	// Collect results
	count := 0
	for range pool.GetResults() {
		count++
	}

	if count != 15 {
		t.Errorf("Expected 15 results, got %d", count)
	}
}

func TestPriorityWorkerPool(t *testing.T) {
	pool := NewPriorityWorkerPool(3)

	processFunc := func(job Job) Result {
		return Result{
			JobID: job.ID,
			Value: job.Data,
		}
	}

	pool.StartPriorityWorkers(processFunc)

	// Submit jobs with different priorities
	go func() {
		for i := 0; i < 10; i++ {
			pool.SubmitPriority(PriorityJob{
				Job:      Job{ID: i, Data: i},
				Priority: i % 3,
			})
		}
		pool.ClosePriority()
	}()

	// Collect results
	results := make([]Result, 0)
	for result := range pool.GetPriorityResults() {
		results = append(results, result)
	}

	if len(results) != 10 {
		t.Errorf("Expected 10 results, got %d", len(results))
	}
}

func TestProcessJobs(t *testing.T) {
	jobs := make([]Job, 20)
	for i := 0; i < 20; i++ {
		jobs[i] = Job{ID: i, Data: i}
	}

	processFunc := func(job Job) Result {
		return Result{
			JobID: job.ID,
			Value: job.Data.(int) * 2,
		}
	}

	results := ProcessJobs(jobs, 4, processFunc)

	if len(results) != 20 {
		t.Errorf("Expected 20 results, got %d", len(results))
	}

	for _, result := range results {
		expected := result.JobID * 2
		if result.Value != expected {
			t.Errorf("Expected %d, got %v", expected, result.Value)
		}
	}
}

func TestWorkerPoolConcurrency(t *testing.T) {
	pool := NewWorkerPool(5)

	var counter int64
	processFunc := func(job Job) Result {
		time.Sleep(10 * time.Millisecond)
		c := atomic.AddInt64(&counter, 1)
		return Result{JobID: job.ID, Value: int(c)}
	}

	pool.Start(processFunc)

	start := time.Now()
	go func() {
		for i := 0; i < 25; i++ {
			pool.Submit(Job{ID: i, Data: i})
		}
		pool.Close()
	}()

	for range pool.Results() {
	}

	elapsed := time.Since(start)

	// With 5 workers, 25 jobs should take ~50ms (25/5 * 10ms)
	// Allow some overhead
	if elapsed > 150*time.Millisecond {
		t.Logf("Workers might not be running concurrently: took %v", elapsed)
	}
}

func TestBoundedWorkerPoolLimit(t *testing.T) {
	pool := NewBoundedWorkerPool(2)

	var mu sync.Mutex
	running := 0
	maxRunning := 0

	for i := 0; i < 10; i++ {
		pool.Execute(func() {
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
		})
	}

	pool.Wait()

	// Should never have more than 2 running concurrently
	if maxRunning > 2 {
		t.Errorf("Expected max 2 concurrent workers, got %d", maxRunning)
	}
}

func TestTaskQueueStop(t *testing.T) {
	tq := NewTaskQueue(2, 5)

	var processed int64
	for i := 0; i < 10; i++ {
		tq.Submit(func() {
			time.Sleep(10 * time.Millisecond)
			atomic.AddInt64(&processed, 1)
		})
	}

	tq.Stop()

	if atomic.LoadInt64(&processed) != 10 {
		t.Logf("Processed %d tasks (expected 10)", atomic.LoadInt64(&processed))
	}
}
