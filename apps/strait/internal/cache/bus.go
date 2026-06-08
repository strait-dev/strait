package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"strait/internal/pubsub"
)

const DefaultBusChannel = "strait:cachebus:v1"

type BusAction string

const (
	BusActionInvalidate BusAction = "invalidate"
	BusActionUpdate     BusAction = "update"
)

type BusMessage struct {
	Action    BusAction       `json:"action"`
	Namespace string          `json:"namespace"`
	Key       string          `json:"key"`
	Version   int64           `json:"version,omitempty"`
	Origin    string          `json:"origin"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	SentAt    time.Time       `json:"sent_at"`
}

type BusConfig struct {
	Channel string
	Origin  string
	Logger  *slog.Logger
}

type Bus struct {
	publisher pubsub.Publisher
	channel   string
	origin    string
	logger    *slog.Logger
}

func NewBus(publisher pubsub.Publisher, cfg BusConfig) *Bus {
	channel := cfg.Channel
	if channel == "" {
		channel = DefaultBusChannel
	}
	origin := cfg.Origin
	if origin == "" {
		origin = newOriginID()
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Bus{
		publisher: publisher,
		channel:   channel,
		origin:    origin,
		logger:    logger,
	}
}

func (b *Bus) Origin() string {
	if b == nil {
		return ""
	}
	return b.origin
}

func (b *Bus) Channel() string {
	if b == nil {
		return DefaultBusChannel
	}
	return b.channel
}

func (b *Bus) PublishInvalidate(ctx context.Context, namespace, key string, version int64) error {
	return b.publish(ctx, BusMessage{
		Action:    BusActionInvalidate,
		Namespace: namespace,
		Key:       key,
		Version:   version,
	})
}

func (b *Bus) PublishUpdate(ctx context.Context, namespace, key string, version int64, payload json.RawMessage) error {
	return b.publish(ctx, BusMessage{
		Action:    BusActionUpdate,
		Namespace: namespace,
		Key:       key,
		Version:   version,
		Payload:   payload,
	})
}

func (b *Bus) publish(ctx context.Context, msg BusMessage) error {
	if b == nil || b.publisher == nil {
		return nil
	}
	msg.Origin = b.origin
	msg.SentAt = time.Now().UTC()
	data, err := marshalBusMessage(msg)
	if err != nil {
		return fmt.Errorf("marshal cachebus message: %w", err)
	}
	if err := b.publisher.Publish(ctx, b.channel, data); err != nil {
		b.logger.Warn("cachebus publish failed; continuing without cross-replica fast invalidation",
			"channel", b.channel,
			"namespace", msg.Namespace,
			"action", msg.Action,
			"error", err,
		)
		recordCacheFailOpen(ctx, msg.Namespace, "cachebus_publish")
		return nil
	}
	recordCacheBusEvent(ctx, string(msg.Action), msg.Namespace, "publish", msg.SentAt)
	return nil
}

func marshalBusMessage(msg BusMessage) ([]byte, error) {
	if len(msg.Payload) > 0 && !json.Valid(msg.Payload) {
		return nil, fmt.Errorf("invalid cachebus payload")
	}

	size := len(`{"action":"","namespace":"","key":"","origin":"","sent_at":""}`) +
		len(msg.Action) + len(msg.Namespace) + len(msg.Key) + len(msg.Origin) + len(time.RFC3339Nano)
	if msg.Version != 0 {
		size += len(`,"version":`) + 20
	}
	if len(msg.Payload) > 0 {
		size += len(`,"payload":`) + len(msg.Payload)
	}
	out := make([]byte, 0, size)
	out = append(out, `{"action":`...)
	out = strconv.AppendQuote(out, string(msg.Action))
	out = append(out, `,"namespace":`...)
	out = strconv.AppendQuote(out, msg.Namespace)
	out = append(out, `,"key":`...)
	out = strconv.AppendQuote(out, msg.Key)
	if msg.Version != 0 {
		out = append(out, `,"version":`...)
		out = strconv.AppendInt(out, msg.Version, 10)
	}
	out = append(out, `,"origin":`...)
	out = strconv.AppendQuote(out, msg.Origin)
	if len(msg.Payload) > 0 {
		out = append(out, `,"payload":`...)
		out = append(out, msg.Payload...)
	}
	out = append(out, `,"sent_at":"`...)
	out = msg.SentAt.AppendFormat(out, time.RFC3339Nano)
	out = append(out, `"}`...)
	return out, nil
}

func (b *Bus) Run(ctx context.Context, registry *Registry) error {
	if b == nil || b.publisher == nil || registry == nil {
		<-ctx.Done()
		return nil
	}
	sub, err := b.publisher.Subscribe(ctx, b.channel)
	if err != nil {
		b.logger.Warn("cachebus subscribe failed; continuing with TTL-backed local cache coherence",
			"channel", b.channel,
			"error", err,
		)
		return nil
	}
	if sub == nil {
		return nil
	}
	defer sub.Close()

	for {
		select {
		case <-ctx.Done():
			return nil
		case data, ok := <-sub.Ch:
			if !ok {
				return nil
			}
			var msg BusMessage
			if err := json.Unmarshal(data, &msg); err == nil {
				recordCacheBusEvent(ctx, string(msg.Action), msg.Namespace, "receive", msg.SentAt)
			}
			registry.Handle(ctx, data)
		}
	}
}
