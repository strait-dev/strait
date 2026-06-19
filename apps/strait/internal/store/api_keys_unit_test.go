package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

type apiKeyScanFunc func(dest ...any) error

func (f apiKeyScanFunc) Scan(dest ...any) error {
	return f(dest...)
}

func TestCreateAPIKey(t *testing.T) {
	t.Parallel()

	t.Run("generates id and stores nil optional secret", func(t *testing.T) {
		t.Parallel()

		createdAt := time.Now().UTC()
		var capturedArgs []any
		db := &mockDBTX{
			queryRowFn: func(_ context.Context, _ string, args ...any) pgx.Row {
				capturedArgs = append([]any(nil), args...)
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*time.Time)) = createdAt
					*(dest[1].(*int64)) = 7
					return nil
				}}
			},
		}
		key := &domain.APIKey{
			ProjectID: "project-1",
			Name:      "Deploy",
			KeyHash:   "hash",
			KeyPrefix: "strait_abcde",
			Scopes:    []string{"runs:read"},
		}

		require.NoError(t, New(db).CreateAPIKey(context.Background(), key))
		require.NotEmpty(t, key.ID)
		require.Equal(t, createdAt, key.CreatedAt)
		require.EqualValues(t, 7, key.CacheVersion)
		require.Len(t, capturedArgs, 13)
		require.Nil(t, capturedArgs[12])
	})

	t.Run("keeps provided id and encrypted rotation secret", func(t *testing.T) {
		t.Parallel()

		var capturedArgs []any
		db := &mockDBTX{
			queryRowFn: func(_ context.Context, _ string, args ...any) pgx.Row {
				capturedArgs = append([]any(nil), args...)
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*time.Time)) = time.Now().UTC()
					*(dest[1].(*int64)) = 9
					return nil
				}}
			},
		}
		key := &domain.APIKey{
			ID:                    "key-1",
			ProjectID:             "project-1",
			Name:                  "Rotate",
			KeyHash:               "hash",
			KeyPrefix:             "strait_abcde",
			Scopes:                []string{"runs:write"},
			RotationWebhookSecret: []byte("ciphertext"),
		}

		require.NoError(t, New(db).CreateAPIKey(context.Background(), key))
		require.Equal(t, "key-1", key.ID)
		require.Equal(t, []byte("ciphertext"), capturedArgs[12])
	})

	t.Run("wraps insert scan errors", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			queryRowFn: func(context.Context, string, ...any) pgx.Row {
				return &mockRow{scanFn: func(...any) error {
					return errors.New("insert failed")
				}}
			},
		}
		key := &domain.APIKey{ID: "key-1"}

		err := New(db).CreateAPIKey(context.Background(), key)
		require.ErrorContains(t, err, "create api key")
		require.ErrorContains(t, err, "insert failed")
	})
}

func TestAPIKeyRowCountGuards(t *testing.T) {
	t.Run("revoke evicts touch cache on success", func(t *testing.T) {
		ClearAPIKeyTouchCacheForTest(t)
		recordAPIKeyTouch("key-1", time.Now().UnixNano())

		db := &mockDBTX{
			execFn: func(_ context.Context, _ string, args ...any) (pgconn.CommandTag, error) {
				require.Equal(t, []any{"key-1"}, args)
				return pgconn.NewCommandTag("UPDATE 1"), nil
			},
		}

		require.NoError(t, New(db).RevokeAPIKey(context.Background(), "key-1"))
		_, ok := apiKeyTouchCache.Load("key-1")
		require.False(t, ok)
		require.Zero(t, apiKeyTouchSize.Load())
	})

	t.Run("revoke preserves touch cache when row is missing", func(t *testing.T) {
		ClearAPIKeyTouchCacheForTest(t)
		recordAPIKeyTouch("key-1", time.Now().UnixNano())

		db := &mockDBTX{
			execFn: func(context.Context, string, ...any) (pgconn.CommandTag, error) {
				return pgconn.NewCommandTag("UPDATE 0"), nil
			},
		}

		err := New(db).RevokeAPIKey(context.Background(), "key-1")
		require.ErrorContains(t, err, "api key not found or already revoked")
		_, ok := apiKeyTouchCache.Load("key-1")
		require.True(t, ok)
		require.EqualValues(t, 1, apiKeyTouchSize.Load())
	})

	t.Run("disable auto rotation reports missing rows", func(t *testing.T) {
		db := &mockDBTX{
			execFn: func(context.Context, string, ...any) (pgconn.CommandTag, error) {
				return pgconn.NewCommandTag("UPDATE 0"), nil
			},
		}

		require.ErrorIs(t, New(db).DisableAPIKeyAutoRotation(context.Background(), "key-1"), ErrAPIKeyNotFound)
	})

	t.Run("disable auto rotation wraps exec error", func(t *testing.T) {
		db := &mockDBTX{
			execFn: func(context.Context, string, ...any) (pgconn.CommandTag, error) {
				return pgconn.CommandTag{}, errors.New("db failed")
			},
		}

		err := New(db).DisableAPIKeyAutoRotation(context.Background(), "key-1")
		require.ErrorContains(t, err, "disable api key auto-rotation")
		require.ErrorContains(t, err, "db failed")
	})

	t.Run("mark rotated rejects unchanged rows", func(t *testing.T) {
		db := &mockDBTX{
			execFn: func(context.Context, string, ...any) (pgconn.CommandTag, error) {
				return pgconn.NewCommandTag("UPDATE 0"), nil
			},
		}

		err := New(db).MarkAPIKeyRotated(context.Background(), "old", "new", time.Now())
		require.ErrorContains(t, err, "api key not found, already revoked, or already rotated")
	})
}

func TestGetAPIKeyNotFoundMapping(t *testing.T) {
	t.Parallel()

	db := &mockDBTX{
		queryRowFn: func(context.Context, string, ...any) pgx.Row {
			return &mockRow{scanFn: func(...any) error {
				return pgx.ErrNoRows
			}}
		},
	}
	q := New(db)

	_, err := q.GetAPIKeyByHash(context.Background(), "missing-hash")
	require.ErrorIs(t, err, ErrAPIKeyNotFound)

	_, err = q.GetAPIKeyByID(context.Background(), "missing-id")
	require.ErrorIs(t, err, ErrAPIKeyNotFound)
}

func TestCreateRotatedAPIKeyValidation(t *testing.T) {
	t.Parallel()

	q := New(&mockDBTX{})
	now := time.Now()

	err := q.CreateRotatedAPIKey(context.Background(), "", &domain.APIKey{}, now)
	require.ErrorContains(t, err, "old key id is required")

	err = q.CreateRotatedAPIKey(context.Background(), "old-key", nil, now)
	require.ErrorContains(t, err, "new key is required")

	newKey := &domain.APIKey{}
	err = q.CreateRotatedAPIKey(context.Background(), "old-key", newKey, now)
	require.ErrorContains(t, err, "create rotated api key")
	require.NotEmpty(t, newKey.ID)
}

func TestScanAPIKey(t *testing.T) {
	t.Parallel()

	t.Run("decodes optional fields", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		orgID := "org-1"
		replacedBy := "key-new"
		rateLimitRequests := 100
		rateLimitWindowSecs := 60
		environmentID := "env-prod"
		rotationWebhookURL := "https://example.com/rotate"
		rotationIntervalDays := 30
		key, err := scanAPIKey(apiKeyScanFunc(func(dest ...any) error {
			*(dest[0].(*string)) = "key-1"
			*(dest[1].(*string)) = "project-1"
			*(dest[2].(**string)) = &orgID
			*(dest[3].(*string)) = "Deploy"
			*(dest[4].(*string)) = "hash"
			*(dest[5].(*string)) = "strait_abcde"
			*(dest[6].(*[]string)) = []string{"runs:read"}
			*(dest[7].(**time.Time)) = &now
			*(dest[8].(**time.Time)) = &now
			*(dest[9].(*time.Time)) = now
			*(dest[10].(**time.Time)) = &now
			*(dest[11].(**string)) = &replacedBy
			*(dest[12].(**time.Time)) = &now
			*(dest[13].(**int)) = &rateLimitRequests
			*(dest[14].(**int)) = &rateLimitWindowSecs
			*(dest[15].(**string)) = &environmentID
			*(dest[16].(**int)) = &rotationIntervalDays
			*(dest[17].(**time.Time)) = &now
			*(dest[18].(**string)) = &rotationWebhookURL
			*(dest[19].(*[]byte)) = []byte("ciphertext")
			*(dest[20].(*int64)) = 42
			return nil
		}))

		require.NoError(t, err)
		require.Equal(t, "key-1", key.ID)
		require.Equal(t, "org-1", key.OrgID)
		require.Equal(t, "key-new", key.ReplacedByKeyID)
		require.Equal(t, 100, key.RateLimitRequests)
		require.Equal(t, 60, key.RateLimitWindowSecs)
		require.Equal(t, "env-prod", key.EnvironmentID)
		require.Equal(t, "https://example.com/rotate", key.RotationWebhookURL)
		require.Equal(t, []byte("ciphertext"), key.RotationWebhookSecret)
		require.EqualValues(t, 42, key.CacheVersion)
	})

	t.Run("leaves nil optional fields at zero values", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		key, err := scanAPIKey(apiKeyScanFunc(func(dest ...any) error {
			*(dest[0].(*string)) = "key-1"
			*(dest[1].(*string)) = "project-1"
			*(dest[3].(*string)) = "Deploy"
			*(dest[4].(*string)) = "hash"
			*(dest[5].(*string)) = "strait_abcde"
			*(dest[6].(*[]string)) = []string{"runs:read"}
			*(dest[9].(*time.Time)) = now
			*(dest[20].(*int64)) = 42
			return nil
		}))

		require.NoError(t, err)
		require.Empty(t, key.OrgID)
		require.Empty(t, key.ReplacedByKeyID)
		require.Zero(t, key.RateLimitRequests)
		require.Zero(t, key.RateLimitWindowSecs)
		require.Empty(t, key.EnvironmentID)
		require.Empty(t, key.RotationWebhookURL)
	})

	t.Run("returns scanner errors", func(t *testing.T) {
		t.Parallel()

		_, err := scanAPIKey(apiKeyScanFunc(func(...any) error {
			return errors.New("scan failed")
		}))
		require.ErrorContains(t, err, "scan failed")
	})
}
