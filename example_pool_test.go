package goroutines_test

import (
	"fmt"
	"time"

	"github.com/viney-shih/goroutines"
)

func ExampleNewPool_withFixedSize() {
	// allocate a pool with maximum size 5, and initialize all goroutines at the beginning.
	p := goroutines.NewPool(5)
	// don't forget to release the resource in the end
	defer p.Release()
}

func ExampleNewPool_withIncreasingSize() {
	// allocate a pool with maximum size 5, and initialize 2 goroutines.
	// if necessary, the number of goroutines increase to 5 and never go down.
	p := goroutines.NewPool(5, goroutines.WithPreAllocWorkers(2))
	// don't forget to release the resource in the end
	defer p.Release()
}

func ExampleNewPool_withAutoScaledSize() {
	// allocate a pool with maximum size 5, and initialize 2 goroutines.
	// if necessary, the number of goroutines increase to 5.
	// if not busy ( by checking the running status every 10 seconds ), the number goes to 2.
	p := goroutines.NewPool(
		5,
		goroutines.WithPreAllocWorkers(2),
		goroutines.WithWorkerAdjustPeriod(time.Duration(time.Second*10)),
	)
	// don't forget to release the resource in the end
	defer p.Release()
}

func ExampleNewPool_withFixedSizeAndQueues() {
	// allocate a pool with maximum size 5, and initialize all goroutines at the beginning.
	// at the same time, prepare a queue for buffering the tasks before sending to goroutines.
	p := goroutines.NewPool(5, goroutines.WithTaskQueueLength(2))
	// don't forget to release the resource in the end
	defer p.Release()
}

func ExamplePool_Schedule() {
	taskN := 7
	rets := make(chan int, taskN)

	// allocate a pool with 5 goroutines to deal with those tasks
	p := goroutines.NewPool(5)
	// don't forget to release the pool in the end
	defer p.Release()

	// assign tasks to asynchronous goroutine pool
	for i := 0; i < taskN; i++ {
		idx := i
		p.Schedule(func() {
			// sleep and return the index
			time.Sleep(20 * time.Millisecond)
			rets <- idx
		})
	}

	// wait until all tasks done
	for i := 0; i < taskN; i++ {
		fmt.Println("index:", <-rets)
	}

	// Unordered output:
	// index: 3
	// index: 1
	// index: 2
	// index: 4
	// index: 5
	// index: 6
	// index: 0
}

func ExamplePool_ScheduleWithTimeout() {
	totalN, taskN := 5, 5
	pause := make(chan struct{})
	rets := make(chan int, taskN)

	// allocate a pool with 5 goroutines to deal with those 5 tasks
	p := goroutines.NewPool(totalN)
	// don't forget to release the pool in the end
	defer p.Release()

	// full the workers which are stopped with the `pause`
	for i := 0; i < taskN; i++ {
		idx := i
		p.ScheduleWithTimeout(50*time.Millisecond, func() {
			<-pause
			rets <- idx
		})
	}

	// no more chance to add any task in Pool, and return `ErrScheduleTimeout`
	if err := p.ScheduleWithTimeout(50*time.Millisecond, func() {
		<-pause
		rets <- taskN
	}); err != nil {
		fmt.Println(err.Error())
	}

	close(pause)
	for i := 0; i < taskN; i++ {
		fmt.Println("index:", <-rets)
	}

	// Unordered output:
	// schedule timeout
	// index: 0
	// index: 3
	// index: 2
	// index: 4
	// index: 1
}
