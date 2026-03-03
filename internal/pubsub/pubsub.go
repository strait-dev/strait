package pubsub

import "context"

type Publisher interface {
	Publish(ctx context.Context, channel string, data []byte) error
	Subscribe(ctx context.Context, channel string) (*Subscription, error)
	Close() error
}

type Subscription struct {
	Ch     <-chan []byte
	cancel context.CancelFunc
}

func (s *Subscription) Close() {
	s.cancel()
}
