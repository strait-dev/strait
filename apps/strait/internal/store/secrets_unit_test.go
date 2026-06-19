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

const secretsUnitEncryptionKey = "secrets-unit-encryption-key"

func encryptedSecretValue(t *testing.T, plaintext string) string {
	t.Helper()

	q := New(&mockDBTX{})
	q.SetSecretEncryptionKey(secretsUnitEncryptionKey)
	key, err := q.secretKey()
	require.NoError(t, err)
	encrypted, err := encryptSecret(plaintext, key)
	require.NoError(t, err)
	return encrypted
}

func fillJobSecretDest(dest []any, encrypted string, withJob bool, now time.Time) {
	*(dest[0].(*string)) = "secret-1"
	*(dest[1].(*string)) = "project-1"
	if withJob {
		jobID := "job-1"
		*(dest[2].(**string)) = &jobID
	}
	*(dest[3].(*string)) = "production"
	*(dest[4].(*string)) = "API_KEY"
	*(dest[5].(*string)) = encrypted
	*(dest[6].(*int)) = 1
	*(dest[7].(*time.Time)) = now
	*(dest[8].(*time.Time)) = now.Add(time.Second)
}

func TestCreateJobSecretUnit(t *testing.T) {
	t.Parallel()

	t.Run("applies defaults verifies job scope and stores encrypted value", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		var insertArgs []any
		call := 0
		db := &mockDBTX{queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
			call++
			switch call {
			case 1:
				require.Contains(t, sql, "FROM jobs WHERE id = $1")
				require.Equal(t, []any{"job-1"}, args)
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*string)) = "project-1"
					*(dest[1].(*string)) = "production"
					return nil
				}}
			case 2:
				require.Contains(t, sql, "INSERT INTO job_secrets")
				insertArgs = append([]any(nil), args...)
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*time.Time)) = now
					*(dest[1].(*time.Time)) = now.Add(time.Second)
					return nil
				}}
			default:
				t.Fatalf("unexpected query call %d: %s", call, sql)
				return &mockRow{}
			}
		}}
		q := New(db)
		q.SetSecretEncryptionKey(secretsUnitEncryptionKey)
		secret := &domain.JobSecret{
			ProjectID:   "project-1",
			JobID:       "job-1",
			Environment: "production",
			SecretKey:   "API_KEY",
			Value:       "plaintext",
		}

		require.NoError(t, q.CreateJobSecret(context.Background(), secret))
		require.NotEmpty(t, secret.ID)
		require.Equal(t, 1, secret.KeyVersion)
		require.Equal(t, now, secret.CreatedAt)
		require.Equal(t, now.Add(time.Second), secret.UpdatedAt)
		require.Len(t, insertArgs, 7)
		require.Equal(t, secret.ID, insertArgs[0])
		require.Equal(t, "project-1", insertArgs[1])
		require.Equal(t, "job-1", insertArgs[2])
		require.Equal(t, "production", insertArgs[3])
		require.Equal(t, "API_KEY", insertArgs[4])
		require.NotEqual(t, "plaintext", insertArgs[5])
		require.Equal(t, 1, insertArgs[6])
		decrypted, err := q.decryptSecretWithFallback(insertArgs[5].(string))
		require.NoError(t, err)
		require.Equal(t, "plaintext", decrypted)
	})

	t.Run("uses encrypted value as plaintext fallback when value is empty", func(t *testing.T) {
		t.Parallel()

		var encrypted string
		db := &mockDBTX{queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
			require.Contains(t, sql, "INSERT INTO job_secrets")
			encrypted = args[5].(string)
			return &mockRow{scanFn: func(dest ...any) error {
				now := time.Now().UTC()
				*(dest[0].(*time.Time)) = now
				*(dest[1].(*time.Time)) = now
				return nil
			}}
		}}
		q := New(db)
		q.SetSecretEncryptionKey(secretsUnitEncryptionKey)

		err := q.CreateJobSecret(context.Background(), &domain.JobSecret{
			ID:             "secret-1",
			ProjectID:      "project-1",
			Environment:    "production",
			SecretKey:      "TOKEN",
			EncryptedValue: "fallback-plaintext",
			KeyVersion:     3,
		})
		require.NoError(t, err)
		got, err := q.decryptSecretWithFallback(encrypted)
		require.NoError(t, err)
		require.Equal(t, "fallback-plaintext", got)
	})

	t.Run("rejects invalid job scope", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name       string
			scan       func(dest ...any) error
			wantErr    error
			wantString string
		}{
			{name: "missing job", scan: func(...any) error { return pgx.ErrNoRows }, wantErr: ErrJobNotFound},
			{name: "job query error", scan: func(...any) error { return errors.New("job lookup failed") }, wantString: "verify job"},
			{name: "wrong project", scan: func(dest ...any) error {
				*(dest[0].(*string)) = "other-project"
				*(dest[1].(*string)) = "production"
				return nil
			}, wantString: "job does not belong to project"},
			{name: "wrong environment", scan: func(dest ...any) error {
				*(dest[0].(*string)) = "project-1"
				*(dest[1].(*string)) = "staging"
				return nil
			}, wantString: "secret environment does not match job environment"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				db := &mockDBTX{queryRowFn: func(context.Context, string, ...any) pgx.Row {
					return &mockRow{scanFn: tt.scan}
				}}
				q := New(db)
				q.SetSecretEncryptionKey(secretsUnitEncryptionKey)
				err := q.CreateJobSecret(context.Background(), &domain.JobSecret{
					ProjectID:   "project-1",
					JobID:       "job-1",
					Environment: "production",
					SecretKey:   "TOKEN",
					Value:       "plaintext",
				})
				if tt.wantErr != nil {
					require.ErrorIs(t, err, tt.wantErr)
					return
				}
				require.ErrorContains(t, err, tt.wantString)
			})
		}
	})

	t.Run("wraps encryption and insert errors", func(t *testing.T) {
		t.Parallel()

		err := New(&mockDBTX{}).CreateJobSecret(context.Background(), &domain.JobSecret{ProjectID: "project-1"})
		require.ErrorContains(t, err, "secret encryption key is not configured")

		insertErr := errors.New("insert failed")
		q := New(&mockDBTX{queryRowFn: func(context.Context, string, ...any) pgx.Row {
			return &mockRow{scanFn: func(...any) error { return insertErr }}
		}})
		q.SetSecretEncryptionKey(secretsUnitEncryptionKey)
		err = q.CreateJobSecret(context.Background(), &domain.JobSecret{
			ProjectID:   "project-1",
			Environment: "production",
			SecretKey:   "TOKEN",
			Value:       "plaintext",
		})
		require.ErrorIs(t, err, insertErr)
		require.ErrorContains(t, err, "create job secret")
	})
}

func TestGetJobSecretUnit(t *testing.T) {
	t.Parallel()

	t.Run("scans and decrypts secret", func(t *testing.T) {
		t.Parallel()

		encrypted := encryptedSecretValue(t, "plaintext")
		now := time.Now().UTC()
		db := &mockDBTX{queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
			require.Contains(t, sql, "FROM job_secrets")
			require.Equal(t, []any{"secret-1", "project-1"}, args)
			return &mockRow{scanFn: func(dest ...any) error {
				fillJobSecretDest(dest, encrypted, true, now)
				return nil
			}}
		}}
		q := New(db)
		q.SetSecretEncryptionKey(secretsUnitEncryptionKey)

		got, err := q.GetJobSecret(context.Background(), "secret-1", "project-1")
		require.NoError(t, err)
		require.Equal(t, "job-1", got.JobID)
		require.Equal(t, "plaintext", got.Value)
		require.Equal(t, encrypted, got.EncryptedValue)
	})

	t.Run("maps missing rows and wraps failures", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{queryRowFn: func(context.Context, string, ...any) pgx.Row {
			return &mockRow{scanFn: func(...any) error { return pgx.ErrNoRows }}
		}}
		got, err := New(db).GetJobSecret(context.Background(), "missing", "project-1")
		require.ErrorIs(t, err, ErrJobSecretNotFound)
		require.Nil(t, got)

		readErr := errors.New("read failed")
		db = &mockDBTX{queryRowFn: func(context.Context, string, ...any) pgx.Row {
			return &mockRow{scanFn: func(...any) error { return readErr }}
		}}
		got, err = New(db).GetJobSecret(context.Background(), "secret-1", "project-1")
		require.ErrorIs(t, err, readErr)
		require.Nil(t, got)

		db = &mockDBTX{queryRowFn: func(context.Context, string, ...any) pgx.Row {
			return &mockRow{scanFn: func(dest ...any) error {
				fillJobSecretDest(dest, "not-hex", false, time.Now().UTC())
				return nil
			}}
		}}
		q := New(db)
		q.SetSecretEncryptionKey(secretsUnitEncryptionKey)
		got, err = q.GetJobSecret(context.Background(), "secret-1", "project-1")
		require.ErrorContains(t, err, "get job secret")
		require.Nil(t, got)
	})
}

func TestListJobSecretsUnit(t *testing.T) {
	t.Parallel()

	t.Run("applies filters cursor limit and decrypts rows", func(t *testing.T) {
		t.Parallel()

		encrypted := encryptedSecretValue(t, "plaintext")
		now := time.Now().UTC()
		cursor := now.Add(time.Minute)
		db := &mockDBTX{queryFn: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
			require.Contains(t, sql, "job_id = $2")
			require.Contains(t, sql, "environment = $3")
			require.Contains(t, sql, "created_at < $4")
			require.Contains(t, sql, "LIMIT $5")
			require.Equal(t, []any{"project-1", "job-1", "production", cursor, 25}, args)
			return &mockRows{scanFns: []func(dest ...any) error{
				func(dest ...any) error {
					fillJobSecretDest(dest, encrypted, true, now)
					return nil
				},
			}}, nil
		}}
		q := New(db)
		q.SetSecretEncryptionKey(secretsUnitEncryptionKey)

		got, err := q.ListJobSecrets(context.Background(), "project-1", "job-1", "production", 25, &cursor)
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.Equal(t, "plaintext", got[0].Value)
	})

	t.Run("wraps query scan decrypt and rows errors", func(t *testing.T) {
		t.Parallel()

		queryErr := errors.New("query failed")
		q := New(&mockDBTX{queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
			return nil, queryErr
		}})
		got, err := q.ListJobSecrets(context.Background(), "project-1", "", "", 10, nil)
		require.ErrorIs(t, err, queryErr)
		require.Nil(t, got)

		scanErr := errors.New("scan failed")
		q = New(&mockDBTX{queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
			return &mockRows{scanFns: []func(dest ...any) error{
				func(...any) error { return scanErr },
			}}, nil
		}})
		got, err = q.ListJobSecrets(context.Background(), "project-1", "", "", 10, nil)
		require.ErrorIs(t, err, scanErr)
		require.Nil(t, got)

		q = New(&mockDBTX{queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
			return &mockRows{scanFns: []func(dest ...any) error{
				func(dest ...any) error {
					fillJobSecretDest(dest, "bad-ciphertext", false, time.Now().UTC())
					return nil
				},
			}}, nil
		}})
		q.SetSecretEncryptionKey(secretsUnitEncryptionKey)
		got, err = q.ListJobSecrets(context.Background(), "project-1", "", "", 10, nil)
		require.ErrorContains(t, err, "list job secrets decrypt")
		require.Nil(t, got)

		rowsErr := errors.New("rows failed")
		q = New(&mockDBTX{queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
			return &mockRows{err: rowsErr}, nil
		}})
		got, err = q.ListJobSecrets(context.Background(), "project-1", "", "", 10, nil)
		require.ErrorIs(t, err, rowsErr)
		require.Nil(t, got)
	})
}

func TestDeleteJobSecretUnit(t *testing.T) {
	t.Parallel()

	t.Run("maps row count and exec errors", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name    string
			tag     pgconn.CommandTag
			execErr error
			wantErr error
		}{
			{name: "deleted", tag: pgconn.NewCommandTag("DELETE 1")},
			{name: "missing", tag: pgconn.NewCommandTag("DELETE 0"), wantErr: ErrJobSecretNotFound},
			{name: "exec error", execErr: errors.New("delete failed")},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				db := &mockDBTX{execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
					require.Equal(t, `DELETE FROM job_secrets WHERE id = $1 AND project_id = $2`, sql)
					require.Equal(t, []any{"secret-1", "project-1"}, args)
					return tt.tag, tt.execErr
				}}
				err := New(db).DeleteJobSecret(context.Background(), "secret-1", "project-1")
				if tt.execErr != nil {
					require.ErrorIs(t, err, tt.execErr)
					require.ErrorContains(t, err, "delete job secret")
					return
				}
				if tt.wantErr != nil {
					require.ErrorIs(t, err, tt.wantErr)
					return
				}
				require.NoError(t, err)
			})
		}
	})
}

func TestListJobSecretsByJobUnit(t *testing.T) {
	t.Parallel()

	t.Run("uses job environment fallback query and decrypts rows", func(t *testing.T) {
		t.Parallel()

		encrypted := encryptedSecretValue(t, "plaintext")
		db := &mockDBTX{queryFn: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
			require.Contains(t, sql, "JOIN jobs j ON j.id = $1")
			require.Contains(t, sql, "ORDER BY s.secret_key ASC")
			require.Equal(t, []any{"job-1", "staging", "production"}, args)
			return &mockRows{scanFns: []func(dest ...any) error{
				func(dest ...any) error {
					fillJobSecretDest(dest, encrypted, true, time.Now().UTC())
					return nil
				},
			}}, nil
		}}
		q := New(db)
		q.SetSecretEncryptionKey(secretsUnitEncryptionKey)

		got, err := q.ListJobSecretsByJob(context.Background(), "job-1", "staging")
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.Equal(t, "plaintext", got[0].Value)
	})

	t.Run("wraps query scan decrypt and rows errors", func(t *testing.T) {
		t.Parallel()

		queryErr := errors.New("query failed")
		q := New(&mockDBTX{queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
			return nil, queryErr
		}})
		got, err := q.ListJobSecretsByJob(context.Background(), "job-1", "production")
		require.ErrorIs(t, err, queryErr)
		require.Nil(t, got)

		scanErr := errors.New("scan failed")
		q = New(&mockDBTX{queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
			return &mockRows{scanFns: []func(dest ...any) error{
				func(...any) error { return scanErr },
			}}, nil
		}})
		got, err = q.ListJobSecretsByJob(context.Background(), "job-1", "production")
		require.ErrorIs(t, err, scanErr)
		require.Nil(t, got)

		q = New(&mockDBTX{queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
			return &mockRows{scanFns: []func(dest ...any) error{
				func(dest ...any) error {
					fillJobSecretDest(dest, "bad-ciphertext", true, time.Now().UTC())
					return nil
				},
			}}, nil
		}})
		q.SetSecretEncryptionKey(secretsUnitEncryptionKey)
		got, err = q.ListJobSecretsByJob(context.Background(), "job-1", "production")
		require.ErrorContains(t, err, "list job secrets by job decrypt")
		require.Nil(t, got)

		rowsErr := errors.New("rows failed")
		q = New(&mockDBTX{queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
			return &mockRows{err: rowsErr}, nil
		}})
		got, err = q.ListJobSecretsByJob(context.Background(), "job-1", "production")
		require.ErrorIs(t, err, rowsErr)
		require.Nil(t, got)
	})
}
