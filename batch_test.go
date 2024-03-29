package goroutines

import (
	"context"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

type batchSuite struct {
	suite.Suite
}

func (s *batchSuite) SetupSuite() {}

func (s *batchSuite) TearDownSuite() {}

func (s *batchSuite) SetupTest() {}

func (s *batchSuite) TearDownTest() {}

func TestBatchSuite(t *testing.T) {
	suite.Run(t, new(batchSuite))
}

func (s *batchSuite) TestQueueAndResults() {
	b := NewBatch(3)
	defer b.Close()

	testN := 1000
	expResults := []int{}
	// need goroutine to play as a producer
	go func() {
		for i := 0; i < testN; i++ {
			num := i
			expResults = append(expResults, num)
			b.Queue(func() (interface{}, error) {
				return num, nil
			})
		}

		b.QueueComplete()
	}()

	results := []int{}
	for ret := range b.Results() {
		s.Require().NoError(ret.Error())
		results = append(results, ret.Value().(int))
	}

	sort.Ints(results)
	s.Require().Equal(expResults, results)
}

func (s *batchSuite) TestQueueAndWaitAll() {
	b := NewBatch(3)
	defer b.Close()

	testN := 1000
	count := int64(0)
	// need goroutine to play as a producer
	go func() {
		for i := 0; i < testN; i++ {
			b.Queue(func() (interface{}, error) {
				atomic.AddInt64(&count, 1)
				return nil, nil
			})
		}

		b.QueueComplete()
	}()

	b.WaitAll()
	s.Require().Equal(int64(testN), count)
}

func (s *batchSuite) TestQueueAllAndResults() {
	testN := 1000
	b := NewBatch(3, WithBatchSize(testN))
	defer b.Close()

	expResults := []int{}
	// no goroutine here
	for i := 0; i < testN; i++ {
		num := i
		expResults = append(expResults, num)
		b.Queue(func() (interface{}, error) {
			return num, nil
		})
	}

	b.QueueComplete()

	results := []int{}
	for ret := range b.Results() {
		s.Require().NoError(ret.Error())
		results = append(results, ret.Value().(int))
	}

	sort.Ints(results)
	s.Require().Equal(expResults, results)
}

func (s *batchSuite) TestQueueAllAndTerminate() {
	testN := 1000
	terminatedPoint := testN / 5
	b := NewBatch(3, WithBatchSize(testN))

	// no goroutine here
	for i := 0; i < testN; i++ {
		num := i
		b.Queue(func() (interface{}, error) {
			time.Sleep(time.Millisecond * 10)
			return num, nil
		})
	}

	b.QueueComplete()

	results := []int{}
	for i := 0; i < terminatedPoint; i++ {
		ret := <-b.Results()
		s.Require().NoError(ret.Error())
		results = append(results, ret.Value().(int))
	}

	// not finished yet. close it directly
	b.Close()

	// total time close to 1000/5/3*0.01 seconds
	s.Require().Equal(terminatedPoint, len(results))
}

func (s *batchSuite) TestQueueWithContext() {
	pause := make(chan struct{})
	wg := &sync.WaitGroup{}

	testN := defaultBatchSize + 1
	taskN := testN + 3 // worker + queue
	b := NewBatch(3, WithBatchSize(testN))
	defer b.Close()

	// full the workers in the pool
	for i := 0; i < taskN; i++ {
		wg.Add(1)
		num := i

		s.Require().NoError(b.Queue(func() (interface{}, error) {
			defer wg.Done()

			<-pause
			return num, nil
		}))
	}

	time.Sleep(time.Millisecond * 50)

	// have reached the limitation, and no response until the timeout comes
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	s.Require().Equal(ErrQueueCTXDone, b.QueueWithContext(ctx, func() (interface{}, error) {
		<-pause
		return 100, nil
	}))

	close(pause)
	wg.Wait()
}

func (s *batchSuite) TestDoNothing() {
	b := NewBatch(100)
	b.Close()
}

func (s *batchSuite) TestWaitAllAndGracefulClose() {
	taskN := 100
	b := NewBatch(3, WithBatchSize(taskN))
	wg := &sync.WaitGroup{}

	for i := 0; i < taskN; i++ {
		wg.Add(1)
		idx := i
		b.Queue(func() (interface{}, error) {
			defer wg.Done()
			return idx, nil
		})
	}

	b.QueueComplete()
	b.WaitAll()
	b.GracefulClose()

	wg.Wait()
}

func (s *batchSuite) TestQueueComplete() {
	b := NewBatch(100)
	s.Require().NoError(b.Queue(func() (interface{}, error) {
		return "Haha", nil
	}))

	b.QueueComplete()

	s.Require().Equal(ErrQueueComplete, b.Queue(func() (interface{}, error) {
		return "Haha", nil
	}))
}

func f(i int) BatchFunc {
	return func() (interface{}, error) {
		if i&1 == 0 {
			time.Sleep(2 * time.Second)
		} else {
			time.Sleep(10 * time.Millisecond)
		}
		return i, nil
	}
}

func (s *batchSuite) TestGoroutineLeakNoTimeout() {
	s.T().Skip("skip leak test") // only works well without `-race`
	before := runtime.NumGoroutine()

	batchSize := 40
	for n := 0; n < 5; n++ {
		b := NewBatch(batchSize, WithBatchSize(batchSize))
		for i := 0; i < batchSize; i++ {
			b.Queue(f(i))
		}
		b.QueueComplete()

		b.WaitAll()
		b.Close()
	}

	s.Require().Equal(before, runtime.NumGoroutine())
}

func (s *batchSuite) TestGoroutineLeakWithTimeout() {
	s.T().Skip("skip leak test") // only works well without `-race`
	before := runtime.NumGoroutine()

	batchSize := 40
	for n := 0; n < 5; n++ {
		timer := time.After(1 * time.Second)
		batch := NewBatch(40, WithBatchSize(batchSize))
		for i := 0; i < batchSize; i++ {
			batch.Queue(f(i))
		}
		batch.QueueComplete()
	resultsLoop:
		for i := 0; i < batchSize; i++ {
			select {
			case <-batch.Results():
			case <-timer:
				break resultsLoop
			}
		}
		batch.Close()
	}

	s.Require().Equal(before, runtime.NumGoroutine())
}
