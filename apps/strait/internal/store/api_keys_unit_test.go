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

func fillAPIKeyScanDest(dest []any, key domain.APIKey) {
	*(dest[0].(*string)) = key.ID
	*(dest[1].(*string)) = key.ProjectID
	if key.OrgID != "" {
		*(dest[2].(**string)) = &key.OrgID
	}
	*(dest[3].(*string)) = key.Name
	*(dest[4].(*string)) = key.KeyHash
	*(dest[5].(*string)) = key.KeyPrefix
	*(dest[6].(*[]string)) = key.Scopes
	*(dest[7].(**time.Time)) = key.ExpiresAt
	*(dest[8].(**time.Time)) = key.LastUsedAt
	*(dest[9].(*time.Time)) = key.CreatedAt
	*(dest[10].(**time.Time)) = key.RevokedAt
	if key.ReplacedByKeyID != "" {
		*(dest[11].(**string)) = &key.ReplacedByKeyID
	}
	*(dest[12].(**time.Time)) = key.GraceExpiresAt
	if key.RateLimitRequests > 0 {
		*(dest[13].(**int)) = &key.RateLimitRequests
	}
	if key.RateLimitWindowSecs > 0 {
		*(dest[14].(**int)) = &key.RateLimitWindowSecs
	}
	if key.EnvironmentID != "" {
		*(dest[15].(**string)) = &key.EnvironmentID
	}
	*(dest[16].(**int)) = key.RotationIntervalDays
	*(dest[17].(**time.Time)) = key.NextRotationAt
	if key.RotationWebhookURL != "" {
		*(dest[18].(**string)) = &key.RotationWebhookURL
	}
	*(dest[19].(*[]byte)) = key.RotationWebhookSecret
	*(dest[20].(*int64)) = key.CacheVersion
}

func apiKeyRow(key domain.APIKey) *mockRow {
	return &mockRow{scanFn: func(dest ...any) error {
		fillAPIKeyScanDest(dest, key)
		return nil
	}}
}

func apiKeyRows(keys ...domain.APIKey) *mockRows {
	scanFns := make([]func(dest ...any) error, 0, len(keys))
	for _, key := range keys {
		scanFns = append(scanFns, func(dest ...any) error {
			fillAPIKeyScanDest(dest, key)
			return nil
		})
	}
	return &mockRows{scanFns: scanFns}
}

type apiKeyTx struct {
	pgx.Tx
	execFn     func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	queryRowFn func(ctx context.Context, sql string, args ...any) pgx.Row
	committed  bool
	rolledBack bool
}

func (tx *apiKeyTx) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if tx.execFn != nil {
		return tx.execFn(ctx, sql, args...)
	}
	return pgconn.NewCommandTag("UPDATE 1"), nil
}

func (tx *apiKeyTx) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return nil, nil
}

func (tx *apiKeyTx) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if tx.queryRowFn != nil {
		return tx.queryRowFn(ctx, sql, args...)
	}
	return &mockRow{}
}

func (tx *apiKeyTx) Commit(context.Context) error {
	tx.committed = true
	return nil
}

func (tx *apiKeyTx) Rollback(context.Context) error {
	tx.rolledBack = true
	return nil
}

type apiKeyBeginner struct {
	mockDBTX
	tx *apiKeyTx
}

func (b *apiKeyBeginner) Begin(context.Context) (pgx.Tx, error) {
	return b.tx, nil
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

func TestAPIKeyLookupAndListQueries(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	key := domain.APIKey{
		ID:           "key-1",
		ProjectID:    "project-1",
		OrgID:        "org-1",
		Name:         "Deploy",
		KeyHash:      "hash",
		KeyPrefix:    "strait_abcde",
		Scopes:       []string{"runs:read"},
		CreatedAt:    now,
		CacheVersion: 12,
	}

	t.Run("gets by hash and id", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name string
			call func(*Queries) (*domain.APIKey, error)
			want []any
		}{
			{
				name: "hash",
				call: func(q *Queries) (*domain.APIKey, error) {
					return q.GetAPIKeyByHash(context.Background(), "hash")
				},
				want: []any{"hash"},
			},
			{
				name: "id",
				call: func(q *Queries) (*domain.APIKey, error) {
					return q.GetAPIKeyByID(context.Background(), "key-1")
				},
				want: []any{"key-1"},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				db := &mockDBTX{
					queryRowFn: func(_ context.Context, _ string, args ...any) pgx.Row {
						require.Equal(t, tt.want, args)
						return apiKeyRow(key)
					},
				}

				got, err := tt.call(New(db))

				require.NoError(t, err)
				require.Equal(t, &key, got)
			})
		}
	})

	t.Run("wraps non-not-found lookup errors", func(t *testing.T) {
		t.Parallel()

		scanErr := errors.New("scan failed")
		db := &mockDBTX{
			queryRowFn: func(context.Context, string, ...any) pgx.Row {
				return &mockRow{scanFn: func(...any) error { return scanErr }}
			},
		}

		_, err := New(db).GetAPIKeyByHash(context.Background(), "hash")
		require.ErrorIs(t, err, scanErr)
		require.ErrorContains(t, err, "get api key by hash")

		_, err = New(db).GetAPIKeyByID(context.Background(), "key-1")
		require.ErrorIs(t, err, scanErr)
		require.ErrorContains(t, err, "get api key by id")
	})

	t.Run("lists project and org keys with cursors", func(t *testing.T) {
		t.Parallel()

		cursor := now.Add(time.Minute)
		tests := []struct {
			name      string
			call      func(*Queries) ([]domain.APIKey, error)
			wantSQL   string
			wantArgs  []any
			wantLabel string
		}{
			{
				name: "project",
				call: func(q *Queries) ([]domain.APIKey, error) {
					return q.ListAPIKeysByProject(context.Background(), "project-1", 25, &cursor)
				},
				wantSQL:  "WHERE project_id = $1",
				wantArgs: []any{"project-1", cursor, 25},
			},
			{
				name: "org",
				call: func(q *Queries) ([]domain.APIKey, error) {
					return q.ListAPIKeysByOrg(context.Background(), "org-1", 25, &cursor)
				},
				wantSQL:  "WHERE org_id = $1",
				wantArgs: []any{"org-1", cursor, 25},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				db := &mockDBTX{
					queryFn: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
						require.Contains(t, sql, tt.wantSQL)
						require.Contains(t, sql, "created_at < $2")
						require.Contains(t, sql, "LIMIT $3")
						require.Equal(t, tt.wantArgs, args)
						return apiKeyRows(key), nil
					},
				}

				keys, err := tt.call(New(db))

				require.NoError(t, err)
				require.Equal(t, []domain.APIKey{key}, keys)
			})
		}
	})

	t.Run("list functions wrap query scan and row errors", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name       string
			call       func(*Queries) ([]domain.APIKey, error)
			rows       pgx.Rows
			queryErr   error
			wantSubstr string
		}{
			{
				name: "project query",
				call: func(q *Queries) ([]domain.APIKey, error) {
					return q.ListAPIKeysByProject(context.Background(), "project-1", 10, nil)
				},
				queryErr:   errors.New("query failed"),
				wantSubstr: "list api keys",
			},
			{
				name: "project scan",
				call: func(q *Queries) ([]domain.APIKey, error) {
					return q.ListAPIKeysByProject(context.Background(), "project-1", 10, nil)
				},
				rows:       &mockRows{scanFns: []func(dest ...any) error{func(...any) error { return errors.New("scan failed") }}},
				wantSubstr: "list api keys scan",
			},
			{
				name: "project rows",
				call: func(q *Queries) ([]domain.APIKey, error) {
					return q.ListAPIKeysByProject(context.Background(), "project-1", 10, nil)
				},
				rows:       &mockRows{err: errors.New("rows failed")},
				wantSubstr: "rows failed",
			},
			{
				name: "org query",
				call: func(q *Queries) ([]domain.APIKey, error) {
					return q.ListAPIKeysByOrg(context.Background(), "org-1", 10, nil)
				},
				queryErr:   errors.New("query failed"),
				wantSubstr: "list api keys by org",
			},
			{
				name: "org scan",
				call: func(q *Queries) ([]domain.APIKey, error) {
					return q.ListAPIKeysByOrg(context.Background(), "org-1", 10, nil)
				},
				rows:       &mockRows{scanFns: []func(dest ...any) error{func(...any) error { return errors.New("scan failed") }}},
				wantSubstr: "list api keys by org scan",
			},
			{
				name: "org rows",
				call: func(q *Queries) ([]domain.APIKey, error) {
					return q.ListAPIKeysByOrg(context.Background(), "org-1", 10, nil)
				},
				rows:       &mockRows{err: errors.New("rows failed")},
				wantSubstr: "rows failed",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				db := &mockDBTX{
					queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
						if tt.queryErr != nil {
							return nil, tt.queryErr
						}
						return tt.rows, nil
					},
				}

				keys, err := tt.call(New(db))

				if tt.queryErr != nil {
					require.Nil(t, keys)
				} else {
					require.Empty(t, keys)
				}
				require.ErrorContains(t, err, tt.wantSubstr)
			})
		}
	})
}

func TestAPIKeyTouchAndRotationUnit(t *testing.T) {
	t.Parallel()

	t.Run("touch treats malformed cache entry as miss and wraps exec errors", func(t *testing.T) {
		ClearAPIKeyTouchCacheForTest(t)
		SetAPIKeyTouchCooldownForTest(t, time.Hour)
		apiKeyTouchCache.Store("key-1", "not-an-int64")
		apiKeyTouchSize.Store(1)

		execErr := errors.New("update failed")
		db := &mockDBTX{
			execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
				require.Contains(t, sql, "UPDATE api_keys SET last_used_at")
				require.Equal(t, []any{"key-1"}, args)
				return pgconn.CommandTag{}, execErr
			},
		}

		err := New(db).TouchAPIKeyLastUsed(context.Background(), "key-1")

		require.ErrorIs(t, err, execErr)
		require.ErrorContains(t, err, "touch api key last used")
	})

	t.Run("mark rotated succeeds with expected arguments", func(t *testing.T) {
		t.Parallel()

		grace := time.Now().UTC()
		db := &mockDBTX{
			execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
				require.Contains(t, sql, "replaced_by_key_id = $2")
				require.Equal(t, []any{"old-key", "new-key", grace}, args)
				return pgconn.NewCommandTag("UPDATE 1"), nil
			},
		}

		require.NoError(t, New(db).MarkAPIKeyRotated(context.Background(), "old-key", "new-key", grace))
	})

	t.Run("mark rotated wraps exec errors", func(t *testing.T) {
		t.Parallel()

		execErr := errors.New("update failed")
		db := &mockDBTX{
			execFn: func(context.Context, string, ...any) (pgconn.CommandTag, error) {
				return pgconn.CommandTag{}, execErr
			},
		}

		err := New(db).MarkAPIKeyRotated(context.Background(), "old-key", "new-key", time.Now())

		require.ErrorIs(t, err, execErr)
		require.ErrorContains(t, err, "mark api key rotated")
	})

	t.Run("create rotated api key commits create and mark in one transaction", func(t *testing.T) {
		t.Parallel()

		grace := time.Now().UTC()
		var insertArgs []any
		var updateArgs []any
		tx := &apiKeyTx{}
		tx.queryRowFn = func(_ context.Context, sql string, args ...any) pgx.Row {
			require.Contains(t, sql, "INSERT INTO api_keys")
			insertArgs = append([]any(nil), args...)
			return &mockRow{scanFn: func(dest ...any) error {
				*(dest[0].(*time.Time)) = grace
				*(dest[1].(*int64)) = 7
				return nil
			}}
		}
		tx.execFn = func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			require.Contains(t, sql, "replaced_by_key_id = $2")
			updateArgs = append([]any(nil), args...)
			return pgconn.NewCommandTag("UPDATE 1"), nil
		}
		newKey := &domain.APIKey{
			ProjectID: "project-1",
			Name:      "New",
			KeyHash:   "hash-new",
			KeyPrefix: "strait_new",
			Scopes:    []string{"jobs:read"},
		}

		err := New(&apiKeyBeginner{tx: tx}).CreateRotatedAPIKey(context.Background(), "old-key", newKey, grace)

		require.NoError(t, err)
		require.NotEmpty(t, newKey.ID)
		require.Equal(t, newKey.ID, insertArgs[0])
		require.Equal(t, []any{"old-key", newKey.ID, grace}, updateArgs)
		require.True(t, tx.committed)
		require.False(t, tx.rolledBack)
	})

	t.Run("create rotated api key rolls back when mark fails", func(t *testing.T) {
		t.Parallel()

		grace := time.Now().UTC()
		markErr := errors.New("mark failed")
		tx := &apiKeyTx{}
		tx.queryRowFn = func(context.Context, string, ...any) pgx.Row {
			return &mockRow{scanFn: func(dest ...any) error {
				*(dest[0].(*time.Time)) = grace
				*(dest[1].(*int64)) = 7
				return nil
			}}
		}
		tx.execFn = func(context.Context, string, ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, markErr
		}
		newKey := &domain.APIKey{
			ID:        "new-key",
			ProjectID: "project-1",
			Name:      "New",
			KeyHash:   "hash-new",
			KeyPrefix: "strait_new",
			Scopes:    []string{"jobs:read"},
		}

		err := New(&apiKeyBeginner{tx: tx}).CreateRotatedAPIKey(context.Background(), "old-key", newKey, grace)

		require.ErrorIs(t, err, markErr)
		require.ErrorContains(t, err, "create rotated api key")
		require.False(t, tx.committed)
		require.True(t, tx.rolledBack)
	})
}

func TestAPIKeyRotationAndExpiryListsUnit(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	key := domain.APIKey{
		ID:           "key-1",
		ProjectID:    "project-1",
		Name:         "Deploy",
		KeyHash:      "hash",
		KeyPrefix:    "strait_abcde",
		Scopes:       []string{"runs:read"},
		CreatedAt:    now,
		CacheVersion: 12,
	}

	t.Run("lists due rotation and expiring soon keys", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name    string
			call    func(*Queries) ([]domain.APIKey, error)
			wantSQL string
			wantArg []any
		}{
			{
				name: "due rotation",
				call: func(q *Queries) ([]domain.APIKey, error) {
					return q.ListAPIKeysDueRotation(context.Background())
				},
				wantSQL: "next_rotation_at <= NOW()",
			},
			{
				name: "expiring soon",
				call: func(q *Queries) ([]domain.APIKey, error) {
					return q.ListAPIKeysExpiringSoon(context.Background(), "project-1", 7)
				},
				wantSQL: "expires_at <= NOW()",
				wantArg: []any{"project-1", 7},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				db := &mockDBTX{
					queryFn: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
						require.Contains(t, sql, tt.wantSQL)
						require.Equal(t, tt.wantArg, args)
						return apiKeyRows(key), nil
					},
				}

				keys, err := tt.call(New(db))

				require.NoError(t, err)
				require.Equal(t, []domain.APIKey{key}, keys)
			})
		}
	})

	t.Run("list rotation and expiry functions wrap query scan and row errors", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name       string
			call       func(*Queries) ([]domain.APIKey, error)
			rows       pgx.Rows
			queryErr   error
			wantSubstr string
		}{
			{
				name: "rotation query",
				call: func(q *Queries) ([]domain.APIKey, error) {
					return q.ListAPIKeysDueRotation(context.Background())
				},
				queryErr:   errors.New("query failed"),
				wantSubstr: "list api keys due rotation",
			},
			{
				name: "rotation scan",
				call: func(q *Queries) ([]domain.APIKey, error) {
					return q.ListAPIKeysDueRotation(context.Background())
				},
				rows:       &mockRows{scanFns: []func(dest ...any) error{func(...any) error { return errors.New("scan failed") }}},
				wantSubstr: "list api keys due rotation scan",
			},
			{
				name: "rotation rows",
				call: func(q *Queries) ([]domain.APIKey, error) {
					return q.ListAPIKeysDueRotation(context.Background())
				},
				rows:       &mockRows{err: errors.New("rows failed")},
				wantSubstr: "rows failed",
			},
			{
				name: "expiry query",
				call: func(q *Queries) ([]domain.APIKey, error) {
					return q.ListAPIKeysExpiringSoon(context.Background(), "project-1", 7)
				},
				queryErr:   errors.New("query failed"),
				wantSubstr: "list api keys expiring soon",
			},
			{
				name: "expiry scan",
				call: func(q *Queries) ([]domain.APIKey, error) {
					return q.ListAPIKeysExpiringSoon(context.Background(), "project-1", 7)
				},
				rows:       &mockRows{scanFns: []func(dest ...any) error{func(...any) error { return errors.New("scan failed") }}},
				wantSubstr: "list api keys expiring soon scan",
			},
			{
				name: "expiry rows",
				call: func(q *Queries) ([]domain.APIKey, error) {
					return q.ListAPIKeysExpiringSoon(context.Background(), "project-1", 7)
				},
				rows:       &mockRows{err: errors.New("rows failed")},
				wantSubstr: "rows failed",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				db := &mockDBTX{
					queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
						if tt.queryErr != nil {
							return nil, tt.queryErr
						}
						return tt.rows, nil
					},
				}

				keys, err := tt.call(New(db))

				if tt.queryErr != nil {
					require.Nil(t, keys)
				} else {
					require.Empty(t, keys)
				}
				require.ErrorContains(t, err, tt.wantSubstr)
			})
		}
	})
}

func TestOrgScopedRunAndJobListsUnit(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	cursor := now.Add(time.Minute)

	t.Run("lists runs by org with cursor", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			queryFn: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
				require.Contains(t, sql, "FROM job_runs jr")
				require.Contains(t, sql, "projects WHERE org_id = $1")
				require.Contains(t, sql, "created_at < $2")
				require.Contains(t, sql, "LIMIT $3")
				require.Equal(t, []any{"org-1", cursor, 25}, args)
				return &mockRows{scanFns: []func(dest ...any) error{runScanFn(now, true)}}, nil
			},
		}

		runs, err := New(db).ListRunsByOrg(context.Background(), "org-1", 25, &cursor)

		require.NoError(t, err)
		require.Len(t, runs, 1)
		require.Equal(t, "run-1", runs[0].ID)
	})

	t.Run("lists jobs by org with cursor", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			queryFn: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
				require.Contains(t, sql, "FROM jobs")
				require.Contains(t, sql, "projects WHERE org_id = $1")
				require.Contains(t, sql, "created_at < $2")
				require.Contains(t, sql, "LIMIT $3")
				require.Equal(t, []any{"org-1", cursor, 25}, args)
				return &mockRows{scanFns: []func(dest ...any) error{jobScanFn(now, true)}}, nil
			},
		}

		jobs, err := New(db).ListJobsByOrg(context.Background(), "org-1", 25, &cursor)

		require.NoError(t, err)
		require.Len(t, jobs, 1)
		require.Equal(t, "job-1", jobs[0].ID)
	})

	t.Run("org list functions wrap query scan and row errors", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name       string
			runList    bool
			rows       pgx.Rows
			queryErr   error
			wantSubstr string
		}{
			{name: "runs query", runList: true, queryErr: errors.New("query failed"), wantSubstr: "list runs by org"},
			{
				name:       "runs scan",
				runList:    true,
				rows:       &mockRows{scanFns: []func(dest ...any) error{func(...any) error { return errors.New("scan failed") }}},
				wantSubstr: "list runs by org scan",
			},
			{name: "runs rows", runList: true, rows: &mockRows{err: errors.New("rows failed")}, wantSubstr: "rows failed"},
			{name: "jobs query", queryErr: errors.New("query failed"), wantSubstr: "list jobs by org"},
			{
				name:       "jobs scan",
				rows:       &mockRows{scanFns: []func(dest ...any) error{func(...any) error { return errors.New("scan failed") }}},
				wantSubstr: "list jobs by org scan",
			},
			{name: "jobs rows", rows: &mockRows{err: errors.New("rows failed")}, wantSubstr: "rows failed"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				db := &mockDBTX{
					queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
						if tt.queryErr != nil {
							return nil, tt.queryErr
						}
						return tt.rows, nil
					},
				}

				var err error
				if tt.runList {
					var runs []domain.JobRun
					runs, err = New(db).ListRunsByOrg(context.Background(), "org-1", 10, nil)
					if tt.queryErr != nil {
						require.Nil(t, runs)
					} else {
						require.Empty(t, runs)
					}
				} else {
					var jobs []domain.Job
					jobs, err = New(db).ListJobsByOrg(context.Background(), "org-1", 10, nil)
					if tt.queryErr != nil {
						require.Nil(t, jobs)
					} else {
						require.Empty(t, jobs)
					}
				}
				require.ErrorContains(t, err, tt.wantSubstr)
			})
		}
	})
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
