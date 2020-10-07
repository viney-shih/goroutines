package goroutines

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

type optionsSuite struct {
	suite.Suite
}

func (s *optionsSuite) SetupSuite() {}

func (s *optionsSuite) TearDownSuite() {}

func (s *optionsSuite) SetupTest() {}

func (s *optionsSuite) TearDownTest() {}

func TestOptionsSuite(t *testing.T) {
	suite.Run(t, new(optionsSuite))
}

func (s *optionsSuite) TestInvalidTaskQueueLength() {
	defer func() {
		r := recover()
		s.Require().NotNil(r)
		s.Require().Equal(errors.New("the length of task queue must be greater than or equal to zero"), r)
	}()

	NewPool(10, WithTaskQueueLength(-1))
}

func (s *optionsSuite) TestInvalidPreAllocWorkers() {
	defer func() {
		r := recover()
		s.Require().NotNil(r)
		s.Require().Equal(errors.New("the pre-allocated number of workers must be greater than or equal to zero"), r)
	}()

	NewPool(10, WithPreAllocWorkers(-1))
}

func (s *optionsSuite) TestInvalidWorkerAdjustPeriod() {
	defer func() {
		r := recover()
		s.Require().NotNil(r)
		s.Require().Equal(errors.New("the period of adjusting workers must be greater than zero"), r)
	}()

	NewPool(10, WithWorkerAdjustPeriod(time.Duration(0)))
}

func (s *optionsSuite) TestValidOptions() {
	p := NewPool(
		10,
		WithTaskQueueLength(0),
		WithPreAllocWorkers(0),
		WithWorkerAdjustPeriod(time.Duration(time.Second)),
	)
	p.Release()
}
