//go:build loadtest

package loadtest

import (
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"

	vegeta "github.com/tsenart/vegeta/v12/lib"
)

func TestMixed_ReadHeavyWorkload(t *testing.T) {
	mustClean(t)
	projectID := "proj-mix-read-" + newID()
	jobID := seedJob(t, projectID)
	seedManyRuns(t, jobID, 50)
	wfID := seedWorkflow(t, projectID)
	seedJobGroup(t, projectID)
	seedEnvironment(t, projectID)

	readPaths := []string{
		"/v1/jobs/?project_id=" + projectID,
		"/v1/jobs/" + jobID + "/",
		"/v1/runs/?project_id=" + projectID,
		"/v1/workflows/?project_id=" + projectID,
		"/v1/workflows/" + wfID + "/",
		"/v1/job-groups/?project_id=" + projectID,
		"/v1/environments/?project_id=" + projectID,
		"/v1/roles",
		"/v1/members",
		"/health",
		"/health/ready",
	}

	var counter atomic.Int64
	tgt := func(tgt *vegeta.Target) error {
		i := counter.Add(1) - 1
		path := readPaths[i%int64(len(readPaths))]
		tgt.Method = "GET"
		tgt.URL = baseURL + path
		tgt.Header = http.Header{
			"X-Internal-Secret": []string{testInternalSecret},
		}
		return nil
	}

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "mixed-read-heavy", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "mixed-read-heavy", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "mixed-read-heavy", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

func TestMixed_WriteHeavyWorkload(t *testing.T) {
	mustClean(t)
	projectID := "proj-mix-write-" + newID()
	jobID := seedJob(t, projectID)

	var counter atomic.Int64
	tgt := func(tgt *vegeta.Target) error {
		i := counter.Add(1) - 1
		switch i % 4 {
		case 0:
			slug := "j-" + newID()
			tgt.Method = "POST"
			tgt.URL = baseURL + "/v1/jobs/"
			tgt.Body = []byte(fmt.Sprintf(
				`{"project_id":"%s","name":"mix-%s","slug":"%s","endpoint_url":"https://example.com/%s","max_attempts":1,"timeout_secs":30}`,
				projectID, slug, slug, slug,
			))
		case 1:
			tgt.Method = "POST"
			tgt.URL = baseURL + "/v1/jobs/" + jobID + "/trigger"
			tgt.Body = []byte(fmt.Sprintf(`{"payload":{"i":%d}}`, i))
		case 2:
			slug := "wf-" + newID()
			tgt.Method = "POST"
			tgt.URL = baseURL + "/v1/workflows/"
			tgt.Body = []byte(fmt.Sprintf(
				`{"project_id":"%s","name":"mix-wf-%s","slug":"%s","enabled":true}`,
				projectID, slug, slug,
			))
		case 3:
			slug := "grp-" + newID()
			tgt.Method = "POST"
			tgt.URL = baseURL + "/v1/job-groups/"
			tgt.Body = []byte(fmt.Sprintf(
				`{"project_id":"%s","name":"mix-grp-%s","slug":"%s"}`,
				projectID, slug, slug,
			))
		}
		tgt.Header = http.Header{
			"X-Internal-Secret": []string{testInternalSecret},
			"Content-Type":      []string{"application/json"},
		}
		return nil
	}

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "mixed-write-heavy", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "mixed-write-heavy", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "mixed-write-heavy", tgt)
		assertSuccessRate(t, m, 0.85)
		assertNoServerErrors(t, m)
	})
}

func TestMixed_ReadWriteRatio(t *testing.T) {
	mustClean(t)
	projectID := "proj-mix-rw-" + newID()
	jobID := seedJob(t, projectID)
	seedManyRuns(t, jobID, 30)

	var counter atomic.Int64
	tgt := func(tgt *vegeta.Target) error {
		i := counter.Add(1) - 1
		tgt.Header = http.Header{
			"X-Internal-Secret": []string{testInternalSecret},
			"Content-Type":      []string{"application/json"},
		}

		// 80% reads, 20% writes
		if i%5 < 4 {
			switch i % 4 {
			case 0:
				tgt.Method = "GET"
				tgt.URL = baseURL + "/v1/jobs/?project_id=" + projectID
			case 1:
				tgt.Method = "GET"
				tgt.URL = baseURL + "/v1/runs/?project_id=" + projectID
			case 2:
				tgt.Method = "GET"
				tgt.URL = baseURL + "/v1/jobs/" + jobID + "/"
			case 3:
				tgt.Method = "GET"
				tgt.URL = baseURL + "/v1/workflows/?project_id=" + projectID
			}
		} else {
			tgt.Method = "POST"
			tgt.URL = baseURL + "/v1/jobs/" + jobID + "/trigger"
			tgt.Body = []byte(fmt.Sprintf(`{"payload":{"rw":%d}}`, i))
		}
		return nil
	}

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "mixed-read-write-80-20", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.98)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "mixed-read-write-80-20", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "mixed-read-write-80-20", tgt)
		assertSuccessRate(t, m, 0.85)
		assertNoServerErrors(t, m)
	})
}

func TestMixed_CrossModuleWorkload(t *testing.T) {
	mustClean(t)
	projectID := "proj-mix-cross-" + newID()
	jobID := seedJob(t, projectID)
	wfID := seedWorkflow(t, projectID)
	groupID := seedJobGroup(t, projectID)
	envID := seedEnvironment(t, projectID)
	seedRole(t)

	type endpoint struct {
		method string
		path   string
		body   string
	}

	endpoints := []endpoint{
		{"GET", "/v1/jobs/?project_id=" + projectID, ""},
		{"GET", "/v1/jobs/" + jobID + "/", ""},
		{"GET", "/v1/runs/?project_id=" + projectID, ""},
		{"GET", "/v1/workflows/" + wfID + "/", ""},
		{"GET", "/v1/job-groups/" + groupID + "/", ""},
		{"GET", "/v1/environments/" + envID + "/", ""},
		{"GET", "/v1/roles", ""},
		{"GET", "/v1/members", ""},
		{"GET", "/v1/stats?project_id=" + projectID, ""},
		{"GET", "/health", ""},
		{"GET", "/health/ready", ""},
		{"POST", "/v1/jobs/" + jobID + "/trigger", `{"payload":{"cross":"module"}}`},
	}

	var counter atomic.Int64
	tgt := func(tgt *vegeta.Target) error {
		i := counter.Add(1) - 1
		ep := endpoints[i%int64(len(endpoints))]
		tgt.Method = ep.method
		tgt.URL = baseURL + ep.path
		tgt.Header = http.Header{
			"X-Internal-Secret": []string{testInternalSecret},
			"Content-Type":      []string{"application/json"},
		}
		if ep.body != "" {
			tgt.Body = []byte(ep.body)
		}
		return nil
	}

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "mixed-cross-module", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.98)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "mixed-cross-module", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "mixed-cross-module", tgt)
		assertSuccessRate(t, m, 0.85)
		assertNoServerErrors(t, m)
	})
}

func TestMixed_HealthEndpoints(t *testing.T) {
	mustClean(t)

	healthPaths := []string{"/health", "/health/ready"}

	var counter atomic.Int64
	tgt := func(tgt *vegeta.Target) error {
		i := counter.Add(1) - 1
		tgt.Method = "GET"
		tgt.URL = baseURL + healthPaths[i%int64(len(healthPaths))]
		tgt.Header = http.Header{
			"X-Internal-Secret": []string{testInternalSecret},
		}
		return nil
	}

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "health-endpoints", tgt, withRate(500))
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "health-endpoints", tgt)
		assertSuccessRate(t, m, 0.99)
		assertNoServerErrors(t, m)
	})
}

func TestMixed_Stats(t *testing.T) {
	mustClean(t)
	projectID := "proj-mix-stats-" + newID()
	jobID := seedJob(t, projectID)
	seedManyRuns(t, jobID, 30)

	tgt := newTargeter("GET", "/v1/stats?project_id="+projectID, nil)

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "stats", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "stats", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "stats", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

func TestMixed_Secrets(t *testing.T) {
	mustClean(t)
	projectID := "proj-mix-sec-" + newID()

	tgt := newTargeter("POST", "/v1/secrets/", func() []byte {
		return []byte(fmt.Sprintf(
			`{"project_id":"%s","secret_key":"SECRET_%s","value":"supersecret-%s"}`,
			projectID, newID(), newID(),
		))
	})

	t.Run("baseline-create", func(t *testing.T) {
		m := runBaseline(t, "create-secret", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})

	listTgt := newTargeter("GET", "/v1/secrets/?project_id="+projectID, nil)

	t.Run("baseline-list", func(t *testing.T) {
		m := runBaseline(t, "list-secrets", listTgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress-list", func(t *testing.T) {
		m := runStress(t, "list-secrets", listTgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
}

func TestMixed_Reference(t *testing.T) {
	mustClean(t)

	var counter atomic.Int64
	tgt := func(tgt *vegeta.Target) error {
		i := counter.Add(1) - 1
		tgt.Method = "GET"
		tgt.Header = http.Header{
			"X-Internal-Secret": []string{testInternalSecret},
		}
		if i%2 == 0 {
			tgt.URL = baseURL + "/reference"
		} else {
			tgt.URL = baseURL + "/reference/openapi.json"
		}
		return nil
	}

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "reference-endpoints", tgt)
		assertSuccessRate(t, m, 0.99)
		assertNoServerErrors(t, m)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "reference-endpoints", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
}
