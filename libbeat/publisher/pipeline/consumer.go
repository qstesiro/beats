// Licensed to Elasticsearch B.V. under one or more contributor
// license agreements. See the NOTICE file distributed with
// this work for additional information regarding copyright
// ownership. Elasticsearch B.V. licenses this file to you under
// the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package pipeline

import (
	"errors"
	"sync"

	"github.com/elastic/beats/v7/libbeat/common/atomic"
	"github.com/elastic/beats/v7/libbeat/logp"
	"github.com/elastic/beats/v7/libbeat/publisher/queue"
)

// eventConsumer collects and forwards events from the queue to the outputs work queue.
// The eventConsumer is managed by the controller and receives additional pause signals
// from the retryer in case of too many events failing to be send or if retryer
// is receiving cancelled batches from outputs to be closed on output reloading.
// @implement libbeat/publisher/pipeline.interruptor
type eventConsumer struct {
	logger *logp.Logger
	ctx    *batchContext

	pause atomic.Bool
	wait  atomic.Bool
	sig   chan consumerSignal
	wg    sync.WaitGroup

	queue    queue.Queue
	consumer queue.Consumer

	out *outputGroup // updOutput函数中更新,此函数outputController.Set函数中被调用
}

type consumerSignal struct {
	tag      consumerEventTag
	consumer queue.Consumer
	out      *outputGroup
}

type consumerEventTag uint8

const (
	sigConsumerCheck consumerEventTag = iota
	sigConsumerUpdateOutput
	sigConsumerUpdateInput
	sigStop
)

var errStopped = errors.New("stopped")

func newEventConsumer(
	log *logp.Logger,
	queue queue.Queue,
	ctx *batchContext,
) *eventConsumer {
	consumer := queue.Consumer()
	c := &eventConsumer{
		logger: log,
		sig:    make(chan consumerSignal, 3), // 只有3个缓冲有什么特殊含义 ???
		out:    nil,

		queue:    queue,
		consumer: consumer,
		ctx:      ctx,
	}
	// 启动处于暂停状态
	c.pause.Store(true)

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.loop(consumer)
	}()
	return c
}

func (c *eventConsumer) close() {
	c.consumer.Close()
	c.sig <- consumerSignal{tag: sigStop}
	c.wg.Wait()
}

// @implement libbeat/publisher/pipeline.interruptor.sigWait
func (c *eventConsumer) sigWait() {
	c.wait.Store(true)
	c.sigHint()
}

// @implement libbeat/publisher/pipeline.interruptor.sigUnWait
func (c *eventConsumer) sigUnWait() {
	c.wait.Store(false)
	c.sigHint()
}

func (c *eventConsumer) sigPause() {
	c.pause.Store(true)
	c.sigHint()
}

func (c *eventConsumer) sigContinue() {
	c.pause.Store(false)
	c.sigHint()
}

func (c *eventConsumer) sigHint() {
	// send signal to unblock a consumer trying to publish events.
	// With flags being set atomically, multiple signals can be compressed into one
	// signal -> drop if queue is not empty
	select {
	case c.sig <- consumerSignal{tag: sigConsumerCheck}:
	default:
	}
}

func (c *eventConsumer) updOutput(grp *outputGroup) {
	// close consumer to break consumer worker from pipeline
	c.consumer.Close()

	// update output
	c.sig <- consumerSignal{
		tag: sigConsumerUpdateOutput,
		out: grp,
	}

	// update eventConsumer with new queue connection
	// 只是更新了output不明白为什么要更新consumer ???
	c.consumer = c.queue.Consumer()
	c.sig <- consumerSignal{
		tag:      sigConsumerUpdateInput,
		consumer: c.consumer,
	}
}

func (c *eventConsumer) loop(consumer queue.Consumer) {
	log := c.logger

	log.Debug("start pipeline event consumer")

	var (
		out    workQueue
		batch  Batch
		paused = true
	)

	handleSignal := func(sig consumerSignal) error { // 独立成个绑定函数比较好
		switch sig.tag {
		case sigStop:
			return errStopped // sigStop退出

		case sigConsumerCheck:

		case sigConsumerUpdateOutput:
			c.out = sig.out

		case sigConsumerUpdateInput:
			consumer = sig.consumer
		}

		paused = c.paused()
		if c.out != nil && batch != nil {
			out = c.out.workQueue
		} else {
			out = nil
		}
		return nil
	}

	for {
		if !paused && c.out != nil && consumer != nil && batch == nil {
			out = c.out.workQueue
			queueBatch, err := consumer.Get(c.out.batchSize)
			if err != nil {
				out = nil
				consumer = nil
				continue
			}
			if queueBatch != nil {
				batch = newBatch(c.ctx, queueBatch, c.out.timeToLive) // 创建pipeline.batch
			}

			paused = c.paused()
			if paused || batch == nil {
				out = nil
			}
		}

		select {
		case sig := <-c.sig:
			if err := handleSignal(sig); err != nil { // sigStop退出
				return
			}
			continue
		default:
		}

		select {
		case sig := <-c.sig:
			if err := handleSignal(sig); err != nil { // sigStop退出
				return
			}
		case out <- batch: // pub-step3
			batch = nil
			if paused {
				out = nil
			}
		}
	}
}

func (c *eventConsumer) paused() bool {
	return c.pause.Load() || c.wait.Load()
}
