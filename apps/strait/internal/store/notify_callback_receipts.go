package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

func (q *Queries) RecordNotifyProviderCallbackReceipt(
	ctx context.Context,
	projectID, providerID, provider, callbackID, eventType, messageID, payloadHash string,
	expiresAt time.Time,
) (bool, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.RecordNotifyProviderCallbackReceipt")
	defer span.End()

	if expiresAt.IsZero() {
		expiresAt = time.Now().UTC().Add(30 * 24 * time.Hour)
	}

	query := `
		INSERT INTO notify_provider_callback_receipts (
			project_id,
			provider_id,
			provider,
			callback_id,
			event_type,
			message_id,
			payload_hash,
			expires_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (project_id, provider_id, callback_id)
		DO NOTHING
		RETURNING id`

	var id string
	err := q.db.QueryRow(ctx, query,
		projectID,
		providerID,
		provider,
		callbackID,
		eventType,
		messageID,
		payloadHash,
		expiresAt,
	).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("record notify provider callback receipt: %w", err)
	}

	return true, nil
}

func (q *Queries) DeleteNotifyProviderCallbackReceipt(ctx context.Context, projectID, providerID, callbackID string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DeleteNotifyProviderCallbackReceipt")
	defer span.End()

	const query = `
		DELETE FROM notify_provider_callback_receipts
		WHERE project_id = $1 AND provider_id = $2 AND callback_id = $3`
	if _, err := q.db.Exec(ctx, query, projectID, providerID, callbackID); err != nil {
		return fmt.Errorf("delete notify provider callback receipt: %w", err)
	}

	return nil
}
