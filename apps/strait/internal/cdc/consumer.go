package cdc

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"strait/internal/domain"
	"strait/internal/pubsub"
	"strait/internal/telemetry"

	"github.com/getsentry/sentry-go"
	"go.opentelemetry.io/otel"
)

// Consumer polls the Sequin Stream and dispatches messages to registered handlers.
type Consumer struct {
	client             *Client
	config             ConsumerConfig
	handlers           map[string]Handler
	additionalHandlers map[string][]Handler
	publisher          EventPublisher
	logger             *slog.Logger
	stop               chan struct{}
	done               chan struct{}
	stopOnce           sync.Once
	pollWG             sync.WaitGroup
	polling            atomic.Int64
	started            atomic.Bool
}

// NewConsumer creates a new CDC consumer.
func NewConsumer(client *Client, cfg ConsumerConfig, logger *slog.Logger) *Consumer {
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 200
	}
	if cfg.WaitTimeMs <= 0 {
		cfg.WaitTimeMs = 1000
	}

	return &Consumer{
		client:             client,
		config:             cfg,
		handlers:           make(map[string]Handler),
		additionalHandlers: make(map[string][]Handler),
		logger:             logger,
		stop:               make(chan struct{}),
		done:               make(chan struct{}),
	}
}

// SetPublisher sets the publisher for batch publishing CDC events.
func (c *Consumer) SetPublisher(pub EventPublisher) {
	c.publisher = pub
}

// RegisterHandler adds a handler for a specific table.
func (c *Consumer) RegisterHandler(h Handler) {
	if h == nil {
		return
	}
	c.handlers[h.Table()] = h
}

// RegisterAdditionalHandler adds a secondary handler for a table.
func (c *Consumer) RegisterAdditionalHandler(h Handler) {
	if h == nil {
		return
	}
	table := h.Table()
	c.additionalHandlers[table] = append(c.additionalHandlers[table], h)
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
	batchMessages := make([]Message, 0, len(messages))
	batch := make([]pubsub.PubSubMessage, 0, len(messages))

	for _, msg := range messages {
		handler, ok := c.handlers[msg.Metadata.TableName]
		if !ok {
			if len(c.additionalHandlers[msg.Metadata.TableName]) > 0 {
				if err := c.runAdditionalHandlers(ctx, msg); err != nil {
					c.logger.Error("additional handler failed",
						"table", msg.Metadata.TableName,
						"action", msg.Action,
						"ack_id", msg.AckID,
						"error", err,
					)
					nackIDs = append(nackIDs, msg.AckID)
					continue
				}
				ackIDs = append(ackIDs, msg.AckID)
				continue
			}
			if msg.Metadata.TableName == "" {
				c.logger.Debug("cdc message has empty table, acking", "ack_id", msg.AckID)
			} else {
				c.logger.Warn("no handler for table, acking",
					"table", msg.Metadata.TableName,
					"ack_id", msg.AckID,
				)
			}
			ackIDs = append(ackIDs, msg.AckID)
			continue
		}

		// Try batch collection first, fall back to inline Handle.
		if ch, ok := handler.(CollectableHandler); ok && c.publisher != nil {
			pubMsg, err := ch.Collect(ctx, msg)
			if err != nil {
				c.logger.Error("handler collect failed",
					"table", msg.Metadata.TableName,
					"action", msg.Action,
					"ack_id", msg.AckID,
					"error", err,
				)
				nackIDs = append(nackIDs, msg.AckID)
				continue
			}
			if pubMsg != nil {
				batch = append(batch, *pubMsg)
			}
			// Track separately; only ACK after successful publish and
			// durable side-effect handlers have completed.
			batchMessages = append(batchMessages, msg)
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

		if err := c.runAdditionalHandlers(ctx, msg); err != nil {
			c.logger.Error("additional handler failed",
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

	// Flush projection fan-out in one Redis pipeline. Projection delivery is
	// best-effort; durable additional handlers below decide ACK/NACK so a
	// transient Redis miss does not redeliver side effects.
	if len(batchMessages) > 0 && len(batch) > 0 && c.publisher != nil {
		if err := c.publisher.PublishBatch(ctx, batch); err != nil {
			c.captureBatchPublishFailure(ctx, err, len(batch))
			c.logger.Error("batch publish failed, continuing with durable handlers", "count", len(batch), "error", err)
		} else {
			c.logger.Debug("batch publish succeeded", "count", len(batch))
		}
		for _, msg := range batchMessages {
			if err := c.runAdditionalHandlers(ctx, msg); err != nil {
				c.logger.Error("additional handler failed",
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
	} else {
		for _, msg := range batchMessages {
			if err := c.runAdditionalHandlers(ctx, msg); err != nil {
				c.logger.Error("additional handler failed",
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

func (c *Consumer) runAdditionalHandlers(ctx context.Context, msg Message) error {
	for _, h := range c.additionalHandlers[msg.Metadata.TableName] {
		if err := h.Handle(ctx, msg); err != nil {
			return fmt.Errorf("%s additional handler: %w", msg.Metadata.TableName, err)
		}
	}
	return nil
}

func (c *Consumer) captureBatchPublishFailure(ctx context.Context, err error, batchCount int) {
	capture := func(hub *sentry.Hub) {
		hub.WithScope(func(scope *sentry.Scope) {
			c.applyBatchPublishFailureSentryScope(scope, batchCount)
			hub.CaptureException(err)
		})
	}
	if hub := sentry.GetHubFromContext(ctx); hub != nil {
		capture(hub)
		return
	}
	capture(sentry.CurrentHub())
}

func (c *Consumer) applyBatchPublishFailureSentryScope(scope *sentry.Scope, batchCount int) {
	telemetry.ApplySentryRuntimeScope(scope, telemetry.SentryRuntime{
		Edition:   string(domain.BuildEdition()),
		Subsystem: telemetry.SubsystemCDC,
	})
	telemetry.SetSentryTag(scope, telemetry.TagConsumer, c.config.ConsumerName)
	telemetry.SetSentryTag(scope, telemetry.TagOperation, "publish_batch")
	scope.SetLevel(sentry.LevelError)
	scope.SetContext("cdc.batch", sentry.Context{
		"consumer":    c.config.ConsumerName,
		"batch_count": batchCount,
	})
	scope.SetFingerprint([]string{"cdc_batch_publish_failed"})
}

func (c *Consumer) registeredTables() []string {
	seen := make(map[string]struct{}, len(c.handlers)+len(c.additionalHandlers))
	for table := range c.handlers {
		seen[table] = struct{}{}
	}
	for table := range c.additionalHandlers {
		seen[table] = struct{}{}
	}
	tables := make([]string, 0, len(seen))
	for table := range seen {
		tables = append(tables, table)
	}
	sort.Strings(tables)
	return tables
}
