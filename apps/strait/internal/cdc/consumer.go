package cdc

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel"
)

// Consumer polls the Sequin Stream and dispatches messages to registered handlers.
type Consumer struct {
	client   *Client
	config   ConsumerConfig
	handlers map[string]Handler
	logger   *slog.Logger
	stop     chan struct{}
	done     chan struct{}
	stopOnce sync.Once
	pollWG   sync.WaitGroup
	polling  atomic.Int64
	started  atomic.Bool
}

// NewConsumer creates a new CDC consumer.
func NewConsumer(client *Client, cfg ConsumerConfig, logger *slog.Logger) *Consumer {
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 10
	}
	if cfg.WaitTimeMs <= 0 {
		cfg.WaitTimeMs = 1000
	}

	return &Consumer{
		client:   client,
		config:   cfg,
		handlers: make(map[string]Handler),
		logger:   logger,
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
}

// RegisterHandler adds a handler for a specific table.
func (c *Consumer) RegisterHandler(h Handler) {
	if h == nil {
		return
	}
	c.handlers[h.Table()] = h
}

// Run starts the consumer loop. It blocks until ctx is canceled.
func (c *Consumer) Run(ctx context.Context) {
	c.started.Store(true)
	defer close(c.done)

	c.logger.Info("cdc consumer started",
		"consumer", c.config.ConsumerName,
		"batch_size", c.config.BatchSize,
		"tables", c.registeredTables(),
	)

	for {
		select {
		case <-ctx.Done():
			c.logger.Info("cdc consumer stopping")
			return
		case <-c.stop:
			c.logger.Info("cdc consumer stopping")
			return
		default:
			c.pollWG.Add(1)
			c.polling.Add(1)
			if err := c.poll(ctx); err != nil {
				c.polling.Add(-1)
				c.pollWG.Done()
				if ctx.Err() != nil {
					return
				}
				c.logger.Error("cdc poll error", "error", err)
				select {
				case <-ctx.Done():
					return
				case <-c.stop:
					return
				case <-time.After(5 * time.Second):
				}
				continue
			}
			c.polling.Add(-1)
			c.pollWG.Done()
		}
	}
}

func (c *Consumer) Shutdown(ctx context.Context) error {
	c.stopOnce.Do(func() {
		close(c.stop)
	})

	if !c.started.Load() {
		return nil
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.done:
	}

	c.pollWG.Wait()
	return nil
}

func (c *Consumer) poll(ctx context.Context) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "cdc.poll")
	defer span.End()

	messages, err := c.client.Receive(ctx, c.config.BatchSize, c.config.WaitTimeMs)
	if err != nil {
		return fmt.Errorf("receive cdc messages: %w", err)
	}

	if len(messages) == 0 {
		return nil
	}

	c.logger.Debug("received cdc messages", "count", len(messages))

	ackIDs := make([]string, 0, len(messages))
	nackIDs := make([]string, 0, len(messages))

	for _, msg := range messages {
		handler, ok := c.handlers[msg.Metadata.TableName]
		if !ok {
			c.logger.Warn("no handler for table, acking",
				"table", msg.Metadata.TableName,
				"ack_id", msg.AckID,
			)
			ackIDs = append(ackIDs, msg.AckID)
			continue
		}

		if err := handler.Handle(ctx, msg); err != nil {
			c.logger.Error("handler failed",
				"table", msg.Metadata.TableName,
				"action", msg.Action,
				"ack_id", msg.AckID,
				"error", err,
			)
			nackIDs = append(nackIDs, msg.AckID)
			continue
		}

		ackIDs = append(ackIDs, msg.AckID)
	}

	if len(ackIDs) > 0 {
		if err := c.client.Ack(ctx, ackIDs); err != nil {
			c.logger.Error("failed to ack messages", "count", len(ackIDs), "error", err)
			return fmt.Errorf("ack cdc messages: %w", err)
		}
	}

	if len(nackIDs) > 0 {
		if err := c.client.Nack(ctx, nackIDs); err != nil {
			c.logger.Error("failed to nack messages", "count", len(nackIDs), "error", err)
			return fmt.Errorf("nack cdc messages: %w", err)
		}
	}

	return nil
}

func (c *Consumer) registeredTables() []string {
	tables := make([]string, 0, len(c.handlers))
	for table := range c.handlers {
		tables = append(tables, table)
	}
	sort.Strings(tables)
	return tables
}
