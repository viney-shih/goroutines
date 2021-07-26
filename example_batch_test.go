package goroutines_test

import (
	"fmt"
	"time"

	"github.com/viney-shih/goroutines"
)

func ExampleBatch_Queue_default() {
	taskN := 14

	// allocate a one-time batch job with 3 goroutines to deal with those tasks.
	// need to spawn an extra goroutine to prevent deadlocks.
	b := goroutines.NewBatch(3)
	// don't forget to close batch job in the end
	defer b.Close()

	// need extra goroutine to play as a producer
	go func() {
		for i := 0; i < taskN; i++ {
			num := i
			b.Queue(func() (interface{}, error) {
				// sleep and return the index
				time.Sleep(10 * time.Millisecond)
				return num, nil
			})
		}

		b.QueueComplete()
	}()

	for ret := range b.Results() {
		if ret.Error() != nil {
			panic("not expected")
		}

		fmt.Println("index:", ret.Value().(int))
	}

	// Unordered output:
	// index: 3
	// index: 1
	// index: 2
	// index: 4
	// index: 5
	// index: 6
	// index: 10
	// index: 7
	// index: 9
	// index: 8
	// index: 0
	// index: 11
	// index: 12
	// index: 13
}

func ExampleBatch_Queue_withBatchSize() {
	taskN := 11

	// allocate a one-time batch job with 3 goroutines to deal with those tasks.
	// no need to spawn extra goroutine by specifing the batch size consisting with the number of tasks.
	b := goroutines.NewBatch(3, goroutines.WithBatchSize(taskN))
	// don't forget to close batch job in the end
	defer b.Close()

	// pull all tasks to this batch queue
	for i := 0; i < taskN; i++ {
		idx := i
		b.Queue(func() (interface{}, error) {
			// sleep and return the index
			time.Sleep(10 * time.Millisecond)
			return idx, nil
		})
	}

	// tell the batch that's all need to do
	// DO NOT FORGET THIS OR GOROUTINES WILL DEADLOCK
	b.QueueComplete()

	for ret := range b.Results() {
		if ret.Error() != nil {
			panic("not expected")
		}

		fmt.Println("index:", ret.Value().(int))
	}

	// Unordered output:
	// index: 3
	// index: 1
	// index: 2
	// index: 4
	// index: 5
	// index: 6
	// index: 10
	// index: 7
	// index: 9
	// index: 8
	// index: 0
}
