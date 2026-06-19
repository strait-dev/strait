package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

const environmentUnitEncryptionKey = "0123456789abcdef0123456789abcdef"

func environmentScanFn(t *testing.T, q *Queries, now time.Time, vars map[string]string, parentID *string) func(dest ...any) error {
	t.Helper()
	return func(dest ...any) error {
		variablesRaw, variablesEncrypted, err := q.prepareEnvironmentVariables("env-1", vars)
		require.NoError(t, err)

		*dest[0].(*string) = "env-1"
		*dest[1].(*string) = "project-1"
		*dest[2].(*string) = "Production"
		*dest[3].(*string) = "production"
		*dest[4].(**string) = parentID
		*dest[5].(*[]byte) = variablesRaw
		*dest[6].(*[]byte) = variablesEncrypted
		*dest[7].(*bool) = true
		*dest[8].(*time.Time) = now
		*dest[9].(*time.Time) = now.Add(time.Minute)
		return nil
	}
}

func environmentChainScanFn(
	t *testing.T,
	q *Queries,
	envID string,
	parentID *string,
	vars map[string]string,
	depth int,
) func(dest ...any) error {
	t.Helper()
	return func(dest ...any) error {
		variablesRaw, variablesEncrypted, err := q.prepareEnvironmentVariables(envID, vars)
		require.NoError(t, err)

		*dest[0].(*string) = envID
		*dest[1].(*string) = "project-1"
		*dest[2].(**string) = parentID
		*dest[3].(*[]byte) = variablesRaw
		*dest[4].(*[]byte) = variablesEncrypted
		*dest[5].(*int) = depth
		return nil
	}
}

func TestEnvironmentCreateUpdateAndOrgLimitUnit(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	var createCalls int
	db := &mockDBTX{}
	db.queryRowFn = func(_ context.Context, sql string, args ...any) pgx.Row {
		switch {
		case strings.Contains(sql, "pg_try_advisory_xact_lock"):
			require.Equal(t, "environment_limit:org-1", args[0])
			return &mockRow{scanFn: func(dest ...any) error {
				*dest[0].(*bool) = true
				return nil
			}}
		case strings.Contains(sql, "COUNT(*)") && strings.Contains(sql, "FROM environments"):
			require.Equal(t, "org-1", args[0])
			return &mockRow{scanFn: func(dest ...any) error {
				*dest[0].(*int) = 1
				return nil
			}}
		case strings.Contains(sql, "INSERT INTO environments"):
			createCalls++
			require.Len(t, args, 8)
			require.NotEmpty(t, args[0])
			require.Equal(t, "project-1", args[1])
			require.Equal(t, "Production", args[2])
			require.Equal(t, "production", args[3])
			require.Nil(t, args[4])
			require.JSONEq(t, `{}`, string(args[5].([]byte)))
			require.Nil(t, args[6])
			require.False(t, args[7].(bool))
			return &mockRow{scanFn: func(dest ...any) error {
				*dest[0].(*time.Time) = now
				*dest[1].(*time.Time) = now.Add(time.Minute)
				return nil
			}}
		default:
			require.Failf(t, "unexpected query", "%s", sql)
			return &mockRow{}
		}
	}

	q := New(db)
	env := &domain.Environment{ProjectID: "project-1", Name: "Production", Slug: "production"}
	require.NoError(t, q.CreateEnvironmentWithOrgLimit(context.Background(), env, "org-1", 2))
	require.NotEmpty(t, env.ID)
	require.Equal(t, now, env.CreatedAt)
	require.Equal(t, now.Add(time.Minute), env.UpdatedAt)
	require.Equal(t, 1, createCalls)

	limitDB := &mockDBTX{queryRowFn: func(_ context.Context, sql string, _ ...any) pgx.Row {
		return &mockRow{scanFn: func(dest ...any) error {
			switch {
			case strings.Contains(sql, "pg_try_advisory_xact_lock"):
				*dest[0].(*bool) = true
			case strings.Contains(sql, "COUNT(*)"):
				*dest[0].(*int) = 3
			default:
				require.Failf(t, "unexpected query", "%s", sql)
			}
			return nil
		}}
	}}
	err := New(limitDB).CreateEnvironmentWithOrgLimit(
		context.Background(),
		&domain.Environment{ProjectID: "project-1", Name: "Extra", Slug: "extra"},
		"org-1",
		3,
	)
	require.ErrorIs(t, err, ErrEnvironmentLimitExceeded)
}

func TestEnvironmentPersistenceUnit(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	parentID := "parent-1"
	q := New(&mockDBTX{})
	q.SetSecretEncryptionKey(environmentUnitEncryptionKey)

	var updateEncrypted []byte
	db := &mockDBTX{}
	db.queryRowFn = func(_ context.Context, sql string, args ...any) pgx.Row {
		switch {
		case strings.Contains(sql, "SELECT id, project_id, name, slug"):
			require.Equal(t, "env-1", args[0])
			require.Equal(t, "project-1", args[1])
			return &mockRow{scanFn: environmentScanFn(t, q, now, map[string]string{"TOKEN": "secret"}, &parentID)}
		case strings.Contains(sql, "UPDATE environments"):
			require.Len(t, args, 7)
			require.Equal(t, "Renamed", args[0])
			require.Equal(t, "renamed", args[1])
			require.Equal(t, parentID, args[2])
			require.JSONEq(t, `{}`, string(args[3].([]byte)))
			updateEncrypted = append([]byte(nil), args[4].([]byte)...)
			require.NotEmpty(t, updateEncrypted)
			require.Equal(t, "env-1", args[5])
			require.Equal(t, "project-1", args[6])
			return &mockRow{scanFn: func(dest ...any) error {
				*dest[0].(*time.Time) = now.Add(2 * time.Minute)
				return nil
			}}
		default:
			require.Failf(t, "unexpected query", "%s", sql)
			return &mockRow{}
		}
	}
	q.db = db

	got, err := q.GetEnvironment(context.Background(), "env-1", "project-1")
	require.NoError(t, err)
	require.Equal(t, "env-1", got.ID)
	require.Equal(t, parentID, got.ParentID)
	require.Equal(t, map[string]string{"TOKEN": "secret"}, got.Variables)
	require.True(t, got.IsStandard)

	got.Name = "Renamed"
	got.Slug = "renamed"
	got.Variables = map[string]string{"TOKEN": "rotated"}
	require.NoError(t, q.UpdateEnvironment(context.Background(), got))
	require.Equal(t, now.Add(2*time.Minute), got.UpdatedAt)
	require.NotEmpty(t, updateEncrypted)
}

func TestEnvironmentListAndDeleteUnit(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	q := New(&mockDBTX{})
	cursor := now.Add(time.Hour)
	db := &mockDBTX{}
	db.queryFn = func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
		require.Contains(t, sql, "created_at < $2")
		require.Contains(t, sql, "LIMIT $3")
		require.Equal(t, []any{"project-1", cursor, 2}, args)
		return &mockRows{scanFns: []func(dest ...any) error{
			environmentScanFn(t, q, now, nil, nil),
			environmentScanFn(t, q, now.Add(time.Minute), nil, nil),
		}}, nil
	}
	var deleteCalls int
	db.execFn = func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
		require.Contains(t, sql, "is_standard = FALSE")
		require.Equal(t, []any{"env-1", "project-1"}, args)
		deleteCalls++
		return pgconn.NewCommandTag("DELETE 1"), nil
	}
	q.db = db

	envs, err := q.ListEnvironments(context.Background(), "project-1", 2, &cursor)
	require.NoError(t, err)
	require.Len(t, envs, 2)
	require.Equal(t, "env-1", envs[0].ID)

	require.NoError(t, q.DeleteEnvironment(context.Background(), "env-1", "project-1"))
	require.Equal(t, 1, deleteCalls)

	db.execFn = func(context.Context, string, ...any) (pgconn.CommandTag, error) {
		return pgconn.NewCommandTag("DELETE 0"), nil
	}
	db.queryRowFn = func(_ context.Context, _ string, _ ...any) pgx.Row {
		return &mockRow{scanFn: func(dest ...any) error {
			*dest[0].(*bool) = true
			return nil
		}}
	}
	err = q.DeleteEnvironment(context.Background(), "env-standard", "project-1")
	require.ErrorIs(t, err, ErrStandardEnvironment)

	db.queryRowFn = func(context.Context, string, ...any) pgx.Row {
		return &mockRow{scanFn: func(...any) error { return pgx.ErrNoRows }}
	}
	err = q.DeleteEnvironment(context.Background(), "missing", "project-1")
	require.ErrorIs(t, err, ErrEnvironmentNotFound)
}

func TestCreateStandardEnvironmentsUnit(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	var slugs []string
	db := &mockDBTX{queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
		require.Contains(t, sql, "INSERT INTO environments")
		require.Equal(t, "project-1", args[1])
		name, ok := domain.StandardEnvironmentNames[args[3].(string)]
		require.True(t, ok)
		require.Equal(t, name, args[2])
		require.True(t, args[7].(bool))
		slugs = append(slugs, args[3].(string))
		return &mockRow{scanFn: func(dest ...any) error {
			*dest[0].(*time.Time) = now
			*dest[1].(*time.Time) = now
			return nil
		}}
	}}

	require.NoError(t, New(db).CreateStandardEnvironments(context.Background(), "project-1"))
	require.Equal(t, domain.StandardEnvironmentSlugs, slugs)

	insertErr := errors.New("insert failed")
	failDB := &mockDBTX{queryRowFn: func(context.Context, string, ...any) pgx.Row {
		return &mockRow{scanFn: func(...any) error { return insertErr }}
	}}
	err := New(failDB).CreateStandardEnvironments(context.Background(), "project-1")
	require.ErrorIs(t, err, insertErr)
	require.Contains(t, err.Error(), "create standard environment development")
}

func TestEnvironmentResolvedVariablesUnit(t *testing.T) {
	t.Parallel()

	q := New(&mockDBTX{})
	q.SetSecretEncryptionKey(environmentUnitEncryptionKey)
	parentID := "parent-1"
	db := &mockDBTX{}
	db.queryFn = func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
		require.Contains(t, sql, "WITH RECURSIVE chain")
		require.Equal(t, []any{"child-1", 10, "project-1"}, args)
		return &mockRows{scanFns: []func(dest ...any) error{
			environmentChainScanFn(t, q, "parent-1", nil, map[string]string{
				"BASE":   "parent",
				"SHARED": "parent",
			}, 2),
			environmentChainScanFn(t, q, "child-1", &parentID, map[string]string{
				"LEAF":   "child",
				"SHARED": "child",
			}, 1),
		}}, nil
	}
	q.db = db

	got, err := q.GetResolvedEnvironmentVariables(context.Background(), "project-1", "child-1")
	require.NoError(t, err)
	require.Equal(t, map[string]string{
		"BASE":   "parent",
		"LEAF":   "child",
		"SHARED": "child",
	}, got)
}

func TestEnvironmentErrorPathsUnit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			name: "get not found maps sentinel",
			run: func(t *testing.T) {
				t.Helper()
				db := &mockDBTX{queryRowFn: func(context.Context, string, ...any) pgx.Row {
					return &mockRow{scanFn: func(...any) error { return pgx.ErrNoRows }}
				}}
				_, err := New(db).GetEnvironment(context.Background(), "missing", "project-1")
				require.ErrorIs(t, err, ErrEnvironmentNotFound)
			},
		},
		{
			name: "list query error wraps",
			run: func(t *testing.T) {
				t.Helper()
				queryErr := errors.New("query failed")
				db := &mockDBTX{queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
					return nil, queryErr
				}}
				_, err := New(db).ListEnvironments(context.Background(), "project-1", 10, nil)
				require.ErrorIs(t, err, queryErr)
				require.Contains(t, err.Error(), "list environments")
			},
		},
		{
			name: "list scan error wraps",
			run: func(t *testing.T) {
				t.Helper()
				scanErr := errors.New("scan failed")
				db := &mockDBTX{queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
					return &mockRows{scanFns: []func(dest ...any) error{func(...any) error { return scanErr }}}, nil
				}}
				_, err := New(db).ListEnvironments(context.Background(), "project-1", 10, nil)
				require.ErrorIs(t, err, scanErr)
				require.Contains(t, err.Error(), "list environments scan")
			},
		},
		{
			name: "list rows error wraps",
			run: func(t *testing.T) {
				t.Helper()
				rowsErr := errors.New("rows failed")
				db := &mockDBTX{queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
					return &mockRows{err: rowsErr}, nil
				}}
				_, err := New(db).ListEnvironments(context.Background(), "project-1", 10, nil)
				require.ErrorIs(t, err, rowsErr)
				require.Contains(t, err.Error(), "list environments rows")
			},
		},
		{
			name: "update not found maps sentinel",
			run: func(t *testing.T) {
				t.Helper()
				db := &mockDBTX{queryRowFn: func(context.Context, string, ...any) pgx.Row {
					return &mockRow{scanFn: func(...any) error { return pgx.ErrNoRows }}
				}}
				err := New(db).UpdateEnvironment(context.Background(), &domain.Environment{ID: "missing", ProjectID: "project-1"})
				require.ErrorIs(t, err, ErrEnvironmentNotFound)
			},
		},
		{
			name: "delete exec error wraps",
			run: func(t *testing.T) {
				t.Helper()
				execErr := errors.New("delete failed")
				db := &mockDBTX{execFn: func(context.Context, string, ...any) (pgconn.CommandTag, error) {
					return pgconn.CommandTag{}, execErr
				}}
				err := New(db).DeleteEnvironment(context.Background(), "env-1", "project-1")
				require.ErrorIs(t, err, execErr)
				require.Contains(t, err.Error(), "delete environment")
			},
		},
		{
			name: "resolve query error wraps",
			run: func(t *testing.T) {
				t.Helper()
				queryErr := errors.New("resolve failed")
				db := &mockDBTX{queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
					return nil, queryErr
				}}
				_, err := New(db).GetResolvedEnvironmentVariables(context.Background(), "project-1", "env-1")
				require.ErrorIs(t, err, queryErr)
				require.Contains(t, err.Error(), "resolve environment variables")
			},
		},
		{
			name: "resolve empty rows maps not found",
			run: func(t *testing.T) {
				t.Helper()
				db := &mockDBTX{queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
					return &mockRows{}, nil
				}}
				_, err := New(db).GetResolvedEnvironmentVariables(context.Background(), "project-1", "missing")
				require.ErrorIs(t, err, ErrEnvironmentNotFound)
			},
		},
		{
			name: "resolve max depth exceeded",
			run: func(t *testing.T) {
				t.Helper()
				q := New(&mockDBTX{})
				ancestorParent := "too-deep"
				db := &mockDBTX{queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
					scans := make([]func(dest ...any) error, 0, 10)
					for i := 0; i < 10; i++ {
						parent := (*string)(nil)
						if i == 0 {
							parent = &ancestorParent
						}
						envID := fmt.Sprintf("env-%d", i)
						depth := 10 - i
						scans = append(scans, environmentChainScanFn(t, q, envID, parent, nil, depth))
					}
					return &mockRows{scanFns: scans}, nil
				}}
				q.db = db
				_, err := q.GetResolvedEnvironmentVariables(context.Background(), "project-1", "env-9")
				require.Error(t, err)
				require.Contains(t, err.Error(), "exceeded max inheritance depth")
			},
		},
		{
			name: "legacy plaintext variables require encryption",
			run: func(t *testing.T) {
				t.Helper()
				q := New(&mockDBTX{})
				_, err := q.decryptEnvironmentVariables("env-1", []byte(`{"TOKEN":"plain"}`), nil)
				require.ErrorIs(t, err, ErrEnvironmentVariableEncryptionRequired)
			},
		},
		{
			name: "invalid variable JSON wraps",
			run: func(t *testing.T) {
				t.Helper()
				_, err := unmarshalEnvironmentVariables([]byte(`{`))
				require.Error(t, err)
				require.Contains(t, err.Error(), "unmarshal environment variables")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.run(t)
		})
	}
}

func TestEnvironmentVariableSerializationUnit(t *testing.T) {
	t.Parallel()

	emptyRaw, err := marshalEnvironmentVariables(nil)
	require.NoError(t, err)
	require.JSONEq(t, `{}`, string(emptyRaw))

	varsRaw, err := marshalEnvironmentVariables(map[string]string{"A": "B"})
	require.NoError(t, err)
	var decoded map[string]string
	require.NoError(t, json.Unmarshal(varsRaw, &decoded))
	require.Equal(t, map[string]string{"A": "B"}, decoded)

	got, err := unmarshalEnvironmentVariables([]byte(`{}`))
	require.NoError(t, err)
	require.Nil(t, got)

	got, err = unmarshalEnvironmentVariables(nil)
	require.NoError(t, err)
	require.Nil(t, got)
}
