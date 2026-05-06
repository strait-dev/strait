//go:build loadtest

package loadtest

import (
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	vegeta "github.com/tsenart/vegeta/v12/lib"
)

func TestEdge_NotFoundResponses(t *testing.T) {
	mustClean(t)

	notFoundPaths := []string{
		"/v1/jobs/nonexistent-job-id/",
		"/v1/runs/nonexistent-run-id/",
		"/v1/workflows/nonexistent-wf-id/",
		"/v1/environments/nonexistent-env-id/",
		"/v1/job-groups/nonexistent-group-id/",
		"/v1/roles/nonexistent-role-id",
	}

	var counter atomic.Int64
	tgt := func(tgt *vegeta.Target) error {
		i := counter.Add(1) - 1
		tgt.Method = "GET"
		tgt.URL = baseURL + notFoundPaths[i%int64(len(notFoundPaths))]
		tgt.Header = http.Header{
			"X-Internal-Secret": []string{"test-secret-value"},
		}
		return nil
	}

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "404-responses", tgt)
		assertStatusCodes(t, m, "404")
		assertLatencySLA(t, m)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "404-responses", tgt)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "404-responses", tgt)
		assertNoServerErrors(t, m)
	})
}

func TestEdge_UnauthenticatedRequests(t *testing.T) {
	mustClean(t)

	unauthPaths := []string{
		"/v1/jobs/",
		"/v1/runs/",
		"/v1/workflows/",
		"/v1/roles",
		"/v1/stats",
		"/v1/secrets/",
	}

	var counter atomic.Int64
	tgt := func(tgt *vegeta.Target) error {
		i := counter.Add(1) - 1
		tgt.Method = "GET"
		tgt.URL = baseURL + unauthPaths[i%int64(len(unauthPaths))]
		tgt.Header = http.Header{
			"Content-Type": []string{"application/json"},
		}
		return nil
	}

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "unauth-requests", tgt)
		assertStatusCodes(t, m, "401")
		assertLatencySLA(t, m)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "unauth-requests", tgt)
		assertNoServerErrors(t, m)
	})
}

func TestEdge_InvalidAuthToken(t *testing.T) {
	mustClean(t)

	tgt := func(tgt *vegeta.Target) error {
		tgt.Method = "GET"
		tgt.URL = baseURL + "/v1/jobs/"
		tgt.Header = http.Header{
			"X-Internal-Secret": []string{"wrong-secret-value"},
		}
		return nil
	}

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "invalid-auth", tgt)
		assertStatusCodes(t, m, "401")
		assertLatencySLA(t, m)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "invalid-auth", tgt)
		assertNoServerErrors(t, m)
	})
}

func TestEdge_InvalidSDKToken(t *testing.T) {
	mustClean(t)

	tgt := newSDKTargeter("POST", "/sdk/v1/runs/fake-run/heartbeat", "invalid-jwt-token", nil)

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "invalid-sdk-token", tgt)
		assertStatusCodes(t, m, "401")
		assertLatencySLA(t, m)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "invalid-sdk-token", tgt)
		assertNoServerErrors(t, m)
	})
}

func TestEdge_ValidationErrors(t *testing.T) {
	mustClean(t)

	type invalidReq struct {
		method string
		path   string
		body   string
	}

	invalidRequests := []invalidReq{
		{"POST", "/v1/jobs/", `{}`},
		{"POST", "/v1/jobs/", `{"name":"no-slug"}`},
		{"POST", "/v1/jobs/", `{"project_id":"p","name":"n","slug":"s"}`},
		{"POST", "/v1/workflows/", `{}`},
		{"POST", "/v1/roles", `{}`},
		{"POST", "/v1/environments/", `{}`},
		{"POST", "/v1/job-groups/", `{}`},
		{"POST", "/v1/secrets/", `{}`},
	}

	var counter atomic.Int64
	tgt := func(tgt *vegeta.Target) error {
		i := counter.Add(1) - 1
		req := invalidRequests[i%int64(len(invalidRequests))]
		tgt.Method = req.method
		tgt.URL = baseURL + req.path
		tgt.Header = http.Header{
			"X-Internal-Secret": []string{"test-secret-value"},
			"Content-Type":      []string{"application/json"},
		}
		tgt.Body = []byte(req.body)
		return nil
	}

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "validation-errors", tgt)
		assertNoServerErrors(t, m)
		assertLatencySLA(t, m)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "validation-errors", tgt)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "validation-errors", tgt)
		assertNoServerErrors(t, m)
	})
}

func TestEdge_MalformedJSON(t *testing.T) {
	mustClean(t)

	malformedBodies := []string{
		`{invalid json`,
		`{"unterminated": `,
		`not json at all`,
		`<xml>nope</xml>`,
		`{"nested": {"deep": {"bad":}}}`,
	}

	var counter atomic.Int64
	tgt := func(tgt *vegeta.Target) error {
		i := counter.Add(1) - 1
		tgt.Method = "POST"
		tgt.URL = baseURL + "/v1/jobs/"
		tgt.Header = http.Header{
			"X-Internal-Secret": []string{"test-secret-value"},
			"Content-Type":      []string{"application/json"},
		}
		tgt.Body = []byte(malformedBodies[i%int64(len(malformedBodies))])
		return nil
	}

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "malformed-json", tgt)
		assertStatusCodes(t, m, "400")
		assertLatencySLA(t, m)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "malformed-json", tgt)
		assertNoServerErrors(t, m)
	})
}

func TestEdge_LargePayload(t *testing.T) {
	mustClean(t)
	projectID := "proj-edge-large-" + newID()
	jobID := seedJob(t, projectID)

	// 100KB payload
	largeData := strings.Repeat("x", 100*1024)
	tgt := newTargeter("POST", "/v1/jobs/"+jobID+"/trigger", func() []byte {
		return fmt.Appendf(nil, `{"payload":{"data":"%s"}}`, largeData)
	})

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "large-payload-100kb", tgt, withRate(50))
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "large-payload-100kb", tgt, withRate(500), withWorkers(20))
		assertNoServerErrors(t, m)
	})
}

func TestEdge_EmptyBody(t *testing.T) {
	mustClean(t)

	emptyPaths := []string{
		"/v1/jobs/",
		"/v1/workflows/",
		"/v1/roles",
		"/v1/environments/",
		"/v1/job-groups/",
	}

	var counter atomic.Int64
	tgt := func(tgt *vegeta.Target) error {
		i := counter.Add(1) - 1
		tgt.Method = "POST"
		tgt.URL = baseURL + emptyPaths[i%int64(len(emptyPaths))]
		tgt.Header = http.Header{
			"X-Internal-Secret": []string{"test-secret-value"},
			"Content-Type":      []string{"application/json"},
		}
		tgt.Body = []byte("")
		return nil
	}

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "empty-body", tgt)
		assertNoServerErrors(t, m)
		assertLatencySLA(t, m)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "empty-body", tgt)
		assertNoServerErrors(t, m)
	})
}

func TestEdge_DuplicateSlug(t *testing.T) {
	mustClean(t)
	projectID := "proj-edge-dup-" + newID()
	slug := "dup-slug-" + newID()

	httpDo(t, "POST", "/v1/jobs/", fmt.Sprintf(
		`{"project_id":"%s","name":"original","slug":"%s","endpoint_url":"https://example.com/orig","max_attempts":1,"timeout_secs":30}`,
		projectID, slug,
	), nil)

	tgt := newTargeter("POST", "/v1/jobs/", func() []byte {
		return fmt.Appendf(nil,
			`{"project_id":"%s","name":"duplicate","slug":"%s","endpoint_url":"https://example.com/dup","max_attempts":1,"timeout_secs":30}`,
			projectID, slug,
		)
	})

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "duplicate-slug", tgt)
		assertNoServerErrors(t, m)
		assertLatencySLA(t, m)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "duplicate-slug", tgt)
		assertNoServerErrors(t, m)
	})
}

func TestEdge_ConcurrentDeleteSameResource(t *testing.T) {
	mustClean(t)
	projectID := "proj-edge-cdel-" + newID()
	jobID := seedJob(t, projectID)

	tgt := func(tgt *vegeta.Target) error {
		tgt.Method = "DELETE"
		tgt.URL = baseURL + "/v1/jobs/" + jobID + "/"
		tgt.Header = http.Header{
			"X-Internal-Secret": []string{"test-secret-value"},
		}
		return nil
	}

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "concurrent-delete-same", tgt, withDuration(5*time.Second))
		assertNoServerErrors(t, m)
	})
}

func TestEdge_RapidCreateDelete(t *testing.T) {
	mustClean(t)
	projectID := "proj-edge-rcd-" + newID()

	var counter atomic.Int64
	tgt := func(tgt *vegeta.Target) error {
		i := counter.Add(1) - 1
		tgt.Header = http.Header{
			"X-Internal-Secret": []string{"test-secret-value"},
			"Content-Type":      []string{"application/json"},
		}
		if i%2 == 0 {
			slug := "rcd-" + newID()
			tgt.Method = "POST"
			tgt.URL = baseURL + "/v1/jobs/"
			tgt.Body = fmt.Appendf(nil,
				`{"project_id":"%s","name":"rcd-%s","slug":"%s","endpoint_url":"https://example.com/%s","max_attempts":1,"timeout_secs":30}`,
				projectID, slug, slug, slug,
			)
		} else {
			tgt.Method = "GET"
			tgt.URL = baseURL + "/v1/jobs/?project_id=" + projectID
		}
		return nil
	}

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "rapid-create-read", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "rapid-create-read", tgt)
		assertNoServerErrors(t, m)
	})
}

func TestEdge_MethodNotAllowed(t *testing.T) {
	mustClean(t)

	tgt := func(tgt *vegeta.Target) error {
		tgt.Method = "PUT"
		tgt.URL = baseURL + "/v1/jobs/"
		tgt.Header = http.Header{
			"X-Internal-Secret": []string{"test-secret-value"},
		}
		return nil
	}

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "method-not-allowed", tgt)
		assertStatusCodes(t, m, "405")
		assertLatencySLA(t, m)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "method-not-allowed", tgt)
		assertNoServerErrors(t, m)
	})
}

func TestEdge_PaginationLimits(t *testing.T) {
	mustClean(t)
	projectID := "proj-edge-page-" + newID()
	seedManyJobs(t, projectID, 50)

	pageSizes := []string{"1", "10", "50", "100", "500"}

	var counter atomic.Int64
	tgt := func(tgt *vegeta.Target) error {
		i := counter.Add(1) - 1
		limit := pageSizes[i%int64(len(pageSizes))]
		tgt.Method = "GET"
		tgt.URL = baseURL + "/v1/jobs/?project_id=" + projectID + "&limit=" + limit
		tgt.Header = http.Header{
			"X-Internal-Secret": []string{"test-secret-value"},
		}
		return nil
	}

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "pagination-limits", tgt)
		assertSuccessRate(t, m, 0.99)
		assertNoServerErrors(t, m)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "pagination-limits", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
}

func TestEdge_IdempotencyKey(t *testing.T) {
	mustClean(t)
	projectID := "proj-edge-idem-" + newID()
	jobID := seedJob(t, projectID)

	idempotencyKey := "idem-" + newID()
	tgt := func(tgt *vegeta.Target) error {
		tgt.Method = "POST"
		tgt.URL = baseURL + "/v1/jobs/" + jobID + "/trigger"
		tgt.Header = http.Header{
			"X-Internal-Secret": []string{"test-secret-value"},
			"Content-Type":      []string{"application/json"},
			"X-Idempotency-Key": []string{idempotencyKey},
			"Idempotency-Key":   []string{idempotencyKey},
		}
		tgt.Body = []byte(`{"payload":{"idempotent":"test"}}`)
		return nil
	}

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "idempotency-key", tgt, withDuration(5*time.Second))
		assertNoServerErrors(t, m)
	})
}
