package goroutines

import (
	"errors"
	"time"
)

const (
	// pool
	defaultPreAllocWorkers = -1
	defaultAdjustPeriod    = 0
	// batch
	defaultBatchSize = 10
)

// PoolOption is an alias for functional argument.
type PoolOption func(opts *poolOption)

// poolOption contains all options which will be applied when initializing a pool.
type poolOption struct {
	// taskQueueLength indicates the length of task queue.
	// 0 (default value) means pure pubsub model without buffers.
	taskQueueLength int
	// preAllocWorkers indicates the number of workers to spawn when initializing Pool.
	preAllocWorkers int
	// workerAdjustPeriod indicates the duration to adjust the worker size.
	// 0 (default value) means no need to adjust worker size.
	workerAdjustPeriod time.Duration
}

func loadPoolOption(options ...PoolOption) *poolOption {
	opts := &poolOption{
		preAllocWorkers:    defaultPreAllocWorkers,
		workerAdjustPeriod: defaultAdjustPeriod,
	}
	for _, option := range options {
		option(opts)
	}

	return opts
}

// WithTaskQueueLength sets up the length of task queue.
func WithTaskQueueLength(length int) PoolOption {
	if length < 0 {
		panic(errors.New("the length of task queue must be greater than or equal to zero"))
	}

	return func(opts *poolOption) {
		opts.taskQueueLength = length
	}
}

// WithPreAllocWorkers sets up the number of workers to spawn when initializing Pool.
func WithPreAllocWorkers(size int) PoolOption {
	if size < 0 {
		panic(errors.New("the pre-allocated number of workers must be greater than or equal to zero"))
	}

	return func(opts *poolOption) {
		opts.preAllocWorkers = size
	}
}

// WithWorkerAdjustPeriod sets up the duration to adjust the worker size.
func WithWorkerAdjustPeriod(period time.Duration) PoolOption {
	if period <= 0 {
		panic(errors.New("the period of adjusting workers must be greater than zero"))
	}

	return func(opts *poolOption) {
		opts.workerAdjustPeriod = period
	}
}

// BatchOption is an alias for functional argument in Batch
type BatchOption func(*batchOption)

type batchOption struct {
	batchSize int
}

// WithBatchSize specifies the batch size used to forward tasks. If it is bigger
// enough, no more need to fork another goroutine to trigger Queue()
// defaultBatchSize is 10.
func WithBatchSize(size int) BatchOption {
	return func(o *batchOption) {
		o.batchSize = size
	}
}
