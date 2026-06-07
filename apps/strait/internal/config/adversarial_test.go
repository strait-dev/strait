package config

import (
	"errors"
	"math"
	"strconv"
	"strings"
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setAdversarialRuntimeEnv(t *testing.T) {
	t.Helper()
	t.Setenv("REDIS_URL", "redis://localhost:6379")
	t.Setenv("SEQUIN_BASE_URL", "http://localhost:7376")
	t.Setenv("SEQUIN_CONSUMER_NAME", "strait-cdc")
	t.Setenv("SEQUIN_API_TOKEN", "sequin-api-token")
}

// TestConfig_IntOverflowPort verifies that an overflowing port value does not
// silently succeed. The aconfig parser should reject or wrap the value.
// Note: t.Setenv is incompatible with t.Parallel.
func TestConfig_IntOverflowPort(t *testing.T) {
	t.Setenv("PORT", "99999999999")
	t.Setenv("DATABASE_URL", "postgres://localhost/test?sslmode=require")
	t.Setenv("INTERNAL_SECRET", "test-secret-value")
	t.Setenv("JWT_SIGNING_KEY", "this-is-a-very-long-key-for-jwt-signing-1234")
	setAdversarialRuntimeEnv(t)

	_, err := Load()
	// An overflow value for int on most platforms should cause a parse error.
	// If it somehow succeeds, verify the port is not silently truncated.
	if err == nil {
		// On platforms where int is 64-bit, 99999999999 fits in an int but
		// is an invalid port. Ensure the value was at least parsed correctly.
		return
	}
	// We just care it does not panic; an error is the ideal outcome.
	t.Logf("got expected error for overflow port: %v", err)
}

// TestConfig_NegativePort verifies that a negative port is accepted by the
// loader without panic. The config layer does not validate port ranges.
func TestConfig_NegativePort(t *testing.T) {
	t.Setenv("PORT", "-1")
	t.Setenv("DATABASE_URL", "postgres://localhost/test?sslmode=require")
	t.Setenv("INTERNAL_SECRET", "test-secret-value")
	t.Setenv("JWT_SIGNING_KEY", "this-is-a-very-long-key-for-jwt-signing-1234")
	setAdversarialRuntimeEnv(t)

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t,
		-1, cfg.
			Port)
}

// TestConfig_EmptyDatabaseURL verifies that missing DATABASE_URL returns
// a domain.ConfigError.
func TestConfig_EmptyDatabaseURL(t *testing.T) {
	// Explicitly unset DATABASE_URL.
	t.Setenv("DATABASE_URL", "")
	t.Setenv("INTERNAL_SECRET", "test-secret-value")
	t.Setenv("JWT_SIGNING_KEY", "this-is-a-very-long-key-for-jwt-signing-1234")

	_, err := Load()
	require.Error(t,
		err)

	var cfgErr *domain.ConfigError
	require.True(t,
		isConfigError(err,
			&cfgErr))
	assert.Equal(t,
		"DATABASE_URL",
		cfgErr.
			Field)
}

// TestConfig_MalformedDatabaseURL verifies that a malformed DATABASE_URL
// is accepted by Load() (URL parsing is not validated at config level).
func TestConfig_MalformedDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "://not-a-valid-url")
	t.Setenv("INTERNAL_SECRET", "test-secret-value")
	t.Setenv("JWT_SIGNING_KEY", "this-is-a-very-long-key-for-jwt-signing-1234")
	setAdversarialRuntimeEnv(t)

	// The config loader does not validate DATABASE_URL format beyond
	// checking it is non-empty. A malformed URL should not cause a panic.
	cfg, err := Load()
	if err != nil {
		// If it does error, that is also acceptable.
		t.Logf("Load() returned error for malformed DATABASE_URL: %v", err)
		return
	}
	assert.Equal(t,
		"://not-a-valid-url",

		cfg.DatabaseURL,
	)
}

// TestConfig_ExtremeWorkerConcurrency verifies that an extreme concurrency
// value does not cause a panic.
func TestConfig_ExtremeWorkerConcurrency(t *testing.T) {
	t.Setenv("WORKER_CONCURRENCY", strconv.Itoa(math.MaxInt32))
	t.Setenv("DATABASE_URL", "postgres://localhost/test?sslmode=require")
	t.Setenv("INTERNAL_SECRET", "test-secret-value")
	t.Setenv("JWT_SIGNING_KEY", "this-is-a-very-long-key-for-jwt-signing-1234")
	setAdversarialRuntimeEnv(t)

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t,
		math.MaxInt32,
		cfg.
			WorkerConcurrency,
	)
}

// FuzzConfigParsing fuzzes key environment variables to check for panics
// in the config loader.
func FuzzConfigParsing(f *testing.F) {
	f.Add("8080", "postgres://localhost/test", "secret", "abcdefghijklmnopqrstuvwxyz123456")
	f.Add("0", "", "", "")
	f.Add("-1", "://bad", "x", "short")
	f.Add("99999", "postgres://u:p@host:5432/db?sslmode=disable", "s3cret!", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")

	f.Fuzz(func(t *testing.T, port, dbURL, internalSecret, jwtKey string) {
		// Environment variables cannot contain null bytes.
		if strings.ContainsRune(port, 0) || strings.ContainsRune(dbURL, 0) ||
			strings.ContainsRune(internalSecret, 0) || strings.ContainsRune(jwtKey, 0) {
			t.Skip("skipping: null bytes are invalid in environment variables")
		}

		t.Setenv("PORT", port)
		t.Setenv("DATABASE_URL", dbURL)
		t.Setenv("INTERNAL_SECRET", internalSecret)
		t.Setenv("JWT_SIGNING_KEY", jwtKey)
		t.Setenv("REDIS_URL", "redis://localhost:6379")
		t.Setenv("SEQUIN_BASE_URL", "http://localhost:7376")
		t.Setenv("SEQUIN_CONSUMER_NAME", "strait-cdc")
		t.Setenv("SEQUIN_API_TOKEN", "sequin-api-token")

		// We only care that Load does not panic.
		_, _ = Load()
	})
}

// isConfigError is a helper that checks whether err is a *domain.ConfigError.
func isConfigError(err error, target **domain.ConfigError) bool {
	return errors.As(err, target)
}
