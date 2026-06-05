//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

// Fuzz tests for run methods.

// FuzzCreateRunPayload fuzzes the payload field of CreateRun.
func FuzzCreateRunPayload(f *testing.F) {
	f.Add([]byte(`{"hello":"world"}`))
	f.Add([]byte(`null`))
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"deeply":{"nested":{"key":"value"}}}`))
	f.Add([]byte(`"just a string"`))

	f.Fuzz(func(t *testing.T, payload []byte) {
		// Only test valid JSON to avoid FK/encoding failures that are not interesting.
		if !json.Valid(payload) {
			t.Skip()
		}

		ctx := context.Background()
		q := mustStore(t)
		mustClean(t, ctx)

		job := mustCreateJob(t, ctx, q, "project-fuzz-payload")
		run := baseRun(job, newID())
		run.Payload = payload
		if err := q.CreateRun(ctx, run); err != nil {
			// Some payloads might trigger DB errors (e.g. too large), that is acceptable.
			return
		}

		got, err := q.GetRun(ctx, run.ID)
		require.NoError(t, err)
		require.NotNil(t, got)

	})
}

// FuzzInsertEventMessage fuzzes the message field of InsertEvent.
func FuzzInsertEventMessage(f *testing.F) {
	f.Add("normal message")
	f.Add("")
	f.Add(strings.Repeat("x", 10000))
	f.Add("message with 'quotes' and \"doubles\"")
	f.Add("message\nwith\nnewlines")

	f.Fuzz(func(t *testing.T, message string) {
		ctx := context.Background()
		q := mustStore(t)
		mustClean(t, ctx)

		job := mustCreateJob(t, ctx, q, "project-fuzz-event")
		run := mustCreateRun(t, ctx, q, job)

		ev := &domain.RunEvent{
			RunID:   run.ID,
			Type:    domain.EventType("log"),
			Message: message,
		}
		if err := q.InsertEvent(ctx, ev); err != nil {
			return
		}

		events, err := q.ListEventsAsc(ctx, run.ID, 10, nil, "")
		require.NoError(t, err)
		require.NotEmpty(t, events)

	})
}

// FuzzBatchBufferPayload fuzzes the payload for batch buffer items.
func FuzzBatchBufferPayload(f *testing.F) {
	f.Add([]byte(`{"data":"test"}`))
	f.Add([]byte(`[]`))
	f.Add([]byte(`null`))
	f.Add([]byte(`"string"`))
	f.Add([]byte(`123`))

	f.Fuzz(func(t *testing.T, payload []byte) {
		if !json.Valid(payload) {
			t.Skip()
		}

		ctx := context.Background()
		q := mustStore(t)
		mustClean(t, ctx)

		job := mustCreateJob(t, ctx, q, "project-fuzz-batch")
		item := &domain.BatchBufferItem{
			JobID:       job.ID,
			ProjectID:   job.ProjectID,
			BatchKey:    "fuzz-key",
			Payload:     payload,
			Tags:        json.RawMessage(`{}`),
			TriggeredBy: "manual",
		}
		if err := q.InsertBatchBufferItem(ctx, item); err != nil {
			return
		}

		count, err := q.CountBatchBufferItems(ctx, job.ID, "fuzz-key")
		require.NoError(t, err)
		require.GreaterOrEqual(
			t,
			count,
			1)

	})
}

// FuzzJobMemoryValue fuzzes the value field for job memory operations.
func FuzzJobMemoryValue(f *testing.F) {
	f.Add([]byte(`"simple"`))
	f.Add([]byte(`{"nested":"obj"}`))
	f.Add([]byte(`null`))
	f.Add([]byte(`12345`))
	f.Add([]byte(`[1,2,3]`))

	f.Fuzz(func(t *testing.T, value []byte) {
		if !json.Valid(value) {
			t.Skip()
		}

		ctx := context.Background()
		q := mustStore(t)
		mustClean(t, ctx)

		job := mustCreateJob(t, ctx, q, "project-fuzz-memory")
		mem := &domain.JobMemory{
			JobID:     job.ID,
			ProjectID: job.ProjectID,
			MemoryKey: "fuzz-key",
			Value:     json.RawMessage(value),
			SizeBytes: len(value),
		}
		if err := q.UpsertJobMemoryWithQuota(ctx, mem, 4096, 10); err != nil {
			return
		}

		got, err := q.GetJobMemory(ctx, job.ID, "fuzz-key")
		require.NoError(t, err)
		require.NotNil(t, got)

	})
}

// FuzzJobSlugLookup fuzzes slug-based job lookups.
func FuzzJobSlugLookup(f *testing.F) {
	f.Add("normal-slug")
	f.Add("slug-with-123")
	f.Add("")
	f.Add("UPPERCASE")
	f.Add("slug_with_underscores")

	f.Fuzz(func(t *testing.T, slug string) {
		if slug == "" || len(slug) > 200 {
			t.Skip()
		}
		// Avoid slugs with special chars that would cause SQL issues.
		for _, c := range slug {
			if c < 32 || c > 126 {
				t.Skip()
			}
		}

		ctx := context.Background()
		q := mustStore(t)
		mustClean(t, ctx)

		projectID := "project-fuzz-slug"
		job := baseJob(newID(), projectID)
		job.Slug = slug
		if err := q.CreateJob(ctx, job); err != nil {
			return
		}

		got, err := q.GetJobBySlug(ctx, projectID, slug)
		require.NoError(t, err)
		require.Equal(t, slug,
			got.
				Slug)

	})
}
