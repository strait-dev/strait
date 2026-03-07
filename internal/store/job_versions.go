package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"orchestrator/internal/dbscan"
	"orchestrator/internal/domain"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

func (q *Queries) CreateJobVersion(ctx context.Context, v *domain.JobVersion) error {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "store.CreateJobVersion")
	defer span.End()

	query := `
		INSERT INTO job_versions (id, job_id, version, name, slug, description, cron, payload_schema,
			tags, endpoint_url, fallback_endpoint_url, max_attempts, timeout_secs, webhook_url, webhook_secret, run_ttl_secs)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb, $10, $11, $12, $13, $14, $15, $16)
		RETURNING created_at`

	var desc, cronStr, webhookURL, webhookSecret *string
	var payloadSchema []byte
	var runTTL *int
	if v.Description != "" {
		desc = &v.Description
	}
	if v.Cron != "" {
		cronStr = &v.Cron
	}
	if len(v.PayloadSchema) > 0 {
		payloadSchema = v.PayloadSchema
	}
	if v.WebhookURL != "" {
		webhookURL = &v.WebhookURL
	}
	if v.WebhookSecret != "" {
		webhookSecret = &v.WebhookSecret
	}
	if v.RunTTLSecs > 0 {
		runTTL = &v.RunTTLSecs
	}

	tagsJSON, err := marshalJobTags(v.Tags)
	if err != nil {
		return fmt.Errorf("create job version: %w", err)
	}

	return q.db.QueryRow(ctx, query,
		v.ID, v.JobID, v.Version, v.Name, v.Slug, desc, cronStr, payloadSchema,
		tagsJSON, v.EndpointURL, dbscan.NilIfEmptyString(v.FallbackEndpointURL), v.MaxAttempts, v.TimeoutSecs, webhookURL, webhookSecret, runTTL,
	).Scan(&v.CreatedAt)
}

func (q *Queries) ListJobVersionsByJob(ctx context.Context, jobID string) ([]domain.JobVersion, error) {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "store.ListJobVersionsByJob")
	defer span.End()

	query := `
		SELECT id, job_id, version, name, slug, description, cron, payload_schema,
		       tags, endpoint_url, fallback_endpoint_url, max_attempts, timeout_secs, webhook_url, webhook_secret, run_ttl_secs, created_at
		FROM job_versions
		WHERE job_id = $1
		ORDER BY version DESC`

	rows, err := q.db.Query(ctx, query, jobID)
	if err != nil {
		return nil, fmt.Errorf("list job versions: %w", err)
	}
	defer rows.Close()

	versions := make([]domain.JobVersion, 0, 16)
	for rows.Next() {
		v, err := scanJobVersion(rows)
		if err != nil {
			return nil, fmt.Errorf("list job versions scan: %w", err)
		}
		versions = append(versions, *v)
	}
	return versions, rows.Err()
}

func (q *Queries) GetJobVersion(ctx context.Context, jobID string, version int) (*domain.JobVersion, error) {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "store.GetJobVersion")
	defer span.End()

	query := `
		SELECT id, job_id, version, name, slug, description, cron, payload_schema,
		       tags, endpoint_url, fallback_endpoint_url, max_attempts, timeout_secs, webhook_url, webhook_secret, run_ttl_secs, created_at
		FROM job_versions
		WHERE job_id = $1 AND version = $2`

	v, err := scanJobVersion(q.db.QueryRow(ctx, query, jobID, version))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("job version not found")
		}
		return nil, fmt.Errorf("get job version: %w", err)
	}
	return v, nil
}

func scanJobVersion(scanner scanTarget) (*domain.JobVersion, error) {
	var v domain.JobVersion
	var description, cronStr, webhookURL, webhookSecret *string
	var fallbackEndpointURL *string
	var payloadSchema []byte
	var tagsJSON []byte
	var runTTLSecs *int

	err := scanner.Scan(
		&v.ID, &v.JobID, &v.Version, &v.Name, &v.Slug,
		&description, &cronStr, &payloadSchema,
		&tagsJSON, &v.EndpointURL, &fallbackEndpointURL, &v.MaxAttempts, &v.TimeoutSecs,
		&webhookURL, &webhookSecret, &runTTLSecs,
		&v.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	if description != nil {
		v.Description = *description
	}
	if cronStr != nil {
		v.Cron = *cronStr
	}
	if payloadSchema != nil {
		v.PayloadSchema = json.RawMessage(payloadSchema)
	}
	if len(tagsJSON) > 0 {
		tags, unmarshalErr := unmarshalJobTags(tagsJSON)
		if unmarshalErr != nil {
			return nil, unmarshalErr
		}
		v.Tags = tags
	}
	if fallbackEndpointURL != nil {
		v.FallbackEndpointURL = *fallbackEndpointURL
	}
	if webhookURL != nil {
		v.WebhookURL = *webhookURL
	}
	if webhookSecret != nil {
		v.WebhookSecret = *webhookSecret
	}
	if runTTLSecs != nil {
		v.RunTTLSecs = *runTTLSecs
	}
	return &v, nil
}
