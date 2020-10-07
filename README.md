# goroutines

[![GoDoc](https://godoc.org/github.com/viney-shih/goroutines?status.svg)](https://godoc.org/github.com/viney-shih/goroutines)
[![GoDev](https://img.shields.io/badge/go.dev-doc-007d9c?style=flat-square&logo=read-the-docs)](https://pkg.go.dev/github.com/viney-shih/goroutines?tab=doc)
[![Build Status](https://travis-ci.com/viney-shih/goroutines.svg?branch=master)](https://travis-ci.com/github/viney-shih/goroutines)
[![Go Report Card](https://goreportcard.com/badge/github.com/viney-shih/goroutines)](https://goreportcard.com/report/github.com/viney-shih/goroutines)
[![Codecov](https://codecov.io/gh/viney-shih/go-lock/branch/master/graph/badge.svg)](https://codecov.io/gh/viney-shih/goroutines)
[![Coverage Status](https://coveralls.io/repos/github/viney-shih/goroutines/badge.svg?branch=master)](https://coveralls.io/github/viney-shih/goroutines?branch=master)
[![License](http://img.shields.io/badge/License-Apache_2-red.svg?style=flat)](http://www.apache.org/licenses/LICENSE-2.0)
[![Sourcegraph](https://sourcegraph.com/github.com/viney-shih/goroutines/-/badge.svg)](https://sourcegraph.com/github.com/viney-shih/goroutines?badge)

<p align="center">
  <img src="logo.png" title="Goroutines" />
</p>

Package **goroutines** is an efficient, flexible, and lightweight goroutine pool written in Go. It provides a easy way to deal with several kinds of concurrent tasks with limited resource. 

Inspired by [fastsocket](https://github.com/faceair/fastsocket), the implementation is based on channel. It adopts pubsub model for dispatching tasks, and holding surplus tasks in queue if submitted more than the capacity of pool.

## Features
- Spawning and managing arbitrary number of asynchronous goroutines as a worker pool.
- Dispatch tasks to workers through pubsub model with specified queue size.
- Adjust the worker numbers based on the usage periodically.
- Easy to use when dealing with concurrent one-time batch jobs.
- Monitor current status by metrics

## Installation

```sh
go get github.com/viney-shih/goroutines
```
## How to use
### Pool example

```go
package main

import (
	"fmt"
	"time"

	"github.com/viney-shih/goroutines"
)

func main() {
	taskN := 100
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
			time.Sleep(100 * time.Millisecond)
			rets <- idx
		})
	}

	// wait until all tasks done
	for i := 0; i < taskN; i++ {
		fmt.Println("index:", <-rets)
	}

	// Output: (the order is not the same with input one)
	// index: 3
	// index: 1
	// index: 2
	// index: 4
	// ...
}
```


### Batch work example

```go
package main

import (
	"fmt"

	"github.com/viney-shih/goroutines"
)

func main() {
	taskN := 100

	// allocate a one-time batch job with 5 goroutines to deal with those tasks.
	// no need to spawn extra goroutine by specifing the batch size consisting with the number of tasks.
	b := goroutines.NewBatch(5, goroutines.WithBatchSize(taskN))
	// don't forget to close batch job in the end
	defer b.Close()

	// pull all tasks to this batch queue
	for i := 0; i < taskN; i++ {
		idx := i
		b.Queue(func() (interface{}, error) {
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

	// Output: (the order is not the same with input one)
	// index: 1
	// index: 5
	// index: 6
	// index: 7
	// ...
}

```

## References
- https://github.com/faceair/fastsocket
- https://github.com/valyala/fasthttp
- https://github.com/Jeffail/tunny
- https://github.com/go-playground/pool
- https://github.com/panjf2000/ants

## License
[Apache-2.0](https://opensource.org/licenses/Apache-2.0)
