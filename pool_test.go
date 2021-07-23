package goroutines

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

type poolSuite struct {
	suite.Suite
}

func (s *poolSuite) SetupSuite() {}

func (s *poolSuite) TearDownSuite() {}

func (s *poolSuite) SetupTest() {}

func (s *poolSuite) TearDownTest() {}

func TestPoolSuite(t *testing.T) {
	suite.Run(t, new(poolSuite))
}

func (s *poolSuite) TestInvalidSize() {
	defer func() {
		r := recover()
		s.Require().NotNil(r)
		s.Require().Equal(errors.New("the total number of workers must be greater than zero"), r)
	}()

	NewPool(0)
}

func (s *poolSuite) TestInvalidPreAllocWorkerNumbers() {
	defer func() {
		r := recover()
		s.Require().NotNil(r)
		s.Require().Equal(errors.New("the number of pre-allocated workers must be less than or equal to total"), r)
	}()

	NewPool(10, WithPreAllocWorkers(100))
}

func (s *poolSuite) TestPoolReleased() {
	p := NewPool(1)
	p.Release()
	s.Require().Equal(ErrPoolRelease, p.Schedule(func() {
		// it won't do anything
	}))
}

func (s *poolSuite) TestScheduleWithFixedWorkers() {
	totalN := 5
	taskN := 100
	rets := make(chan struct{}, taskN)

	p := NewPool(totalN)
	// now running is 5
	time.Sleep(time.Millisecond * 50)
	s.Require().Equal(totalN, p.Workers())
	s.Require().Equal(0, p.Running())

	for i := 0; i < taskN; i++ {
		s.Require().NoError(p.Schedule(func() {
			rets <- struct{}{}
		}))
	}
	for i := 0; i < taskN; i++ {
		<-rets
	}

	// close all, and now running is 0
	p.Release()
	s.Require().Equal(0, p.Workers())
	s.Require().Equal(0, p.Running())
}

func (s *poolSuite) TestScheduleWithAutoScaledWorkers() {
	totalN := 10
	initN := 0
	taskN := 8
	pause := make(chan struct{})
	rets := make(chan struct{}, taskN)

	// default number of workers is initN
	p := NewPool(totalN, WithPreAllocWorkers(initN))
	// now running is 1
	time.Sleep(time.Millisecond * 50)
	s.Require().Equal(initN+1, p.Workers())
	s.Require().Equal(0, p.Running())

	for i := 0; i < taskN; i++ {
		s.Require().NoError(p.Schedule(func() {
			<-pause
			rets <- struct{}{}
		}))
	}

	time.Sleep(time.Millisecond * 50)
	s.Require().Equal(taskN, p.Running())

	close(pause)
	for i := 0; i < taskN; i++ {
		<-rets
	}
	// now running number is taskN+1, extra one is created by miner waiting for next request
	s.Require().Equal(taskN+1, p.Workers())
	s.Require().Equal(0, p.Running())

	// close all, and now running is 0
	p.Release()
	s.Require().Equal(0, p.Workers())
	s.Require().Equal(0, p.Running())
}

func (s *poolSuite) TestScheduleWithTimeout() {
	totalN := 10
	initN := 0
	taskN := 10
	pause := make(chan struct{})
	rets := make(chan struct{}, taskN)

	// default number of workers is initN
	p := NewPool(totalN, WithPreAllocWorkers(initN))
	// now running is 1
	time.Sleep(time.Millisecond * 50)
	s.Require().Equal(initN+1, p.Workers())
	s.Require().Equal(0, p.Running())

	// full the workers
	for i := 0; i < taskN; i++ {
		s.Require().NoError(p.ScheduleWithTimeout(50*time.Millisecond, func() {
			<-pause
			rets <- struct{}{}
		}))
	}

	time.Sleep(time.Millisecond * 50)
	s.Require().Equal(taskN, p.Running())

	// have reached the limitation, and no response until the timeout comes
	s.Require().Equal(ErrScheduleTimeout, p.ScheduleWithTimeout(50*time.Millisecond, func() {
		<-pause
		rets <- struct{}{}
	}))

	close(pause)
	for i := 0; i < taskN; i++ {
		<-rets
	}
	// now running number is totalN
	s.Require().Equal(totalN, p.Workers())
	s.Require().Equal(0, p.Running())

	// close all, and now running is 0
	p.Release()
	s.Require().Equal(0, p.Workers())
	s.Require().Equal(0, p.Running())
}

func (s *poolSuite) TestScheduleWithContext() {
	totalN := 10
	initN := 0
	taskN := 10
	pause := make(chan struct{})
	rets := make(chan struct{}, taskN)

	// default number of workers is initN
	p := NewPool(totalN, WithPreAllocWorkers(initN))
	defer p.Release()
	// now running is 1
	time.Sleep(time.Millisecond * 50)
	s.Require().Equal(initN+1, p.Workers())
	s.Require().Equal(0, p.Running())

	// full the workers
	for i := 0; i < taskN; i++ {
		s.Require().NoError(p.ScheduleWithTimeout(50*time.Millisecond, func() {
			<-pause
			rets <- struct{}{}
		}))
	}

	time.Sleep(time.Millisecond * 50)
	s.Require().Equal(taskN, p.Running())

	// have reached the limitation, and no response until the timeout comes
	ctxT, cancelT := context.WithTimeout(context.Background(), 50*time.Millisecond)
	s.Require().Equal(ErrScheduleTimeout, p.ScheduleWithContext(ctxT, func() {
		<-pause
		rets <- struct{}{}
	}))
	cancelT()

	// cancel manually
	ctxM, cancelM := context.WithCancel(context.Background())
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()

		s.Require().Equal(ErrScheduleTimeout, p.ScheduleWithContext(ctxM, func() {
			<-pause
			rets <- struct{}{}
		}))
	}()

	time.Sleep(time.Millisecond * 50)
	cancelM()
	wg.Wait()

	close(pause)
	for i := 0; i < taskN; i++ {
		<-rets
	}
}

func (s *poolSuite) TestQueueFirstStrategy() {
	totalN := 100
	initN := 3
	taskN := 1000
	wg := &sync.WaitGroup{}
	pause := make(chan struct{})

	// default number of workers is initN
	p := NewPool(totalN, WithPreAllocWorkers(initN))
	defer p.Release()

	time.Sleep(time.Millisecond * 50)
	s.Require().Equal(initN+1, p.Workers())
	s.Require().Equal(0, p.Running())

	for i := 0; i < taskN; i++ {
		wg.Add(1)
		s.Require().NoError(p.Schedule(func() {
			pause <- struct{}{}
			wg.Done()
		}))

		<-pause
	}

	wg.Wait()
	// initN+1, extra one is created by miner waiting for next request
	s.Require().Equal(initN+1, p.Workers())
	s.Require().Equal(0, p.Running())
}

func (s *poolSuite) TestScheduleWithAutoScaledWorkersAndRecycler() {
	totalN := 10
	initN := 0
	taskN := 10
	phase1 := make(chan struct{})
	phase2 := make(chan struct{})
	phase3 := make(chan struct{})
	p := NewPool(
		totalN,
		WithPreAllocWorkers(initN),
		WithWorkerAdjustPeriod(time.Duration(time.Millisecond*150)),
	)
	defer p.Release()

	tests := []struct {
		Desc       string
		SetupTest  func()
		ExpWorkers int
		ExpRunning int
	}{
		{
			Desc: "initialize pool",
			SetupTest: func() {
				time.Sleep(time.Millisecond * 50)
			},
			ExpWorkers: 1, // extra one is created by miner waiting for next request, but not running
			ExpRunning: 0,
		},
		{
			Desc: "full workers",
			SetupTest: func() {
				for i := taskN - 1; i >= 0; i-- {
					if i == 0 {
						// require one waiting for phase2
						s.Require().NoError(p.Schedule(func() {
							<-phase2
						}))
						continue
					}

					s.Require().NoError(p.Schedule(func() {
						<-phase1
					}))
				}

				time.Sleep(time.Millisecond * 50)
			},
			ExpWorkers: taskN,
			ExpRunning: taskN,
		},
		{
			Desc: "one worker running",
			SetupTest: func() {
				// wait for Recycler recognizing the max worker size
				time.Sleep(time.Millisecond * 100) // 50+50+100 > 150 (WorkerAdjustPeriod)
				// keep one worker running
				close(phase1)
				// wait for Recycler recognizing one running worker
				time.Sleep(time.Millisecond * 500) // 500 > 150 * 3
			},
			ExpWorkers: 2, // extra one is created by miner waiting for next request, but not running
			ExpRunning: 1,
		},
		{
			Desc: "no worker running",
			SetupTest: func() {
				// keep no worker running
				close(phase2)
				// wait for Recycler recognizing one running worker
				time.Sleep(time.Millisecond * 500) // 500 > 150 * 3
			},
			ExpWorkers: 1, // extra one is created by miner waiting for next request, but not running
			ExpRunning: 0,
		},
		{
			Desc: "full workers again",
			SetupTest: func() {
				for i := taskN - 1; i >= 0; i-- {
					s.Require().NoError(p.Schedule(func() {
						<-phase3
					}))
				}

				time.Sleep(time.Millisecond * 50)
			},
			ExpWorkers: taskN,
			ExpRunning: taskN,
		},
		{
			Desc: "no worker running again",
			SetupTest: func() {
				// wait for Recycler recognizing the max worker size
				time.Sleep(time.Millisecond * 150) // 50+50+100 > 150 (WorkerAdjustPeriod)
				// keep no worker running
				close(phase3)
				// wait for Recycler recognizing one running worker
				time.Sleep(time.Millisecond * 500) // 500 > 150 * 3
			},
			ExpWorkers: 1, // extra one is created by miner waiting for next request, but not running
			ExpRunning: 0,
		},
	}

	for _, t := range tests {
		if t.SetupTest != nil {
			t.SetupTest()
		}

		s.Require().Equal(t.ExpWorkers, p.Workers(), t.Desc)
		s.Require().Equal(t.ExpRunning, p.Running(), t.Desc)
	}
}
