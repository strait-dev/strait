package worker

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"strait/internal/domain"
)

func TestMarshalOnFailurePayload(t *testing.T) {
	t.Parallel()

	payload, err := marshalOnFailurePayload(
		&domain.Job{ID: "job-1"},
		&domain.JobRun{
			ID:         "run-1",
			Status:     domain.StatusCrashed,
			ErrorClass: "server",
			Attempt:    2,
			Payload:    json.RawMessage(`{"input":"data"}`),
		},
		"segfault",
	)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"source_job_id":"job-1",
		"source_run_id":"run-1",
		"error":"segfault",
		"error_class":"server",
		"status":"crashed",
		"attempt":2,
		"original_input":{"input":"data"}
	}`, string(payload))
}

func TestMarshalOnFailurePayloadNilOriginalInput(t *testing.T) {
	t.Parallel()

	payload, err := marshalOnFailurePayload(
		&domain.Job{ID: "job-1"},
		&domain.JobRun{
			ID:      "run-1",
			Status:  domain.StatusDeadLetter,
			Attempt: 1,
		},
		"failed",
	)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"source_job_id":"job-1",
		"source_run_id":"run-1",
		"error":"failed",
		"error_class":"",
		"status":"dead_letter",
		"attempt":1,
		"original_input":null
	}`, string(payload))
}

func TestMarshalOnFailurePayloadInvalidOriginalInput(t *testing.T) {
	t.Parallel()

	_, err := marshalOnFailurePayload(
		&domain.Job{ID: "job-1"},
		&domain.JobRun{
			ID:      "run-1",
			Status:  domain.StatusDeadLetter,
			Attempt: 1,
			Payload: json.RawMessage(`{`),
		},
		"failed",
	)
	require.Error(t, err)
}

func BenchmarkMarshalOnFailurePayload(b *testing.B) {
	job := &domain.Job{ID: "job-1"}
	run := &domain.JobRun{
		ID:         "run-1",
		Status:     domain.StatusDeadLetter,
		ErrorClass: "server",
		Attempt:    3,
		Payload:    json.RawMessage(`{"order_id":"123","nested":{"amount":42}}`),
	}

	b.ReportAllocs()
	for b.Loop() {
		payload, err := marshalOnFailurePayload(job, run, "connection refused")
		if err != nil {
			b.Fatal(err)
		}
		if len(payload) == 0 {
			b.Fatal("empty payload")
		}
	}
}
