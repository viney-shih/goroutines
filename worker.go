package goroutines

import (
	"errors"
	"sync"
	"sync/atomic"
	"unsafe"
)

var (
	// ErrStateCorrupted indicates the worker state is corrupted.
	ErrStateCorrupted = errors.New("state corrupted")
)

type workerState int32

const (
	wStatUndefined workerState = iota - 1 // -1
	wStatInit                             // 0
	wStatRunning                          // 1
	wStatStopped                          // 2
)

func getState(n int32) workerState {
	switch st := workerState(n); {
	case st == wStatInit:
		fallthrough
	case st == wStatRunning:
		fallthrough
	case st == wStatStopped:
		return st
	default:
		// actually, it should not happened.
		return wStatUndefined
	}
}

func (w *worker) getState() workerState {
	n := atomic.LoadInt32((*int32)(unsafe.Pointer(&w.state)))

	return getState(n)
}

type worker struct {
	taskQueueChan <-chan TaskFunc
	workerPool    *sync.Pool
	metric        Metric

	stopChan chan struct{}
	state    workerState
}

func newWorker(taskQueue <-chan TaskFunc, workerPool *sync.Pool, metric Metric) *worker {
	return &worker{
		taskQueueChan: taskQueue,
		workerPool:    workerPool,
		metric:        metric,
		stopChan:      make(chan struct{}),
		state:         wStatInit,
	}
}

func (w *worker) run(task TaskFunc) {
	n := atomic.AddInt32((*int32)(unsafe.Pointer(&w.state)), 1) // wStatInit -> wStatRunning
	if getState(n) != wStatRunning {
		panic(ErrStateCorrupted)
	}

	go func() {
		defer func() {
			atomic.AddInt32((*int32)(unsafe.Pointer(&w.state)), 1) // wStatRunning -> wStatStopped
			close(w.stopChan)
		}()

		w.metric.IncBusyWorker()
		task()
		w.metric.DecBusyWorker()

		for {
			select {
			case <-w.stopChan:
				return
			default:
			}

			select {
			case <-w.stopChan:
				return
			case taskInQueue := <-w.taskQueueChan:
				w.metric.IncBusyWorker()
				taskInQueue()
				w.metric.DecBusyWorker()
			}
		}
	}()
}

func (w *worker) stop() {
	switch w.getState() {
	case wStatInit:
		// do nothing. there is no goroutine.
	case wStatRunning:
		// trigger stopping
		w.stopChan <- struct{}{}
	default:
		panic(ErrStateCorrupted)
	}
}

func (w *worker) join() {
	switch w.getState() {
	case wStatInit:
		// recycle it directly
		w.workerPool.Put(w)
	case wStatRunning, wStatStopped:
		// wait for response
		<-w.stopChan

		// reset to default
		atomic.StoreInt32((*int32)(unsafe.Pointer(&w.state)), int32(wStatInit))
		w.stopChan = make(chan struct{})
		w.workerPool.Put(w)
	default:
		panic(ErrStateCorrupted)
	}
}
