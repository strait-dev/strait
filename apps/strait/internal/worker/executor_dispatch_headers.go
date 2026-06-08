package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	straitcrypto "strait/internal/crypto"
	"strait/internal/domain"

	"github.com/golang-jwt/jwt/v5"
)

// addHMACHeaders injects X-Strait-Signature and X-Strait-Timestamp into
// headers when the job has an endpoint_signing_secret configured. The
// signature covers "<unix_timestamp>.<body>" using HMAC-SHA256.
func addHMACHeaders(headers map[string]string, secret string, body []byte) {
	if secret == "" {
		return
	}
	ts := strconv.FormatInt(time.Now().UTC().Unix(), 10)
	headers["X-Strait-Timestamp"] = ts
	headers["X-Strait-Signature"] = SignHTTPDispatch(secret, ts, body)
}

func (e *Executor) endpointSigningSecret(job *domain.Job) (string, error) {
	secret, err := straitcrypto.DecryptField(e.secretDecryptor, job.EndpointSigningSecret)
	if err != nil {
		return "", fmt.Errorf("decrypt endpoint signing secret: %w", err)
	}
	return secret, nil
}

func dispatchSecretsCacheKey(job *domain.Job) string {
	return "secrets:" + job.ID + ":" + job.EnvironmentID
}

func (e *Executor) dispatchSecrets(ctx context.Context, job *domain.Job) ([]domain.JobSecret, error) {
	secretsCacheKey := dispatchSecretsCacheKey(job)
	if cached, ok := dispatchCacheGet[[]domain.JobSecret](ctx, secretsCacheKey); ok {
		return cached, nil
	}

	secrets, err := e.store.ListJobSecretsByJob(ctx, job.ID, job.EnvironmentID)
	if err != nil {
		return nil, fmt.Errorf("load job %s secrets: %w", job.ID, err)
	}
	dispatchCacheSet(ctx, secretsCacheKey, secrets)
	return secrets, nil
}

type dispatchHeaderInputs struct {
	secrets    []domain.JobSecret
	checkpoint *domain.RunCheckpoint
}

func (e *Executor) dispatchHeaderInputs(ctx context.Context, job *domain.Job, run *domain.JobRun) (dispatchHeaderInputs, error) {
	secrets, err := e.dispatchSecrets(ctx, job)
	if err != nil {
		return dispatchHeaderInputs{}, err
	}
	return dispatchHeaderInputs{
		secrets:    secrets,
		checkpoint: e.dispatchCheckpoint(ctx, run),
	}, nil
}

func (e *Executor) dispatchHeaders(ctx context.Context, job *domain.Job, run *domain.JobRun) (map[string]string, error) {
	inputs, err := e.dispatchHeaderInputs(ctx, job, run)
	if err != nil {
		return nil, err
	}
	return e.buildDispatchHeaders(job, run, inputs.secrets, inputs.checkpoint)
}

// buildDispatchHeaders constructs the headers injected on an HTTP dispatch: the
// job's decrypted secrets (X-Secret-*), the run-token JWT (X-Run-Token) the
// endpoint SDK uses to call back to Strait, the HMAC body+timestamp signature,
// and on retries the durable-resume headers (X-Last-Checkpoint / X-Checkpoint-At
// / X-Previous-Error). It is shared by the primary and fallback dispatch paths so
// failover preserves authentication and durable-resume semantics rather than
// silently dropping them.
func (e *Executor) buildDispatchHeaders(job *domain.Job, run *domain.JobRun, secrets []domain.JobSecret, cp *domain.RunCheckpoint) (map[string]string, error) {
	headerCount := len(secrets)
	if e.jwtSigningKey != "" {
		headerCount++
	}
	if job.EndpointSigningSecret != "" {
		headerCount += 2
	}
	if run.Attempt > 1 {
		headerCount++
		if cp != nil {
			headerCount += 2
		}
	}
	headers := make(map[string]string, headerCount)
	for _, secret := range secrets {
		value := secret.Value
		if value == "" {
			value = secret.EncryptedValue
		}
		headers["X-Secret-"+secret.SecretKey] = value
	}

	// Generate a JWT run token so the endpoint's SDK can call back to Strait.
	if e.jwtSigningKey != "" {
		expiresAt := time.Now().Add(time.Duration(job.TimeoutSecs)*time.Second + 60*time.Second)
		if run.ExpiresAt != nil {
			expiresAt = *run.ExpiresAt
		}
		claims := struct {
			Attempt int `json:"attempt,omitempty"`
			jwt.RegisteredClaims
		}{
			Attempt: run.Attempt,
			RegisteredClaims: jwt.RegisteredClaims{
				Issuer:    domain.RunTokenIssuer,
				Subject:   run.ID,
				ExpiresAt: jwt.NewNumericDate(expiresAt),
				IssuedAt:  jwt.NewNumericDate(time.Now()),
			},
		}
		tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		if signed, signErr := tok.SignedString([]byte(e.jwtSigningKey)); signErr == nil {
			headers["X-Run-Token"] = signed
		}
	}

	// Add HMAC body+timestamp signing so the endpoint can verify request authenticity.
	signingSecret, err := e.endpointSigningSecret(job)
	if err != nil {
		return nil, err
	}
	addHMACHeaders(headers, signingSecret, run.Payload)

	if run.Attempt > 1 {
		if cp != nil {
			data, _ := json.Marshal(cp.State)
			if len(data) <= 65536 {
				headers["X-Last-Checkpoint"] = string(data)
				headers["X-Checkpoint-At"] = cp.CreatedAt.Format(time.RFC3339)
			}
		}
		if run.Error != "" {
			headers["X-Previous-Error"] = run.Error
		}
	}
	return headers, nil
}

// dispatchCheckpoint loads the latest run checkpoint for a retry, preferring the
// per-execution dispatch cache populated by the primary path so the fallback path
// reuses it instead of re-querying. Returns nil on the first attempt.
func (e *Executor) dispatchCheckpoint(ctx context.Context, run *domain.JobRun) *domain.RunCheckpoint {
	if run.Attempt <= 1 {
		return nil
	}
	checkpointCacheKey := "checkpoint:" + run.ID
	if cached, ok := dispatchCacheGet[*domain.RunCheckpoint](ctx, checkpointCacheKey); ok {
		return cached
	}
	cp, _ := e.store.GetLatestCheckpoint(ctx, run.ID)
	if cp != nil {
		dispatchCacheSet(ctx, checkpointCacheKey, cp)
	}
	return cp
}
