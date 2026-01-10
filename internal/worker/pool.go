// Package worker provides a generic worker pool implementation for concurrent task processing.
package worker

import (
	"context"
	"sync"
)

// Job represents a unit of work with an index for ordering.
type Job[T any] struct {
	Index int
	Data  T
}

// Result represents the outcome of processing a Job.
type Result[T any] struct {
	Index int
	Value T
	Err   error
}

// ProcessFunc processes a job and returns a result.
type ProcessFunc[I, O any] func(job Job[I]) (O, error)

// ProgressFunc is called after each job completes.
type ProgressFunc func(completed, total int)

// Pool manages concurrent job processing with a fixed number of workers.
type Pool[I, O any] struct {
	workers    int
	process    ProcessFunc[I, O]
	onProgress ProgressFunc
	jobChan    chan Job[I]
	resultChan chan Result[O]
	wg         sync.WaitGroup
	ctx        context.Context
	cancel     context.CancelFunc
}

// PoolOptions configures pool behavior.
type PoolOptions struct {
	Workers    int
	BufferSize int // If 0, defaults to Workers
}

// NewPool creates a new worker pool.
func NewPool[I, O any](opts PoolOptions, process ProcessFunc[I, O]) *Pool[I, O] {
	if opts.Workers <= 0 {
		opts.Workers = 1
	}
	if opts.BufferSize <= 0 {
		opts.BufferSize = opts.Workers
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Pool[I, O]{
		workers:    opts.Workers,
		process:    process,
		jobChan:    make(chan Job[I], opts.BufferSize),
		resultChan: make(chan Result[O], opts.BufferSize),
		ctx:        ctx,
		cancel:     cancel,
	}
}

// SetProgressCallback sets a callback to be called after each job completes.
func (p *Pool[I, O]) SetProgressCallback(fn ProgressFunc) {
	p.onProgress = fn
}

// Start begins the worker pool processing.
func (p *Pool[I, O]) Start() {
	for i := 0; i < p.workers; i++ {
		p.wg.Add(1)
		go p.worker()
	}
}

// worker is the goroutine that processes jobs.
func (p *Pool[I, O]) worker() {
	defer p.wg.Done()
	for {
		select {
		case <-p.ctx.Done():
			return
		case job, ok := <-p.jobChan:
			if !ok {
				return
			}
			result, err := p.process(job)
			p.resultChan <- Result[O]{
				Index: job.Index,
				Value: result,
				Err:   err,
			}
		}
	}
}

// Submit adds a job to the pool.
func (p *Pool[I, O]) Submit(job Job[I]) {
	p.jobChan <- job
}

// SubmitAll submits multiple jobs.
func (p *Pool[I, O]) SubmitAll(jobs []Job[I]) {
	for _, job := range jobs {
		p.Submit(job)
	}
}

// Close stops accepting new jobs.
func (p *Pool[I, O]) Close() {
	close(p.jobChan)
}

// Wait waits for all workers to complete and closes the results channel.
func (p *Pool[I, O]) Wait() {
	p.wg.Wait()
	close(p.resultChan)
}

// Results returns the results channel for reading.
func (p *Pool[I, O]) Results() <-chan Result[O] {
	return p.resultChan
}

// Cancel stops all workers immediately.
func (p *Pool[I, O]) Cancel() {
	p.cancel()
}

// Run is a convenience method that submits all jobs, starts workers,
// and collects results in order.
func (p *Pool[I, O]) Run(jobs []Job[I]) []Result[O] {
	total := len(jobs)
	results := make([]Result[O], total)

	// Start workers
	p.Start()

	// Submit all jobs and wait for completion in a goroutine
	go func() {
		p.SubmitAll(jobs)
		p.Close() // Close jobChan - workers will finish current jobs
		p.Wait()  // Wait for workers to finish, then close resultChan
	}()

	// Collect results
	completed := 0
	for result := range p.Results() {
		if result.Index >= 0 && result.Index < total {
			results[result.Index] = result
		}
		completed++
		if p.onProgress != nil {
			p.onProgress(completed, total)
		}
	}

	return results
}

// Process is a helper function that creates a pool, processes all jobs,
// and returns ordered results. This is the simplest way to use the pool.
func Process[I, O any](items []I, workers int, process ProcessFunc[I, O], onProgress ProgressFunc) ([]O, error) {
	if len(items) == 0 {
		return nil, nil
	}

	// Adjust workers if we have fewer items
	if workers > len(items) {
		workers = len(items)
	}

	// Create jobs
	jobs := make([]Job[I], len(items))
	for i, item := range items {
		jobs[i] = Job[I]{Index: i, Data: item}
	}

	// Create and run pool
	pool := NewPool[I, O](PoolOptions{Workers: workers, BufferSize: len(items)}, process)
	pool.SetProgressCallback(onProgress)
	results := pool.Run(jobs)

	// Extract values and check for errors
	output := make([]O, len(results))
	for i, result := range results {
		if result.Err != nil {
			return nil, result.Err
		}
		output[i] = result.Value
	}

	return output, nil
}

// ProcessWithErrors is like Process but collects all results even if some fail.
// Returns both successful results and any errors that occurred.
func ProcessWithErrors[I, O any](items []I, workers int, process ProcessFunc[I, O], onProgress ProgressFunc) ([]O, []error) {
	if len(items) == 0 {
		return nil, nil
	}

	if workers > len(items) {
		workers = len(items)
	}

	jobs := make([]Job[I], len(items))
	for i, item := range items {
		jobs[i] = Job[I]{Index: i, Data: item}
	}

	pool := NewPool[I, O](PoolOptions{Workers: workers, BufferSize: len(items)}, process)
	pool.SetProgressCallback(onProgress)
	results := pool.Run(jobs)

	output := make([]O, len(results))
	var errors []error
	for i, result := range results {
		output[i] = result.Value
		if result.Err != nil {
			errors = append(errors, result.Err)
		}
	}

	return output, errors
}
