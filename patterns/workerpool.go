/*
Worker Pool Patterns
====================

Worker pool implementations for parallel task processing.

Applications:
- Concurrent task processing
- Load balancing
- Resource pooling
- Parallel computation
*/

package patterns

import (
	"sync"
)

// =============================================================================
// Fixed Worker Pool
// =============================================================================

// WorkerPool implements a fixed-size worker pool
type WorkerPool struct {
	numWorkers int
	jobs       chan Job
	results    chan Result
	done       chan struct{}
	wg         sync.WaitGroup
}

// NewWorkerPool creates a new worker pool
func NewWorkerPool(numWorkers int) *WorkerPool {
	return &WorkerPool{
		numWorkers: numWorkers,
		jobs:       make(chan Job, numWorkers*2),
		results:    make(chan Result, numWorkers*2),
		done:       make(chan struct{}),
	}
}

// Start starts the worker pool
func (wp *WorkerPool) Start(processFunc func(Job) Result) {
	for i := 0; i < wp.numWorkers; i++ {
		wp.wg.Add(1)
		go wp.worker(i, processFunc)
	}
}

// worker is the worker goroutine
func (wp *WorkerPool) worker(id int, processFunc func(Job) Result) {
	defer wp.wg.Done()
	for {
		select {
		case job, ok := <-wp.jobs:
			if !ok {
				return
			}
			result := processFunc(job)
			select {
			case wp.results <- result:
			case <-wp.done:
				return
			}
		case <-wp.done:
			return
		}
	}
}

// Submit submits a job to the pool
func (wp *WorkerPool) Submit(job Job) {
	select {
	case wp.jobs <- job:
	case <-wp.done:
	}
}

// Results returns the results channel
func (wp *WorkerPool) Results() <-chan Result {
	return wp.results
}

// Close closes the worker pool
func (wp *WorkerPool) Close() {
	close(wp.jobs)
	wp.wg.Wait()
	close(wp.results)
	close(wp.done)
}

// =============================================================================
// Dynamic Worker Pool
// =============================================================================

// DynamicPool adjusts number of workers based on load
type DynamicPool struct {
	minWorkers int
	maxWorkers int
	jobs       chan Job
	results    chan Result
	done       chan struct{}
	workers    map[int]chan struct{}
	mu         sync.Mutex
	wg         sync.WaitGroup
}

// NewDynamicPool creates a dynamic worker pool
func NewDynamicPool(minWorkers, maxWorkers int) *DynamicPool {
	dp := &DynamicPool{
		minWorkers: minWorkers,
		maxWorkers: maxWorkers,
		jobs:       make(chan Job, 100),
		results:    make(chan Result, 100),
		done:       make(chan struct{}),
		workers:    make(map[int]chan struct{}),
	}

	// Start minimum workers
	for i := 0; i < minWorkers; i++ {
		dp.addWorker(i)
	}

	return dp
}

func (dp *DynamicPool) addWorker(id int) {
	dp.mu.Lock()
	defer dp.mu.Unlock()

	workerDone := make(chan struct{})
	dp.workers[id] = workerDone
	dp.wg.Add(1)

	go func() {
		defer dp.wg.Done()
		for {
			select {
			case job := <-dp.jobs:
				// Process job (simplified)
				result := Result{JobID: job.ID, Value: job.Data}
				select {
				case dp.results <- result:
				case <-dp.done:
					return
				case <-workerDone:
					return
				}
			case <-dp.done:
				return
			case <-workerDone:
				return
			}
		}
	}()
}

// SubmitJob submits a job
func (dp *DynamicPool) SubmitJob(job Job) {
	dp.jobs <- job
}

// GetResults returns results channel
func (dp *DynamicPool) GetResults() <-chan Result {
	return dp.results
}

// Shutdown shuts down the pool
func (dp *DynamicPool) Shutdown() {
	close(dp.done)
	dp.wg.Wait()
	close(dp.results)
}

// =============================================================================
// Bounded Worker Pool
// =============================================================================

// BoundedWorkerPool limits concurrent work
type BoundedWorkerPool struct {
	semaphore chan struct{}
	wg        sync.WaitGroup
}

// NewBoundedWorkerPool creates a bounded pool
func NewBoundedWorkerPool(maxConcurrent int) *BoundedWorkerPool {
	return &BoundedWorkerPool{
		semaphore: make(chan struct{}, maxConcurrent),
	}
}

// Execute executes a function with concurrency limit
func (bwp *BoundedWorkerPool) Execute(fn func()) {
	bwp.wg.Add(1)
	bwp.semaphore <- struct{}{} // Acquire

	go func() {
		defer func() {
			<-bwp.semaphore // Release
			bwp.wg.Done()
		}()
		fn()
	}()
}

// Wait waits for all tasks to complete
func (bwp *BoundedWorkerPool) Wait() {
	bwp.wg.Wait()
}

// =============================================================================
// Task Queue Pattern
// =============================================================================

// TaskQueue manages a queue of tasks with workers
type TaskQueue struct {
	tasks   chan func()
	workers int
	wg      sync.WaitGroup
	done    chan struct{}
}

// NewTaskQueue creates a task queue
func NewTaskQueue(workers int, bufferSize int) *TaskQueue {
	tq := &TaskQueue{
		tasks:   make(chan func(), bufferSize),
		workers: workers,
		done:    make(chan struct{}),
	}

	// Start workers
	for i := 0; i < workers; i++ {
		tq.wg.Add(1)
		go tq.worker()
	}

	return tq
}

func (tq *TaskQueue) worker() {
	defer tq.wg.Done()
	for {
		select {
		case task, ok := <-tq.tasks:
			if !ok {
				return
			}
			task()
		case <-tq.done:
			return
		}
	}
}

// Submit submits a task
func (tq *TaskQueue) Submit(task func()) {
	select {
	case tq.tasks <- task:
	case <-tq.done:
	}
}

// Stop stops the queue
func (tq *TaskQueue) Stop() {
	close(tq.tasks)
	tq.wg.Wait()
}

// =============================================================================
// Pipeline Worker Pattern
// =============================================================================

// WorkerPipelineStage represents a stage in processing pipeline
type WorkerPipelineStage func(<-chan interface{}) <-chan interface{}

// WorkerPipeline creates a multi-stage processing pipeline
func WorkerPipeline(done <-chan struct{}, stages ...WorkerPipelineStage) WorkerPipelineStage {
	return func(in <-chan interface{}) <-chan interface{} {
		c := in
		for _, stage := range stages {
			c = stage(c)
		}
		return c
	}
}

// =============================================================================
// Work Stealing Pattern
// =============================================================================

// WorkStealingPool implements work stealing between workers
type WorkStealingPool struct {
	numWorkers int
	queues     []chan Job
	results    chan Result
	done       chan struct{}
	wg         sync.WaitGroup
}

// NewWorkStealingPool creates a work-stealing pool
func NewWorkStealingPool(numWorkers int) *WorkStealingPool {
	wsp := &WorkStealingPool{
		numWorkers: numWorkers,
		queues:     make([]chan Job, numWorkers),
		results:    make(chan Result, numWorkers*2),
		done:       make(chan struct{}),
	}

	for i := 0; i < numWorkers; i++ {
		wsp.queues[i] = make(chan Job, 10)
	}

	return wsp
}

// StartWorkers starts the workers
func (wsp *WorkStealingPool) StartWorkers(processFunc func(Job) Result) {
	for i := 0; i < wsp.numWorkers; i++ {
		wsp.wg.Add(1)
		go wsp.worker(i, processFunc)
	}
}

func (wsp *WorkStealingPool) worker(id int, processFunc func(Job) Result) {
	defer wsp.wg.Done()

	for {
		select {
		case job, ok := <-wsp.queues[id]:
			if !ok {
				return
			}
			result := processFunc(job)
			select {
			case wsp.results <- result:
			case <-wsp.done:
				return
			}
		case <-wsp.done:
			return
		default:
			// Try to steal work from other queues
			stolen := false
			for j := 0; j < wsp.numWorkers; j++ {
				if j == id {
					continue
				}
				select {
				case job, ok := <-wsp.queues[j]:
					if ok {
						result := processFunc(job)
						select {
						case wsp.results <- result:
						case <-wsp.done:
							return
						}
						stolen = true
					}
				default:
				}
				if stolen {
					break
				}
			}
		}
	}
}

// SubmitToWorker submits job to specific worker
func (wsp *WorkStealingPool) SubmitToWorker(workerID int, job Job) {
	select {
	case wsp.queues[workerID] <- job:
	case <-wsp.done:
	}
}

// GetResults returns results channel
func (wsp *WorkStealingPool) GetResults() <-chan Result {
	return wsp.results
}

// Shutdown shuts down the pool
func (wsp *WorkStealingPool) Shutdown() {
	close(wsp.done)
	for _, queue := range wsp.queues {
		close(queue)
	}
	wsp.wg.Wait()
	close(wsp.results)
}

// =============================================================================
// Priority Worker Pool
// =============================================================================

// PriorityJob extends Job with priority
type PriorityJob struct {
	Job
	Priority int
}

// PriorityWorkerPool processes jobs by priority
type PriorityWorkerPool struct {
	numWorkers int
	jobs       chan PriorityJob
	results    chan Result
	done       chan struct{}
	wg         sync.WaitGroup
}

// NewPriorityWorkerPool creates a priority-based pool
func NewPriorityWorkerPool(numWorkers int) *PriorityWorkerPool {
	return &PriorityWorkerPool{
		numWorkers: numWorkers,
		jobs:       make(chan PriorityJob, numWorkers*2),
		results:    make(chan Result, numWorkers*2),
		done:       make(chan struct{}),
	}
}

// StartPriorityWorkers starts workers
func (pwp *PriorityWorkerPool) StartPriorityWorkers(processFunc func(Job) Result) {
	for i := 0; i < pwp.numWorkers; i++ {
		pwp.wg.Add(1)
		go pwp.priorityWorker(processFunc)
	}
}

func (pwp *PriorityWorkerPool) priorityWorker(processFunc func(Job) Result) {
	defer pwp.wg.Done()
	for {
		select {
		case pJob, ok := <-pwp.jobs:
			if !ok {
				return
			}
			result := processFunc(pJob.Job)
			select {
			case pwp.results <- result:
			case <-pwp.done:
				return
			}
		case <-pwp.done:
			return
		}
	}
}

// SubmitPriority submits a priority job
func (pwp *PriorityWorkerPool) SubmitPriority(job PriorityJob) {
	select {
	case pwp.jobs <- job:
	case <-pwp.done:
	}
}

// GetPriorityResults returns results
func (pwp *PriorityWorkerPool) GetPriorityResults() <-chan Result {
	return pwp.results
}

// ClosePriority closes the pool
func (pwp *PriorityWorkerPool) ClosePriority() {
	close(pwp.jobs)
	pwp.wg.Wait()
	close(pwp.results)
}

// =============================================================================
// Helper Functions
// =============================================================================

// ProcessJobs is a helper to process jobs concurrently
func ProcessJobs(jobs []Job, numWorkers int, processFunc func(Job) Result) []Result {
	pool := NewWorkerPool(numWorkers)
	pool.Start(processFunc)

	// Submit all jobs
	go func() {
		for _, job := range jobs {
			pool.Submit(job)
		}
		pool.Close()
	}()

	// Collect results
	results := make([]Result, 0, len(jobs))
	for result := range pool.Results() {
		results = append(results, result)
	}

	return results
}
