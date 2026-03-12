package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"sort"
	"strings"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/go-chi/chi/v5"
	"github.com/samber/lo"
)

type approveWorkflowStepRequest struct {
	Approver string `json:"approver" validate:"required"`
}

type skipStepRequest struct {
	Reason string `json:"reason,omitempty"`
}

type forceCompleteStepRequest struct {
	Result json.RawMessage `json:"result,omitempty"`
}

func (s *Server) handleListWorkflowRuns(w http.ResponseWriter, r *http.Request) {
	workflowID := chi.URLParam(r, "workflowID")

	limit, cursor, err := parsePaginationParams(r)
	if err != nil {
		respondError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	runs, err := s.store.ListWorkflowRuns(r.Context(), workflowID, limit+1, cursor)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to list workflow runs")
		return
	}

	respondJSON(w, http.StatusOK, paginatedResult(runs, limit, func(run domain.WorkflowRun) string {
		return run.CreatedAt.Format(time.RFC3339Nano)
	}))
}

func (s *Server) handleListWorkflowRunsByProject(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	projectID := query.Get("project_id")
	if projectID == "" {
		respondError(w, r, http.StatusBadRequest, "project_id is required")
		return
	}

	tagKey := query.Get("tag_key")
	tagValue := query.Get("tag_value")
	if tagValue != "" && tagKey == "" {
		respondError(w, r, http.StatusBadRequest, "tag_key is required when tag_value is provided")
		return
	}

	var status *domain.WorkflowRunStatus
	if statusRaw := query.Get("status"); statusRaw != "" {
		parsed := domain.WorkflowRunStatus(statusRaw)
		if !parsed.IsValid() {
			respondError(w, r, http.StatusBadRequest, "status is invalid")
			return
		}
		status = &parsed
	}

	limit, cursor, err := parsePaginationParams(r)
	if err != nil {
		respondError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	var runs []domain.WorkflowRun
	if tagKey != "" {
		runs, err = s.store.ListWorkflowRunsByTag(r.Context(), projectID, tagKey, tagValue, limit+1, cursor)
	} else {
		runs, err = s.store.ListWorkflowRunsByProject(r.Context(), projectID, status, limit+1, cursor)
	}
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to list workflow runs")
		return
	}

	respondJSON(w, http.StatusOK, paginatedResult(runs, limit, func(run domain.WorkflowRun) string {
		return run.CreatedAt.Format(time.RFC3339Nano)
	}))
}

func (s *Server) handleGetWorkflowRun(w http.ResponseWriter, r *http.Request) {
	workflowRunID := chi.URLParam(r, "workflowRunID")
	run, err := s.store.GetWorkflowRun(r.Context(), workflowRunID)
	if err != nil {
		if errors.Is(err, store.ErrWorkflowRunNotFound) {
			respondError(w, r, http.StatusNotFound, "workflow run not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to get workflow run")
		return
	}

	respondJSON(w, http.StatusOK, run)
}

func (s *Server) handleCancelWorkflowRun(w http.ResponseWriter, r *http.Request) {
	workflowRunID := chi.URLParam(r, "workflowRunID")
	run, err := s.store.GetWorkflowRun(r.Context(), workflowRunID)
	if err != nil {
		if errors.Is(err, store.ErrWorkflowRunNotFound) {
			respondError(w, r, http.StatusNotFound, "workflow run not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to get workflow run")
		return
	}

	if run.Status.IsTerminal() {
		respondError(w, r, http.StatusBadRequest, "workflow run already in terminal state")
		return
	}

	if err := s.store.UpdateWorkflowRunStatus(r.Context(), run.ID, run.Status, domain.WfStatusCanceled, map[string]any{
		"finished_at": time.Now(),
		"error":       "canceled by user",
	}); err != nil {
		respondError(w, r, http.StatusConflict, "failed to cancel workflow run")
		return
	}
	now := time.Now()
	reason := "workflow canceled by user"

	// Bulk-cancel all non-terminal step runs in one UPDATE.
	if _, err := s.store.CancelNonTerminalStepRuns(r.Context(), run.ID, now, reason); err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to cancel workflow step runs")
		return
	}

	// Bulk-cancel all non-terminal job runs linked to this workflow run.
	if _, err := s.store.CancelJobRunsByWorkflowRun(r.Context(), run.ID, now, reason); err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to cancel workflow job runs")
		return
	}

	// Cancel any pending event triggers for this workflow (non-fatal).
	if _, triggerErr := s.store.CancelEventTriggersByWorkflowRun(r.Context(), run.ID); triggerErr != nil {
		slog.Warn("failed to cancel event triggers for workflow (non-fatal)", "workflow_run_id", run.ID, "error", triggerErr)
	}

	updatedRun, err := s.store.GetWorkflowRun(r.Context(), run.ID)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to get updated workflow run")
		return
	}
	s.publishWorkflowRunHook(r.Context(), updatedRun, run.Status, domain.WfStatusCanceled, "cancel")

	respondJSON(w, http.StatusOK, updatedRun)
}

func (s *Server) handlePauseWorkflowRun(w http.ResponseWriter, r *http.Request) {
	workflowRunID := chi.URLParam(r, "workflowRunID")
	run, err := s.store.GetWorkflowRun(r.Context(), workflowRunID)
	if err != nil {
		if errors.Is(err, store.ErrWorkflowRunNotFound) {
			respondError(w, r, http.StatusNotFound, "workflow run not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to get workflow run")
		return
	}
	if run.Status.IsTerminal() {
		respondError(w, r, http.StatusBadRequest, "workflow run already in terminal state")
		return
	}
	if run.Status == domain.WfStatusPaused {
		respondJSON(w, http.StatusOK, run)
		return
	}
	if run.Status != domain.WfStatusRunning {
		respondError(w, r, http.StatusBadRequest, "workflow run can only be paused from running state")
		return
	}

	if err := s.store.UpdateWorkflowRunStatus(r.Context(), run.ID, domain.WfStatusRunning, domain.WfStatusPaused, nil); err != nil {
		respondError(w, r, http.StatusConflict, "failed to pause workflow run")
		return
	}

	updatedRun, err := s.store.GetWorkflowRun(r.Context(), run.ID)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to get updated workflow run")
		return
	}
	s.publishWorkflowRunHook(r.Context(), updatedRun, run.Status, domain.WfStatusPaused, "pause")
	respondJSON(w, http.StatusOK, updatedRun)
}

func (s *Server) handleResumeWorkflowRun(w http.ResponseWriter, r *http.Request) {
	if s.workflowCallback == nil {
		respondError(w, r, http.StatusServiceUnavailable, "workflow callback unavailable")
		return
	}

	workflowRunID := chi.URLParam(r, "workflowRunID")
	run, err := s.store.GetWorkflowRun(r.Context(), workflowRunID)
	if err != nil {
		if errors.Is(err, store.ErrWorkflowRunNotFound) {
			respondError(w, r, http.StatusNotFound, "workflow run not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to get workflow run")
		return
	}
	if run.Status != domain.WfStatusPaused {
		respondError(w, r, http.StatusBadRequest, "workflow run is not paused")
		return
	}

	if err := s.workflowCallback.ResumeWorkflowRun(r.Context(), workflowRunID); err != nil {
		respondError(w, r, http.StatusConflict, err.Error())
		return
	}

	updatedRun, err := s.store.GetWorkflowRun(r.Context(), run.ID)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to get updated workflow run")
		return
	}
	s.publishWorkflowRunHook(r.Context(), updatedRun, run.Status, domain.WfStatusRunning, "resume")
	respondJSON(w, http.StatusOK, updatedRun)
}

func (s *Server) handleGetWorkflowRunLabels(w http.ResponseWriter, r *http.Request) {
	workflowRunID := chi.URLParam(r, "workflowRunID")
	labels, err := s.store.ListWorkflowRunLabels(r.Context(), workflowRunID)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to list workflow run labels")
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"labels": labels})
}

func (s *Server) handleListWorkflowStepRuns(w http.ResponseWriter, r *http.Request) {
	workflowRunID := chi.URLParam(r, "workflowRunID")

	limit, cursor, err := parsePaginationParams(r)
	if err != nil {
		respondError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	stepRuns, err := s.store.ListStepRunsByWorkflowRun(r.Context(), workflowRunID, limit+1, cursor)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to list workflow step runs")
		return
	}

	respondJSON(w, http.StatusOK, paginatedResult(stepRuns, limit, func(sr domain.WorkflowStepRun) string {
		return sr.CreatedAt.Format(time.RFC3339Nano)
	}))
}

func (s *Server) handleApproveWorkflowStep(w http.ResponseWriter, r *http.Request) {
	if s.workflowCallback == nil {
		respondError(w, r, http.StatusServiceUnavailable, "workflow callback unavailable")
		return
	}

	workflowRunID := chi.URLParam(r, "workflowRunID")
	stepRef := chi.URLParam(r, "stepRef")
	beforeRun, beforeErr := s.store.GetWorkflowRun(r.Context(), workflowRunID)
	if beforeErr != nil {
		slog.Warn("failed to get workflow run before approve step", "workflow_run_id", workflowRunID, "error", beforeErr)
	}

	var req approveWorkflowStepRequest
	if err := s.decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	if !s.validateRequest(w, r, &req) {
		return
	}

	if err := s.workflowCallback.ApproveStep(r.Context(), workflowRunID, stepRef, req.Approver); err != nil {
		respondError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	stepRun, err := s.store.GetStepRunByWorkflowRunAndRef(r.Context(), workflowRunID, stepRef)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to fetch workflow step run")
		return
	}
	approval, err := s.store.GetWorkflowStepApprovalByStepRunID(r.Context(), stepRun.ID)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to fetch workflow step approval")
		return
	}

	afterRun, afterErr := s.store.GetWorkflowRun(r.Context(), workflowRunID)
	if afterErr != nil {
		slog.Warn("failed to get workflow run after approve step", "workflow_run_id", workflowRunID, "error", afterErr)
	}
	if beforeErr == nil && afterErr == nil && beforeRun != nil && afterRun != nil && beforeRun.Status != afterRun.Status {
		s.publishWorkflowRunHook(r.Context(), afterRun, beforeRun.Status, afterRun.Status, "approve_step")
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"step_run": stepRun,
		"approval": approval,
	})
}

func (s *Server) handleSkipWorkflowStep(w http.ResponseWriter, r *http.Request) {
	if s.workflowCallback == nil {
		respondError(w, r, http.StatusServiceUnavailable, "workflow callback unavailable")
		return
	}

	workflowRunID := chi.URLParam(r, "workflowRunID")
	stepRef := chi.URLParam(r, "stepRef")
	beforeRun, beforeErr := s.store.GetWorkflowRun(r.Context(), workflowRunID)
	if beforeErr != nil {
		slog.Warn("failed to get workflow run before skip step", "workflow_run_id", workflowRunID, "error", beforeErr)
	}

	var req skipStepRequest
	if err := s.decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := s.workflowCallback.SkipStep(r.Context(), workflowRunID, stepRef, req.Reason); err != nil {
		respondError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	stepRun, err := s.store.GetStepRunByWorkflowRunAndRef(r.Context(), workflowRunID, stepRef)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to fetch workflow step run")
		return
	}

	afterRun, afterErr := s.store.GetWorkflowRun(r.Context(), workflowRunID)
	if afterErr != nil {
		slog.Warn("failed to get workflow run after skip step", "workflow_run_id", workflowRunID, "error", afterErr)
	}
	if beforeErr == nil && afterErr == nil && beforeRun != nil && afterRun != nil && beforeRun.Status != afterRun.Status {
		s.publishWorkflowRunHook(r.Context(), afterRun, beforeRun.Status, afterRun.Status, "skip_step")
	}

	respondJSON(w, http.StatusOK, map[string]any{"step_run": stepRun})
}

func (s *Server) handleForceCompleteWorkflowStep(w http.ResponseWriter, r *http.Request) {
	if s.workflowCallback == nil {
		respondError(w, r, http.StatusServiceUnavailable, "workflow callback unavailable")
		return
	}

	workflowRunID := chi.URLParam(r, "workflowRunID")
	stepRef := chi.URLParam(r, "stepRef")
	beforeRun, beforeErr := s.store.GetWorkflowRun(r.Context(), workflowRunID)
	if beforeErr != nil {
		slog.Warn("failed to get workflow run before force-complete step", "workflow_run_id", workflowRunID, "error", beforeErr)
	}

	var req forceCompleteStepRequest
	if err := s.decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := s.workflowCallback.ForceCompleteStep(r.Context(), workflowRunID, stepRef, req.Result); err != nil {
		respondError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	stepRun, err := s.store.GetStepRunByWorkflowRunAndRef(r.Context(), workflowRunID, stepRef)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to fetch workflow step run")
		return
	}

	afterRun, afterErr := s.store.GetWorkflowRun(r.Context(), workflowRunID)
	if afterErr != nil {
		slog.Warn("failed to get workflow run after force-complete step", "workflow_run_id", workflowRunID, "error", afterErr)
	}
	if beforeErr == nil && afterErr == nil && beforeRun != nil && afterRun != nil && beforeRun.Status != afterRun.Status {
		s.publishWorkflowRunHook(r.Context(), afterRun, beforeRun.Status, afterRun.Status, "force_complete_step")
	}

	respondJSON(w, http.StatusOK, map[string]any{"step_run": stepRun})
}

func (s *Server) handleRetryWorkflowRun(w http.ResponseWriter, r *http.Request) {
	if s.workflowEngine == nil {
		respondError(w, r, http.StatusServiceUnavailable, "workflow engine unavailable")
		return
	}

	workflowRunID := chi.URLParam(r, "workflowRunID")
	run, err := s.store.GetWorkflowRun(r.Context(), workflowRunID)
	if err != nil {
		if errors.Is(err, store.ErrWorkflowRunNotFound) {
			respondError(w, r, http.StatusNotFound, "workflow run not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to get workflow run")
		return
	}

	if !run.Status.IsTerminal() {
		respondError(w, r, http.StatusBadRequest, "can only retry a workflow run in terminal state")
		return
	}

	newRun, err := s.workflowEngine.RetryWorkflowRun(r.Context(), workflowRunID)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, fmt.Sprintf("failed to retry workflow run: %v", err))
		return
	}

	s.publishWorkflowRunHook(r.Context(), newRun, domain.WfStatusPending, newRun.Status, "retry")

	respondJSON(w, http.StatusCreated, newRun)
}

type workflowRunGraphNode struct {
	StepRef    string                  `json:"step_ref"`
	Type       domain.WorkflowStepType `json:"type"`
	Status     domain.StepRunStatus    `json:"status"`
	DependsOn  []string                `json:"depends_on,omitempty"`
	Attempt    int                     `json:"attempt"`
	StartedAt  *time.Time              `json:"started_at,omitempty"`
	FinishedAt *time.Time              `json:"finished_at,omitempty"`
	DurationMS int64                   `json:"duration_ms,omitempty"`
}

func (s *Server) handleGetWorkflowRunGraph(w http.ResponseWriter, r *http.Request) {
	workflowRunID := chi.URLParam(r, "workflowRunID")
	run, err := s.store.GetWorkflowRun(r.Context(), workflowRunID)
	if err != nil {
		respondError(w, r, http.StatusNotFound, "workflow run not found")
		return
	}
	steps, err := s.store.ListStepsByWorkflowVersion(r.Context(), run.WorkflowID, run.WorkflowVersion)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to list workflow steps")
		return
	}
	stepRuns, err := s.store.ListStepRunsByWorkflowRun(r.Context(), workflowRunID, 10000, nil)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to list step runs")
		return
	}

	runByRef := make(map[string]domain.WorkflowStepRun, len(stepRuns))
	for _, sr := range stepRuns {
		runByRef[sr.StepRef] = sr
	}

	now := time.Now()
	nodes := make([]workflowRunGraphNode, 0, len(steps))
	edges := make([]map[string]string, 0)
	roots := make([]string, 0)
	runnable := make([]string, 0)
	for _, st := range steps {
		sr := runByRef[st.StepRef]
		node := workflowRunGraphNode{StepRef: st.StepRef, Type: st.StepType, Status: sr.Status, DependsOn: st.DependsOn, Attempt: sr.Attempt}
		if sr.StartedAt != nil {
			node.StartedAt = sr.StartedAt
			if sr.FinishedAt != nil {
				node.DurationMS = sr.FinishedAt.Sub(*sr.StartedAt).Milliseconds()
			} else {
				node.DurationMS = now.Sub(*sr.StartedAt).Milliseconds()
			}
		}
		if sr.FinishedAt != nil {
			node.FinishedAt = sr.FinishedAt
		}
		nodes = append(nodes, node)
		if len(st.DependsOn) == 0 {
			roots = append(roots, st.StepRef)
		}
		if sr.Status == domain.StepPending || sr.Status == domain.StepWaiting {
			if sr.DepsRequired == sr.DepsCompleted {
				runnable = append(runnable, st.StepRef)
			}
		}
		for _, dep := range st.DependsOn {
			edges = append(edges, map[string]string{"from": dep, "to": st.StepRef})
		}
	}
	sort.Strings(roots)
	sort.Strings(runnable)
	criticalPath, estimatedDurationMS, estimatedRemainingMS := estimateWorkflowCriticalPath(steps, runByRef, now)

	respondJSON(w, http.StatusOK, map[string]any{
		"workflow_run_id":            run.ID,
		"workflow_id":                run.WorkflowID,
		"version":                    run.WorkflowVersion,
		"nodes":                      nodes,
		"edges":                      edges,
		"roots":                      roots,
		"runnable":                   runnable,
		"critical_path":              criticalPath,
		"critical_path_estimate_ms":  estimatedDurationMS,
		"critical_path_remaining_ms": estimatedRemainingMS,
	})
}

func estimateWorkflowCriticalPath(steps []domain.WorkflowStep, runByRef map[string]domain.WorkflowStepRun, now time.Time) ([]string, int64, int64) {
	if len(steps) == 0 {
		return nil, 0, 0
	}

	stepByRef := lo.KeyBy(steps, func(step domain.WorkflowStep) string { return step.StepRef })
	indegree := make(map[string]int, len(steps))
	children := make(map[string][]string, len(steps))
	for _, step := range steps {
		indegree[step.StepRef] = 0
		children[step.StepRef] = []string{}
	}
	for _, step := range steps {
		for _, dep := range step.DependsOn {
			if _, ok := indegree[dep]; !ok {
				continue
			}
			children[dep] = append(children[dep], step.StepRef)
			indegree[step.StepRef]++
		}
	}

	queue := make([]string, 0, len(steps))
	for ref, degree := range indegree {
		if degree == 0 {
			queue = append(queue, ref)
		}
	}
	sort.Strings(queue)

	prev := make(map[string]string, len(steps))
	longestByRef := make(map[string]int64, len(steps))
	totalEstimateByRef := make(map[string]int64, len(steps))
	remainingByRef := make(map[string]int64, len(steps))
	for len(queue) > 0 {
		ref := queue[0]
		queue = queue[1:]

		step := stepByRef[ref]
		stepRun := runByRef[ref]
		totalEstimateMS, remainingMS := estimateStepTiming(step, stepRun, now)
		totalEstimateByRef[ref] = totalEstimateMS
		remainingByRef[ref] = remainingMS

		bestParentRef := ""
		bestParentDistance := int64(0)
		for _, dep := range step.DependsOn {
			distance, ok := longestByRef[dep]
			if !ok {
				continue
			}
			if distance > bestParentDistance {
				bestParentDistance = distance
				bestParentRef = dep
			}
		}
		prev[ref] = bestParentRef
		longestByRef[ref] = bestParentDistance + totalEstimateMS

		for _, child := range children[ref] {
			indegree[child]--
			if indegree[child] == 0 {
				queue = append(queue, child)
			}
		}
		sort.Strings(queue)
	}

	pathEnd := ""
	pathDistance := int64(0)
	for ref, distance := range longestByRef {
		if distance > pathDistance || (distance == pathDistance && (pathEnd == "" || ref < pathEnd)) {
			pathEnd = ref
			pathDistance = distance
		}
	}

	path := make([]string, 0, len(steps))
	for ref := pathEnd; ref != ""; ref = prev[ref] {
		path = append(path, ref)
	}
	slices.Reverse(path)

	remainingMS := int64(0)
	for _, ref := range path {
		remainingMS += remainingByRef[ref]
	}
	return path, pathDistance, remainingMS
}

func estimateStepTiming(step domain.WorkflowStep, stepRun domain.WorkflowStepRun, now time.Time) (int64, int64) {
	totalEstimateMS := int64(0)
	if step.TimeoutSecsOverride > 0 {
		totalEstimateMS = int64(step.TimeoutSecsOverride) * 1000
	}

	spentMS := int64(0)
	if stepRun.StartedAt != nil {
		spentMS = now.Sub(*stepRun.StartedAt).Milliseconds()
		spentMS = max(spentMS, 0)
	}
	if stepRun.StartedAt != nil && stepRun.FinishedAt != nil {
		actualMS := stepRun.FinishedAt.Sub(*stepRun.StartedAt).Milliseconds()
		actualMS = max(actualMS, 0)
		spentMS = actualMS
		totalEstimateMS = actualMS
	}
	if totalEstimateMS == 0 {
		totalEstimateMS = spentMS
	}
	if stepRun.Status.IsTerminal() {
		return totalEstimateMS, totalEstimateMS
	}
	if spentMS > totalEstimateMS {
		spentMS = totalEstimateMS
	}
	if totalEstimateMS == 0 {
		return 0, 0
	}
	return totalEstimateMS, totalEstimateMS - spentMS
}

func (s *Server) handleGetWorkflowRunExplain(w http.ResponseWriter, r *http.Request) {
	workflowRunID := chi.URLParam(r, "workflowRunID")
	limit, cursor, err := parsePaginationParams(r)
	if err != nil {
		respondError(w, r, http.StatusBadRequest, err.Error())
		return
	}
	decisions, err := s.store.ListWorkflowStepDecisions(r.Context(), workflowRunID, r.URL.Query().Get("step_ref"), r.URL.Query().Get("decision_type"), limit+1, cursor)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to list workflow decisions")
		return
	}
	respondJSON(w, http.StatusOK, paginatedResult(decisions, limit, func(d domain.WorkflowStepDecision) string {
		return d.CreatedAt.Format(time.RFC3339Nano)
	}))
}

func (s *Server) handleRetryWorkflowStep(w http.ResponseWriter, r *http.Request) {
	if s.workflowCallback == nil {
		respondError(w, r, http.StatusServiceUnavailable, "workflow callback unavailable")
		return
	}
	workflowRunID := chi.URLParam(r, "workflowRunID")
	stepRef := chi.URLParam(r, "stepRef")

	run, err := s.store.GetWorkflowRun(r.Context(), workflowRunID)
	if err != nil {
		respondError(w, r, http.StatusNotFound, "workflow run not found")
		return
	}

	stepRun, err := s.store.GetStepRunByWorkflowRunAndRef(r.Context(), workflowRunID, stepRef)
	if err != nil || stepRun == nil {
		respondError(w, r, http.StatusNotFound, "workflow step run not found")
		return
	}
	if !stepRun.Status.IsTerminal() {
		respondError(w, r, http.StatusBadRequest, "step run must be terminal to retry")
		return
	}

	// If the workflow run is terminal, transition it back to running so
	// ResumeWorkflowRun can proceed. If it is paused, ResumeWorkflowRun
	// handles the transition internally.
	if run.Status.IsTerminal() {
		if err := s.store.UpdateWorkflowRunStatus(r.Context(), run.ID, run.Status, domain.WfStatusRunning, nil); err != nil {
			respondError(w, r, http.StatusConflict, "failed to reopen workflow run for retry")
			return
		}
	}

	if err := s.store.UpdateStepRunStatus(r.Context(), stepRun.ID, domain.StepPending, map[string]any{"started_at": nil, "finished_at": nil, "error": "", "output": nil, "event_key": nil}); err != nil {
		respondError(w, r, http.StatusConflict, "failed to reset step run")
		return
	}

	// Only call ResumeWorkflowRun if the run was paused (it handles pause->running).
	// If we already set it to running above, just schedule directly.
	if run.Status == domain.WfStatusPaused {
		if err := s.workflowCallback.ResumeWorkflowRun(r.Context(), workflowRunID); err != nil {
			respondError(w, r, http.StatusConflict, err.Error())
			return
		}
	}

	updated, _ := s.store.GetStepRunByWorkflowRunAndRef(r.Context(), workflowRunID, stepRef)
	respondJSON(w, http.StatusOK, map[string]any{"step_run": updated})
}

func (s *Server) handleReplayWorkflowSubtree(w http.ResponseWriter, r *http.Request) {
	if s.workflowCallback == nil {
		respondError(w, r, http.StatusServiceUnavailable, "workflow callback unavailable")
		return
	}
	workflowRunID := chi.URLParam(r, "workflowRunID")
	stepRef := chi.URLParam(r, "stepRef")
	run, err := s.store.GetWorkflowRun(r.Context(), workflowRunID)
	if err != nil {
		respondError(w, r, http.StatusNotFound, "workflow run not found")
		return
	}
	steps, err := s.store.ListStepsByWorkflowVersion(r.Context(), run.WorkflowID, run.WorkflowVersion)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to list workflow steps")
		return
	}
	children := map[string][]string{}
	exists := false
	for _, st := range steps {
		if st.StepRef == stepRef {
			exists = true
		}
		for _, dep := range st.DependsOn {
			children[dep] = append(children[dep], st.StepRef)
		}
	}
	if !exists {
		respondError(w, r, http.StatusNotFound, "step not found in workflow version")
		return
	}
	toReset := map[string]struct{}{stepRef: {}}
	queue := []string{stepRef}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, ch := range children[cur] {
			if _, ok := toReset[ch]; ok {
				continue
			}
			toReset[ch] = struct{}{}
			queue = append(queue, ch)
		}
	}
	stepRuns, err := s.store.ListStepRunsByWorkflowRun(r.Context(), workflowRunID, 10000, nil)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to list workflow step runs")
		return
	}
	var resetErrs []string
	reset := 0
	for _, sr := range stepRuns {
		if _, ok := toReset[sr.StepRef]; !ok {
			continue
		}
		if err := s.store.UpdateStepRunStatus(r.Context(), sr.ID, domain.StepPending, map[string]any{"started_at": nil, "finished_at": nil, "error": "", "output": nil, "event_key": nil}); err != nil {
			resetErrs = append(resetErrs, fmt.Sprintf("%s: %v", sr.StepRef, err))
			continue
		}
		reset++
	}
	if len(resetErrs) > 0 {
		respondError(w, r, http.StatusConflict, fmt.Sprintf("failed to reset %d step(s): %s", len(resetErrs), strings.Join(resetErrs, "; ")))
		return
	}
	if err := s.workflowCallback.ResumeWorkflowRun(r.Context(), workflowRunID); err != nil {
		respondError(w, r, http.StatusConflict, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"reset_steps": reset})
}
