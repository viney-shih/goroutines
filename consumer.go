package goroutines

import "sync"

// consumer handles tasks delivered by the producer via channel.
// It plays like a worker in the pubsub model, listening on input
// channel, execute tasks and output result in the output channel.
// When input channel is closed, it close itself, too.
type consumer struct {
	closeChan  chan struct{} // trigger closing
	closedChan chan struct{} // feedback closed
	closeOnce  sync.Once
}

func newConsumer() *consumer {
	c := consumer{
		closeChan:  make(chan struct{}),
		closedChan: make(chan struct{}),
	}

	return &c
}

func (c *consumer) stop() {
	c.closeOnce.Do(func() {
		close(c.closeChan)
	})
}

func (c *consumer) join() {
	<-c.closedChan
}
