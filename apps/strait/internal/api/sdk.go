package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type contextKey string

const ctxRunIDKey contextKey = "run_id"
const ctxRunAttemptKey contextKey = "run_attempt"
const ctxSDKVersionKey contextKey = "sdk_version"
const ctxSDKCapabilitiesKey contextKey = "sdk_capabilities"

type SDKCapabilities struct {
	Progress   bool
	Checkpoint bool
}

type runTokenClaims struct {
	Attempt      int    `json:"attempt,omitempty"`
	AssignmentID string `json:"assignment_id,omitempty"`
	jwt.RegisteredClaims
}

type runTokenStateGetter interface {
	GetRunTokenState(context.Context, string) (domain.RunStatus, int, string, error)
}

// errRunTokenStateUnsupported is returned when the configured store does not
// implement runTokenStateGetter. Production stores always do; this sentinel
// surfaces a misconfigured test fake so runTokenAuth can fail closed (401)
// rather than fall back to attempt=0 and silently bypass the staleness check.
var errRunTokenStateUnsupported = errors.New("store does not support run token state lookup")

type activeRunMutationStore interface {
	EnsureRunActiveForAttempt(context.Context, string, int) error
	InsertEventForActiveRun(context.Context, *domain.RunEvent, int) error
	UpdateRunMetadataForActiveRun(context.Context, string, map[string]string, int) error
	UpdateHeartbeatForActiveRun(context.Context, string, int) error
	CreateRunCheckpointForActiveRun(context.Context, *domain.RunCheckpoint, int) error
	UpsertRunStateForActiveRun(context.Context, *domain.RunState, int) error
	GetRunStateForActiveRun(context.Context, string, string, int) (*domain.RunState, error)
	ListRunStateForActiveRun(context.Context, string, int) ([]domain.RunState, error)
	DeleteRunStateForActiveRun(context.Context, string, string, int) error
	UpsertRunOutputForActiveRun(context.Context, *domain.RunOutput, int) error
	UpsertJobMemoryWithQuotaForActiveRun(context.Context, string, *domain.JobMemory, int, int, int) error
	GetJobMemoryForActiveRun(context.Context, string, string, string, int) (*domain.JobMemory, error)
	ListJobMemoryForActiveRun(context.Context, string, string, int) ([]domain.JobMemory, error)
	DeleteJobMemoryForActiveRun(context.Context, string, string, string, int) error
	CreateRunResourceSnapshotForActiveRun(context.Context, *domain.RunResourceSnapshot, int) error
	UpdateRunStatusForActiveRun(context.Context, string, domain.RunStatus, domain.RunStatus, map[string]any, int) error
}

func runTokenAttemptFromContext(ctx context.Context) int {
	attempt, _ := ctx.Value(ctxRunAttemptKey).(int)
	return attempt
}

func (s *Server) guardedSDKMutationError(ctx context.Context, err error) error {
	if errors.Is(err, store.ErrRunConflict) {
		if revalidateErr := s.revalidateRunTokenState(ctx); revalidateErr != nil {
			return revalidateErr
		}
		return huma.Error409Conflict("run is not active for this SDK token")
	}
	if errors.Is(err, store.ErrRunNotFound) {
		return huma.Error404NotFound("run not found")
	}
	return nil
}

func (s *Server) ensureSDKRunActive(ctx context.Context, runID string) error {
	if guardedStore, ok := s.store.(activeRunMutationStore); ok {
		if err := guardedStore.EnsureRunActiveForAttempt(ctx, runID, runTokenAttemptFromContext(ctx)); err != nil {
			if sdkErr := s.guardedSDKMutationError(ctx, err); sdkErr != nil {
				return sdkErr
			}
			return huma.Error500InternalServerError("failed to verify run status")
		}
		return nil
	}
	return s.revalidateRunTokenState(ctx)
}

func sdkVersionFromContext(ctx context.Context) string {
	v, _ := ctx.Value(ctxSDKVersionKey).(string)
	return v
}

func sdkCapabilitiesFromContext(ctx context.Context) SDKCapabilities {
	v, ok := ctx.Value(ctxSDKCapabilitiesKey).(SDKCapabilities)
	if !ok {
		return SDKCapabilities{}
	}
	return v
}

func sdkCapabilitiesHeader(c SDKCapabilities) string {
	switch {
	case c.Progress && c.Checkpoint:
		return "progress,checkpoint"
	case c.Progress:
		return "progress"
	case c.Checkpoint:
		return "checkpoint"
	default:
		return "none"
	}
}

func resolveSDKCapabilities(version string) SDKCapabilities {
	version = strings.TrimSpace(version)
	if version == "" {
		return SDKCapabilities{}
	}
	if len(version) == 1 {
		return sdkCapabilitiesForOneDigitMajor(version[0])
	}
	if version[1] == '.' {
		return sdkCapabilitiesForOneDigitMajor(version[0])
	}
	majorRaw := version
	if idx := strings.IndexByte(majorRaw, '.'); idx >= 0 {
		majorRaw = majorRaw[:idx]
	}
	major, err := strconv.Atoi(majorRaw)
	if err != nil || major < 2 {
		return SDKCapabilities{}
	}
	return SDKCapabilities{Progress: true, Checkpoint: true}
}

func sdkCapabilitiesForOneDigitMajor(major byte) SDKCapabilities {
	if major >= '2' && major <= '9' {
		return SDKCapabilities{Progress: true, Checkpoint: true}
	}
	return SDKCapabilities{}
}

func applySDKResponseHeaders(ctx context.Context, w http.ResponseWriter) {
	version := sdkVersionFromContext(ctx)
	if version == "" {
		version = "legacy"
	}
	w.Header().Set("X-SDK-Version-Accepted", version)
	w.Header().Set("X-SDK-Capabilities", sdkCapabilitiesHeader(sdkCapabilitiesFromContext(ctx)))
}

func (s *Server) sdkResponseHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		applySDKResponseHeaders(r.Context(), w)
		next.ServeHTTP(w, r)
	})
}

func (s *Server) runTokenAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" || !strings.HasPrefix(auth, "Bearer ") {
			recordAuthDecision(r.Context(), "jwt", "failure")
			respondError(w, r, http.StatusUnauthorized, "missing or invalid authorization header")
			return
		}
		tokenString := strings.TrimPrefix(auth, "Bearer ")
		issuerPresent := runTokenIssuerPresent(tokenString)
		claims := &runTokenClaims{}
		// jwt.WithExpirationRequired rejects tokens that omit `exp`. Without
		// it the library silently treats a missing exp as valid forever, so
		// a forged or accidentally-non-expiring token would never time out.
		// Issuer is bound to domain.RunTokenIssuer so a token issued for a
		// different audience (e.g. an SSE token) cannot be replayed against
		// the SDK plane.
		token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (any, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return []byte(s.config.JWTSigningKey), nil
		}, jwt.WithExpirationRequired(), jwt.WithIssuer(domain.RunTokenIssuer))
		if err != nil || !token.Valid {
			recordAuthDecision(r.Context(), "jwt", "failure")
			respondError(w, r, http.StatusUnauthorized, "invalid run token")
			return
		}
		subject := claims.Subject
		if subject == "" {
			recordAuthDecision(r.Context(), "jwt", "failure")
			respondError(w, r, http.StatusUnauthorized, "missing run ID in token")
			return
		}
		if claims.Attempt <= 0 {
			recordAuthDecision(r.Context(), "jwt", "failure")
			respondError(w, r, http.StatusUnauthorized, "missing run attempt in token")
			return
		}
		urlRunID := chi.URLParam(r, "runID")
		if urlRunID != "" && subject != urlRunID {
			recordAuthDecision(r.Context(), "jwt", "failure")
			respondError(w, r, http.StatusForbidden, "token does not match run ID")
			return
		}
		// Reject run tokens for runs that have already reached a terminal
		// state. The token's exp typically outlives the run, so without this
		// guard a stolen token could keep writing logs/state/memory long
		// after the runtime is gone. We use the lightweight GetRunStatus
		// (single indexed lookup) so this stays cheap on hot SDK paths like
		// heartbeat. Read-only post-mortem fetches must use the regular API
		// key plane, not the run-token plane.
		status, attempt, projectID, statusErr := s.getRunTokenState(r.Context(), subject)
		if statusErr != nil {
			if errors.Is(statusErr, store.ErrRunNotFound) {
				recordAuthDecision(r.Context(), "jwt", "failure")
				respondError(w, r, http.StatusNotFound, "run not found")
				return
			}
			if errors.Is(statusErr, errRunTokenStateUnsupported) {
				recordAuthDecision(r.Context(), "jwt", "failure")
				respondError(w, r, http.StatusUnauthorized, "run token state lookup unavailable")
				return
			}
			recordAuthDecision(r.Context(), "jwt", "failure")
			respondError(w, r, http.StatusInternalServerError, "failed to verify run status")
			return
		}
		if status.IsTerminal() {
			recordAuthDecision(r.Context(), "jwt", "failure")
			s.emitRunTokenRejectedAudit(r.Context(), subject, projectID, "terminal_run", issuerPresent)
			respondError(w, r, http.StatusGone, "run has reached a terminal state")
			return
		}
		if attempt > 0 && attempt != claims.Attempt {
			recordAuthDecision(r.Context(), "jwt", "failure")
			s.emitRunTokenRejectedAudit(r.Context(), subject, projectID, "stale_attempt", issuerPresent)
			respondError(w, r, http.StatusUnauthorized, "run token attempt is stale")
			return
		}
		if claims.AssignmentID != "" {
			if err := s.verifyRunTokenAssignment(r.Context(), subject, projectID, claims.AssignmentID); err != nil {
				recordAuthDecision(r.Context(), "jwt", "failure")
				s.emitRunTokenRejectedAudit(r.Context(), subject, projectID, "assignment_mismatch", issuerPresent)
				respondError(w, r, http.StatusUnauthorized, err.Error())
				return
			}
		}
		recordAuthDecision(r.Context(), "jwt", "success")
		if claims.IssuedAt != nil {
			recordAuthTokenAge(r.Context(), "jwt", claims.IssuedAt.Time)
		}
		sdkVersion := strings.TrimSpace(r.Header.Get("X-SDK-Version"))
		sdkCaps := resolveSDKCapabilities(sdkVersion)
		ctx := context.WithValue(r.Context(), ctxRunIDKey, subject)
		ctx = context.WithValue(ctx, ctxRunAttemptKey, claims.Attempt)
		ctx = context.WithValue(ctx, ctxSDKVersionKey, sdkVersion)
		ctx = context.WithValue(ctx, ctxSDKCapabilitiesKey, sdkCaps)
		ctx = context.WithValue(ctx, ctxProjectIDKey, projectID)
		ctx = context.WithValue(ctx, ctxActorTypeKey, "run_token")
		ctx = context.WithValue(ctx, ctxActorIDKey, "run:"+subject)
		s.serveWithSentryScope(next, w, r.WithContext(ctx))
	})
}

func runTokenIssuerPresent(tokenString string) bool {
	claims := &runTokenClaims{}
	_, _, err := jwt.NewParser().ParseUnverified(tokenString, claims)
	return err == nil && claims.Issuer != ""
}

func (s *Server) emitRunTokenRejectedAudit(ctx context.Context, runID, projectID, reason string, issuerPresent bool) {
	if projectID == "" {
		return
	}
	ctx = context.WithValue(ctx, ctxProjectIDKey, projectID)
	s.emitAuditEventAsync(ctx, domain.AuditActionAuthRunTokenRejected, "run", runID, map[string]any{
		"reason":         reason,
		"run_id":         runID,
		"issuer_present": issuerPresent,
	})
}

func (s *Server) revalidateRunTokenState(ctx context.Context) error {
	runID, _ := ctx.Value(ctxRunIDKey).(string)
	if runID == "" {
		return nil
	}
	status, attempt, _, err := s.getRunTokenState(ctx, runID)
	if err != nil {
		if errors.Is(err, store.ErrRunNotFound) {
			return huma.Error404NotFound("run not found")
		}
		if errors.Is(err, errRunTokenStateUnsupported) {
			return nil
		}
		return huma.Error500InternalServerError("failed to verify run status")
	}
	if status.IsTerminal() {
		return huma.Error410Gone("run has reached a terminal state")
	}
	tokenAttempt, _ := ctx.Value(ctxRunAttemptKey).(int)
	if attempt > 0 && tokenAttempt > 0 && attempt != tokenAttempt {
		return huma.Error401Unauthorized("run token attempt is stale")
	}
	return nil
}

func (s *Server) getRunTokenState(ctx context.Context, runID string) (domain.RunStatus, int, string, error) {
	if getter, ok := s.store.(runTokenStateGetter); ok {
		return getter.GetRunTokenState(ctx, runID)
	}
	return "", 0, "", errRunTokenStateUnsupported
}

func (s *Server) verifyRunTokenAssignment(ctx context.Context, runID, projectID, assignmentID string) error {
	getter, ok := s.store.(interface {
		GetWorkerTask(context.Context, string) (*domain.WorkerTask, error)
	})
	if !ok {
		return errors.New("failed to verify run assignment")
	}
	task, err := getter.GetWorkerTask(ctx, assignmentID)
	if err != nil {
		return errors.New("run assignment not found")
	}
	if task == nil {
		return errors.New("run assignment not found")
	}
	if task.RunID != runID || task.ProjectID != projectID {
		return errors.New("run token assignment mismatch")
	}
	if !isActiveWorkerTaskAssignmentStatus(task.Status) {
		return errors.New("run assignment is no longer active")
	}
	return nil
}

func isActiveWorkerTaskAssignmentStatus(status domain.WorkerTaskStatus) bool {
	switch status {
	case domain.WorkerTaskStatusAssigned, domain.WorkerTaskStatusAccepted:
		return true
	default:
		return false
	}
}

type SDKRunIDInput struct {
	RunID string `path:"runID"`
}

type SDKGetPayloadOutput struct{ Body any }

func (s *Server) handleSDKGetPayload(ctx context.Context, input *SDKRunIDInput) (*SDKGetPayloadOutput, error) {
	run, err := s.store.GetRun(ctx, input.RunID)
	if err != nil {
		if errors.Is(err, store.ErrRunNotFound) {
			return nil, huma.Error404NotFound("run not found")
		}
		return nil, huma.Error500InternalServerError("failed to get run")
	}
	if len(run.Payload) == 0 {
		return &SDKGetPayloadOutput{Body: nil}, nil
	}
	var payload any
	if err := json.Unmarshal(run.Payload, &payload); err != nil {
		return nil, huma.Error500InternalServerError("failed to parse payload")
	}
	return &SDKGetPayloadOutput{Body: payload}, nil
}

type SDKLogRequest struct {
	Type    string          `json:"type,omitempty"`
	Level   string          `json:"level,omitempty"`
	Message string          `json:"message" validate:"required"`
	Data    json.RawMessage `json:"data,omitempty"`
}
type SDKLogInput struct {
	RunID string `path:"runID"`
	Body  SDKLogRequest
}
type SDKLogOutput struct{ Body *domain.RunEvent }

func (s *Server) handleSDKLog(ctx context.Context, input *SDKLogInput) (*SDKLogOutput, error) {
	runID := input.RunID
	req := input.Body
	if err := s.validate.Struct(&req); err != nil {
		return nil, newValidationError(err)
	}
	eventType := domain.EventType(req.Type)
	if eventType == "" {
		eventType = domain.EventLog
	}
	if req.Level == "" {
		req.Level = "info"
	}
	data := req.Data
	if len(data) == 0 {
		data = json.RawMessage(`{}`)
	}
	event := &domain.RunEvent{RunID: runID, Type: eventType, Level: req.Level, Message: req.Message, Data: data}
	var err error
	if guardedStore, ok := s.store.(activeRunMutationStore); ok {
		err = guardedStore.InsertEventForActiveRun(ctx, event, runTokenAttemptFromContext(ctx))
	} else {
		err = s.store.InsertEvent(ctx, event)
	}
	if err != nil {
		if sdkErr := s.guardedSDKMutationError(ctx, err); sdkErr != nil {
			return nil, sdkErr
		}
		slog.Error("failed to insert event", "run_id", runID, "error", err)
		return nil, huma.Error500InternalServerError("failed to insert event")
	}
	if s.pubsub != nil {
		payload, err := marshalSDKRunEventPayload(eventType, runID, req.Level, req.Message, data, time.Now().UTC())
		if err != nil {
			slog.Warn("failed to marshal event payload", "run_id", runID, "error", err)
		} else {
			if err := s.pubsub.Publish(ctx, apiRunPubSubChannel(runID), payload); err != nil {
				slog.Warn("failed to publish event", "run_id", runID, "error", err)
			}
		}
	}
	return &SDKLogOutput{Body: event}, nil
}

type SDKProgressRequest struct {
	Percent    float64 `json:"percent" validate:"min=0,max=100"`
	Message    string  `json:"message" validate:"required"`
	Step       string  `json:"step,omitempty"`
	ETASeconds int     `json:"eta_seconds,omitempty"`
}
type SDKProgressInput struct {
	RunID string `path:"runID"`
	Body  SDKProgressRequest
}
type SDKProgressOutput struct{ Body *domain.RunEvent }

func (s *Server) handleSDKProgress(ctx context.Context, input *SDKProgressInput) (*SDKProgressOutput, error) {
	runID := input.RunID
	req := input.Body
	if err := s.validate.Struct(&req); err != nil {
		return nil, newValidationError(err)
	}
	data, err := marshalSDKProgressData(req.Percent, req.Step, req.ETASeconds)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to marshal progress payload")
	}
	event := &domain.RunEvent{RunID: runID, Type: domain.EventProgress, Level: "info", Message: req.Message, Data: data}
	var eventErr error
	if guardedStore, ok := s.store.(activeRunMutationStore); ok {
		eventErr = guardedStore.InsertEventForActiveRun(ctx, event, runTokenAttemptFromContext(ctx))
	} else {
		eventErr = s.store.InsertEvent(ctx, event)
	}
	if eventErr != nil {
		if sdkErr := s.guardedSDKMutationError(ctx, eventErr); sdkErr != nil {
			return nil, sdkErr
		}
		slog.Error("failed to insert progress event", "run_id", runID, "error", eventErr)
		return nil, huma.Error500InternalServerError("failed to insert event")
	}
	if s.pubsub != nil {
		payload, err := marshalSDKProgressEventPayload(runID, req.Message, req.Percent, req.Step, req.ETASeconds, time.Now().UTC())
		if err != nil {
			slog.Warn("failed to marshal progress payload", "run_id", runID, "error", err)
		} else {
			if err := s.pubsub.Publish(ctx, apiRunPubSubChannel(runID), payload); err != nil {
				slog.Warn("failed to publish progress event", "run_id", runID, "error", err)
			}
		}
	}
	return &SDKProgressOutput{Body: event}, nil
}

type SDKAnnotateRequest struct {
	Annotations map[string]string `json:"annotations" validate:"required,min=1"`
}
type SDKAnnotateInput struct {
	RunID string `path:"runID"`
	Body  SDKAnnotateRequest
}
type SDKAnnotateOutput struct{ Body *domain.JobRun }

func (s *Server) handleSDKAnnotate(ctx context.Context, input *SDKAnnotateInput) (*SDKAnnotateOutput, error) {
	runID := input.RunID
	req := input.Body
	if err := s.validate.Struct(&req); err != nil {
		return nil, newValidationError(err)
	}
	if len(req.Annotations) > 50 {
		return nil, huma.Error400BadRequest("too many annotations (max 50)")
	}
	for key, value := range req.Annotations {
		if strings.TrimSpace(key) == "" {
			return nil, huma.Error400BadRequest("annotation keys must be non-empty")
		}
		if len(key) > 128 {
			return nil, huma.Error400BadRequest("annotation key too long (max 128 characters)")
		}
		if len(value) > 1024 {
			return nil, huma.Error400BadRequest("annotation value too long (max 1024 characters)")
		}
	}
	var err error
	if guardedStore, ok := s.store.(activeRunMutationStore); ok {
		err = guardedStore.UpdateRunMetadataForActiveRun(ctx, runID, req.Annotations, runTokenAttemptFromContext(ctx))
	} else {
		err = s.store.UpdateRunMetadata(ctx, runID, req.Annotations)
	}
	if err != nil {
		if sdkErr := s.guardedSDKMutationError(ctx, err); sdkErr != nil {
			return nil, sdkErr
		}
		return nil, huma.Error500InternalServerError("failed to update run annotations")
	}
	updatedRun, err := s.store.GetRun(ctx, runID)
	if err != nil {
		if errors.Is(err, store.ErrRunNotFound) {
			return nil, huma.Error404NotFound("run not found")
		}
		return nil, huma.Error500InternalServerError("failed to fetch run")
	}
	return &SDKAnnotateOutput{Body: updatedRun}, nil
}

type SDKHeartbeatOutput struct{ Body map[string]string }

func (s *Server) handleSDKHeartbeat(ctx context.Context, input *SDKRunIDInput) (*SDKHeartbeatOutput, error) {
	var err error
	if guardedStore, ok := s.store.(activeRunMutationStore); ok {
		err = guardedStore.UpdateHeartbeatForActiveRun(ctx, input.RunID, runTokenAttemptFromContext(ctx))
	} else {
		err = s.store.UpdateHeartbeat(ctx, input.RunID)
	}
	if err != nil {
		if sdkErr := s.guardedSDKMutationError(ctx, err); sdkErr != nil {
			return nil, sdkErr
		}
		slog.Error("failed to update heartbeat", "run_id", input.RunID, "error", err)
		return nil, huma.Error500InternalServerError("failed to update heartbeat")
	}
	return &SDKHeartbeatOutput{Body: map[string]string{"status": "ok"}}, nil
}

type SDKCheckpointRequest struct {
	Source string          `json:"source,omitempty"`
	State  json.RawMessage `json:"state" validate:"required"`
}
type SDKCheckpointInput struct {
	RunID string `path:"runID"`
	Body  SDKCheckpointRequest
}
type SDKCheckpointOutput struct{ Body *domain.RunCheckpoint }

func (s *Server) handleSDKCheckpoint(ctx context.Context, input *SDKCheckpointInput) (*SDKCheckpointOutput, error) {
	runID := input.RunID
	req := input.Body
	if err := s.validate.Struct(&req); err != nil {
		return nil, newValidationError(err)
	}
	source := strings.TrimSpace(req.Source)
	if source == "" {
		source = "sdk"
	}
	checkpoint := &domain.RunCheckpoint{ID: uuid.Must(uuid.NewV7()).String(), RunID: runID, Source: source, State: req.State}
	if guardedStore, ok := s.store.(activeRunMutationStore); ok {
		err := guardedStore.CreateRunCheckpointForActiveRun(ctx, checkpoint, runTokenAttemptFromContext(ctx))
		if err != nil {
			if sdkErr := s.guardedSDKMutationError(ctx, err); sdkErr != nil {
				return nil, sdkErr
			}
			return nil, huma.Error500InternalServerError("failed to create checkpoint")
		}
		return &SDKCheckpointOutput{Body: checkpoint}, nil
	}
	return nil, huma.Error500InternalServerError("active run checkpoint guard unavailable")
}
