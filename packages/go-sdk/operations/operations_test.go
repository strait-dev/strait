package operations

import (
	"context"
	"testing"
)

type mockRequester struct {
	lastMethod string
	lastPath   string
}

func (m *mockRequester) DoRequest(_ context.Context, method, path string, _ map[string]string, _ map[string]string, _ any, _ any) error {
	m.lastMethod = method
	m.lastPath = path
	return nil
}

func TestPathParams(t *testing.T) {
	result := PathParams("/v1/jobs/{jobID}/versions/{versionID}", map[string]string{
		"jobID":     "job_1",
		"versionID": "v_2",
	})
	if result != "/v1/jobs/job_1/versions/v_2" {
		t.Errorf("expected substituted path, got %q", result)
	}
}

func TestHealthService(t *testing.T) {
	tests := []struct {
		name   string
		call   func(*HealthService) error
		method string
		path   string
	}{
		{"List", func(s *HealthService) error { _, err := s.List(context.Background()); return err }, "GET", "/health"},
		{"GetReady", func(s *HealthService) error { _, err := s.GetReady(context.Background()); return err }, "GET", "/health/ready"},
		{"ListMetrics", func(s *HealthService) error { _, err := s.ListMetrics(context.Background()); return err }, "GET", "/metrics"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &mockRequester{}
			s := NewHealthService(m)
			_ = tt.call(s)
			if m.lastMethod != tt.method {
				t.Errorf("expected method %q, got %q", tt.method, m.lastMethod)
			}
			if m.lastPath != tt.path {
				t.Errorf("expected path %q, got %q", tt.path, m.lastPath)
			}
		})
	}
}

func TestJobsService(t *testing.T) {
	tests := []struct {
		name   string
		call   func(*JobsService) error
		method string
		path   string
	}{
		{"List", func(s *JobsService) error { _, err := s.List(context.Background(), nil); return err }, "GET", "/v1/jobs"},
		{"Create", func(s *JobsService) error { _, err := s.Create(context.Background(), nil); return err }, "POST", "/v1/jobs"},
		{"Get", func(s *JobsService) error { _, err := s.Get(context.Background(), "j1"); return err }, "GET", "/v1/jobs/j1"},
		{"Update", func(s *JobsService) error { _, err := s.Update(context.Background(), "j1", nil); return err }, "PATCH", "/v1/jobs/j1"},
		{"Delete", func(s *JobsService) error { return s.Delete(context.Background(), "j1") }, "DELETE", "/v1/jobs/j1"},
		{"Trigger", func(s *JobsService) error { _, err := s.Trigger(context.Background(), "j1", nil); return err }, "POST", "/v1/jobs/j1/trigger"},
		{"BulkTrigger", func(s *JobsService) error { _, err := s.BulkTrigger(context.Background(), "j1", nil); return err }, "POST", "/v1/jobs/j1/trigger/bulk"},
		{"Batch", func(s *JobsService) error { _, err := s.Batch(context.Background(), nil); return err }, "POST", "/v1/jobs/batch"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &mockRequester{}
			s := NewJobsService(m)
			_ = tt.call(s)
			if m.lastMethod != tt.method {
				t.Errorf("expected method %q, got %q", tt.method, m.lastMethod)
			}
			if m.lastPath != tt.path {
				t.Errorf("expected path %q, got %q", tt.path, m.lastPath)
			}
		})
	}
}

func TestRunsService(t *testing.T) {
	tests := []struct {
		name   string
		call   func(*RunsService) error
		method string
		path   string
	}{
		{"List", func(s *RunsService) error { _, err := s.List(context.Background(), nil); return err }, "GET", "/v1/runs"},
		{"Get", func(s *RunsService) error { _, err := s.Get(context.Background(), "r1"); return err }, "GET", "/v1/runs/r1"},
		{"Delete", func(s *RunsService) error { return s.Delete(context.Background(), "r1") }, "DELETE", "/v1/runs/r1"},
		{"Replay", func(s *RunsService) error { _, err := s.Replay(context.Background(), "r1", nil); return err }, "POST", "/v1/runs/r1/replay"},
		{"BulkCancel", func(s *RunsService) error { _, err := s.BulkCancel(context.Background(), nil); return err }, "POST", "/v1/runs/bulk-cancel"},
		{"GetDlq", func(s *RunsService) error { _, err := s.GetDlq(context.Background(), nil); return err }, "GET", "/v1/runs/dlq"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &mockRequester{}
			s := NewRunsService(m)
			_ = tt.call(s)
			if m.lastMethod != tt.method {
				t.Errorf("expected method %q, got %q", tt.method, m.lastMethod)
			}
			if m.lastPath != tt.path {
				t.Errorf("expected path %q, got %q", tt.path, m.lastPath)
			}
		})
	}
}

func TestWorkflowsService(t *testing.T) {
	m := &mockRequester{}
	s := NewWorkflowsService(m)

	_, _ = s.List(context.Background(), nil)
	if m.lastPath != "/v1/workflows" {
		t.Errorf("expected /v1/workflows, got %q", m.lastPath)
	}

	_, _ = s.Trigger(context.Background(), "wf1", nil)
	if m.lastPath != "/v1/workflows/wf1/trigger" {
		t.Errorf("expected trigger path, got %q", m.lastPath)
	}
}

func TestWorkflowRunsService(t *testing.T) {
	m := &mockRequester{}
	s := NewWorkflowRunsService(m)

	_, _ = s.ApproveStep(context.Background(), "wr1", "step1", nil)
	if m.lastPath != "/v1/workflow-runs/wr1/steps/step1/approve" {
		t.Errorf("expected approve step path, got %q", m.lastPath)
	}

	_, _ = s.Pause(context.Background(), "wr1")
	if m.lastPath != "/v1/workflow-runs/wr1/pause" {
		t.Errorf("expected pause path, got %q", m.lastPath)
	}
}

func TestDeploymentsService(t *testing.T) {
	m := &mockRequester{}
	s := NewDeploymentsService(m)

	_, _ = s.Finalize(context.Background(), "dep1", nil)
	if m.lastPath != "/v1/deployments/dep1/finalize" {
		t.Errorf("expected finalize path, got %q", m.lastPath)
	}
}

func TestSDKRunsService(t *testing.T) {
	m := &mockRequester{}
	s := NewSDKRunsService(m)

	_, _ = s.CompleteRun(context.Background(), "r1", nil)
	if m.lastPath != "/sdk/v1/runs/r1/complete" {
		t.Errorf("expected complete path, got %q", m.lastPath)
	}

	_, _ = s.HeartbeatRun(context.Background(), "r1")
	if m.lastPath != "/sdk/v1/runs/r1/heartbeat" {
		t.Errorf("expected heartbeat path, got %q", m.lastPath)
	}
}
