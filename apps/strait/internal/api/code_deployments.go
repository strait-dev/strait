package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"strait/internal/build"
	"strait/internal/domain"
	"strait/internal/objectstore"
	"strait/internal/store"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	otelmetric "go.opentelemetry.io/otel/metric"
)

// presignUploadTTL is how long the presigned upload URL remains valid.
// The CLI must upload the tarball within this window after calling create.
const presignUploadTTL = 15 * time.Minute

// --- Create deployment.

type createCodeDeploymentRequest struct {
	ProjectID string `json:"project_id" validate:"required"`
	JobID     string `json:"job_id" validate:"required"`
	Runtime   string `json:"runtime" validate:"required,oneof=python typescript ruby rust go"`
	// SourceHash is the SHA-256 hex digest of the tarball the CLI will upload.
	// The confirm endpoint verifies the uploaded object matches this hash.
	SourceHash      string `json:"source_hash" validate:"required,len=64,hexadecimal"`
	SourceSizeBytes int64  `json:"source_size_bytes" validate:"required,min=1"`
}

type CreateCodeDeploymentInput struct {
	Body createCodeDeploymentRequest
}

type CreateCodeDeploymentOutput struct {
	Body struct {
		Deployment *domain.CodeDeployment `json:"deployment"`
		// UploadURL is a presigned PUT URL valid for 15 minutes.
		// The CLI must PUT the tarball directly to this URL.
		UploadURL string `json:"upload_url"`
	}
}

func (s *Server) handleCreateCodeDeployment(ctx context.Context, input *CreateCodeDeploymentInput) (*CreateCodeDeploymentOutput, error) {
	req := input.Body
	if err := s.validate.Struct(&req); err != nil {
		return nil, newValidationError(err)
	}
	if err := requireProjectMatch(ctx, req.ProjectID); err != nil {
		return nil, huma.Error403Forbidden("project_id does not match authenticated project")
	}

	if req.SourceSizeBytes > build.MaxTarballBytes {
		return nil, huma.Error400BadRequest(fmt.Sprintf(
			"source_size_bytes %d exceeds maximum allowed tarball size of %d bytes (%d MB)",
			req.SourceSizeBytes, build.MaxTarballBytes, build.MaxTarballBytes/1024/1024,
		))
	}

	runtime := domain.Runtime(req.Runtime)
	if !runtime.IsValid() {
		return nil, huma.Error400BadRequest(fmt.Sprintf("invalid runtime %q: must be python, typescript, ruby, rust, or go", req.Runtime))
	}

	// Verify the job belongs to this project.
	job, err := s.store.GetJob(ctx, req.JobID)
	if err != nil {
		if errors.Is(err, store.ErrJobNotFound) {
			return nil, huma.Error404NotFound("job not found")
		}
		return nil, huma.Error500InternalServerError("failed to fetch job")
	}
	if job.ProjectID != req.ProjectID {
		return nil, huma.Error403Forbidden("job does not belong to the authenticated project")
	}

	if s.objectStore == nil {
		return nil, huma.Error503ServiceUnavailable("code-first deployments are not configured on this instance")
	}

	// Pre-generate the ID so source_uri contains the correct deployment ID
	// before the INSERT. Without this, the DB would store a path with an empty
	// ID (e.g. "projects/{pid}/jobs/{jid}/deploys/.tar.gz") that can never be
	// resolved by HeadObject at confirm time.
	deploymentID := uuid.Must(uuid.NewV7()).String()
	deployment := &domain.CodeDeployment{
		ID:              deploymentID,
		JobID:           req.JobID,
		ProjectID:       req.ProjectID,
		Runtime:         runtime,
		SourceHash:      req.SourceHash,
		SourceSizeBytes: req.SourceSizeBytes,
		SourceURI:       objectstore.DeploymentKey(req.ProjectID, req.JobID, deploymentID),
		Status:          domain.DeploymentStatusPending,
		CreatedBy:       actorFromContext(ctx),
	}

	if err := s.store.CreateCodeDeployment(ctx, deployment); err != nil {
		return nil, huma.Error500InternalServerError("failed to create deployment")
	}

	uploadURL, err := s.objectStore.PresignUpload(ctx, deployment.SourceURI, presignUploadTTL, deployment.SourceSizeBytes)
	if err != nil {
		slog.Error("failed to generate presigned upload URL",
			"deployment_id", deployment.ID,
			"error", err,
		)
		// Mark the orphaned record as failed so it is not left pending forever.
		_ = s.store.UpdateCodeDeploymentStatus(ctx, deployment.ID, domain.DeploymentStatusFailed,
			map[string]any{"error_message": "failed to generate presigned upload URL"})
		return nil, huma.Error500InternalServerError("failed to generate upload URL")
	}

	s.emitAuditEvent(ctx, "code_deployment.created", "code_deployment", deployment.ID, map[string]any{
		"job_id":           deployment.JobID,
		"runtime":          string(deployment.Runtime),
		"source_size_bytes": deployment.SourceSizeBytes,
	})

	out := &CreateCodeDeploymentOutput{}
	out.Body.Deployment = deployment
	out.Body.UploadURL = uploadURL
	return out, nil
}

// --- Confirm deployment (trigger build).

type ConfirmCodeDeploymentInput struct {
	JobID        string `path:"jobID"`
	DeploymentID string `path:"deploymentID"`
	Body         struct {
		ProjectID string `json:"project_id" validate:"required"`
	}
}

type ConfirmCodeDeploymentOutput struct {
	Body *domain.CodeDeployment
}

func (s *Server) handleConfirmCodeDeployment(ctx context.Context, input *ConfirmCodeDeploymentInput) (*ConfirmCodeDeploymentOutput, error) {
	if err := requireProjectMatch(ctx, input.Body.ProjectID); err != nil {
		return nil, huma.Error403Forbidden("project_id does not match authenticated project")
	}

	d, err := s.store.GetCodeDeployment(ctx, input.DeploymentID, input.Body.ProjectID)
	if err != nil {
		if errors.Is(err, store.ErrCodeDeploymentNotFound) {
			return nil, huma.Error404NotFound("deployment not found")
		}
		return nil, huma.Error500InternalServerError("failed to fetch deployment")
	}

	if d.JobID != input.JobID {
		return nil, huma.Error404NotFound("deployment not found")
	}

	if d.Status != domain.DeploymentStatusPending {
		return nil, huma.Error409Conflict(
			fmt.Sprintf("deployment is already in status %q; only pending deployments can be confirmed", d.Status),
		)
	}

	// Verify the object was actually uploaded, matches the declared size, and
	// has the expected SHA-256 hash. All three checks prevent corrupt or tampered
	// tarballs from reaching the BuildKit build stage.
	if s.objectStore != nil {
		actualSize, headErr := s.objectStore.HeadObject(ctx, d.SourceURI)
		if headErr != nil {
			if errors.Is(headErr, objectstore.ErrObjectNotFound) {
				return nil, huma.Error422UnprocessableEntity("tarball not found at upload URL — ensure the upload completed before confirming")
			}
			return nil, huma.Error500InternalServerError("failed to verify upload")
		}

		if actualSize != d.SourceSizeBytes {
			return nil, huma.Error422UnprocessableEntity(fmt.Sprintf(
				"uploaded file size %d B does not match declared source_size_bytes %d B — re-upload the tarball",
				actualSize, d.SourceSizeBytes,
			))
		}

		rc, getErr := s.objectStore.GetObject(ctx, d.SourceURI)
		if getErr != nil {
			return nil, huma.Error500InternalServerError("failed to retrieve tarball for hash verification")
		}
		defer rc.Close()
		h := sha256.New()
		if _, copyErr := io.Copy(h, io.LimitReader(rc, build.MaxTarballBytes+1)); copyErr != nil {
			return nil, huma.Error500InternalServerError("failed to read tarball for hash verification")
		}
		if got := hex.EncodeToString(h.Sum(nil)); got != d.SourceHash {
			return nil, huma.Error422UnprocessableEntity("tarball SHA-256 hash does not match declared source_hash — re-upload the tarball")
		}

		// Record the verified tarball size. Recorded here (after hash check) so only
		// successfully validated tarballs contribute to the distribution.
		if s.metrics != nil && d.SourceSizeBytes > 0 {
			s.metrics.CodeDeployTarballBytes.Record(ctx, d.SourceSizeBytes,
				otelmetric.WithAttributes(attribute.String("runtime", string(d.Runtime))),
			)
		}
	}

	// Atomically transition pending → building. The WHERE status='pending' guard
	// inside ConfirmCodeDeployment ensures that concurrent confirm requests cannot
	// both succeed — at most one will see RowsAffected > 0.
	if err := s.store.ConfirmCodeDeployment(ctx, d.ID); err != nil {
		if errors.Is(err, store.ErrCodeDeploymentNotFound) {
			return nil, huma.Error409Conflict("deployment was already confirmed by a concurrent request")
		}
		return nil, huma.Error500InternalServerError("failed to confirm deployment")
	}
	d.Status = domain.DeploymentStatusBuilding

	s.emitAuditEvent(ctx, "code_deployment.confirmed", "code_deployment", d.ID, map[string]any{
		"job_id":            d.JobID,
		"runtime":           string(d.Runtime),
		"source_size_bytes": d.SourceSizeBytes,
	})

	return &ConfirmCodeDeploymentOutput{Body: d}, nil
}

// --- Get deployment.

type GetCodeDeploymentInput struct {
	JobID        string `path:"jobID"`
	DeploymentID string `path:"deploymentID"`
}

type GetCodeDeploymentOutput struct {
	Body *domain.CodeDeployment
}

func (s *Server) handleGetCodeDeployment(ctx context.Context, input *GetCodeDeploymentInput) (*GetCodeDeploymentOutput, error) {
	projectID := projectIDFromContext(ctx)

	d, err := s.store.GetCodeDeployment(ctx, input.DeploymentID, projectID)
	if err != nil {
		if errors.Is(err, store.ErrCodeDeploymentNotFound) {
			return nil, huma.Error404NotFound("deployment not found")
		}
		return nil, huma.Error500InternalServerError("failed to fetch deployment")
	}
	if d.JobID != input.JobID {
		return nil, huma.Error404NotFound("deployment not found")
	}
	return &GetCodeDeploymentOutput{Body: d}, nil
}

// --- List deployments.

type ListCodeDeploymentsInput struct {
	JobID  string `path:"jobID"`
	Limit  string `query:"limit"`
	Cursor string `query:"cursor"`
}

type ListCodeDeploymentsOutput struct {
	Body PaginatedResponse
}

func (s *Server) handleListCodeDeployments(ctx context.Context, input *ListCodeDeploymentsInput) (*ListCodeDeploymentsOutput, error) {
	projectID := projectIDFromContext(ctx)

	limit, cursor, err := parsePaginationFromStrings(input.Limit, input.Cursor)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}

	deployments, err := s.store.ListCodeDeployments(ctx, input.JobID, projectID, limit+1, cursor)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list deployments")
	}

	return &ListCodeDeploymentsOutput{Body: paginatedResult(deployments, limit, func(d domain.CodeDeployment) string {
		return d.CreatedAt.Format(time.RFC3339Nano)
	})}, nil
}

// --- Rollback.

type RollbackCodeDeploymentInput struct {
	JobID        string `path:"jobID"`
	DeploymentID string `path:"deploymentID"`
	Body         struct {
		ProjectID string `json:"project_id" validate:"required"`
	}
}

type RollbackCodeDeploymentOutput struct {
	Body *domain.CodeDeployment
}

func (s *Server) handleRollbackCodeDeployment(ctx context.Context, input *RollbackCodeDeploymentInput) (*RollbackCodeDeploymentOutput, error) {
	if err := requireProjectMatch(ctx, input.Body.ProjectID); err != nil {
		return nil, huma.Error403Forbidden("project_id does not match authenticated project")
	}

	d, err := s.store.GetCodeDeployment(ctx, input.DeploymentID, input.Body.ProjectID)
	if err != nil {
		if errors.Is(err, store.ErrCodeDeploymentNotFound) {
			return nil, huma.Error404NotFound("deployment not found")
		}
		return nil, huma.Error500InternalServerError("failed to fetch deployment")
	}
	if d.JobID != input.JobID {
		return nil, huma.Error404NotFound("deployment not found")
	}

	if d.Status != domain.DeploymentStatusReady {
		return nil, huma.Error409Conflict(
			fmt.Sprintf("cannot roll back to deployment in status %q; only ready deployments can be activated", d.Status),
		)
	}

	if err := s.store.RollbackToDeployment(ctx, input.JobID, input.DeploymentID, input.Body.ProjectID); err != nil {
		if errors.Is(err, store.ErrCodeDeploymentNotFound) {
			return nil, huma.Error404NotFound("deployment not found or not in ready status")
		}
		return nil, huma.Error500InternalServerError("failed to roll back deployment")
	}

	s.emitAuditEvent(ctx, "code_deployment.rolled_back", "code_deployment", d.ID, map[string]any{
		"job_id":  d.JobID,
		"runtime": string(d.Runtime),
	})

	return &RollbackCodeDeploymentOutput{Body: d}, nil
}

// --- Build log streaming.

// handleDeploymentLogs streams build logs for a code deployment.
//
// Behaviour:
//   - Terminal deployment (ready/failed/timed_out): returns the stored build_logs
//     field as text/plain.
//   - Building deployment + ?stream=true: subscribes to the pub/sub channel
//     "deploy:{id}:logs" and streams chunks as SSE (text/event-stream). A
//     {"done":true} sentinel closes the stream. Falls back to stored logs when
//     pubsub is unavailable.
//   - Building deployment + ?stream=false (default): returns whatever logs have
//     been stored so far as text/plain.
func (s *Server) handleDeploymentLogs(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "jobID")
	deploymentID := chi.URLParam(r, "deploymentID")
	projectID, _ := r.Context().Value(ctxProjectIDKey).(string)

	if projectID == "" {
		respondError(w, r, http.StatusUnauthorized, "project context missing")
		return
	}

	d, err := s.store.GetCodeDeployment(r.Context(), deploymentID, projectID)
	if err != nil {
		if errors.Is(err, store.ErrCodeDeploymentNotFound) {
			respondError(w, r, http.StatusNotFound, "deployment not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to fetch deployment")
		return
	}
	if d.JobID != jobID {
		respondError(w, r, http.StatusNotFound, "deployment not found")
		return
	}

	wantStream := r.URL.Query().Get("stream") == "true"
	isBuilding := d.Status == domain.DeploymentStatusBuilding

	// Non-streaming path: return stored logs as text/plain.
	if !wantStream || !isBuilding || s.pubsub == nil {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(d.BuildLogs)) // #nosec G705 -- served as text/plain, not HTML
		return
	}

	// SSE streaming path: subscribe and forward chunks.
	flusher, ok := w.(http.Flusher)
	if !ok {
		respondError(w, r, http.StatusInternalServerError, "streaming not supported")
		return
	}

	maxDuration := s.config.SSEMaxConnDuration
	if maxDuration <= 0 {
		maxDuration = 30 * time.Minute
	}
	ctx, cancel := context.WithTimeout(r.Context(), maxDuration)
	defer cancel()

	sub, err := s.pubsub.Subscribe(ctx, build.BuildLogChannel(deploymentID))
	if err != nil {
		slog.Warn("deployment logs: subscribe failed", "deployment_id", deploymentID, "error", err)
		// Fall back to stored logs.
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(d.BuildLogs)) // #nosec G705 -- served as text/plain, not HTML
		return
	}
	defer sub.Close()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	keepalive := s.config.SSEKeepaliveInterval
	if keepalive <= 0 {
		keepalive = 15 * time.Second
	}
	ticker := time.NewTicker(keepalive)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-sub.Ch:
			if !ok {
				return
			}
			// Sanitize the message to prevent SSE protocol injection via embedded
			// newlines. Raw CR or LF bytes in the data field would allow an attacker
			// who can publish to the pub/sub channel to inject fake SSE events.
			safe := strings.NewReplacer("\r\n", " ", "\r", " ", "\n", " ").Replace(string(msg))
			fmt.Fprintf(w, "event: log\ndata: %s\n\n", safe)
			flusher.Flush()
			// Check for the done sentinel — builder sends {"done":true} when build ends.
			if string(msg) == `{"done":true}` {
				return
			}
		case <-ticker.C:
			fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}

// --- Admin: list deployments across an org.

// ListAdminOrgDeploymentsInput is the input for the admin org deployments list.
type ListAdminOrgDeploymentsInput struct {
	OrgID  string `path:"orgID"`
	Limit  string `query:"limit"`
	Cursor string `query:"cursor"`
}

// ListAdminOrgDeploymentsOutput wraps the paginated deployment list.
type ListAdminOrgDeploymentsOutput struct{ Body PaginatedResponse }

// handleListAdminOrgDeployments lists all code deployments across every project
// in an org. Protected by X-Internal-Secret; never accessible with a project API key.
func (s *Server) handleListAdminOrgDeployments(ctx context.Context, input *ListAdminOrgDeploymentsInput) (*ListAdminOrgDeploymentsOutput, error) {
	if input.OrgID == "" {
		return nil, huma.Error400BadRequest("orgID is required")
	}
	limit, cursor, err := parsePaginationFromStrings(input.Limit, input.Cursor)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	deployments, err := s.store.ListCodeDeploymentsByOrg(ctx, input.OrgID, limit+1, cursor)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list deployments")
	}
	return &ListAdminOrgDeploymentsOutput{
		Body: paginatedResult(deployments, limit, func(d domain.CodeDeployment) string { return d.CreatedAt.Format(time.RFC3339Nano) }),
	}, nil
}
