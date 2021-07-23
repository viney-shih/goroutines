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

// Batch is the struct containing all Batch operations
type Batch struct {
	pool         *Pool
	inputChan    chan *input
	outputChan   chan Result
	retChan      chan Result
	completeChan chan struct{}
	inputOnce    sync.Once
	outputOnce   sync.Once
	resultOnce   sync.Once
	consumers    []*consumer
}

// NewBatch creates a asynchronous goroutine pool with the given size indicating
// total numbers of workers, and register consumers to deal with tasks past by producers.
func NewBatch(size int, options ...BatchOption) *Batch {
	// load options
	o := &batchOption{}
	for _, opt := range options {
		opt(o)
	}

	batchSize := defaultBatchSize
	if o.batchSize > batchSize {
		batchSize = o.batchSize
	}

	b := Batch{
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

// Queue plays as a producer to queue a task into pool, and
// starts processing immediately
// HINT: make sure not to call QueueComplete concurrently
func (b *Batch) Queue(fn BatchFunc) error {
	return b.queue(context.Background(), fn)
}

// QueueWithContext plays as a producer to queue a task into pool, or
// return ErrQueueCTXDone due to ctx is done (timeout or cancellation).
// HINT: make sure not to call QueueComplete concurrently
func (b *Batch) QueueWithContext(ctx context.Context, fn BatchFunc) error {
	return b.queue(ctx, fn)
}

// QueueComplete means finishing queuing tasks
// HINT: make sure not to call Queue concurrently
func (b *Batch) QueueComplete() {
	b.inputOnce.Do(func() {
		close(b.completeChan)
		close(b.inputChan)
	})
}

// Results returns a Result channel that will output all completed tasks.
func (b *Batch) Results() <-chan Result {
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

// WaitAll is an alternative to Results() where you may want/need to wait
// until all work has been processed, but don't need to check results.
func (b *Batch) WaitAll() {
	for range b.Results() {
	}
}

// Close will terminate all workers and close the job channel of this pool.
func (b *Batch) Close() {
	b.QueueComplete()
	b.WaitAll()
	b.terminateAll()
	b.pool.Release()
}

// GracefulClose will terminate all workers and close the job channel of this pool in the background.
func (b *Batch) GracefulClose() {
	go func() {
		b.Close()
	}()
}

func (b *Batch) queue(ctx context.Context, fn BatchFunc) error {
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

// terminateAll is going to terminate all consumers if possilbe
func (b *Batch) terminateAll() {
	for i := 0; i < len(b.consumers); i++ {
		b.consumers[i].stop()
	}
	for i := 0; i < len(b.consumers); i++ {
		b.consumers[i].join()
	}
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
