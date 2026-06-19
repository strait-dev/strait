package store

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

func fillJobVersionScanDest(dest []any, v domain.JobVersion) {
	*(dest[0].(*string)) = v.ID
	*(dest[1].(*string)) = v.JobID
	*(dest[2].(*int)) = v.Version
	if v.VersionID != "" {
		*(dest[3].(**string)) = &v.VersionID
	}
	*(dest[4].(*bool)) = v.BackwardsCompatible
	*(dest[5].(*string)) = v.Name
	*(dest[6].(*string)) = v.Slug
	if v.Description != "" {
		*(dest[7].(**string)) = &v.Description
	}
	if v.Cron != "" {
		*(dest[8].(**string)) = &v.Cron
	}
	if v.PayloadSchema != nil {
		*(dest[9].(*[]byte)) = []byte(v.PayloadSchema)
	}
	if v.Tags != nil {
		tagsJSON, err := json.Marshal(v.Tags)
		if err != nil {
			panic(err)
		}
		*(dest[10].(*[]byte)) = tagsJSON
	}
	*(dest[11].(*string)) = v.EndpointURL
	if v.FallbackEndpointURL != "" {
		*(dest[12].(**string)) = &v.FallbackEndpointURL
	}
	*(dest[13].(*int)) = v.MaxAttempts
	*(dest[14].(*int)) = v.TimeoutSecs
	if v.WebhookURL != "" {
		*(dest[15].(**string)) = &v.WebhookURL
	}
	if v.WebhookSecret != "" {
		*(dest[16].(**string)) = &v.WebhookSecret
	}
	if v.RunTTLSecs > 0 {
		*(dest[17].(**int)) = &v.RunTTLSecs
	}
	*(dest[18].(*time.Time)) = v.CreatedAt
}

func jobVersionScanFn(v domain.JobVersion) func(dest ...any) error {
	return func(dest ...any) error {
		fillJobVersionScanDest(dest, v)
		return nil
	}
}

func TestCreateJobVersionUnit(t *testing.T) {
	t.Parallel()

	t.Run("normalizes empty optional fields", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		var args []any
		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, gotArgs ...any) pgx.Row {
				require.Contains(t, sql, "INSERT INTO job_versions")
				args = append([]any(nil), gotArgs...)
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*time.Time)) = now
					return nil
				}}
			},
		}
		v := &domain.JobVersion{
			ID:          "version-row-1",
			JobID:       "job-1",
			Version:     1,
			Name:        "Job One",
			Slug:        "job-one",
			EndpointURL: "https://example.com/run",
			MaxAttempts: 3,
			TimeoutSecs: 30,
		}

		require.NoError(t, New(db).CreateJobVersion(context.Background(), v))

		require.Equal(t, now, v.CreatedAt)
		require.Nil(t, args[3])
		require.Nil(t, args[7])
		require.Nil(t, args[8])
		require.Nil(t, args[9])
		require.JSONEq(t, `{}`, string(args[10].([]byte)))
		require.Nil(t, args[12])
		require.Nil(t, args[15])
		require.Nil(t, args[16])
		require.Nil(t, args[17])
	})

	t.Run("passes populated optional fields", func(t *testing.T) {
		t.Parallel()

		var args []any
		db := &mockDBTX{
			queryRowFn: func(_ context.Context, _ string, gotArgs ...any) pgx.Row {
				args = append([]any(nil), gotArgs...)
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*time.Time)) = time.Now().UTC()
					return nil
				}}
			},
		}
		v := &domain.JobVersion{
			ID:                  "version-row-1",
			JobID:               "job-1",
			Version:             2,
			VersionID:           "version-id-1",
			BackwardsCompatible: true,
			Name:                "Job One",
			Slug:                "job-one",
			Description:         "description",
			Cron:                "* * * * *",
			PayloadSchema:       json.RawMessage(`{"type":"object"}`),
			Tags:                map[string]string{"team": "core"},
			EndpointURL:         "https://example.com/run",
			FallbackEndpointURL: "https://example.com/fallback",
			MaxAttempts:         3,
			TimeoutSecs:         30,
			WebhookURL:          "https://example.com/webhook",
			WebhookSecret:       "secret",
			RunTTLSecs:          3600,
		}

		require.NoError(t, New(db).CreateJobVersion(context.Background(), v))

		require.Equal(t, "version-id-1", args[3])
		require.Equal(t, &v.Description, args[7])
		require.Equal(t, &v.Cron, args[8])
		require.Equal(t, []byte(v.PayloadSchema), args[9])
		require.JSONEq(t, `{"team":"core"}`, string(args[10].([]byte)))
		require.Equal(t, "https://example.com/fallback", args[12])
		require.Equal(t, &v.WebhookURL, args[15])
		require.Equal(t, &v.WebhookSecret, args[16])
		require.Equal(t, &v.RunTTLSecs, args[17])
	})

	t.Run("wraps insert errors", func(t *testing.T) {
		t.Parallel()

		insertErr := errors.New("insert failed")
		db := &mockDBTX{
			queryRowFn: func(context.Context, string, ...any) pgx.Row {
				return &mockRow{scanFn: func(...any) error { return insertErr }}
			},
		}
		v := &domain.JobVersion{ID: "version-row-1", JobID: "job-1", Version: 1}

		err := New(db).CreateJobVersion(context.Background(), v)

		require.ErrorIs(t, err, insertErr)
	})
}

func TestJobVersionQueriesUnit(t *testing.T) {
	t.Parallel()

	t.Run("lists with cursor and scans optional fields", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		cursor := now.Add(time.Minute)
		want := domain.JobVersion{
			ID:                  "version-row-1",
			JobID:               "job-1",
			Version:             3,
			VersionID:           "version-id-1",
			BackwardsCompatible: true,
			Name:                "Job One",
			Slug:                "job-one",
			Description:         "description",
			Cron:                "* * * * *",
			PayloadSchema:       json.RawMessage(`{"type":"object"}`),
			Tags:                map[string]string{"team": "core"},
			EndpointURL:         "https://example.com/run",
			FallbackEndpointURL: "https://example.com/fallback",
			MaxAttempts:         3,
			TimeoutSecs:         30,
			WebhookURL:          "https://example.com/webhook",
			WebhookSecret:       "secret",
			RunTTLSecs:          3600,
			CreatedAt:           now,
		}
		var args []any
		db := &mockDBTX{
			queryFn: func(_ context.Context, sql string, gotArgs ...any) (pgx.Rows, error) {
				require.Contains(t, sql, "created_at < $2")
				require.Contains(t, sql, "LIMIT $3")
				args = append([]any(nil), gotArgs...)
				return &mockRows{scanFns: []func(dest ...any) error{jobVersionScanFn(want)}}, nil
			},
		}

		got, err := New(db).ListJobVersionsByJob(context.Background(), "job-1", 10, &cursor)

		require.NoError(t, err)
		require.Equal(t, []domain.JobVersion{want}, got)
		require.Equal(t, []any{"job-1", cursor, 10}, args)
	})

	t.Run("list wraps query scan and row errors", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name       string
			rows       pgx.Rows
			queryErr   error
			wantSubstr string
		}{
			{name: "query", queryErr: errors.New("query failed"), wantSubstr: "list job versions"},
			{
				name: "scan",
				rows: &mockRows{scanFns: []func(dest ...any) error{
					func(...any) error { return errors.New("scan failed") },
				}},
				wantSubstr: "list job versions scan",
			},
			{name: "rows", rows: &mockRows{err: errors.New("rows failed")}, wantSubstr: "rows failed"},
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

				versions, err := New(db).ListJobVersionsByJob(context.Background(), "job-1", 10, nil)

				if tt.queryErr != nil {
					require.Nil(t, versions)
				} else {
					require.Empty(t, versions)
				}
				require.ErrorContains(t, err, tt.wantSubstr)
			})
		}
	})

	t.Run("gets by job version and maps not found", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		want := domain.JobVersion{
			ID:          "version-row-1",
			JobID:       "job-1",
			Version:     3,
			Name:        "Job One",
			Slug:        "job-one",
			EndpointURL: "https://example.com/run",
			MaxAttempts: 3,
			TimeoutSecs: 30,
			CreatedAt:   now,
		}
		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
				require.Contains(t, sql, "WHERE job_id = $1 AND version = $2")
				require.Equal(t, []any{"job-1", 3}, args)
				return &mockRow{scanFn: jobVersionScanFn(want)}
			},
		}

		got, err := New(db).GetJobVersion(context.Background(), "job-1", 3)

		require.NoError(t, err)
		require.Equal(t, &want, got)
	})

	t.Run("get by job version maps no rows and wraps other errors", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name       string
			scanErr    error
			wantSubstr string
		}{
			{name: "not found", scanErr: pgx.ErrNoRows, wantSubstr: "job version not found"},
			{name: "other", scanErr: errors.New("scan failed"), wantSubstr: "get job version"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				db := &mockDBTX{
					queryRowFn: func(context.Context, string, ...any) pgx.Row {
						return &mockRow{scanFn: func(...any) error { return tt.scanErr }}
					},
				}

				got, err := New(db).GetJobVersion(context.Background(), "job-1", 3)

				require.Nil(t, got)
				require.ErrorContains(t, err, tt.wantSubstr)
			})
		}
	})
}

func TestScanJobVersionUnit(t *testing.T) {
	t.Parallel()

	t.Run("keeps zero values when optional scan fields are nil or empty", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		want := domain.JobVersion{
			ID:          "version-row-1",
			JobID:       "job-1",
			Version:     1,
			Name:        "Job One",
			Slug:        "job-one",
			EndpointURL: "https://example.com/run",
			MaxAttempts: 3,
			TimeoutSecs: 30,
			CreatedAt:   now,
		}

		got, err := scanJobVersion(&mockRow{scanFn: jobVersionScanFn(want)})

		require.NoError(t, err)
		require.Equal(t, &want, got)
	})

	t.Run("rejects invalid tags json", func(t *testing.T) {
		t.Parallel()

		_, err := scanJobVersion(&mockRow{scanFn: func(dest ...any) error {
			fillJobVersionScanDest(dest, domain.JobVersion{
				ID:          "version-row-1",
				JobID:       "job-1",
				Version:     1,
				Name:        "Job One",
				Slug:        "job-one",
				EndpointURL: "https://example.com/run",
				MaxAttempts: 3,
				TimeoutSecs: 30,
				CreatedAt:   time.Now().UTC(),
			})
			*(dest[10].(*[]byte)) = []byte(`{`)
			return nil
		}})

		require.Error(t, err)
	})
}

func TestGetJobAtVersionUnit(t *testing.T) {
	t.Parallel()

	t.Run("scans snapshot row", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
				require.Contains(t, sql, "FROM job_versions jv")
				require.Equal(t, []any{"job-1", 2}, args)
				return &mockRow{scanFn: jobScanFn(now, true)}
			},
		}

		job, err := New(db).GetJobAtVersion(context.Background(), "job-1", 2)

		require.NoError(t, err)
		require.NotNil(t, job)
		require.Equal(t, "job-1", job.ID)
	})

	t.Run("falls back to live job when snapshot is missing", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		calls := 0
		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
				calls++
				if calls == 1 {
					require.Contains(t, sql, "FROM job_versions jv")
					require.Equal(t, []any{"job-1", 1}, args)
					return &mockRow{scanFn: func(...any) error { return pgx.ErrNoRows }}
				}
				require.Contains(t, sql, "FROM jobs")
				require.Equal(t, []any{"job-1"}, args)
				return &mockRow{scanFn: jobScanFn(now, false)}
			},
		}

		job, err := New(db).GetJobAtVersion(context.Background(), "job-1", 1)

		require.NoError(t, err)
		require.NotNil(t, job)
		require.Equal(t, "job-1", job.ID)
		require.Equal(t, 2, calls)
	})

	t.Run("wraps snapshot scan errors", func(t *testing.T) {
		t.Parallel()

		scanErr := errors.New("scan failed")
		db := &mockDBTX{
			queryRowFn: func(context.Context, string, ...any) pgx.Row {
				return &mockRow{scanFn: func(...any) error { return scanErr }}
			},
		}

		job, err := New(db).GetJobAtVersion(context.Background(), "job-1", 2)

		require.Nil(t, job)
		require.ErrorIs(t, err, scanErr)
		require.ErrorContains(t, err, "get job at version")
	})
}

func TestGetJobVersionByVersionIDUnit(t *testing.T) {
	t.Parallel()

	t.Run("scans version id lookup", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		want := domain.JobVersion{
			ID:          "version-row-1",
			JobID:       "job-1",
			Version:     2,
			VersionID:   "version-id-1",
			Name:        "Job One",
			Slug:        "job-one",
			EndpointURL: "https://example.com/run",
			MaxAttempts: 3,
			TimeoutSecs: 30,
			CreatedAt:   now,
		}
		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
				require.Contains(t, sql, "WHERE version_id = $1")
				require.Equal(t, []any{"version-id-1"}, args)
				return &mockRow{scanFn: jobVersionScanFn(want)}
			},
		}

		got, err := New(db).GetJobVersionByVersionID(context.Background(), "version-id-1")

		require.NoError(t, err)
		require.Equal(t, &want, got)
	})

	t.Run("maps no rows and wraps other errors", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name    string
			scanErr error
			wantErr error
		}{
			{name: "not found", scanErr: pgx.ErrNoRows, wantErr: ErrJobNotFound},
			{name: "other", scanErr: errors.New("scan failed")},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				db := &mockDBTX{
					queryRowFn: func(context.Context, string, ...any) pgx.Row {
						return &mockRow{scanFn: func(...any) error { return tt.scanErr }}
					},
				}

				got, err := New(db).GetJobVersionByVersionID(context.Background(), "version-id-1")

				require.Nil(t, got)
				if tt.wantErr != nil {
					require.ErrorIs(t, err, tt.wantErr)
				} else {
					require.ErrorIs(t, err, tt.scanErr)
					require.ErrorContains(t, err, "get job version by version_id")
				}
			})
		}
	})
}
