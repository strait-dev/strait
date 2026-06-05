package grpc

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	workerv1 "strait/internal/api/grpc/proto/workerv1"
)

// TestValidateRegistration covers every rejection branch in
// validateRegistration plus a handful of accepted boundary cases. Slot-
// count checks defend the dispatcher: PickWorkerForQueue ranks workers
// by SlotsAvailable, so an unbounded value would let a malicious worker
// monopolize a project's dispatch, and a negative or inconsistent value
// could wedge it.
func TestValidateRegistration(t *testing.T) {
	t.Parallel()

	base := func() *workerv1.WorkerRegistration {
		return &workerv1.WorkerRegistration{
			WorkerId:       "w1",
			Queues:         []string{"default"},
			SlotsTotal:     4,
			SlotsAvailable: 4,
		}
	}

	cases := []struct {
		name        string
		mutate      func(r *workerv1.WorkerRegistration)
		wantErr     bool
		wantSubstr  string
		wantCode    codes.Code
		isAccepted  bool // boundary case where mutation should still pass
		registerNil bool
	}{
		{
			name:        "nil registration",
			registerNil: true,
			wantErr:     true,
			wantSubstr:  "must not be nil",
			wantCode:    codes.InvalidArgument,
		},
		{
			name:       "empty worker_id",
			mutate:     func(r *workerv1.WorkerRegistration) { r.WorkerId = "" },
			wantErr:    true,
			wantSubstr: "worker_id must be non-empty",
			wantCode:   codes.InvalidArgument,
		},
		{
			name:       "blank worker_id",
			mutate:     func(r *workerv1.WorkerRegistration) { r.WorkerId = " \t" },
			wantErr:    true,
			wantSubstr: "worker_id must be non-empty",
			wantCode:   codes.InvalidArgument,
		},
		{
			name:       "oversized worker_id",
			mutate:     func(r *workerv1.WorkerRegistration) { r.WorkerId = strings.Repeat("x", maxWorkerIDLen+1) },
			wantErr:    true,
			wantSubstr: "worker_id exceeds",
			wantCode:   codes.InvalidArgument,
		},
		{
			name:       "boundary worker_id (=max)",
			mutate:     func(r *workerv1.WorkerRegistration) { r.WorkerId = strings.Repeat("x", maxWorkerIDLen) },
			isAccepted: true,
		},
		{
			name: "too many queues",
			mutate: func(r *workerv1.WorkerRegistration) {
				r.Queues = make([]string, maxQueuesPerWorker+1)
			},
			wantErr:    true,
			wantSubstr: "too many queues",
			wantCode:   codes.InvalidArgument,
		},
		{
			name:       "empty queue",
			mutate:     func(r *workerv1.WorkerRegistration) { r.Queues = []string{""} },
			wantErr:    true,
			wantSubstr: "queue must be non-empty",
			wantCode:   codes.InvalidArgument,
		},
		{
			name:       "blank queue",
			mutate:     func(r *workerv1.WorkerRegistration) { r.Queues = []string{" \t"} },
			wantErr:    true,
			wantSubstr: "queue must be non-empty",
			wantCode:   codes.InvalidArgument,
		},
		{
			name:       "oversized queue",
			mutate:     func(r *workerv1.WorkerRegistration) { r.Queues = []string{strings.Repeat("q", maxQueueNameBytes+1)} },
			wantErr:    true,
			wantSubstr: "queue exceeds",
			wantCode:   codes.InvalidArgument,
		},
		{
			name:       "boundary queue (=max)",
			mutate:     func(r *workerv1.WorkerRegistration) { r.Queues = []string{strings.Repeat("q", maxQueueNameBytes)} },
			isAccepted: true,
		},
		{
			name: "too many job_slugs",
			mutate: func(r *workerv1.WorkerRegistration) {
				r.JobSlugs = make([]string, maxJobSlugsPerWorker+1)
				for i := range r.JobSlugs {
					r.JobSlugs[i] = "job"
				}
			},
			wantErr:    true,
			wantSubstr: "too many job_slugs",
			wantCode:   codes.InvalidArgument,
		},
		{
			name:       "empty job_slug",
			mutate:     func(r *workerv1.WorkerRegistration) { r.JobSlugs = []string{""} },
			wantErr:    true,
			wantSubstr: "job_slug must be non-empty",
			wantCode:   codes.InvalidArgument,
		},
		{
			name:       "blank job_slug",
			mutate:     func(r *workerv1.WorkerRegistration) { r.JobSlugs = []string{" \t"} },
			wantErr:    true,
			wantSubstr: "job_slug must be non-empty",
			wantCode:   codes.InvalidArgument,
		},
		{
			name:       "oversized job_slug",
			mutate:     func(r *workerv1.WorkerRegistration) { r.JobSlugs = []string{strings.Repeat("s", maxJobSlugBytes+1)} },
			wantErr:    true,
			wantSubstr: "job_slug exceeds",
			wantCode:   codes.InvalidArgument,
		},
		{
			name:       "boundary job_slug (=max)",
			mutate:     func(r *workerv1.WorkerRegistration) { r.JobSlugs = []string{strings.Repeat("s", maxJobSlugBytes)} },
			isAccepted: true,
		},
		{
			name: "boundary job_slug count (=max)",
			mutate: func(r *workerv1.WorkerRegistration) {
				r.JobSlugs = make([]string, maxJobSlugsPerWorker)
				for i := range r.JobSlugs {
					r.JobSlugs[i] = "job"
				}
			},
			isAccepted: true,
		},
		{
			name: "too many metadata entries",
			mutate: func(r *workerv1.WorkerRegistration) {
				r.Metadata = make(map[string]string, maxRegistrationMetadataEntries+1)
				for i := range maxRegistrationMetadataEntries + 1 {
					r.Metadata[fmt.Sprintf("k%d", i)] = "v"
				}
			},
			wantErr:    true,
			wantSubstr: "too many metadata entries",
			wantCode:   codes.InvalidArgument,
		},
		{
			name:       "empty metadata key",
			mutate:     func(r *workerv1.WorkerRegistration) { r.Metadata = map[string]string{"": "v"} },
			wantErr:    true,
			wantSubstr: "metadata key must be non-empty",
			wantCode:   codes.InvalidArgument,
		},
		{
			name:       "blank metadata key",
			mutate:     func(r *workerv1.WorkerRegistration) { r.Metadata = map[string]string{" \t": "v"} },
			wantErr:    true,
			wantSubstr: "metadata key must be non-empty",
			wantCode:   codes.InvalidArgument,
		},
		{
			name: "oversized metadata key",
			mutate: func(r *workerv1.WorkerRegistration) {
				r.Metadata = map[string]string{strings.Repeat("k", maxRegistrationMetadataKeyBytes+1): "v"}
			},
			wantErr:    true,
			wantSubstr: "metadata key exceeds",
			wantCode:   codes.InvalidArgument,
		},
		{
			name: "oversized metadata value",
			mutate: func(r *workerv1.WorkerRegistration) {
				r.Metadata = map[string]string{"k": strings.Repeat("v", maxRegistrationMetadataValueBytes+1)}
			},
			wantErr:    true,
			wantSubstr: "metadata value exceeds",
			wantCode:   codes.InvalidArgument,
		},
		{
			name: "boundary metadata (=max)",
			mutate: func(r *workerv1.WorkerRegistration) {
				r.Metadata = map[string]string{
					strings.Repeat("k", maxRegistrationMetadataKeyBytes): strings.Repeat("v", maxRegistrationMetadataValueBytes),
				}
			},
			isAccepted: true,
		},
		{
			name: "too many in-flight tasks",
			mutate: func(r *workerv1.WorkerRegistration) {
				r.InFlightTasks = make([]*workerv1.InFlightTask, maxInFlightTasks+1)
			},
			wantErr:    true,
			wantSubstr: "too many in-flight tasks",
			wantCode:   codes.InvalidArgument,
		},
		{
			name:       "negative slots_total",
			mutate:     func(r *workerv1.WorkerRegistration) { r.SlotsTotal = -1 },
			wantErr:    true,
			wantSubstr: "slots_total must be non-negative",
			wantCode:   codes.InvalidArgument,
		},
		{
			name:       "oversized slots_total",
			mutate:     func(r *workerv1.WorkerRegistration) { r.SlotsTotal = maxSlotsPerWorker + 1; r.SlotsAvailable = 0 },
			wantErr:    true,
			wantSubstr: "slots_total exceeds",
			wantCode:   codes.InvalidArgument,
		},
		{
			name: "boundary slots_total (=max)",
			mutate: func(r *workerv1.WorkerRegistration) {
				r.SlotsTotal = maxSlotsPerWorker
				r.SlotsAvailable = maxSlotsPerWorker
			},
			isAccepted: true,
		},
		{
			name:       "negative slots_available",
			mutate:     func(r *workerv1.WorkerRegistration) { r.SlotsAvailable = -1 },
			wantErr:    true,
			wantSubstr: "slots_available must be non-negative",
			wantCode:   codes.InvalidArgument,
		},
		{
			name:       "slots_available > slots_total",
			mutate:     func(r *workerv1.WorkerRegistration) { r.SlotsAvailable = r.SlotsTotal + 1 },
			wantErr:    true,
			wantSubstr: "exceeds slots_total",
			wantCode:   codes.InvalidArgument,
		},
		{
			name:       "slots_available == slots_total",
			mutate:     func(r *workerv1.WorkerRegistration) { r.SlotsAvailable = r.SlotsTotal },
			isAccepted: true,
		},
		{
			name:       "zero slots (idle worker)",
			mutate:     func(r *workerv1.WorkerRegistration) { r.SlotsTotal = 0; r.SlotsAvailable = 0 },
			isAccepted: true,
		},
		{
			name:       "oversized hostname",
			mutate:     func(r *workerv1.WorkerRegistration) { r.Hostname = strings.Repeat("h", maxHostnameBytes+1) },
			wantErr:    true,
			wantSubstr: "hostname exceeds",
			wantCode:   codes.InvalidArgument,
		},
		{
			name:       "boundary hostname (=max)",
			mutate:     func(r *workerv1.WorkerRegistration) { r.Hostname = strings.Repeat("h", maxHostnameBytes) },
			isAccepted: true,
		},
		{
			name:       "oversized sdk_version",
			mutate:     func(r *workerv1.WorkerRegistration) { r.SdkVersion = strings.Repeat("v", maxSDKVersionBytes+1) },
			wantErr:    true,
			wantSubstr: "sdk_version exceeds",
			wantCode:   codes.InvalidArgument,
		},
		{
			name:       "boundary sdk_version (=max)",
			mutate:     func(r *workerv1.WorkerRegistration) { r.SdkVersion = strings.Repeat("v", maxSDKVersionBytes) },
			isAccepted: true,
		},
		{
			name:       "oversized sdk_language",
			mutate:     func(r *workerv1.WorkerRegistration) { r.SdkLanguage = strings.Repeat("l", maxSDKLanguageBytes+1) },
			wantErr:    true,
			wantSubstr: "sdk_language exceeds",
			wantCode:   codes.InvalidArgument,
		},
		{
			name:       "boundary sdk_language (=max)",
			mutate:     func(r *workerv1.WorkerRegistration) { r.SdkLanguage = strings.Repeat("l", maxSDKLanguageBytes) },
			isAccepted: true,
		},
		{
			name:       "oversized name",
			mutate:     func(r *workerv1.WorkerRegistration) { r.Name = strings.Repeat("n", maxNameBytes+1) },
			wantErr:    true,
			wantSubstr: "name exceeds",
			wantCode:   codes.InvalidArgument,
		},
		{
			name:       "boundary name (=max)",
			mutate:     func(r *workerv1.WorkerRegistration) { r.Name = strings.Repeat("n", maxNameBytes) },
			isAccepted: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var reg *workerv1.WorkerRegistration
			if !tc.registerNil {
				reg = base()
				if tc.mutate != nil {
					tc.mutate(reg)
				}
			}

			err := validateRegistration(reg)

			switch {
			case tc.isAccepted:
				if err != nil {
					require.Failf(t, "test failure",

						"expected acceptance, got error: %v", err)
				}
			case tc.wantErr:
				if err == nil {
					require.Fail(t,

						"expected error, got nil")
				}
				if !strings.Contains(err.Error(), tc.wantSubstr) {
					require.Failf(t, "test failure",

						"error %q does not contain %q", err.Error(), tc.wantSubstr)
				}
				if st, ok := status.FromError(err); !ok || st.Code() != tc.wantCode {
					require.Failf(t, "test failure",

						"expected gRPC code %v, got error %v", tc.wantCode, err)
				}
			}
		})
	}
}
