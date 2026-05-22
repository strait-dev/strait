package grpc

import (
	"context"

	"strait/internal/pubsub"
)

// noopPub is a non-nil pubsub.Publisher used by unit tests that need to
// satisfy NewServer's pub != nil precondition without bringing up a real
// Redis. Subscribe returns a subscription that closes when its context
// cancels; Publish is a no-op. Mirrors noopPublisher in the integration
// suite but is available without the integration build tag.
type noopPub struct{}

func (noopPub) Publish(_ context.Context, _ string, _ []byte) error { return nil }
func (noopPub) PublishBatch(_ context.Context, _ []pubsub.PubSubMessage) error {
	return nil
}
func (noopPub) Subscribe(ctx context.Context, _ string) (*pubsub.Subscription, error) {
	ch := make(chan []byte)
	ctx2, cancel := context.WithCancel(ctx)
	go func() {
		<-ctx2.Done()
		close(ch)
	}()
	return pubsub.NewSubscription(ch, cancel), nil
}
func (noopPub) Close() error { return nil }
