package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

func fillDeploymentVersionDest(dest []any, id string, status domain.DeploymentVersionStatus, now time.Time, includeOptional bool) {
	*(dest[0].(*string)) = id
	*(dest[1].(*string)) = "project-1"
	*(dest[2].(*string)) = "production"
	*(dest[3].(*string)) = "go"
	*(dest[4].(*string)) = "oci://registry/app:v1"
	*(dest[5].(*[]byte)) = []byte(`{"image":"app:v1"}`)
	*(dest[7].(*domain.DeploymentVersionStatus)) = status
	*(dest[8].(*string)) = string(domain.DeploymentStrategyCanary)
	*(dest[16].(*time.Time)) = now
	*(dest[17].(*time.Time)) = now.Add(time.Second)

	percent := 25
	*(dest[9].(**int)) = &percent

	if !includeOptional {
		return
	}

	checksum := "sha256:abc"
	durationUs := float64((2 * time.Minute).Microseconds())
	finalizedAt := now.Add(2 * time.Second)
	promotedAt := now.Add(3 * time.Second)
	rollbackFromID := "deployment-old"
	createdBy := "creator-1"
	updatedBy := "operator-1"

	*(dest[6].(**string)) = &checksum
	*(dest[10].(**float64)) = &durationUs
	*(dest[11].(**time.Time)) = &finalizedAt
	*(dest[12].(**time.Time)) = &promotedAt
	*(dest[13].(**string)) = &rollbackFromID
	*(dest[14].(**string)) = &createdBy
	*(dest[15].(**string)) = &updatedBy
}

func TestScanDeploymentVersionUnit(t *testing.T) {
	t.Parallel()

	t.Run("scans optional fields", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		got, err := scanDeploymentVersion(&mockRow{scanFn: func(dest ...any) error {
			fillDeploymentVersionDest(dest, "deployment-1", domain.DeploymentVersionStatusPromoted, now, true)
			return nil
		}})
		require.NoError(t, err)
		require.Equal(t, "deployment-1", got.ID)
		require.Equal(t, domain.DeploymentVersionStatusPromoted, got.Status)
		require.Equal(t, domain.DeploymentStrategyCanary, got.Strategy)
		require.JSONEq(t, `{"image":"app:v1"}`, string(got.Manifest))
		require.Equal(t, "sha256:abc", got.Checksum)
		require.NotNil(t, got.CanaryDuration)
		require.Equal(t, 2*time.Minute, *got.CanaryDuration)
		require.Equal(t, "deployment-old", got.RollbackFromDeployment)
		require.Equal(t, "creator-1", got.CreatedBy)
		require.Equal(t, "operator-1", got.UpdatedBy)
	})

	t.Run("passes scan errors through", func(t *testing.T) {
		t.Parallel()

		scanErr := errors.New("scan failed")
		got, err := scanDeploymentVersion(&mockRow{scanFn: func(...any) error { return scanErr }})
		require.ErrorIs(t, err, scanErr)
		require.Nil(t, got)
	})
}

func TestCreateDeploymentVersionUnit(t *testing.T) {
	t.Parallel()

	t.Run("applies defaults and insert arguments", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		duration := 90 * time.Second
		var args []any
		db := &mockDBTX{queryRowFn: func(_ context.Context, sql string, gotArgs ...any) pgx.Row {
			require.Contains(t, sql, "INSERT INTO deployment_versions")
			args = append([]any(nil), gotArgs...)
			return &mockRow{scanFn: func(dest ...any) error {
				*(dest[0].(*time.Time)) = now
				*(dest[1].(*time.Time)) = now.Add(time.Second)
				return nil
			}}
		}}
		deployment := &domain.DeploymentVersion{
			ProjectID:      "project-1",
			Environment:    "production",
			Runtime:        "go",
			ArtifactURI:    "oci://registry/app:v1",
			Checksum:       "sha256:abc",
			CanaryDuration: &duration,
			CreatedBy:      "creator-1",
		}

		require.NoError(t, New(db).CreateDeploymentVersion(context.Background(), deployment))
		require.NotEmpty(t, deployment.ID)
		require.Equal(t, domain.DeploymentVersionStatusDraft, deployment.Status)
		require.Equal(t, domain.DeploymentStrategyDirect, deployment.Strategy)
		require.Equal(t, now, deployment.CreatedAt)
		require.Equal(t, now.Add(time.Second), deployment.UpdatedAt)
		require.Len(t, args, 13)
		require.Equal(t, deployment.ID, args[0])
		require.Equal(t, "project-1", args[1])
		require.JSONEq(t, `{}`, string(args[5].(json.RawMessage)))
		require.Equal(t, "draft", args[7])
		require.Equal(t, "direct", args[8])
		require.NotNil(t, args[10])
		require.Equal(t, int(duration.Seconds()), int(*args[10].(*float64)))
	})

	t.Run("rejects invalid status before insert", func(t *testing.T) {
		t.Parallel()

		called := false
		db := &mockDBTX{queryRowFn: func(context.Context, string, ...any) pgx.Row {
			called = true
			return &mockRow{}
		}}
		err := New(db).CreateDeploymentVersion(context.Background(), &domain.DeploymentVersion{
			Status: domain.DeploymentVersionStatus("bad"),
		})
		require.ErrorContains(t, err, "invalid status")
		require.False(t, called)
	})

	t.Run("wraps insert error", func(t *testing.T) {
		t.Parallel()

		insertErr := errors.New("insert failed")
		db := &mockDBTX{queryRowFn: func(context.Context, string, ...any) pgx.Row {
			return &mockRow{scanFn: func(...any) error { return insertErr }}
		}}
		err := New(db).CreateDeploymentVersion(context.Background(), &domain.DeploymentVersion{
			ProjectID:   "project-1",
			Environment: "production",
			Status:      domain.DeploymentVersionStatusDraft,
		})
		require.ErrorIs(t, err, insertErr)
		require.ErrorContains(t, err, "create deployment version")
	})
}

func TestGetAndFinalizeDeploymentVersionUnit(t *testing.T) {
	t.Parallel()

	t.Run("get maps missing and wraps scan errors", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{queryRowFn: func(context.Context, string, ...any) pgx.Row {
			return &mockRow{scanFn: func(...any) error { return pgx.ErrNoRows }}
		}}
		got, err := New(db).GetDeploymentVersion(context.Background(), "missing", "project-1")
		require.ErrorIs(t, err, ErrDeploymentVersionNotFound)
		require.Nil(t, got)

		readErr := errors.New("read failed")
		db = &mockDBTX{queryRowFn: func(context.Context, string, ...any) pgx.Row {
			return &mockRow{scanFn: func(...any) error { return readErr }}
		}}
		got, err = New(db).GetDeploymentVersion(context.Background(), "deployment-1", "project-1")
		require.ErrorIs(t, err, readErr)
		require.Nil(t, got)
	})

	t.Run("finalize returns updated deployment", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		db := &mockDBTX{queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
			require.Contains(t, sql, "SET status = 'finalized'")
			require.Equal(t, []any{"deployment-1", "project-1", "operator-1"}, args)
			return &mockRow{scanFn: func(dest ...any) error {
				fillDeploymentVersionDest(dest, "deployment-1", domain.DeploymentVersionStatusFinalized, now, true)
				return nil
			}}
		}}

		got, err := New(db).FinalizeDeploymentVersion(context.Background(), "deployment-1", "project-1", "operator-1")
		require.NoError(t, err)
		require.Equal(t, domain.DeploymentVersionStatusFinalized, got.Status)
		require.NotNil(t, got.FinalizedAt)
	})

	t.Run("finalize maps missing and wraps scan errors", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{queryRowFn: func(context.Context, string, ...any) pgx.Row {
			return &mockRow{scanFn: func(...any) error { return pgx.ErrNoRows }}
		}}
		got, err := New(db).FinalizeDeploymentVersion(context.Background(), "missing", "project-1", "")
		require.ErrorIs(t, err, ErrDeploymentVersionNotFound)
		require.Nil(t, got)

		updateErr := errors.New("update failed")
		db = &mockDBTX{queryRowFn: func(context.Context, string, ...any) pgx.Row {
			return &mockRow{scanFn: func(...any) error { return updateErr }}
		}}
		got, err = New(db).FinalizeDeploymentVersion(context.Background(), "deployment-1", "project-1", "")
		require.ErrorIs(t, err, updateErr)
		require.Nil(t, got)
	})
}

func TestListDeploymentVersionsUnit(t *testing.T) {
	t.Parallel()

	t.Run("defaults limit and scans rows", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		cursor := now.Add(time.Minute)
		db := &mockDBTX{queryFn: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
			require.Contains(t, sql, "ORDER BY created_at DESC")
			require.Equal(t, []any{"project-1", "production", &cursor, 50}, args)
			return &mockRows{scanFns: []func(dest ...any) error{
				func(dest ...any) error {
					fillDeploymentVersionDest(dest, "deployment-1", domain.DeploymentVersionStatusDraft, now, false)
					return nil
				},
			}}, nil
		}}

		got, err := New(db).ListDeploymentVersions(context.Background(), "project-1", "production", 0, &cursor)
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.Equal(t, "deployment-1", got[0].ID)
	})

	t.Run("wraps query scan and rows errors", func(t *testing.T) {
		t.Parallel()

		queryErr := errors.New("query failed")
		db := &mockDBTX{queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
			return nil, queryErr
		}}
		got, err := New(db).ListDeploymentVersions(context.Background(), "project-1", "", 10, nil)
		require.ErrorIs(t, err, queryErr)
		require.Nil(t, got)

		scanErr := errors.New("scan failed")
		db = &mockDBTX{queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
			return &mockRows{scanFns: []func(dest ...any) error{
				func(...any) error { return scanErr },
			}}, nil
		}}
		got, err = New(db).ListDeploymentVersions(context.Background(), "project-1", "", 10, nil)
		require.ErrorIs(t, err, scanErr)
		require.Nil(t, got)

		rowsErr := errors.New("rows failed")
		db = &mockDBTX{queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
			return &mockRows{err: rowsErr}, nil
		}}
		got, err = New(db).ListDeploymentVersions(context.Background(), "project-1", "", 10, nil)
		require.ErrorIs(t, err, rowsErr)
		require.Nil(t, got)
	})
}

func TestPromoteAndRollbackDeploymentVersionUnit(t *testing.T) {
	t.Parallel()

	t.Run("requires transactional database", func(t *testing.T) {
		t.Parallel()

		got, err := New(&mockDBTX{}).PromoteDeploymentVersion(context.Background(), "deployment-1", "project-1", "production", "")
		require.ErrorContains(t, err, "transactional database required")
		require.Nil(t, got)
	})

	t.Run("promotes and clears prior deployment", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		tx := &jobMemoryTx{}
		tx.queryRowFn = func(_ context.Context, sql string, args ...any) pgx.Row {
			switch {
			case strings.Contains(sql, "SELECT id FROM deployment_versions"):
				require.Equal(t, []any{"project-1", "production"}, args)
				return &mockRow{scanFn: func(dest ...any) error {
					previousID := "deployment-old"
					*(dest[0].(**string)) = &previousID
					return nil
				}}
			case strings.Contains(sql, "UPDATE deployment_versions") && strings.Contains(sql, "rollback_from_deployment_id"):
				require.Equal(t, []any{"deployment-2", "project-1", "production", "operator-1", false, "deployment-old"}, args)
				return &mockRow{scanFn: func(dest ...any) error {
					fillDeploymentVersionDest(dest, "deployment-2", domain.DeploymentVersionStatusPromoted, now, true)
					return nil
				}}
			default:
				t.Fatalf("unexpected query: %s", sql)
				return &mockRow{}
			}
		}
		tx.execFn = func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			require.Contains(t, sql, "promoted_at = NULL")
			require.Equal(t, []any{"project-1", "production", "operator-1"}, args)
			return pgconn.NewCommandTag("UPDATE 1"), nil
		}

		got, err := New(&jobMemoryTxBeginner{tx: tx}).PromoteDeploymentVersion(
			context.Background(), "deployment-2", "project-1", "production", "operator-1",
		)
		require.NoError(t, err)
		require.True(t, tx.committed)
		require.Equal(t, domain.DeploymentVersionStatusPromoted, got.Status)
	})

	t.Run("rollback records previous promoted deployment", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		tx := &jobMemoryTx{execFn: func(context.Context, string, ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("UPDATE 1"), nil
		}}
		tx.queryRowFn = func(_ context.Context, sql string, args ...any) pgx.Row {
			switch {
			case strings.Contains(sql, "SELECT id FROM deployment_versions"):
				return &mockRow{scanFn: func(dest ...any) error {
					previousID := "deployment-current"
					*(dest[0].(**string)) = &previousID
					return nil
				}}
			case strings.Contains(sql, "UPDATE deployment_versions") && strings.Contains(sql, "rollback_from_deployment_id"):
				require.Equal(t, []any{"deployment-target", "project-1", "production", "operator-1", true, "deployment-current"}, args)
				return &mockRow{scanFn: func(dest ...any) error {
					fillDeploymentVersionDest(dest, "deployment-target", domain.DeploymentVersionStatusPromoted, now, true)
					return nil
				}}
			default:
				t.Fatalf("unexpected query: %s", sql)
				return &mockRow{}
			}
		}

		got, err := New(&jobMemoryTxBeginner{tx: tx}).RollbackDeploymentVersion(
			context.Background(), "deployment-target", "project-1", "production", "operator-1",
		)
		require.NoError(t, err)
		require.True(t, tx.committed)
		require.Equal(t, domain.DeploymentVersionStatusPromoted, got.Status)
	})

	t.Run("handles no previous promoted deployment", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		tx := &jobMemoryTx{execFn: func(context.Context, string, ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("UPDATE 0"), nil
		}}
		tx.queryRowFn = func(_ context.Context, sql string, args ...any) pgx.Row {
			if strings.Contains(sql, "SELECT id FROM deployment_versions") {
				return &mockRow{scanFn: func(...any) error { return pgx.ErrNoRows }}
			}
			require.Equal(t, []any{"deployment-1", "project-1", "production", "", false, ""}, args)
			return &mockRow{scanFn: func(dest ...any) error {
				fillDeploymentVersionDest(dest, "deployment-1", domain.DeploymentVersionStatusPromoted, now, false)
				return nil
			}}
		}

		got, err := New(&jobMemoryTxBeginner{tx: tx}).PromoteDeploymentVersion(
			context.Background(), "deployment-1", "project-1", "production", "",
		)
		require.NoError(t, err)
		require.True(t, tx.committed)
		require.Equal(t, "deployment-1", got.ID)
	})

	t.Run("rolls back on transaction step errors", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name       string
			configure  func(*jobMemoryTx)
			wantErr    error
			wantString string
		}{
			{
				name: "load current",
				configure: func(tx *jobMemoryTx) {
					tx.queryRowFn = func(context.Context, string, ...any) pgx.Row {
						return &mockRow{scanFn: func(...any) error { return errors.New("select failed") }}
					}
				},
				wantString: "load currently promoted",
			},
			{
				name: "clear current",
				configure: func(tx *jobMemoryTx) {
					tx.queryRowFn = func(context.Context, string, ...any) pgx.Row {
						return &mockRow{scanFn: func(...any) error { return pgx.ErrNoRows }}
					}
					tx.execFn = func(context.Context, string, ...any) (pgconn.CommandTag, error) {
						return pgconn.CommandTag{}, errors.New("clear failed")
					}
				},
				wantString: "clear promoted",
			},
			{
				name: "missing target",
				configure: func(tx *jobMemoryTx) {
					tx.queryRowFn = func(_ context.Context, sql string, _ ...any) pgx.Row {
						if strings.Contains(sql, "SELECT id FROM deployment_versions") {
							return &mockRow{scanFn: func(...any) error { return pgx.ErrNoRows }}
						}
						return &mockRow{scanFn: func(...any) error { return pgx.ErrNoRows }}
					}
					tx.execFn = func(context.Context, string, ...any) (pgconn.CommandTag, error) {
						return pgconn.NewCommandTag("UPDATE 0"), nil
					}
				},
				wantErr: ErrDeploymentVersionNotFound,
			},
			{
				name: "promote scan",
				configure: func(tx *jobMemoryTx) {
					tx.queryRowFn = func(_ context.Context, sql string, _ ...any) pgx.Row {
						if strings.Contains(sql, "SELECT id FROM deployment_versions") {
							return &mockRow{scanFn: func(...any) error { return pgx.ErrNoRows }}
						}
						return &mockRow{scanFn: func(...any) error { return errors.New("promote failed") }}
					}
					tx.execFn = func(context.Context, string, ...any) (pgconn.CommandTag, error) {
						return pgconn.NewCommandTag("UPDATE 0"), nil
					}
				},
				wantString: "promote deployment version",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				tx := &jobMemoryTx{}
				tt.configure(tx)
				got, err := New(&jobMemoryTxBeginner{tx: tx}).PromoteDeploymentVersion(
					context.Background(), "deployment-1", "project-1", "production", "",
				)
				require.Nil(t, got)
				require.True(t, tx.rolledBack)
				if tt.wantErr != nil {
					require.ErrorIs(t, err, tt.wantErr)
					return
				}
				require.ErrorContains(t, err, tt.wantString)
			})
		}
	})
}
