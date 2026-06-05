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
	"io"
	"log/slog"
	"time"

	"golang.org/x/crypto/hkdf"

	"strait/internal/crypto"
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
	if secret.JobID != "" && secret.Environment != "" {
		var jobProjectID, jobEnvironmentID string
		err := q.db.QueryRow(ctx, `SELECT project_id, COALESCE(environment_id, '') FROM jobs WHERE id = $1`, secret.JobID).Scan(&jobProjectID, &jobEnvironmentID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrJobNotFound
			}
			return fmt.Errorf("create job secret verify job: %w", err)
		}
		if jobProjectID != secret.ProjectID {
			return fmt.Errorf("create job secret: job does not belong to project")
		}
		if jobEnvironmentID != "" && jobEnvironmentID != secret.Environment {
			return fmt.Errorf("create job secret: secret environment does not match job environment")
		}
	}

	encryptionKey, err := q.secretKey()
	if err != nil {
		return fmt.Errorf("create job secret: %w", err)
	}

	plaintext := secret.Value
	if plaintext == "" {
		plaintext = secret.EncryptedValue
	}
	encrypted, err := encryptSecret(plaintext, encryptionKey)
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

func (q *Queries) GetJobSecret(ctx context.Context, id, projectID string) (*domain.JobSecret, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetJobSecret")
	defer span.End()

	query := `
		SELECT id, project_id, job_id, environment, secret_key, encrypted_value, key_version, created_at, updated_at
		FROM job_secrets
		WHERE id = $1 AND project_id = $2`

	secret, err := scanJobSecret(q.db.QueryRow(ctx, query, id, projectID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrJobSecretNotFound
		}
		return nil, fmt.Errorf("get job secret: %w", err)
	}

	decrypted, err := q.decryptSecretWithFallback(secret.EncryptedValue)
	if err != nil {
		return nil, fmt.Errorf("get job secret: %w", err)
	}
	secret.Value = decrypted

	return secret, nil
}

func (q *Queries) ListJobSecrets(ctx context.Context, projectID, jobID, environment string, limit int, cursor *time.Time) ([]domain.JobSecret, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListJobSecrets")
	defer span.End()

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

		decrypted, decryptErr := q.decryptSecretWithFallback(secret.EncryptedValue)
		if decryptErr != nil {
			return nil, fmt.Errorf("list job secrets decrypt: %w", decryptErr)
		}
		secret.Value = decrypted

		secrets = append(secrets, *secret)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list job secrets rows: %w", err)
	}

	return secrets, nil
}

func (q *Queries) DeleteJobSecret(ctx context.Context, id, projectID string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DeleteJobSecret")
	defer span.End()

	query := `DELETE FROM job_secrets WHERE id = $1 AND project_id = $2`
	tag, err := q.db.Exec(ctx, query, id, projectID)
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

	query := `
		SELECT s.id, s.project_id, s.job_id, s.environment, s.secret_key, s.encrypted_value, s.key_version, s.created_at, s.updated_at
			FROM job_secrets s
			JOIN jobs j ON j.id = $1
			WHERE s.project_id = j.project_id
			  AND s.job_id = $1
			  AND s.environment = COALESCE(
			    NULLIF(j.environment_id, ''),
			    NULLIF($2, ''),
			    (
			      SELECT e.id
			      FROM environments e
			      WHERE e.project_id = j.project_id
			        AND e.slug = $3
			      LIMIT 1
			    ),
			    $3
			  )
			ORDER BY s.secret_key ASC, s.created_at ASC`

	rows, err := q.db.Query(ctx, query, jobID, environment, "production")
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

		decrypted, decryptErr := q.decryptSecretWithFallback(secret.EncryptedValue)
		if decryptErr != nil {
			return nil, fmt.Errorf("list job secrets by job decrypt: %w", decryptErr)
		}
		secret.Value = decrypted

		secrets = append(secrets, *secret)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list job secrets by job rows: %w", err)
	}

	return secrets, nil
}

func (q *Queries) secretKey() ([]byte, error) {
	return deriveSecretKey(q.secretEncryptionKey)
}

func deriveSecretKey(secretEncryptionKey string) ([]byte, error) {
	if secretEncryptionKey == "" {
		return nil, fmt.Errorf("secret encryption key is not configured")
	}

	hkdfReader := hkdf.New(sha256.New, []byte(secretEncryptionKey), []byte("secret-store-encryption"), nil)
	derived := make([]byte, 32)
	if _, err := io.ReadFull(hkdfReader, derived); err != nil {
		return nil, fmt.Errorf("hkdf derive secret key: %w", err)
	}
	return derived, nil
}

// secretKeyLegacy returns the old SHA-256 derived key for backward-compatible
// decryption of secrets encrypted before the HKDF migration.
func deriveSecretKeyLegacy(secretEncryptionKey string) ([]byte, error) {
	if secretEncryptionKey == "" {
		return nil, fmt.Errorf("secret encryption key is not configured")
	}

	sum := sha256.Sum256([]byte(secretEncryptionKey))
	return sum[:], nil
}

func (q *Queries) secretEncryptionKeyCandidates() []string {
	if q.secretEncryptionKey == "" {
		return nil
	}
	candidates := []string{q.secretEncryptionKey}
	seen := map[string]struct{}{q.secretEncryptionKey: {}}
	for _, key := range q.oldSecretEncryptionKeys {
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		candidates = append(candidates, key)
	}
	return candidates
}

func (q *Queries) secretEncryptor() (*crypto.KeyRotator, error) {
	if q.secretEncryptionKey == "" {
		return nil, fmt.Errorf("secret encryption key is not configured")
	}
	return crypto.NewKeyRotatorFromStrings(q.secretEncryptionKey, q.oldSecretEncryptionKeys...)
}

// decryptSecretWithFallback tries the HKDF-derived key first, then falls back
// to old primary keys and legacy SHA-256 derivation for pre-migration secrets.
func (q *Queries) decryptSecretWithFallback(ciphertext string) (string, error) {
	var firstErr error
	for i, candidate := range q.secretEncryptionKeyCandidates() {
		primaryKey, err := deriveSecretKey(candidate)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}

		plaintext, err := decryptSecret(ciphertext, primaryKey)
		if err == nil {
			if i > 0 {
				slog.Warn("decrypted secret using old encryption key; re-encrypt to use primary key")
			}
			return plaintext, nil
		}
		if firstErr == nil {
			firstErr = err
		}

		legacyKey, legacyErr := deriveSecretKeyLegacy(candidate)
		if legacyErr != nil {
			continue
		}

		plaintext, legacyErr = decryptSecret(ciphertext, legacyKey)
		if legacyErr == nil {
			slog.Warn("decrypted secret using legacy SHA-256 key; re-encrypt to use HKDF-derived key")
			return plaintext, nil
		}
	}

	if firstErr != nil {
		return "", firstErr
	}
	return "", fmt.Errorf("secret encryption key is not configured")
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
