package store

import (
	"context"
	"errors"
	"fmt"

	"strait/internal/dbscan"
	"strait/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

func (q *Queries) UpsertNotificationPreference(ctx context.Context, pref *domain.NotificationPreference) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UpsertNotificationPreference")
	defer span.End()

	if pref.ID == "" {
		pref.ID = uuid.Must(uuid.NewV7()).String()
	}
	if pref.Scope == "" {
		pref.Scope = "global"
	}
	if pref.Timezone == "" {
		pref.Timezone = "UTC"
	}
	if pref.DigestPolicy == "" {
		pref.DigestPolicy = "immediate"
	}
	query := `
		INSERT INTO notification_preferences (
			id, recipient_type, recipient_id, scope, channel_prefs, quiet_hours, phone, timezone, digest_policy, critical_override, rate_limit_override
		)
		VALUES ($1, $2, $3, $4, COALESCE($5::jsonb, '{}'::jsonb), $6, $7, $8, $9, $10, $11)
		ON CONFLICT (recipient_type, recipient_id, scope)
		DO UPDATE SET
			channel_prefs = COALESCE(notification_preferences.channel_prefs, '{}'::jsonb) || COALESCE(EXCLUDED.channel_prefs, '{}'::jsonb),
			quiet_hours = EXCLUDED.quiet_hours,
			phone = EXCLUDED.phone,
			timezone = EXCLUDED.timezone,
			digest_policy = EXCLUDED.digest_policy,
			critical_override = EXCLUDED.critical_override,
			rate_limit_override = EXCLUDED.rate_limit_override,
			updated_at = NOW()
		RETURNING id, created_at, updated_at`

	err := q.db.QueryRow(
		ctx,
		query,
		pref.ID,
		pref.RecipientType,
		pref.RecipientID,
		pref.Scope,
		pref.ChannelPrefs,
		dbscan.NilIfEmptyRawMessage(pref.QuietHours),
		dbscan.NilIfEmptyString(pref.Phone),
		pref.Timezone,
		pref.DigestPolicy,
		pref.CriticalOverride,
		pref.RateLimitOverride,
	).Scan(&pref.ID, &pref.CreatedAt, &pref.UpdatedAt)
	if err != nil {
		return fmt.Errorf("upsert notification preference: %w", err)
	}

	return nil
}

func (q *Queries) GetNotificationPreference(ctx context.Context, recipientType, recipientID, scope string) (*domain.NotificationPreference, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetNotificationPreference")
	defer span.End()

	query := `
		SELECT id, recipient_type, recipient_id, scope, channel_prefs, quiet_hours, phone, timezone, digest_policy, critical_override, rate_limit_override, created_at, updated_at
		FROM notification_preferences
		WHERE recipient_type = $1 AND recipient_id = $2 AND scope = $3`

	pref, err := scanNotificationPreference(q.db.QueryRow(ctx, query, recipientType, recipientID, scope))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotificationPreferenceNotFound
		}
		return nil, fmt.Errorf("get notification preference: %w", err)
	}

	return pref, nil
}

func (q *Queries) ListNotificationPreferences(ctx context.Context, recipientType, recipientID string) ([]domain.NotificationPreference, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListNotificationPreferences")
	defer span.End()

	query := `
		SELECT id, recipient_type, recipient_id, scope, channel_prefs, quiet_hours, phone, timezone, digest_policy, critical_override, rate_limit_override, created_at, updated_at
		FROM notification_preferences
		WHERE recipient_type = $1 AND recipient_id = $2
		ORDER BY created_at DESC`

	rows, err := q.db.Query(ctx, query, recipientType, recipientID)
	if err != nil {
		return nil, fmt.Errorf("list notification preferences: %w", err)
	}
	defer rows.Close()

	preferences := make([]domain.NotificationPreference, 0, 8)
	for rows.Next() {
		pref, scanErr := scanNotificationPreference(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list notification preferences scan: %w", scanErr)
		}
		preferences = append(preferences, *pref)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list notification preferences rows: %w", err)
	}

	return preferences, nil
}

func (q *Queries) DisableNotificationChannelPreference(ctx context.Context, recipientType, recipientID, scope, channel string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DisableNotificationChannelPreference")
	defer span.End()

	return q.setNotificationChannelPreference(ctx, recipientType, recipientID, scope, channel, false)
}

func (q *Queries) EnableNotificationChannelPreference(ctx context.Context, recipientType, recipientID, scope, channel string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.EnableNotificationChannelPreference")
	defer span.End()

	return q.setNotificationChannelPreference(ctx, recipientType, recipientID, scope, channel, true)
}

func (q *Queries) setNotificationChannelPreference(ctx context.Context, recipientType, recipientID, scope, channel string, enabled bool) error {
	if scope == "" {
		scope = "global"
	}
	if channel == "" {
		return &domain.FieldError{Field: "channel"}
	}

	query := `
		INSERT INTO notification_preferences (
			recipient_type, recipient_id, scope, channel_prefs
		)
		VALUES (
			$1, $2, $3, jsonb_build_object($4::text, $5::boolean)
		)
		ON CONFLICT (recipient_type, recipient_id, scope)
		DO UPDATE SET
			channel_prefs = jsonb_set(
				CASE
					WHEN jsonb_typeof(notification_preferences.channel_prefs) = 'object' THEN notification_preferences.channel_prefs
					ELSE '{}'::jsonb
				END,
				ARRAY[$4::text],
				to_jsonb($5::boolean),
				true
			),
			updated_at = NOW()`

	if _, err := q.db.Exec(ctx, query, recipientType, recipientID, scope, channel, enabled); err != nil {
		action := "disable"
		if enabled {
			action = "enable"
		}
		return fmt.Errorf("%s notification channel preference: %w", action, err)
	}

	return nil
}

func scanNotificationPreference(scanner scanTarget) (*domain.NotificationPreference, error) {
	var pref domain.NotificationPreference
	var quietHours []byte
	var phone *string

	err := scanner.Scan(
		&pref.ID,
		&pref.RecipientType,
		&pref.RecipientID,
		&pref.Scope,
		&pref.ChannelPrefs,
		&quietHours,
		&phone,
		&pref.Timezone,
		&pref.DigestPolicy,
		&pref.CriticalOverride,
		&pref.RateLimitOverride,
		&pref.CreatedAt,
		&pref.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if len(quietHours) > 0 {
		pref.QuietHours = quietHours
	}
	if phone != nil {
		pref.Phone = *phone
	}
	if len(pref.ChannelPrefs) == 0 {
		pref.ChannelPrefs = []byte("{}")
	}

	return &pref, nil
}
