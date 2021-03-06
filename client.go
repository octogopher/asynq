// Copyright 2020 Kentaro Hibino. All rights reserved.
// Use of this source code is governed by a MIT license
// that can be found in the LICENSE file.

package asynq

import (
	"strings"
	"time"

	"github.com/hibiken/asynq/internal/base"
	"github.com/hibiken/asynq/internal/rdb"
	"github.com/rs/xid"
)

// A Client is responsible for scheduling tasks.
//
// A Client is used to register tasks that should be processed
// immediately or some time in the future.
//
// Clients are safe for concurrent use by multiple goroutines.
type Client struct {
	rdb *rdb.RDB
}

// NewClient and returns a new Client given a redis connection option.
func NewClient(r RedisConnOpt) *Client {
	rdb := rdb.NewRDB(createRedisClient(r))
	return &Client{rdb}
}

// Option specifies the task processing behavior.
type Option interface{}

// Internal option representations.
type (
	retryOption    int
	queueOption    string
	timeoutOption  time.Duration
	deadlineOption time.Time
)

// MaxRetry returns an option to specify the max number of times
// the task will be retried.
//
// Negative retry count is treated as zero retry.
func MaxRetry(n int) Option {
	if n < 0 {
		n = 0
	}
	return retryOption(n)
}

// Queue returns an option to specify the queue to enqueue the task into.
//
// Queue name is case-insensitive and the lowercased version is used.
func Queue(name string) Option {
	return queueOption(strings.ToLower(name))
}

// Timeout returns an option to specify how long a task may run.
//
// Zero duration means no limit.
func Timeout(d time.Duration) Option {
	return timeoutOption(d)
}

// Deadline returns an option to specify the deadline for the given task.
func Deadline(t time.Time) Option {
	return deadlineOption(t)
}

type option struct {
	retry    int
	queue    string
	timeout  time.Duration
	deadline time.Time
}

func composeOptions(opts ...Option) option {
	res := option{
		retry:    defaultMaxRetry,
		queue:    base.DefaultQueueName,
		timeout:  0,
		deadline: time.Time{},
	}
	for _, opt := range opts {
		switch opt := opt.(type) {
		case retryOption:
			res.retry = int(opt)
		case queueOption:
			res.queue = string(opt)
		case timeoutOption:
			res.timeout = time.Duration(opt)
		case deadlineOption:
			res.deadline = time.Time(opt)
		default:
			// ignore unexpected option
		}
	}
	return res
}

const (
	// Max retry count by default
	defaultMaxRetry = 25
)

// EnqueueAt schedules task to be enqueued at the specified time.
//
// EnqueueAt returns nil if the task is scheduled successfully, otherwise returns a non-nil error.
//
// The argument opts specifies the behavior of task processing.
// If there are conflicting Option values the last one overrides others.
func (c *Client) EnqueueAt(t time.Time, task *Task, opts ...Option) error {
	opt := composeOptions(opts...)
	msg := &base.TaskMessage{
		ID:       xid.New(),
		Type:     task.Type,
		Payload:  task.Payload.data,
		Queue:    opt.queue,
		Retry:    opt.retry,
		Timeout:  opt.timeout.String(),
		Deadline: opt.deadline.Format(time.RFC3339),
	}
	return c.enqueue(msg, t)
}

// Enqueue enqueues task to be processed immediately.
//
// Enqueue returns nil if the task is enqueued successfully, otherwise returns a non-nil error.
//
// The argument opts specifies the behavior of task processing.
// If there are conflicting Option values the last one overrides others.
func (c *Client) Enqueue(task *Task, opts ...Option) error {
	return c.EnqueueAt(time.Now(), task, opts...)
}

// EnqueueIn schedules task to be enqueued after the specified delay.
//
// EnqueueIn returns nil if the task is scheduled successfully, otherwise returns a non-nil error.
//
// The argument opts specifies the behavior of task processing.
// If there are conflicting Option values the last one overrides others.
func (c *Client) EnqueueIn(d time.Duration, task *Task, opts ...Option) error {
	return c.EnqueueAt(time.Now().Add(d), task, opts...)
}

func (c *Client) enqueue(msg *base.TaskMessage, t time.Time) error {
	if time.Now().After(t) {
		return c.rdb.Enqueue(msg)
	}
	return c.rdb.Schedule(msg, t)
}
