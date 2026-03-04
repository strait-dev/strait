package cdc

import (
	"context"
	"encoding/json"
)

// Action represents the type of CDC change.
type Action string

const (
	ActionInsert Action = "insert"
	ActionUpdate Action = "update"
	ActionDelete Action = "delete"
	ActionRead   Action = "read"
)

// Message represents a single CDC change event from Sequin Stream.
type Message struct {
	AckID    string          `json:"ack_id"`
	Record   json.RawMessage `json:"record"`
	Changes  json.RawMessage `json:"changes,omitempty"`
	Action   Action          `json:"action"`
	Metadata Metadata        `json:"metadata"`
}

// Metadata contains context about the change event.
type Metadata struct {
	TableSchema     string `json:"table_schema"`
	TableName       string `json:"table_name"`
	CommitTimestamp string `json:"commit_timestamp"`
	IdempotencyKey  string `json:"idempotency_key"`
	ConsumerName    string `json:"-"`
}

// Handler processes CDC messages for a specific table.
type Handler interface {
	// Table returns the table name this handler processes (e.g., "job_runs").
	Table() string
	// Handle processes a single CDC message. Return nil to ack, error to nack.
	Handle(ctx context.Context, msg Message) error
}

// HandlerFunc is a convenience adapter for simple handlers.
type HandlerFunc struct {
	TableName string
	Fn        func(ctx context.Context, msg Message) error
}

func (h HandlerFunc) Table() string { return h.TableName }

func (h HandlerFunc) Handle(ctx context.Context, msg Message) error { return h.Fn(ctx, msg) }

// ConsumerConfig holds configuration for the CDC consumer.
type ConsumerConfig struct {
	// BaseURL is the Sequin API base URL (e.g., "http://localhost:7376").
	BaseURL string
	// ConsumerName is the Sequin Stream consumer name or ID.
	ConsumerName string
	// Credential is the authentication token for Sequin API.
	Credential string
	// BatchSize is the number of messages to fetch per receive call (default 10).
	BatchSize int
	// WaitTimeMs is the long-poll wait time in milliseconds (default 5000).
	WaitTimeMs int
	// VisibilityTimeoutMs overrides the visibility timeout per receive (0 = use server default).
	VisibilityTimeoutMs int
}
