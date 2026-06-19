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

func fillJobScanDest(dest []any, now time.Time, withOptionalFields bool) {
	*(dest[0].(*string)) = "job-1"
	*(dest[1].(*string)) = "project-1"
	*(dest[3].(*string)) = "Job One"
	*(dest[4].(*string)) = "job-one"
	*(dest[9].(*string)) = "https://example.com/run"
	*(dest[11].(*int)) = 3
	*(dest[12].(*int)) = 30
	*(dest[19].(*bool)) = true
	*(dest[26].(*int)) = 2
	*(dest[29].(*bool)) = true
	*(dest[32].(*time.Time)) = now
	*(dest[33].(*time.Time)) = now.Add(time.Second)
	*(dest[37].(*int)) = 4
	*(dest[55].(*bool)) = false
	*(dest[58].(*string)) = "signing-secret"
	*(dest[59].(*int64)) = 12

	if !withOptionalFields {
		return
	}

	description := "description"
	groupID := "group-1"
	cron := "* * * * *"
	fallback := "https://example.com/fallback"
	maxConcurrency := 10
	executionWindowCron := "0 9 * * *"
	timezone := "UTC"
	rateLimitMax := 100
	rateLimitWindowSecs := 60
	dedupWindowSecs := 120
	webhookURL := "https://example.com/webhook"
	webhookSecret := "webhook-secret"
	runTTL := 3600
	retryStrategy := "fixed"
	environmentID := "env-prod"
	versionID := "version-1"
	versionPolicy := string(domain.VersionPolicyLatest)
	createdBy := "creator"
	updatedBy := "updater"
	maxConcurrencyPerKey := 2
	cronOverlapPolicy := string(domain.OverlapPolicySkip)
	debounceWindowSecs := 5
	batchWindowSecs := 10
	batchMaxSize := 25
	executionMode := string(domain.ExecutionModeWorker)
	queueName := "critical"
	onCompleteWorkflow := "workflow-next"
	onCompleteJob := "job-next"
	onFailureJob := "job-failed"
	onFailureWorkflow := "workflow-failed"
	pausedAt := now.Add(2 * time.Second)
	pauseReason := "maintenance"

	*(dest[2].(**string)) = &groupID
	*(dest[5].(**string)) = &description
	*(dest[6].(**string)) = &cron
	*(dest[7].(*[]byte)) = []byte(`{"type":"object"}`)
	*(dest[8].(*[]byte)) = []byte(`{"tier":"gold"}`)
	*(dest[10].(**string)) = &fallback
	*(dest[13].(**int)) = &maxConcurrency
	*(dest[14].(**string)) = &executionWindowCron
	*(dest[15].(**string)) = &timezone
	*(dest[16].(**int)) = &rateLimitMax
	*(dest[17].(**int)) = &rateLimitWindowSecs
	*(dest[18].(**int)) = &dedupWindowSecs
	*(dest[20].(**string)) = &webhookURL
	*(dest[21].(**string)) = &webhookSecret
	*(dest[22].(**int)) = &runTTL
	*(dest[23].(**string)) = &retryStrategy
	*(dest[24].(*[]int)) = []int{1, 2, 3}
	*(dest[25].(**string)) = &environmentID
	*(dest[27].(**string)) = &versionID
	*(dest[28].(**string)) = &versionPolicy
	*(dest[30].(**string)) = &createdBy
	*(dest[31].(**string)) = &updatedBy
	*(dest[34].(**int)) = &maxConcurrencyPerKey
	*(dest[35].(*[]byte)) = []byte(`[{"name":"tenant","max":10,"window_secs":60}]`)
	*(dest[36].(*[]byte)) = []byte(`{"tenant":"acme"}`)
	dlq := 20
	queueDepth := 200
	poison := 3
	*(dest[38].(**int)) = &dlq
	*(dest[39].(**int)) = &queueDepth
	*(dest[40].(**int)) = &poison
	*(dest[41].(**string)) = &cronOverlapPolicy
	*(dest[42].(*[]byte)) = []byte(`{"result":true}`)
	*(dest[43].(**int)) = &debounceWindowSecs
	*(dest[44].(**int)) = &batchWindowSecs
	*(dest[45].(**int)) = &batchMaxSize
	*(dest[46].(**string)) = &executionMode
	*(dest[47].(*[]string)) = []string{"iad", "sfo"}
	*(dest[48].(**string)) = &queueName
	*(dest[49].(**string)) = &onCompleteWorkflow
	*(dest[50].(**string)) = &onCompleteJob
	*(dest[51].(*[]byte)) = []byte(`{"complete":true}`)
	*(dest[52].(**string)) = &onFailureJob
	*(dest[53].(**string)) = &onFailureWorkflow
	*(dest[54].(*[]byte)) = []byte(`{"failure":true}`)
	*(dest[55].(*bool)) = true
	*(dest[56].(**time.Time)) = &pausedAt
	*(dest[57].(**string)) = &pauseReason
}

func jobScanFn(now time.Time, withOptionalFields bool) func(dest ...any) error {
	return func(dest ...any) error {
		fillJobScanDest(dest, now, withOptionalFields)
		return nil
	}
}

func TestJobCreateUpdateAndLookupUnit(t *testing.T) {
	t.Parallel()

	t.Run("creates job with generated defaults and nullable arguments", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		var args []any
		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, gotArgs ...any) pgx.Row {
				require.Contains(t, sql, "INSERT INTO jobs")
				args = append([]any(nil), gotArgs...)
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*time.Time)) = now
					*(dest[1].(*time.Time)) = now
					*(dest[2].(*int)) = 1
					*(dest[3].(*int64)) = 8
					return nil
				}}
			},
		}
		job := &domain.Job{
			ProjectID:   "project-1",
			Name:        "Job One",
			Slug:        "job-one",
			EndpointURL: "https://example.com/run",
			MaxAttempts: 3,
			TimeoutSecs: 30,
			Enabled:     true,
		}

		require.NoError(t, New(db).CreateJob(context.Background(), job))
		require.NotEmpty(t, job.ID)
		require.Equal(t, 1, job.Version)
		require.NotEmpty(t, job.VersionID)
		require.Equal(t, domain.VersionPolicyPin, job.VersionPolicy)
		require.Equal(t, domain.ExecutionModeHTTP, job.ExecutionMode)
		require.Equal(t, domain.OverlapPolicyAllow, job.CronOverlapPolicy)
		require.Equal(t, defaultJobQueueName, job.Queue)
		require.Nil(t, args[2])
		require.Nil(t, args[5])
		require.Nil(t, args[22])
		require.JSONEq(t, `{}`, string(args[8].([]byte)))
		require.JSONEq(t, `[]`, string(args[32].([]byte)))
		require.JSONEq(t, `{}`, string(args[33].([]byte)))
		require.Equal(t, int64(8), job.CacheVersion)
	})

	t.Run("create maps duplicate slug and wraps other errors", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			queryRowFn: func(context.Context, string, ...any) pgx.Row {
				return &mockRow{scanFn: func(...any) error { return &pgconn.PgError{Code: "23505"} }}
			},
		}
		err := New(db).CreateJob(context.Background(), &domain.Job{Slug: "duplicate"})
		require.ErrorIs(t, err, ErrJobSlugConflict)

		createErr := errors.New("insert failed")
		db.queryRowFn = func(context.Context, string, ...any) pgx.Row {
			return &mockRow{scanFn: func(...any) error { return createErr }}
		}
		err = New(db).CreateJob(context.Background(), &domain.Job{})
		require.ErrorContains(t, err, "create job")
		require.ErrorIs(t, err, createErr)
	})

	t.Run("get and get by slug map not found and scan errors", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
				switch {
				case strings.Contains(sql, "WHERE id = $1"):
					require.Equal(t, []any{"job-1"}, args)
					return &mockRow{scanFn: jobScanFn(now, true)}
				case strings.Contains(sql, "WHERE project_id = $1 AND slug = $2"):
					require.Equal(t, []any{"project-1", "job-one"}, args)
					return &mockRow{scanFn: jobScanFn(now, false)}
				default:
					require.Failf(t, "unexpected query", "sql=%s args=%v", sql, args)
					return &mockRow{}
				}
			},
		}

		got, err := New(db).GetJob(context.Background(), "job-1")
		require.NoError(t, err)
		require.Equal(t, "critical", got.Queue)
		require.Equal(t, domain.ExecutionModeWorker, got.ExecutionMode)
		require.Equal(t, map[string]string{"tier": "gold"}, got.Tags)
		require.JSONEq(t, `{"tenant":"acme"}`, mustMarshalString(t, got.DefaultRunMetadata))

		got, err = New(db).GetJobBySlug(context.Background(), "project-1", "job-one")
		require.NoError(t, err)
		require.Equal(t, domain.ExecutionModeHTTP, got.ExecutionMode)
		require.Empty(t, got.Queue)

		db.queryRowFn = func(context.Context, string, ...any) pgx.Row {
			return &mockRow{scanFn: func(...any) error { return pgx.ErrNoRows }}
		}
		_, err = New(db).GetJob(context.Background(), "missing")
		require.ErrorIs(t, err, ErrJobNotFound)
		_, err = New(db).GetJobBySlug(context.Background(), "project-1", "missing")
		require.ErrorIs(t, err, ErrJobNotFound)

		scanErr := errors.New("scan failed")
		db.queryRowFn = func(context.Context, string, ...any) pgx.Row {
			return &mockRow{scanFn: func(...any) error { return scanErr }}
		}
		_, err = New(db).GetJob(context.Background(), "job-1")
		require.ErrorContains(t, err, "get job")
		require.ErrorIs(t, err, scanErr)
		_, err = New(db).GetJobBySlug(context.Background(), "project-1", "job-one")
		require.ErrorContains(t, err, "get job by slug")
		require.ErrorIs(t, err, scanErr)
	})

	t.Run("updates job default queue and maps version conflicts", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		var args []any
		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, gotArgs ...any) pgx.Row {
				require.Contains(t, sql, "UPDATE jobs AS j")
				args = append([]any(nil), gotArgs...)
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*time.Time)) = now
					*(dest[1].(*int)) = 2
					*(dest[2].(*string)) = "version-2"
					*(dest[3].(*int64)) = 9
					return nil
				}}
			},
		}
		job := &domain.Job{ID: "job-1", Name: "Job One", Slug: "job-one", EndpointURL: "https://example.com/run", Version: 1}

		require.NoError(t, New(db).UpdateJob(context.Background(), job))
		require.Equal(t, defaultJobQueueName, job.Queue)
		require.Equal(t, defaultJobQueueName, args[44])
		require.Equal(t, 1, args[51])
		require.Equal(t, "version-2", job.VersionID)

		db.queryRowFn = func(context.Context, string, ...any) pgx.Row {
			return &mockRow{scanFn: func(...any) error { return pgx.ErrNoRows }}
		}
		err := New(db).UpdateJob(context.Background(), job)
		require.ErrorIs(t, err, ErrJobVersionConflict)

		updateErr := errors.New("update failed")
		db.queryRowFn = func(context.Context, string, ...any) pgx.Row {
			return &mockRow{scanFn: func(...any) error { return updateErr }}
		}
		err = New(db).UpdateJob(context.Background(), job)
		require.ErrorContains(t, err, "update job")
		require.ErrorIs(t, err, updateErr)
	})
}

func TestJobCronScheduleLimitWrappersUnit(t *testing.T) {
	t.Parallel()

	t.Run("create bypasses cron limit when guard inputs are empty", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name         string
			cron         string
			orgID        string
			maxSchedules int
		}{
			{name: "no cron", orgID: "org-1", maxSchedules: 1},
			{name: "no org", cron: "* * * * *", maxSchedules: 1},
			{name: "negative limit", cron: "* * * * *", orgID: "org-1", maxSchedules: -1},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				db := createJobResultDB(t, false)
				job := &domain.Job{ProjectID: "project-1", Name: "Job", Slug: "job", Cron: tc.cron}
				require.NoError(t, New(db).CreateJobWithCronScheduleLimit(context.Background(), job, tc.orgID, tc.maxSchedules))
			})
		}
	})

	t.Run("create enforces cron schedule limit before writing", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name      string
			count     int
			wantError error
		}{
			{name: "quota exceeded", count: 2, wantError: ErrCronScheduleLimitExceeded},
			{name: "quota allows create", count: 1},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				db := cronLimitJobDB(t, tc.count, false)
				job := &domain.Job{ProjectID: "project-1", Name: "Job", Slug: "job", Cron: "* * * * *"}
				err := New(db).CreateJobWithCronScheduleLimit(context.Background(), job, "org-1", 2)
				if tc.wantError != nil {
					require.ErrorIs(t, err, tc.wantError)
					return
				}
				require.NoError(t, err)
				require.Equal(t, 1, job.Version)
			})
		}
	})

	t.Run("update bypasses cron limit when guard inputs are empty", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name         string
			cron         string
			orgID        string
			maxSchedules int
		}{
			{name: "no cron", orgID: "org-1", maxSchedules: 1},
			{name: "no org", cron: "* * * * *", maxSchedules: 1},
			{name: "negative limit", cron: "* * * * *", orgID: "org-1", maxSchedules: -1},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				db := createJobResultDB(t, true)
				job := &domain.Job{ID: "job-1", ProjectID: "project-1", Name: "Job", Slug: "job", Cron: tc.cron, Version: 1}
				require.NoError(t, New(db).UpdateJobWithCronScheduleLimit(context.Background(), job, tc.orgID, tc.maxSchedules))
			})
		}
	})

	t.Run("update enforces cron schedule limit before writing", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name      string
			count     int
			wantError error
		}{
			{name: "quota exceeded", count: 2, wantError: ErrCronScheduleLimitExceeded},
			{name: "quota allows update", count: 1},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				db := cronLimitJobDB(t, tc.count, true)
				job := &domain.Job{ID: "job-1", ProjectID: "project-1", Name: "Job", Slug: "job", Cron: "* * * * *", Version: 1}
				err := New(db).UpdateJobWithCronScheduleLimit(context.Background(), job, "org-1", 2)
				if tc.wantError != nil {
					require.ErrorIs(t, err, tc.wantError)
					return
				}
				require.NoError(t, err)
				require.Equal(t, 2, job.Version)
			})
		}
	})
}

func TestJobListAndTagUnit(t *testing.T) {
	t.Parallel()

	t.Run("lists jobs with cursor and maps errors", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		cursor := now.Add(time.Minute)
		db := &mockDBTX{
			queryFn: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
				require.Contains(t, sql, "created_at < $2")
				require.Contains(t, sql, "LIMIT $3")
				require.Equal(t, []any{"project-1", cursor, 20}, args)
				return &mockRows{scanFns: []func(dest ...any) error{jobScanFn(now, true)}}, nil
			},
		}

		got, err := New(db).ListJobs(context.Background(), "project-1", 20, &cursor)
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.Equal(t, "job-1", got[0].ID)

		assertJobListErrors(t, func(q *Queries) ([]domain.Job, error) {
			return q.ListJobs(context.Background(), "project-1", 10, nil)
		}, "list jobs", "list jobs scan", "list jobs rows")
	})

	t.Run("lists cron jobs and maps errors", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		db := &mockDBTX{
			queryFn: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
				require.Contains(t, sql, "cron IS NOT NULL")
				require.Empty(t, args)
				return &mockRows{scanFns: []func(dest ...any) error{jobScanFn(now, false)}}, nil
			},
		}

		got, err := New(db).ListCronJobs(context.Background())
		require.NoError(t, err)
		require.Len(t, got, 1)

		assertJobListErrors(t, func(q *Queries) ([]domain.Job, error) {
			return q.ListCronJobs(context.Background())
		}, "list cron jobs", "list cron jobs scan", "list cron jobs rows")
	})

	t.Run("lists jobs by tag with value and key-only filters", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		cursor := now.Add(time.Minute)
		tests := []struct {
			name      string
			tagValue  string
			cursor    *time.Time
			wantSQL   []string
			wantArgs  []any
			wantNoSQL string
		}{
			{
				name:      "key only",
				wantSQL:   []string{"tags ? $2", "LIMIT $3"},
				wantArgs:  []any{"project-1", "tier", 10},
				wantNoSQL: "tags ->> $2",
			},
			{
				name:     "key value cursor",
				tagValue: "gold",
				cursor:   &cursor,
				wantSQL:  []string{"tags ->> $2 = $3", "created_at < $4", "LIMIT $5"},
				wantArgs: []any{"project-1", "tier", "gold", cursor, 10},
			},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				db := &mockDBTX{
					queryFn: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
						for _, want := range tc.wantSQL {
							require.Contains(t, sql, want)
						}
						if tc.wantNoSQL != "" {
							require.NotContains(t, sql, tc.wantNoSQL)
						}
						require.Equal(t, tc.wantArgs, args)
						return &mockRows{scanFns: []func(dest ...any) error{jobScanFn(now, false)}}, nil
					},
				}

				got, err := New(db).ListJobsByTag(context.Background(), "project-1", "tier", tc.tagValue, 10, tc.cursor)
				require.NoError(t, err)
				require.Len(t, got, 1)
			})
		}

		assertJobListErrors(t, func(q *Queries) ([]domain.Job, error) {
			return q.ListJobsByTag(context.Background(), "project-1", "tier", "", 10, nil)
		}, "list jobs by tag", "list jobs by tag scan", "list jobs by tag rows")
	})
}

func TestJobDeleteBatchPauseAndEndpointUnit(t *testing.T) {
	t.Parallel()

	t.Run("delete job checks existence active runs and side table errors", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name       string
			exists     bool
			active     int
			failExecAt int
			finalTag   pgconn.CommandTag
			wantErr    string
			wantIs     error
		}{
			{name: "missing", wantIs: ErrJobNotFound},
			{name: "active runs", exists: true, active: 1, wantIs: ErrJobHasActiveRuns},
			{name: "side rows error", exists: true, failExecAt: 1, wantErr: "delete job run side rows"},
			{name: "job runs error", exists: true, failExecAt: 2, wantErr: "delete job runs"},
			{name: "versions error", exists: true, failExecAt: 3, wantErr: "delete job versions"},
			{name: "dependencies error", exists: true, failExecAt: 4, wantErr: "delete job dependencies"},
			{name: "memory error", exists: true, failExecAt: 5, wantErr: "delete job memory"},
			{name: "final delete error", exists: true, failExecAt: 6, wantErr: "delete job"},
			{name: "final delete missing", exists: true, finalTag: pgconn.NewCommandTag("DELETE 0"), wantIs: ErrJobNotFound},
			{name: "success", exists: true, finalTag: pgconn.NewCommandTag("DELETE 1")},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				var queryRows int
				var execs int
				db := &mockDBTX{
					queryRowFn: func(context.Context, string, ...any) pgx.Row {
						queryRows++
						return &mockRow{scanFn: func(dest ...any) error {
							if queryRows == 1 {
								*(dest[0].(*bool)) = tc.exists
								return nil
							}
							*(dest[0].(*int)) = tc.active
							return nil
						}}
					},
					execFn: func(context.Context, string, ...any) (pgconn.CommandTag, error) {
						execs++
						if tc.failExecAt == execs {
							return pgconn.CommandTag{}, errors.New("exec failed")
						}
						if execs == 6 {
							if tc.finalTag.String() != "" {
								return tc.finalTag, nil
							}
							return pgconn.NewCommandTag("DELETE 1"), nil
						}
						return pgconn.NewCommandTag("DELETE 1"), nil
					},
				}

				err := New(db).DeleteJob(context.Background(), "job-1")
				if tc.wantIs != nil {
					require.ErrorIs(t, err, tc.wantIs)
					return
				}
				if tc.wantErr != "" {
					require.ErrorContains(t, err, tc.wantErr)
					return
				}
				require.NoError(t, err)
			})
		}
	})

	t.Run("batch updates jobs enabled", func(t *testing.T) {
		t.Parallel()

		empty, err := New(&mockDBTX{}).BatchUpdateJobsEnabled(context.Background(), nil, true, "")
		require.NoError(t, err)
		require.Zero(t, empty)

		tests := []struct {
			name      string
			projectID string
			wantArgs  []any
		}{
			{name: "unscoped", wantArgs: []any{true, []string{"job-1", "job-2"}}},
			{name: "project scoped", projectID: "project-1", wantArgs: []any{true, []string{"job-1", "job-2"}, "project-1"}},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				db := &mockDBTX{
					execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
						require.Contains(t, sql, "UPDATE jobs SET enabled")
						require.Equal(t, tc.wantArgs, args)
						return pgconn.NewCommandTag("UPDATE 2"), nil
					},
				}

				affected, err := New(db).BatchUpdateJobsEnabled(context.Background(), []string{"job-1", "job-2"}, true, tc.projectID)
				require.NoError(t, err)
				require.EqualValues(t, 2, affected)
			})
		}

		execErr := errors.New("update failed")
		db := &mockDBTX{
			execFn: func(context.Context, string, ...any) (pgconn.CommandTag, error) {
				return pgconn.CommandTag{}, execErr
			},
		}
		_, err = New(db).BatchUpdateJobsEnabled(context.Background(), []string{"job-1"}, true, "")
		require.ErrorContains(t, err, "batch update jobs enabled")
		require.ErrorIs(t, err, execErr)
	})

	t.Run("pause resume and endpoint updates map found and not found", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
				require.Contains(t, sql, "SELECT EXISTS")
				require.NotEmpty(t, args)
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*bool)) = true
					return nil
				}}
			},
			execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
				require.Contains(t, sql, "endpoint_signing_secret")
				require.Equal(t, []any{"job-1", "project-1", "https://example.com/new", (*string)(nil), "secret"}, args)
				return pgconn.NewCommandTag("UPDATE 1"), nil
			},
		}

		require.NoError(t, New(db).PauseJob(context.Background(), "job-1", "maintenance"))
		require.NoError(t, New(db).ResumeJob(context.Background(), "job-1"))
		require.NoError(t, New(db).UpdateJobEndpoint(context.Background(), "job-1", "project-1", "https://example.com/new", "", "secret"))

		db.queryRowFn = func(context.Context, string, ...any) pgx.Row {
			return &mockRow{scanFn: func(dest ...any) error {
				*(dest[0].(*bool)) = false
				return nil
			}}
		}
		require.ErrorIs(t, New(db).PauseJob(context.Background(), "missing", ""), ErrJobNotFound)
		require.ErrorIs(t, New(db).ResumeJob(context.Background(), "missing"), ErrJobNotFound)

		scanErr := errors.New("scan failed")
		db.queryRowFn = func(context.Context, string, ...any) pgx.Row {
			return &mockRow{scanFn: func(...any) error { return scanErr }}
		}
		err := New(db).PauseJob(context.Background(), "job-1", "")
		require.ErrorContains(t, err, "pause job")
		require.ErrorIs(t, err, scanErr)
		err = New(db).ResumeJob(context.Background(), "job-1")
		require.ErrorContains(t, err, "resume job")
		require.ErrorIs(t, err, scanErr)

		db.execFn = func(context.Context, string, ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("UPDATE 0"), nil
		}
		require.ErrorIs(t, New(db).UpdateJobEndpoint(context.Background(), "missing", "project-1", "url", "fallback", "secret"), ErrJobNotFound)

		execErr := errors.New("endpoint failed")
		db.execFn = func(context.Context, string, ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, execErr
		}
		err = New(db).UpdateJobEndpoint(context.Background(), "job-1", "project-1", "url", "fallback", "secret")
		require.ErrorContains(t, err, "update job endpoint")
		require.ErrorIs(t, err, execErr)
	})
}

func TestJobQuotaAndCountersUnit(t *testing.T) {
	t.Parallel()

	t.Run("gets project quota optional fields", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
				require.Contains(t, sql, "FROM project_quotas")
				require.Equal(t, []any{"project-1"}, args)
				return &mockRow{scanFn: func(dest ...any) error {
					maxQueued := 10
					maxExecuting := 5
					maxJobs := 100
					timezone := "UTC"
					maxCost := int64(1000)
					maxDaily := int64(5000)
					requests := 50
					window := 60
					region := "iad"
					plan := "pro"
					memoryPerKey := 1024
					memoryPerJob := 4096
					lifetimeDays := 90
					*(dest[0].(*string)) = "project-1"
					*(dest[1].(**int)) = &maxQueued
					*(dest[2].(**int)) = &maxExecuting
					*(dest[3].(**int)) = &maxJobs
					*(dest[4].(**string)) = &timezone
					*(dest[5].(**int64)) = &maxCost
					*(dest[6].(**int64)) = &maxDaily
					*(dest[7].(**int)) = &requests
					*(dest[8].(**int)) = &window
					*(dest[9].(**string)) = &region
					*(dest[10].(**string)) = &plan
					*(dest[11].(**int)) = &memoryPerKey
					*(dest[12].(**int)) = &memoryPerJob
					*(dest[13].(**int)) = &lifetimeDays
					*(dest[14].(*int64)) = 7
					return nil
				}}
			},
		}

		got, err := New(db).GetProjectQuota(context.Background(), "project-1")
		require.NoError(t, err)
		require.Equal(t, 10, got.MaxQueuedRuns)
		require.Equal(t, "UTC", got.Timezone)
		require.Equal(t, "iad", got.DefaultRegion)
		require.Equal(t, int64(7), got.CacheVersion)

		db.queryRowFn = func(context.Context, string, ...any) pgx.Row {
			return &mockRow{scanFn: func(...any) error { return pgx.ErrNoRows }}
		}
		got, err = New(db).GetProjectQuota(context.Background(), "missing")
		require.NoError(t, err)
		require.Nil(t, got)

		scanErr := errors.New("scan failed")
		db.queryRowFn = func(context.Context, string, ...any) pgx.Row {
			return &mockRow{scanFn: func(...any) error { return scanErr }}
		}
		_, err = New(db).GetProjectQuota(context.Background(), "project-1")
		require.ErrorContains(t, err, "get project quota")
		require.ErrorIs(t, err, scanErr)
	})

	t.Run("counts project and org runs", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name       string
			call       func(*Queries) (int, error)
			wantSQL    string
			wantArg    string
			wantString string
		}{
			{name: "queued", call: func(q *Queries) (int, error) { return q.CountProjectQueuedRuns(context.Background(), "project-1") }, wantSQL: "queued", wantArg: "project-1", wantString: "count project queued runs"},
			{name: "active", call: func(q *Queries) (int, error) { return q.CountProjectActiveRuns(context.Background(), "project-1") }, wantSQL: "dequeued", wantArg: "project-1", wantString: "count project active runs"},
			{name: "org executing", call: func(q *Queries) (int, error) { return q.CountExecutingRunsByOrg(context.Background(), "org-1") }, wantSQL: "org_id", wantArg: "org-1", wantString: "count executing runs by org"},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				db := &mockDBTX{
					queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
						require.Contains(t, sql, tc.wantSQL)
						require.Equal(t, []any{tc.wantArg}, args)
						return &mockRow{scanFn: func(dest ...any) error {
							*(dest[0].(*int)) = 4
							return nil
						}}
					},
				}
				got, err := tc.call(New(db))
				require.NoError(t, err)
				require.Equal(t, 4, got)

				scanErr := errors.New("scan failed")
				db.queryRowFn = func(context.Context, string, ...any) pgx.Row {
					return &mockRow{scanFn: func(...any) error { return scanErr }}
				}
				_, err = tc.call(New(db))
				require.ErrorContains(t, err, tc.wantString)
				require.ErrorIs(t, err, scanErr)
			})
		}
	})

	t.Run("bulk counts and lists orgs with executing runs", func(t *testing.T) {
		t.Parallel()

		empty, err := New(&mockDBTX{}).BulkCountExecutingRunsByOrg(context.Background(), nil)
		require.NoError(t, err)
		require.Empty(t, empty)

		db := &mockDBTX{
			queryFn: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
				require.Contains(t, sql, "GROUP BY p.org_id")
				require.Equal(t, []any{[]string{"org-1", "org-2"}}, args)
				return &mockRows{scanFns: []func(dest ...any) error{
					func(dest ...any) error {
						*(dest[0].(*string)) = "org-1"
						*(dest[1].(*int)) = 3
						return nil
					},
				}}, nil
			},
		}
		got, err := New(db).BulkCountExecutingRunsByOrg(context.Background(), []string{"org-1", "org-2"})
		require.NoError(t, err)
		require.Equal(t, map[string]int{"org-1": 3}, got)

		db.queryFn = func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
			require.Contains(t, sql, "SELECT DISTINCT p.org_id")
			require.Empty(t, args)
			return &mockRows{scanFns: []func(dest ...any) error{
				func(dest ...any) error {
					*(dest[0].(*string)) = "org-1"
					return nil
				},
			}}, nil
		}
		orgs, err := New(db).ListOrgsWithExecutingRuns(context.Background())
		require.NoError(t, err)
		require.Equal(t, []string{"org-1"}, orgs)

		queryErr := errors.New("query failed")
		db.queryFn = func(context.Context, string, ...any) (pgx.Rows, error) { return nil, queryErr }
		_, err = New(db).BulkCountExecutingRunsByOrg(context.Background(), []string{"org-1"})
		require.ErrorContains(t, err, "bulk count executing runs by org")
		_, err = New(db).ListOrgsWithExecutingRuns(context.Background())
		require.ErrorContains(t, err, "listing orgs with executing runs")

		scanErr := errors.New("scan failed")
		db.queryFn = func(context.Context, string, ...any) (pgx.Rows, error) {
			return &mockRows{scanFns: []func(dest ...any) error{func(...any) error { return scanErr }}}, nil
		}
		_, err = New(db).BulkCountExecutingRunsByOrg(context.Background(), []string{"org-1"})
		require.ErrorContains(t, err, "scanning bulk executing run count")
		_, err = New(db).ListOrgsWithExecutingRuns(context.Background())
		require.ErrorContains(t, err, "scanning org_id")

		rowsErr := errors.New("rows failed")
		db.queryFn = func(context.Context, string, ...any) (pgx.Rows, error) {
			return &mockRows{err: rowsErr}, nil
		}
		_, err = New(db).BulkCountExecutingRunsByOrg(context.Background(), []string{"org-1"})
		require.ErrorIs(t, err, rowsErr)
		_, err = New(db).ListOrgsWithExecutingRuns(context.Background())
		require.ErrorIs(t, err, rowsErr)
	})

	t.Run("updates project quota fields", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name     string
			call     func(*Queries) error
			wantSQL  string
			wantArgs []any
			wantErr  string
		}{
			{
				name:     "default region",
				call:     func(q *Queries) error { return q.UpdateProjectDefaultRegion(context.Background(), "project-1", "iad") },
				wantSQL:  "default_region",
				wantArgs: []any{"project-1", "iad"},
				wantErr:  "update project default region",
			},
			{
				name: "key lifetime",
				call: func(q *Queries) error {
					return q.UpdateProjectMaxKeyLifetimeDays(context.Background(), "project-1", 90)
				},
				wantSQL:  "max_key_lifetime_days",
				wantArgs: []any{"project-1", 90},
				wantErr:  "update project max key lifetime days",
			},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				db := &mockDBTX{
					execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
						require.Contains(t, sql, tc.wantSQL)
						require.Equal(t, tc.wantArgs, args)
						return pgconn.NewCommandTag("INSERT 0 1"), nil
					},
				}
				require.NoError(t, tc.call(New(db)))

				execErr := errors.New("exec failed")
				db.execFn = func(context.Context, string, ...any) (pgconn.CommandTag, error) {
					return pgconn.CommandTag{}, execErr
				}
				err := tc.call(New(db))
				require.ErrorContains(t, err, tc.wantErr)
				require.ErrorIs(t, err, execErr)
			})
		}
	})
}

func TestJobScannerAndJSONHelpersUnit(t *testing.T) {
	t.Parallel()

	t.Run("applies nullable fields and JSON defaults", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		got, err := scanJob(&mockRow{scanFn: jobScanFn(now, true)})
		require.NoError(t, err)
		require.Equal(t, "group-1", got.GroupID)
		require.Equal(t, "description", got.Description)
		require.Equal(t, []int{1, 2, 3}, got.RetryDelaysSecs)
		require.Equal(t, []string{"iad", "sfo"}, got.PreferredRegions)
		require.Equal(t, []domain.RateLimitKey{{Name: "tenant", Max: 10, WindowSecs: 60}}, got.RateLimitKeys)
		require.JSONEq(t, `{"complete":true}`, string(got.OnCompletePayloadMapping))
		require.True(t, got.Paused)
		require.NotNil(t, got.PausedAt)

		got, err = scanJob(&mockRow{scanFn: jobScanFn(now, false)})
		require.NoError(t, err)
		require.Equal(t, domain.ExecutionModeHTTP, got.ExecutionMode)
		require.Empty(t, got.Queue)
		require.Empty(t, got.Tags)

		scanErr := errors.New("scan failed")
		_, err = scanJob(&mockRow{scanFn: func(...any) error { return scanErr }})
		require.ErrorIs(t, err, scanErr)
	})

	t.Run("scanner rejects invalid JSON fields", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name string
			data scannedJobNullables
			want string
		}{
			{name: "tags", data: scannedJobNullables{tagsJSON: []byte(`{bad}`)}, want: "unmarshal job tags"},
			{name: "rate limit keys", data: scannedJobNullables{rateLimitKeysJSON: []byte(`{bad}`)}, want: "unmarshal rate_limit_keys"},
			{name: "default metadata", data: scannedJobNullables{defaultRunMetadataJSON: []byte(`{bad}`)}, want: "unmarshal default_run_metadata"},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				_, err := applyScannedJobNullables(&domain.Job{}, tc.data)
				require.ErrorContains(t, err, tc.want)
			})
		}
	})

	t.Run("marshal and unmarshal helper defaults", func(t *testing.T) {
		t.Parallel()

		tags, err := marshalTags(nil)
		require.NoError(t, err)
		require.JSONEq(t, `{}`, string(tags))

		tags, err = marshalTags(map[string]string{"tier": "gold"})
		require.NoError(t, err)
		require.JSONEq(t, `{"tier":"gold"}`, string(tags))

		decoded, err := unmarshalTags([]byte(`{}`))
		require.NoError(t, err)
		require.Nil(t, decoded)

		decoded, err = unmarshalTags([]byte(`{"tier":"gold"}`))
		require.NoError(t, err)
		require.Equal(t, map[string]string{"tier": "gold"}, decoded)

		_, err = unmarshalTags([]byte(`{bad}`))
		require.ErrorContains(t, err, "unmarshal job tags")

		require.JSONEq(t, `[]`, string(marshalJSONBOrDefault(nil, "[]")))
		require.JSONEq(t, `[]`, string(marshalJSONBOrDefault([]domain.RateLimitKey{}, "[]")))
		require.JSONEq(t, `{}`, string(marshalJSONBOrDefault(map[string]string{}, "{}")))
		require.JSONEq(t, `{"tier":"gold"}`, string(marshalJSONBOrDefault(map[string]string{"tier": "gold"}, "{}")))
		require.JSONEq(t, `[]`, string(marshalJSONBOrDefault(func() {}, "[]")))
	})
}

func createJobResultDB(t *testing.T, update bool) *mockDBTX {
	t.Helper()

	return &mockDBTX{
		queryRowFn: func(_ context.Context, sql string, _ ...any) pgx.Row {
			if update {
				require.Contains(t, sql, "UPDATE jobs AS j")
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*time.Time)) = time.Now().UTC()
					*(dest[1].(*int)) = 2
					*(dest[2].(*string)) = "version-2"
					*(dest[3].(*int64)) = 9
					return nil
				}}
			}
			require.Contains(t, sql, "INSERT INTO jobs")
			return &mockRow{scanFn: func(dest ...any) error {
				now := time.Now().UTC()
				*(dest[0].(*time.Time)) = now
				*(dest[1].(*time.Time)) = now
				*(dest[2].(*int)) = 1
				*(dest[3].(*int64)) = 8
				return nil
			}}
		},
	}
}

func cronLimitJobDB(t *testing.T, existingCronSchedules int, update bool) *mockDBTX {
	t.Helper()

	var queryRows int
	return &mockDBTX{
		queryRowFn: func(_ context.Context, sql string, _ ...any) pgx.Row {
			queryRows++
			switch queryRows {
			case 1:
				require.Contains(t, sql, "pg_try_advisory_xact_lock")
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*bool)) = true
					return nil
				}}
			case 2:
				require.Contains(t, sql, "COUNT(*) FROM jobs")
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*int)) = existingCronSchedules
					return nil
				}}
			default:
				if update {
					require.Contains(t, sql, "UPDATE jobs AS j")
					return &mockRow{scanFn: func(dest ...any) error {
						*(dest[0].(*time.Time)) = time.Now().UTC()
						*(dest[1].(*int)) = 2
						*(dest[2].(*string)) = "version-2"
						*(dest[3].(*int64)) = 9
						return nil
					}}
				}
				require.Contains(t, sql, "INSERT INTO jobs")
				return &mockRow{scanFn: func(dest ...any) error {
					now := time.Now().UTC()
					*(dest[0].(*time.Time)) = now
					*(dest[1].(*time.Time)) = now
					*(dest[2].(*int)) = 1
					*(dest[3].(*int64)) = 8
					return nil
				}}
			}
		},
	}
}

func assertJobListErrors(t *testing.T, call func(*Queries) ([]domain.Job, error), queryWrap, scanWrap, rowsWrap string) {
	t.Helper()

	tests := []struct {
		name       string
		queryErr   error
		scanErr    error
		rowErr     error
		wantString string
	}{
		{name: "query", queryErr: errors.New("query failed"), wantString: queryWrap},
		{name: "scan", scanErr: errors.New("scan failed"), wantString: scanWrap},
		{name: "rows", rowErr: errors.New("rows failed"), wantString: rowsWrap},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			db := &mockDBTX{
				queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
					if tc.queryErr != nil {
						return nil, tc.queryErr
					}
					rows := &mockRows{err: tc.rowErr}
					if tc.scanErr != nil {
						rows.scanFns = []func(dest ...any) error{func(...any) error { return tc.scanErr }}
					}
					return rows, nil
				},
			}

			_, err := call(New(db))
			require.ErrorContains(t, err, tc.wantString)
		})
	}
}

func mustMarshalString(t *testing.T, value any) string {
	t.Helper()

	encoded, err := json.Marshal(value)
	require.NoError(t, err)
	return string(encoded)
}
