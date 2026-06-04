package telemetry

import (
	"errors"
	"reflect"
	"testing"

	"github.com/getsentry/sentry-go"
	"github.com/jackc/pgx/v5/pgconn"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"strait/internal/domain"
)

func TestBuildSentryRelease(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		version string
		commit  string
		want    string
	}{
		{name: "version only", version: "v1.2.3", commit: "none", want: "v1.2.3"},
		{name: "version and short commit", version: "v1.2.3", commit: "abc123", want: "v1.2.3+abc123"},
		{name: "version and long commit", version: "v1.2.3", commit: "abcdef1234567890", want: "v1.2.3+abcdef123456"},
		{name: "default version", version: "", commit: "", want: "dev"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := BuildSentryRelease(tc.version, tc.commit); got != tc.want {
				t.Fatalf("BuildSentryRelease() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestApplySentryFingerprint_Rules(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		event *sentry.Event
		err   error
		want  []string
	}{
		{
			name:  "pg error uses code and table",
			event: &sentry.Event{},
			err: &pgconn.PgError{
				Code:      "23505",
				TableName: "job_runs",
				Message:   `duplicate key value violates unique constraint "job_runs_pkey"`,
			},
			want: []string{"db", "23505", "job_runs"},
		},
		{
			name: "grpc uses service rpc and code",
			event: &sentry.Event{Tags: map[string]string{
				"service": "strait.worker.v1.WorkerService",
				"rpc":     "StreamTasks",
			}},
			err:  status.Error(codes.Unavailable, "worker unavailable"),
			want: []string{"grpc", "strait.worker.v1.WorkerService", "StreamTasks", "Unavailable"},
		},
		{
			name: "workflow uses step breadcrumb and error class",
			event: &sentry.Event{
				Tags: map[string]string{
					"subsystem":   "workflow",
					"error_class": "server",
				},
				Breadcrumbs: []*sentry.Breadcrumb{{
					Category: "workflow.step",
					Data: map[string]any{
						"step_type": "job",
					},
				}},
			},
			err:  errors.New("step failed"),
			want: []string{"workflow", "job", "server"},
		},
		{
			name:  "domain transition error",
			event: &sentry.Event{},
			err:   &domain.TransitionError{From: domain.StatusQueued, To: domain.StatusCompleted},
			want:  []string{"domain.transition", "queued", "completed"},
		},
		{
			name:  "endpoint error uses status class",
			event: &sentry.Event{},
			err:   &domain.EndpointError{StatusCode: 503},
			want:  []string{"endpoint", "5xx"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := BeforeSend(tc.event, &sentry.EventHint{OriginalException: tc.err})
			if got == nil {
				t.Fatal("BeforeSend dropped event")
				return
			}
			if !reflect.DeepEqual(got.Fingerprint, tc.want) {
				t.Fatalf("fingerprint = %#v, want %#v", got.Fingerprint, tc.want)
			}
		})
	}
}

func TestApplySentryFingerprint_PreservesExplicitFingerprint(t *testing.T) {
	t.Parallel()

	event := &sentry.Event{Fingerprint: []string{"custom", "fingerprint"}}
	got := BeforeSend(event, &sentry.EventHint{
		OriginalException: &pgconn.PgError{Code: "23505", TableName: "job_runs"},
	})
	if got == nil {
		t.Fatal("BeforeSend dropped event")
		return
	}
	if !reflect.DeepEqual(got.Fingerprint, []string{"custom", "fingerprint"}) {
		t.Fatalf("fingerprint = %#v, want explicit fingerprint", got.Fingerprint)
	}
}

func TestSentryClientOptionsCarriesRelease(t *testing.T) {
	t.Parallel()

	opts := SentryClientOptions(SentryConfig{
		DSN:         "https://public@example.com/1",
		Environment: "test",
		Release:     "v1.2.3+abc123",
	}, 0)
	if opts.Release != "v1.2.3+abc123" {
		t.Fatalf("Release = %q, want v1.2.3+abc123", opts.Release)
	}
}

func TestSentryClientOptionsEnablesTracingWithSampler(t *testing.T) {
	t.Parallel()

	opts := SentryClientOptions(SentryConfig{
		DSN:                     "https://public@example.com/1",
		MaxBreadcrumbs:          64,
		MaxSpans:                256,
		MaxErrorDepth:           16,
		StrictTraceContinuation: true,
	}, 0.25)
	if !opts.EnableTracing {
		t.Fatal("expected tracing to be enabled")
	}
	if opts.TracesSampler == nil {
		t.Fatal("expected traces sampler")
	}
	if opts.TracesSampleRate != 0 {
		t.Fatalf("TracesSampleRate = %v, want sampler-only config", opts.TracesSampleRate)
	}
	if opts.MaxBreadcrumbs != 64 || opts.MaxSpans != 256 || opts.MaxErrorDepth != 16 {
		t.Fatalf("Sentry limits = breadcrumbs:%d spans:%d depth:%d", opts.MaxBreadcrumbs, opts.MaxSpans, opts.MaxErrorDepth)
	}
	if !opts.StrictTraceContinuation {
		t.Fatal("expected strict trace continuation")
	}
}
