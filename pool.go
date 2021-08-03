package goroutines

import (
	"context"
	"errors"
	"sync"
	"time"
)

var (
	// ErrPoolRelease indicates the pool is released and closed.
	ErrPoolRelease = errors.New("pool released")
	// ErrScheduleTimeout indicates there is no resource to handle this task within specified period.
	ErrScheduleTimeout = errors.New("schedule timeout")
)

// TaskFunc is the task function assigned by caller, running in the goroutine pool
type TaskFunc func()

// Pool is the struct handling the interacetion with asynchronous goroutines
type Pool struct {
	initN         int
	totalN        int
	scalable      bool
	taskQueueChan chan TaskFunc
	workerChan    chan *worker
	wg            sync.WaitGroup
	stopOnce      sync.Once
	stopChan      chan struct{}

	workerPool sync.Pool
	workers    []*worker
	workerMut  sync.Mutex
	workerCond *sync.Cond

	metric Metric
}

// NewPool creates an instance of asynchronously goroutine pool
// with the given size which indicates total numbers of workers.
func NewPool(size int, options ...PoolOption) *Pool {
	// load options
	o := loadPoolOption(options...)

	if size <= 0 {
		panic(errors.New("the total number of workers must be greater than zero"))
	}
	if o.preAllocWorkers == defaultPreAllocWorkers {
		o.preAllocWorkers = size
	}
	if o.preAllocWorkers > size {
		panic(errors.New("the number of pre-allocated workers must be less than or equal to total"))
	}

	p := Pool{
		initN:         o.preAllocWorkers,
		totalN:        size,
		scalable:      o.preAllocWorkers != size, // needs helper goroutines (Miner/Recycler) to adjust the worker size
		taskQueueChan: make(chan TaskFunc, o.taskQueueLength),
		workerChan:    make(chan *worker),
		stopChan:      make(chan struct{}),
		metric:        newMetric(),
	}

	p.workerPool.New = func() interface{} {
		return newWorker(p.taskQueueChan, &p.workerPool, p.metric)
	}

	// pre-allocate workers
	p.adjustWorkerSize(p.initN, false)

	p.workerCond = sync.NewCond(&p.workerMut)

	// init helper goroutines (Miner/Recycler) if necessary
	if p.scalable {
		p.startMiner()
		if o.workerAdjustPeriod != defaultAdjustPeriod {
			p.startRecycler(o.workerAdjustPeriod)
		}
	}

	return &p
}

// Schedule schedules the task executed by worker (goroutines) in the Pool.
// It will be blocked until the works accepting the request.
func (p *Pool) Schedule(task TaskFunc) error {
	return p.schedule(context.Background(), task)
}

// ScheduleWithTimeout schedules the task executed by worker (goroutines)
// in the Pool within the specified period. Or return ErrScheduleTimeout.
func (p *Pool) ScheduleWithTimeout(timeout time.Duration, task TaskFunc) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	return p.schedule(ctx, task)
}

// ScheduleWithContext schedules the task executed by worker (goroutines)
// in the Pool. It will be blocked until works accepting the request, or
// return ErrScheduleTimeout because ctx is done (timeout or cancellation).
func (p *Pool) ScheduleWithContext(ctx context.Context, task TaskFunc) error {
	return p.schedule(ctx, task)
}

// Release will terminate all workers, and force them finishing
// what they are working on ASAP.
func (p *Pool) Release() {
	// only allow calling Release() once
	p.stopOnce.Do(func() {
		close(p.stopChan)
		// trigger Minor waking up again
		p.workerCond.Broadcast()
		// wait for helper goroutines (Minor / Recycler) to stop
		p.wg.Wait()
		// wait for goroutines in Pool to stop
		p.adjustWorkerSize(0, true)
	})
}

// Workers returns the numbers of workers created.
func (p *Pool) Workers() int {
	p.workerMut.Lock()
	defer p.workerMut.Unlock()

	return len(p.workers)
}

// Running returns the number of workers running for tasks.
func (p *Pool) Running() int {
	return int(p.metric.BusyWorkers())
}

// force stands for forcing adjusting the size, only true in Release()
func (p *Pool) adjustWorkerSize(n int, force bool) {
	p.workerMut.Lock()
	defer p.workerMut.Unlock()

	workerN := len(p.workers)
	if n == workerN {
		return
	} else if n > p.totalN {
		n = p.totalN
	} else if n == 0 && !force && workerN > 0 && p.workers[0].getState() == wStatInit {
		// keep Minor alive when adjustWorkerSize() is triggered by Recycler
		n = 1
	}

	for i := workerN; i < n; i++ {
		w := p.workerPool.Get().(*worker)
		p.workers = append(p.workers, w)
		w.run(func() {})
	}

	for i := n; i < workerN; i++ {
		p.workers[i].stop()
	}

	for i := n; i < workerN; i++ {
		p.workers[i].join()
		// prevent it from memory leak
		p.workers[i] = nil
	}

	p.workers = p.workers[:n]
}

func (p *Pool) allocWorker() *worker {
	p.workerMut.Lock()
	defer p.workerMut.Unlock()

	// Check availability
	for {
		select {
		case <-p.stopChan:
			return nil
		default:
		}

		if len(p.workers) < p.totalN {
			break
		}

		// wait for recycler reducing workers
		p.workerCond.Wait()
	}

	w := p.workerPool.Get().(*worker)
	// prepend to p.workers
	p.workers = append(p.workers, nil)
	copy(p.workers[1:], p.workers)
	p.workers[0] = w

	return w
}

func (p *Pool) schedule(ctx context.Context, task TaskFunc) error {
	select {
	case <-p.stopChan:
		return ErrPoolRelease
	default:
	}

	// queue first strategy
	select {
	case <-p.stopChan:
		return ErrPoolRelease
	case <-ctx.Done():
		// timeout or cancellation
		return ErrScheduleTimeout
	case p.taskQueueChan <- task:
		return nil
	default:
	}

	select {
	case <-p.stopChan:
		return ErrPoolRelease
	case <-ctx.Done():
		// timeout or cancellation
		return ErrScheduleTimeout
	case p.taskQueueChan <- task:
		return nil
	case w := <-p.workerChan:
		select {
		case <-p.stopChan:
			return ErrPoolRelease
		default:
			w.run(task)
			return nil
		}
	}
}

func (p *Pool) startMiner() {
	p.wg.Add(1)

	go func() {
		defer p.wg.Done()

		for {
			w := p.allocWorker()
			if w == nil {
				// stopChan is closed
				return
			}

			select {
			case p.workerChan <- w:
				// retrieve worker, and send to those in need
			case <-p.stopChan:
				return
			}
		}

	}()
}

func max(values []uint64) (max uint64) {
	for i, v := range values {
		if i == 0 || v > max {
			max = v
		}
	}

	return max
}

func (p *Pool) startRecycler(period time.Duration) {
	p.wg.Add(1)
	ticker := time.NewTicker(period)
	windows := []uint64{0, 0, 0}

	go func() {
		defer p.wg.Done()

		for {
			select {
			case <-ticker.C:
				// append to windows with the latest number of busy workers
				copy(windows, windows[1:])
				windows[len(windows)-1] = p.metric.BusyWorkers()

				// shrink the number of workers based on history, and keep at least one alive for Minor
				p.adjustWorkerSize(int(max(windows)), false)

				// trigger Minor waking up again
				p.workerCond.Broadcast()
			case <-p.stopChan:
				ticker.Stop()
				return
			}
		}
	}()
}
