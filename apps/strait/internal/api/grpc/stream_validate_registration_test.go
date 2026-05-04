package grpc

import (
	"strings"
	"testing"

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
					t.Fatalf("expected acceptance, got error: %v", err)
				}
			case tc.wantErr:
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tc.wantSubstr) {
					t.Fatalf("error %q does not contain %q", err.Error(), tc.wantSubstr)
				}
				if st, ok := status.FromError(err); !ok || st.Code() != tc.wantCode {
					t.Fatalf("expected gRPC code %v, got error %v", tc.wantCode, err)
				}
			}
		})
	}
}
