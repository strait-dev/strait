package pubsub

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
)

const defaultFailureThreshold = 3

var ErrRedisUnavailable = errors.New("redis unavailable")

type pingPublisher interface {
	Ping(ctx context.Context) error
}

type ResilientPublisher struct {
	publisher        Publisher
	logger           *slog.Logger
	failureThreshold int32

	consecutiveFailures atomic.Int32
	healthy             atomic.Bool
}

func NewResilientPublisher(publisher Publisher, logger *slog.Logger, failureThreshold int) *ResilientPublisher {
	if logger == nil {
		logger = slog.Default()
	}
	if failureThreshold <= 0 {
		failureThreshold = defaultFailureThreshold
	}

	r := &ResilientPublisher{
		publisher:        publisher,
		logger:           logger,
		failureThreshold: int32(failureThreshold),
	}
	r.healthy.Store(true)

	return r
}

func (r *ResilientPublisher) Publish(ctx context.Context, channel string, data []byte) error {
	if r.publisher == nil {
		r.handleFailure("publish", channel, ErrRedisUnavailable)
		return nil
	}

	if err := r.publisher.Publish(ctx, channel, data); err != nil {
		r.handleFailure("publish", channel, err)
		return nil
	}

	r.handleSuccess("publish", channel)
	return nil
}

func (r *ResilientPublisher) PublishBatch(ctx context.Context, messages []PubSubMessage) error {
	if r.publisher == nil {
		r.handleFailure("publish_batch", "", ErrRedisUnavailable)
		return nil
	}

	if err := r.publisher.PublishBatch(ctx, messages); err != nil {
		r.handleFailure("publish_batch", "", err)
		return nil
	}

	r.handleSuccess("publish_batch", "")
	return nil
}

func (r *ResilientPublisher) Subscribe(ctx context.Context, channel string) (*Subscription, error) {
	if r.publisher == nil {
		r.handleFailure("subscribe", channel, ErrRedisUnavailable)
		return closedSubscription(), nil
	}

	sub, err := r.publisher.Subscribe(ctx, channel)
	if err != nil {
		r.handleFailure("subscribe", channel, err)
		return closedSubscription(), nil
	}

	r.handleSuccess("subscribe", channel)
	return sub, nil
}

func (r *ResilientPublisher) Close() error {
	if r.publisher == nil {
		r.handleFailure("close", "", ErrRedisUnavailable)
		return nil
	}

	if err := r.publisher.Close(); err != nil {
		r.handleFailure("close", "", err)
		return nil
	}

	r.handleSuccess("close", "")
	return nil
}

func (r *ResilientPublisher) Ping(ctx context.Context) error {
	p, ok := r.publisher.(pingPublisher)
	if !ok {
		err := ErrRedisUnavailable
		r.handleFailure("ping", "", err)
		return err
	}

	if err := p.Ping(ctx); err != nil {
		r.handleFailure("ping", "", err)
		return errors.Join(ErrRedisUnavailable, err)
	}

	r.handleSuccess("ping", "")
	return nil
}

func (r *ResilientPublisher) IsHealthy() bool {
	return r.healthy.Load()
}

func (r *ResilientPublisher) handleFailure(operation, channel string, err error) {
	failures := r.consecutiveFailures.Add(1)
	r.logger.Warn("redis operation failed; continuing in fail-open mode", "operation", operation, "channel", channel, "error", err, "consecutive_failures", failures)

	if failures >= r.failureThreshold && r.healthy.Swap(false) {
		r.logger.Warn("redis publisher marked degraded", "operation", operation, "channel", channel, "consecutive_failures", failures, "failure_threshold", r.failureThreshold)
	}
}

func (r *ResilientPublisher) handleSuccess(operation, channel string) {
	prevFailures := r.consecutiveFailures.Load()
	if prevFailures != 0 {
		prevFailures = r.consecutiveFailures.Swap(0)
	}
	if !r.healthy.Load() {
		r.healthy.Store(true)
		r.logger.Info("redis publisher recovered", "operation", operation, "channel", channel, "previous_failures", prevFailures)
	}
}

func closedSubscription() *Subscription {
	ch := make(chan []byte)
	close(ch)
	return NewSubscription(ch, func() {})
}
