# goroutines

[![GoDev](https://img.shields.io/badge/go.dev-doc-007d9c?style=flat-square&logo=read-the-docs)](https://pkg.go.dev/github.com/viney-shih/goroutines?tab=doc)
[![Build Status](https://travis-ci.com/viney-shih/goroutines.svg?branch=master)](https://travis-ci.com/github/viney-shih/goroutines)
[![Go Report Card](https://goreportcard.com/badge/github.com/viney-shih/goroutines)](https://goreportcard.com/report/github.com/viney-shih/goroutines)
[![codecov](https://codecov.io/gh/viney-shih/goroutines/branch/master/graph/badge.svg)](https://codecov.io/gh/viney-shih/goroutines)
[![Coverage Status](https://coveralls.io/repos/github/viney-shih/goroutines/badge.svg?branch=master)](https://coveralls.io/github/viney-shih/goroutines?branch=master)
[![License](http://img.shields.io/badge/License-Apache_2-red.svg?style=flat)](http://www.apache.org/licenses/LICENSE-2.0)
[![Sourcegraph](https://sourcegraph.com/github.com/viney-shih/goroutines/-/badge.svg)](https://sourcegraph.com/github.com/viney-shih/goroutines?badge)
[![FOSSA Status](https://app.fossa.com/api/projects/git%2Bgithub.com%2Fviney-shih%2Fgoroutines.svg?type=shield)](https://app.fossa.com/projects/git%2Bgithub.com%2Fviney-shih%2Fgoroutines?ref=badge_shield)

<p align="center">
  <img src="assets/logo.png" title="Goroutines" />
</p>

Package **goroutines** is an efficient, flexible, and lightweight goroutine pool written in Go. It provides a easy way to deal with several kinds of concurrent tasks with limited resource. 

Inspired by [fastsocket](https://github.com/faceair/fastsocket), the implementation is based on **channel**. It adopts pubsub model for dispatching tasks, and holding surplus tasks in queue if submitted more than the capacity of pool.

## Features
- Spawning and managing arbitrary number of asynchronous goroutines as a worker pool.
- Dispatch tasks to workers through pubsub model with specified queue size.
- Adjust the worker numbers based on the usage periodically.
- Easy to use when dealing with concurrent one-time batch jobs.
- Monitor current status by metrics

## Table of Contents

* [Installation](#Installation)
* [Get Started](#Get-Started)
	* [Basic usage of Pool (blocking mode)](#Basic-usage-of-Pool-in-blocking-mode)
	* [Basic usage of Pool (non-blocking mode)](#Basic-usage-of-Pool-in-nonblocking-mode)
	* [Advanced usage (one-time batch jobs)](#Advanced-usage-of-Batch-jobs)
* [Options](#Options)
	* [PoolOption](#PoolOption)
		* [WithTaskQueueLength(length int)](#PoolOption)
		* [WithPreAllocWorkers(size int)](#PoolOption)
		* [WithWorkerAdjustPeriod(period time.Duration)](#PoolOption)
	* [BatchOption](#BatchOption)
		* [WithBatchSize(size int)](#BatchOption)
* [References](#References)
* [License](#License)

## Installation

```sh
go get github.com/viney-shih/goroutines
```
## Get Started
### Basic usage of Pool in blocking mode

By calling `Schedule()`, it schedules the task executed by worker (goroutines) in the Pool.
It will be blocked until the workers accepting the request.

```go
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
```

### Basic usage of Pool in nonblocking mode

By calling `ScheduleWithTimeout()`, it schedules the task executed by worker (goroutines) in the Pool within the specified period.
If it exceeds the time and doesn't be accepted, it will return error `ErrScheduleTimeout`.

```go
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
```
### Advanced usage of Batch jobs

To deal with batch jobs and consider the performance, we need to run tasks concurrently. However, the use case usually happen once and need not maintain a Pool for reusing it. I wrap this patten and call it `Batch`. Here comes an example.

```go
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
```

See the [examples](https://pkg.go.dev/github.com/viney-shih/goroutines#pkg-examples), [documentation](https://pkg.go.dev/github.com/viney-shih/goroutines) and [article](https://medium.com/17media-tech/%E9%82%A3%E4%BA%9B%E5%B9%B4%E6%88%91%E5%80%91%E8%BF%BD%E7%9A%84-goroutine-pool-e8d211757ee) for more details.

## Options
### PoolOption
The `PoolOption` interface is passed to `NewPool` when creating Pool.

**• WithTaskQueueLength(** ***length*** `int` **)**

It sets up the length of task queue for buffering tasks before sending to goroutines. The default queue length is `0`.

**• WithPreAllocWorkers(** ***size*** `int` **)**

It sets up the number of workers to spawn when initializing Pool. Without specifying this, It initialize all numbers of goroutines consisting with Pool size at the beginning.

**• WithWorkerAdjustPeriod(** ***period*** `time.Duration` **)**

It sets up the duration to adjust the worker size, and needs to be used with `WithPreAllocWorkers` at the same time. By specifying both, it enables the mechanism to adjust the number of goroutines according to the usage dynamically.

### BatchOption
The `BatchOption` interface is passed to `NewBatch` when creating Batch.

**• WithBatchSize(** ***size*** `int` **)**

It specifies the batch size used to forward tasks.
By default, it needs to spawn an extra goroutine to prevent deadlocks.
It's helpful by specifing the batch size consisting with the number of tasks without an extra goroutine (see the [example](https://pkg.go.dev/github.com/viney-shih/goroutines#pkg-examples)). The default batch size is `10`.

## References
- https://github.com/faceair/fastsocket
- https://github.com/valyala/fasthttp
- https://github.com/Jeffail/tunny
- https://github.com/go-playground/pool
- https://github.com/panjf2000/ants

## License
[Apache-2.0](https://opensource.org/licenses/Apache-2.0)

[![FOSSA Status](https://app.fossa.com/api/projects/git%2Bgithub.com%2Fviney-shih%2Fgoroutines.svg?type=large)](https://app.fossa.com/projects/git%2Bgithub.com%2Fviney-shih%2Fgoroutines?ref=badge_large)
