/*
Error Group Patterns
====================

Error group patterns for parallel execution with coordinated error handling.

Applications:
- Parallel task execution
- Coordinated cancellation
- Error aggregation
- Resource cleanup
*/

package patterns

import (
	"context"
	"fmt"
	"sync"
)

// =============================================================================
// Basic Error Group
// =============================================================================

// ErrorGroup runs a group of goroutines and returns first error
type ErrorGroup struct {
	wg     sync.WaitGroup
	errMu  sync.Mutex
	err    error
	cancel func()
}

// NewErrorGroup creates a new error group
func NewErrorGroup() *ErrorGroup {
	return &ErrorGroup{}
}

// Go runs a function in a goroutine
func (eg *ErrorGroup) Go(fn func() error) {
	eg.wg.Add(1)
	go func() {
		defer eg.wg.Done()
		if err := fn(); err != nil {
			eg.errMu.Lock()
			if eg.err == nil {
				eg.err = err
				if eg.cancel != nil {
					eg.cancel()
				}
			}
			eg.errMu.Unlock()
		}
	}()
}

// Wait waits for all goroutines to complete
func (eg *ErrorGroup) Wait() error {
	eg.wg.Wait()
	return eg.err
}

// =============================================================================
// Context-Aware Error Group
// =============================================================================

// ContextErrorGroup is an error group with context cancellation
type ContextErrorGroup struct {
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	errMu  sync.Mutex
	err    error
}

// WithContext creates an error group with context
func WithContext(ctx context.Context) (*ContextErrorGroup, context.Context) {
	newCtx, cancel := context.WithCancel(ctx)
	return &ContextErrorGroup{
		ctx:    newCtx,
		cancel: cancel,
	}, newCtx
}

// Go runs a function with context
func (ceg *ContextErrorGroup) Go(fn func(context.Context) error) {
	ceg.wg.Add(1)
	go func() {
		defer ceg.wg.Done()
		if err := fn(ceg.ctx); err != nil {
			ceg.errMu.Lock()
			if ceg.err == nil {
				ceg.err = err
				ceg.cancel()
			}
			ceg.errMu.Unlock()
		}
	}()
}

// Wait waits for all goroutines and cancels context
func (ceg *ContextErrorGroup) Wait() error {
	ceg.wg.Wait()
	ceg.cancel()
	return ceg.err
}

// =============================================================================
// Multi-Error Group
// =============================================================================

// MultiErrorGroup collects all errors instead of just first
type MultiErrorGroup struct {
	wg     sync.WaitGroup
	errMu  sync.Mutex
	errors []error
}

// NewMultiErrorGroup creates a multi-error group
func NewMultiErrorGroup() *MultiErrorGroup {
	return &MultiErrorGroup{
		errors: make([]error, 0),
	}
}

// Go runs a function and collects errors
func (meg *MultiErrorGroup) Go(fn func() error) {
	meg.wg.Add(1)
	go func() {
		defer meg.wg.Done()
		if err := fn(); err != nil {
			meg.errMu.Lock()
			meg.errors = append(meg.errors, err)
			meg.errMu.Unlock()
		}
	}()
}

// Wait waits and returns all errors
func (meg *MultiErrorGroup) Wait() []error {
	meg.wg.Wait()
	meg.errMu.Lock()
	defer meg.errMu.Unlock()
	if len(meg.errors) == 0 {
		return nil
	}
	return meg.errors
}

// Error returns combined error message
func (meg *MultiErrorGroup) Error() error {
	errors := meg.Wait()
	if len(errors) == 0 {
		return nil
	}

	msg := fmt.Sprintf("%d errors occurred:", len(errors))
	for i, err := range errors {
		msg += fmt.Sprintf("\n  %d. %v", i+1, err)
	}
	return fmt.Errorf("%s", msg)
}

// =============================================================================
// Limited Concurrency Error Group
// =============================================================================

// LimitedErrorGroup limits concurrent goroutines
type LimitedErrorGroup struct {
	sem    chan struct{}
	wg     sync.WaitGroup
	errMu  sync.Mutex
	err    error
	cancel func()
}

// NewLimitedErrorGroup creates error group with concurrency limit
func NewLimitedErrorGroup(limit int) *LimitedErrorGroup {
	return &LimitedErrorGroup{
		sem: make(chan struct{}, limit),
	}
}

// Go runs function with concurrency limit
func (leg *LimitedErrorGroup) Go(fn func() error) {
	leg.wg.Add(1)
	leg.sem <- struct{}{} // Acquire

	go func() {
		defer func() {
			<-leg.sem // Release
			leg.wg.Done()
		}()

		if err := fn(); err != nil {
			leg.errMu.Lock()
			if leg.err == nil {
				leg.err = err
				if leg.cancel != nil {
					leg.cancel()
				}
			}
			leg.errMu.Unlock()
		}
	}()
}

// Wait waits for all goroutines
func (leg *LimitedErrorGroup) Wait() error {
	leg.wg.Wait()
	return leg.err
}

// SetCancelFunc sets cancel function
func (leg *LimitedErrorGroup) SetCancelFunc(cancel func()) {
	leg.cancel = cancel
}

// =============================================================================
// Typed Error Group
// =============================================================================

// TypedErrorGroup groups errors by type
type TypedErrorGroup struct {
	wg     sync.WaitGroup
	errMu  sync.Mutex
	errors map[string][]error
}

// NewTypedErrorGroup creates typed error group
func NewTypedErrorGroup() *TypedErrorGroup {
	return &TypedErrorGroup{
		errors: make(map[string][]error),
	}
}

// Go runs function with error type
func (teg *TypedErrorGroup) Go(errType string, fn func() error) {
	teg.wg.Add(1)
	go func() {
		defer teg.wg.Done()
		if err := fn(); err != nil {
			teg.errMu.Lock()
			teg.errors[errType] = append(teg.errors[errType], err)
			teg.errMu.Unlock()
		}
	}()
}

// Wait waits and returns errors by type
func (teg *TypedErrorGroup) Wait() map[string][]error {
	teg.wg.Wait()
	teg.errMu.Lock()
	defer teg.errMu.Unlock()
	if len(teg.errors) == 0 {
		return nil
	}
	return teg.errors
}

// HasErrors checks if any errors occurred
func (teg *TypedErrorGroup) HasErrors() bool {
	teg.errMu.Lock()
	defer teg.errMu.Unlock()
	return len(teg.errors) > 0
}

// ErrorsByType returns errors for specific type
func (teg *TypedErrorGroup) ErrorsByType(errType string) []error {
	teg.errMu.Lock()
	defer teg.errMu.Unlock()
	return teg.errors[errType]
}

// =============================================================================
// Pipeline Error Group
// =============================================================================

// PipelineStage represents a stage in error group pipeline
type PipelineStage struct {
	Name string
	Fn   func(context.Context) error
}

// PipelineErrorGroup executes stages in sequence
type PipelineErrorGroup struct {
	stages []PipelineStage
	ctx    context.Context
	cancel context.CancelFunc
}

// NewPipelineErrorGroup creates pipeline error group
func NewPipelineErrorGroup(ctx context.Context) *PipelineErrorGroup {
	newCtx, cancel := context.WithCancel(ctx)
	return &PipelineErrorGroup{
		stages: make([]PipelineStage, 0),
		ctx:    newCtx,
		cancel: cancel,
	}
}

// AddStage adds a pipeline stage
func (peg *PipelineErrorGroup) AddStage(name string, fn func(context.Context) error) {
	peg.stages = append(peg.stages, PipelineStage{Name: name, Fn: fn})
}

// Execute runs all stages in sequence
func (peg *PipelineErrorGroup) Execute() error {
	defer peg.cancel()

	for _, stage := range peg.stages {
		select {
		case <-peg.ctx.Done():
			return peg.ctx.Err()
		default:
			if err := stage.Fn(peg.ctx); err != nil {
				return fmt.Errorf("stage '%s' failed: %w", stage.Name, err)
			}
		}
	}
	return nil
}

// =============================================================================
// Result Error Group
// =============================================================================

// ErrorGroupResult holds result and error
type ErrorGroupResult struct {
	Value interface{}
	Error error
	Index int
}

// ResultErrorGroup collects results from parallel operations
type ResultErrorGroup struct {
	wg      sync.WaitGroup
	results []ErrorGroupResult
	mu      sync.Mutex
}

// NewResultErrorGroup creates result error group
func NewResultErrorGroup() *ResultErrorGroup {
	return &ResultErrorGroup{
		results: make([]ErrorGroupResult, 0),
	}
}

// Go executes function and collects result
func (reg *ResultErrorGroup) Go(index int, fn func() (interface{}, error)) {
	reg.wg.Add(1)
	go func() {
		defer reg.wg.Done()
		value, err := fn()
		reg.mu.Lock()
		reg.results = append(reg.results, ErrorGroupResult{
			Value: value,
			Error: err,
			Index: index,
		})
		reg.mu.Unlock()
	}()
}

// Wait waits and returns all results
func (reg *ResultErrorGroup) Wait() []ErrorGroupResult {
	reg.wg.Wait()
	reg.mu.Lock()
	defer reg.mu.Unlock()
	return reg.results
}

// GetSuccessful returns only successful results
func (reg *ResultErrorGroup) GetSuccessful() []ErrorGroupResult {
	results := reg.Wait()
	successful := make([]ErrorGroupResult, 0)
	for _, r := range results {
		if r.Error == nil {
			successful = append(successful, r)
		}
	}
	return successful
}

// GetErrors returns only errors
func (reg *ResultErrorGroup) GetErrors() []ErrorGroupResult {
	results := reg.Wait()
	errors := make([]ErrorGroupResult, 0)
	for _, r := range results {
		if r.Error != nil {
			errors = append(errors, r)
		}
	}
	return errors
}

// =============================================================================
// Retry Error Group
// =============================================================================

// RetryErrorGroup retries failed operations
type RetryErrorGroup struct {
	maxRetries int
	wg         sync.WaitGroup
	errMu      sync.Mutex
	err        error
}

// NewRetryErrorGroup creates retry error group
func NewRetryErrorGroup(maxRetries int) *RetryErrorGroup {
	return &RetryErrorGroup{
		maxRetries: maxRetries,
	}
}

// Go executes function with retries
func (reg *RetryErrorGroup) Go(fn func() error) {
	reg.wg.Add(1)
	go func() {
		defer reg.wg.Done()

		var lastErr error
		for attempt := 0; attempt <= reg.maxRetries; attempt++ {
			if err := fn(); err != nil {
				lastErr = err
				continue
			}
			return // Success
		}

		// All retries failed
		reg.errMu.Lock()
		if reg.err == nil {
			reg.err = fmt.Errorf("failed after %d retries: %w", reg.maxRetries, lastErr)
		}
		reg.errMu.Unlock()
	}()
}

// Wait waits for all operations
func (reg *RetryErrorGroup) Wait() error {
	reg.wg.Wait()
	return reg.err
}

// =============================================================================
// Batch Error Group
// =============================================================================

// BatchErrorGroup processes items in batches
type BatchErrorGroup struct {
	batchSize int
	eg        *MultiErrorGroup
}

// NewBatchErrorGroup creates batch error group
func NewBatchErrorGroup(batchSize int) *BatchErrorGroup {
	return &BatchErrorGroup{
		batchSize: batchSize,
		eg:        NewMultiErrorGroup(),
	}
}

// Process processes items in batches
func (beg *BatchErrorGroup) Process(items []interface{}, processFn func(interface{}) error) []error {
	for i := 0; i < len(items); i += beg.batchSize {
		end := i + beg.batchSize
		if end > len(items) {
			end = len(items)
		}

		batch := items[i:end]
		for _, item := range batch {
			item := item // Capture for closure
			beg.eg.Go(func() error {
				return processFn(item)
			})
		}
		beg.eg.Wait()
	}

	return beg.eg.Wait()
}

// =============================================================================
// Hierarchical Error Group
// =============================================================================

// HierarchicalErrorGroup supports nested error groups
type HierarchicalErrorGroup struct {
	parent   *HierarchicalErrorGroup
	children []*HierarchicalErrorGroup
	eg       *ErrorGroup
	name     string
	mu       sync.Mutex
}

// NewHierarchicalErrorGroup creates hierarchical error group
func NewHierarchicalErrorGroup(name string) *HierarchicalErrorGroup {
	return &HierarchicalErrorGroup{
		name:     name,
		eg:       NewErrorGroup(),
		children: make([]*HierarchicalErrorGroup, 0),
	}
}

// CreateChild creates child error group
func (heg *HierarchicalErrorGroup) CreateChild(name string) *HierarchicalErrorGroup {
	heg.mu.Lock()
	defer heg.mu.Unlock()

	child := NewHierarchicalErrorGroup(name)
	child.parent = heg
	heg.children = append(heg.children, child)
	return child
}

// Go runs function in this group
func (heg *HierarchicalErrorGroup) Go(fn func() error) {
	heg.eg.Go(fn)
}

// Wait waits for this group and all children
func (heg *HierarchicalErrorGroup) Wait() error {
	// Wait for children first
	for _, child := range heg.children {
		if err := child.Wait(); err != nil {
			return fmt.Errorf("child '%s' failed: %w", child.name, err)
		}
	}

	// Wait for this group
	if err := heg.eg.Wait(); err != nil {
		return fmt.Errorf("group '%s' failed: %w", heg.name, err)
	}

	return nil
}

// =============================================================================
// Helper Functions
// =============================================================================

// ParallelExecute executes functions in parallel with error handling
func ParallelExecute(fns ...func() error) error {
	eg := NewErrorGroup()
	for _, fn := range fns {
		fn := fn
		eg.Go(fn)
	}
	return eg.Wait()
}

// ParallelExecuteWithContext executes functions with context
func ParallelExecuteWithContext(ctx context.Context, fns ...func(context.Context) error) error {
	eg, ctx := WithContext(ctx)
	for _, fn := range fns {
		fn := fn
		eg.Go(fn)
	}
	return eg.Wait()
}

// ParallelMap applies function to all items in parallel
func ParallelMap(items []interface{}, fn func(interface{}) error) error {
	eg := NewErrorGroup()
	for _, item := range items {
		item := item
		eg.Go(func() error {
			return fn(item)
		})
	}
	return eg.Wait()
}

// ParallelMapWithLimit applies function with concurrency limit
func ParallelMapWithLimit(items []interface{}, limit int, fn func(interface{}) error) error {
	eg := NewLimitedErrorGroup(limit)
	for _, item := range items {
		item := item
		eg.Go(func() error {
			return fn(item)
		})
	}
	return eg.Wait()
}
