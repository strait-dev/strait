package store

import (
	"context"
	"errors"
	"fmt"

	"strait/internal/domain"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

func (q *Queries) UpsertKnownActor(ctx context.Context, id, email, name string) error {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "store.UpsertKnownActor")
	defer span.End()

	query := `
		INSERT INTO known_actors (id, email, name, synced_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (id) DO UPDATE SET
			email = COALESCE(NULLIF(EXCLUDED.email, ''), known_actors.email),
			name = COALESCE(NULLIF(EXCLUDED.name, ''), known_actors.name),
			synced_at = NOW()`

	_, err := q.db.Exec(ctx, query, id, email, name)
	if err != nil {
		return fmt.Errorf("upsert known actor: %w", err)
	}

	return nil
}

func (q *Queries) GetKnownActor(ctx context.Context, id string) (*domain.KnownActor, error) {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "store.GetKnownActor")
	defer span.End()

	query := `SELECT id, email, name, avatar_url, synced_at FROM known_actors WHERE id = $1`

	var actor domain.KnownActor
	var email, name, avatarURL *string
	err := q.db.QueryRow(ctx, query, id).Scan(&actor.ID, &email, &name, &avatarURL, &actor.SyncedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get known actor: %w", err)
	}

	if email != nil {
		actor.Email = *email
	}
	if name != nil {
		actor.Name = *name
	}
	if avatarURL != nil {
		actor.AvatarURL = *avatarURL
	}

	return &actor, nil
}
