package store

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"strait/internal/dbscan"
	"strait/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

func (q *Queries) CreateJobSecret(ctx context.Context, secret *domain.JobSecret) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateJobSecret")
	defer span.End()

	if secret.ID == "" {
		secret.ID = uuid.Must(uuid.NewV7()).String()
	}
	if secret.KeyVersion == 0 {
		secret.KeyVersion = 1
	}

	encryptionKey, err := q.secretKey()
	if err != nil {
		return fmt.Errorf("create job secret: %w", err)
	}

	encrypted, err := encryptSecret(secret.EncryptedValue, encryptionKey)
	if err != nil {
		return fmt.Errorf("create job secret: %w", err)
	}

	query := `
		INSERT INTO job_secrets (id, project_id, job_id, environment, secret_key, encrypted_value, key_version)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING created_at, updated_at`

	err = q.db.QueryRow(
		ctx,
		query,
		secret.ID,
		secret.ProjectID,
		dbscan.NilIfEmptyString(secret.JobID),
		secret.Environment,
		secret.SecretKey,
		encrypted,
		secret.KeyVersion,
	).Scan(&secret.CreatedAt, &secret.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create job secret: %w", err)
	}

	return nil
}

func (q *Queries) GetJobSecret(ctx context.Context, id string) (*domain.JobSecret, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetJobSecret")
	defer span.End()

	encryptionKey, err := q.secretKey()
	if err != nil {
		return nil, fmt.Errorf("get job secret: %w", err)
	}

	query := `
		SELECT id, project_id, job_id, environment, secret_key, encrypted_value, key_version, created_at, updated_at
		FROM job_secrets
		WHERE id = $1`

	secret, err := scanJobSecret(q.db.QueryRow(ctx, query, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrJobSecretNotFound
		}
		return nil, fmt.Errorf("get job secret: %w", err)
	}

	decrypted, err := decryptSecret(secret.EncryptedValue, encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("get job secret: %w", err)
	}
	secret.EncryptedValue = decrypted

	return secret, nil
}

func (q *Queries) ListJobSecrets(ctx context.Context, projectID, jobID, environment string, limit int, cursor *time.Time) ([]domain.JobSecret, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListJobSecrets")
	defer span.End()

	encryptionKey, err := q.secretKey()
	if err != nil {
		return nil, fmt.Errorf("list job secrets: %w", err)
	}

	query := `
		SELECT id, project_id, job_id, environment, secret_key, encrypted_value, key_version, created_at, updated_at
		FROM job_secrets
		WHERE project_id = $1`
	args := []any{projectID}
	param := 2

	if jobID != "" {
		query += fmt.Sprintf(" AND job_id = $%d", param)
		args = append(args, jobID)
		param++
	}

	if environment != "" {
		query += fmt.Sprintf(" AND environment = $%d", param)
		args = append(args, environment)
		param++
	}

	if cursor != nil {
		query += fmt.Sprintf(" AND created_at < $%d", param)
		args = append(args, *cursor)
		param++
	}

	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d", param)
	args = append(args, limit)

	rows, err := q.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list job secrets: %w", err)
	}
	defer rows.Close()

	secrets := make([]domain.JobSecret, 0, limit)
	for rows.Next() {
		secret, scanErr := scanJobSecret(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list job secrets scan: %w", scanErr)
		}

		decrypted, decryptErr := decryptSecret(secret.EncryptedValue, encryptionKey)
		if decryptErr != nil {
			return nil, fmt.Errorf("list job secrets decrypt: %w", decryptErr)
		}
		secret.EncryptedValue = decrypted

		secrets = append(secrets, *secret)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list job secrets rows: %w", err)
	}

	return secrets, nil
}

func (q *Queries) DeleteJobSecret(ctx context.Context, id string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DeleteJobSecret")
	defer span.End()

	query := `DELETE FROM job_secrets WHERE id = $1`
	tag, err := q.db.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("delete job secret: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrJobSecretNotFound
	}

	return nil
}

func (q *Queries) ListJobSecretsByJob(ctx context.Context, jobID, environment string) ([]domain.JobSecret, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListJobSecretsByJob")
	defer span.End()

	encryptionKey, err := q.secretKey()
	if err != nil {
		return nil, fmt.Errorf("list job secrets by job: %w", err)
	}

	query := `
		SELECT s.id, s.project_id, s.job_id, s.environment, s.secret_key, s.encrypted_value, s.key_version, s.created_at, s.updated_at
		FROM job_secrets s
		WHERE s.project_id = (SELECT project_id FROM jobs WHERE id = $1)
		  AND (s.job_id = $1 OR s.job_id IS NULL)
		  AND s.environment = $2
		ORDER BY s.created_at ASC`

	rows, err := q.db.Query(ctx, query, jobID, environment)
	if err != nil {
		return nil, fmt.Errorf("list job secrets by job: %w", err)
	}
	defer rows.Close()

	secrets := make([]domain.JobSecret, 0, 64)
	for rows.Next() {
		secret, scanErr := scanJobSecret(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list job secrets by job scan: %w", scanErr)
		}

		decrypted, decryptErr := decryptSecret(secret.EncryptedValue, encryptionKey)
		if decryptErr != nil {
			return nil, fmt.Errorf("list job secrets by job decrypt: %w", decryptErr)
		}
		secret.EncryptedValue = decrypted

		secrets = append(secrets, *secret)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list job secrets by job rows: %w", err)
	}

	return secrets, nil
}

func (q *Queries) secretKey() ([]byte, error) {
	if q.secretEncryptionKey == "" {
		return nil, fmt.Errorf("secret encryption key is not configured")
	}

	sum := sha256.Sum256([]byte(q.secretEncryptionKey))
	return sum[:], nil
}

func encryptSecret(plaintext string, key []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("encrypt secret: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("encrypt secret: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("encrypt secret: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, []byte(plaintext), nil)
	payload := make([]byte, 0, len(nonce)+len(ciphertext))
	payload = append(payload, nonce...)
	payload = append(payload, ciphertext...)

	return hex.EncodeToString(payload), nil
}

func decryptSecret(ciphertext string, key []byte) (string, error) {
	raw, err := hex.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("decrypt secret: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("decrypt secret: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("decrypt secret: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(raw) < nonceSize {
		return "", fmt.Errorf("decrypt secret: ciphertext too short")
	}

	nonce := raw[:nonceSize]
	encrypted := raw[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, encrypted, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt secret: %w", err)
	}

	return string(plaintext), nil
}

func scanJobSecret(scanner scanTarget) (*domain.JobSecret, error) {
	var secret domain.JobSecret
	var jobID *string

	err := scanner.Scan(
		&secret.ID,
		&secret.ProjectID,
		&jobID,
		&secret.Environment,
		&secret.SecretKey,
		&secret.EncryptedValue,
		&secret.KeyVersion,
		&secret.CreatedAt,
		&secret.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	if jobID != nil {
		secret.JobID = *jobID
	}

	return &secret, nil
}
