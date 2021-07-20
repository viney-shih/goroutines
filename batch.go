package goroutines

import (
	"context"
	"errors"
	"sync"
)

var (
	// ErrQueueComplete indicates no more incoming tasks allowed to put in the pool
	ErrQueueComplete = errors.New("queue has completed already")
	// ErrQueueCTXDone indicates context in queue is done due to timeout or cancellation.
	ErrQueueCTXDone = errors.New("context in queue is done")
)

// Result is the interface returned by Results()
type Result interface {
	// Value returns the value
	Value() interface{}
	// Error returns the error
	Error() error
}

// BatchFunc is the task function assigned by caller, running in the goroutine pool
type BatchFunc func() (interface{}, error)

// Batch is the interface containing all batch operations
type Batch interface {
	// Queue queues a task to be run in the pool and starts processing immediately
	Queue(fn BatchFunc) error
	// QueueWithContext queues a task to be run in the pool, or return ErrQueueCTXDone
	// because ctx is done (timeout or cancellation).
	QueueWithContext(ctx context.Context, fn BatchFunc) error
	// QueueComplete means finishing queuing tasks
	QueueComplete()
	// Results returns a Result channel that will output all
	// completed tasks
	Results() <-chan Result
	// WaitAll is an alternative to Results() where you may want/need to wait
	// until all work has been processed, but don't need to check results.
	WaitAll()
	// Close will terminate all workers and close the job channel of this pool
	Close()
	// GracefulClose will terminate all workers and close the job channel of this pool in the background
	GracefulClose()
}

// NewBatch creates a new asynchronous goroutine pool and related consumers
// with given size, and register the consumer at the same time.
func NewBatch(size int, options ...BatchOption) Batch {
	// load options
	o := &batchOption{}
	for _, opt := range options {
		opt(o)
	}

	batchSize := defaultBatchSize
	if o.batchSize > batchSize {
		batchSize = o.batchSize
	}

	b := batch{
		pool:         NewPool(size),
		inputChan:    make(chan *input, batchSize),
		outputChan:   make(chan Result, batchSize),
		completeChan: make(chan struct{}),
		retChan:      make(chan Result),
	}

	for i := 0; i < size; i++ {
		c := newConsumer()
		b.pool.Schedule(func() {
			defer close(c.closedChan)
			// listen on inputChan until it's closed or interrupted by others
			for {
				select {
				case <-c.closeChan:
					return
				default:
				}

				select {
				case <-c.closeChan:
					return
				case in, ok := <-b.inputChan:
					if !ok {
						return
					}
					intf, err := in.fn()
					ret := &result{value: intf, err: err}
					b.outputChan <- ret
				}
			}
		})

		b.consumers = append(b.consumers, c)
	}

	return &b
}

// batch includes a goroutine pool, several channels and mutex lock
type batch struct {
	pool         Pool
	inputChan    chan *input
	outputChan   chan Result
	retChan      chan Result
	completeChan chan struct{}
	inputOnce    sync.Once
	outputOnce   sync.Once
	resultOnce   sync.Once
	consumers    []*consumer
}

type input struct {
	fn BatchFunc
}

type result struct {
	value interface{}
	err   error
}

func (r *result) Value() interface{} {
	return r.value
}

func (r *result) Error() error {
	return r.err
}

// Queue plays as a producer to put a task into pool.
// HINT: make sure not to call QueueComplete concurrently
func (b *batch) Queue(fn BatchFunc) error {
	return b.queue(context.Background(), fn)
}

// QueueWithContext plays as a producer to put a task into pool.
// HINT: make sure not to call QueueComplete concurrently
func (b *batch) QueueWithContext(ctx context.Context, fn BatchFunc) error {
	return b.queue(ctx, fn)
}

func (b *batch) queue(ctx context.Context, fn BatchFunc) error {
	// The first try-receive operation is to try to exit the goroutine
	// as early as possible. It is not essential but a good practice to handle
	// racing case continue putting task in queue.
	select {
	case <-b.completeChan:
		return ErrQueueComplete
	default:
	}

	select {
	case <-b.completeChan:
		return ErrQueueComplete
	case <-ctx.Done():
		// timeout or cancellation
		return ErrQueueCTXDone
	default:
	}

	select {
	case <-b.completeChan:
		return ErrQueueComplete
	case <-ctx.Done():
		// timeout or cancellation
		return ErrQueueCTXDone
	case b.inputChan <- &input{fn: fn}:
		return nil
	}
}

// QueueComplete means finishing queuing tasks
// HINT: make sure not to call Queue concurrently
func (b *batch) QueueComplete() {
	b.inputOnce.Do(func() {
		close(b.completeChan)
		close(b.inputChan)
	})
}

// Results returns a Result channel that will output all
// completed tasks
func (b *batch) Results() <-chan Result {
	b.resultOnce.Do(func() {
		go func() {
			for i := 0; i < len(b.consumers); i++ {
				b.consumers[i].join()
			}
			b.outputOnce.Do(func() {
				close(b.outputChan)
			})
		}()

		go func() {
			for output := range b.outputChan {
				b.retChan <- output
			}
			close(b.retChan)
		}()
	})

	return b.retChan
}

// WaitAll is an alternative to Results() where you
// may want/need to wait until all work has been
// processed, but don't need to check results.
func (b *batch) WaitAll() {
	for range b.Results() {
	}
}

// terminateAll is going to terminate all consumers if possilbe
func (b *batch) terminateAll() {
	for i := 0; i < len(b.consumers); i++ {
		b.consumers[i].stop()
	}
	for i := 0; i < len(b.consumers); i++ {
		b.consumers[i].join()
	}
}

// Close will terminate all workers and close the job channel of this pool
func (b *batch) Close() {
	b.QueueComplete()
	b.terminateAll()
	b.WaitAll()
	b.pool.Release()
}

// GracefulClose will terminate all workers and close the job channel of this pool in the background
func (b *batch) GracefulClose() {
	go func() {
		b.Close()
	}()
}
