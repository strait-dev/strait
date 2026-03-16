package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/go-chi/chi/v5"
)

type createDeploymentVersionRequest struct {
	ProjectID   string `json:"project_id" validate:"required"`
	Environment string `json:"environment" validate:"required"`
	Runtime     string `json:"runtime" validate:"required"`
	ArtifactURI string `json:"artifact_uri" validate:"required,url"`
	Manifest    any    `json:"manifest"`
	Checksum    string `json:"checksum"`
}

type deploymentVersionMutationRequest struct {
	ProjectID   string `json:"project_id" validate:"required"`
	Environment string `json:"environment" validate:"required"`
}

func marshalRaw(value any) json.RawMessage {
	if value == nil {
		return json.RawMessage(`{}`)
	}

	b, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage(`{}`)
	}

	return json.RawMessage(b)
}

func (s *Server) handleCreateDeploymentVersion(w http.ResponseWriter, r *http.Request) {
	var req createDeploymentVersionRequest
	if err := s.decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}
	if !s.validateRequest(w, r, &req) {
		return
	}

	status := domain.DeploymentVersionStatusDraft
	if req.Runtime == "" {
		req.Runtime = "node"
	}

	manifest := marshalRaw(req.Manifest)

	deployment := &domain.DeploymentVersion{
		ProjectID:   req.ProjectID,
		Environment: req.Environment,
		Runtime:     req.Runtime,
		ArtifactURI: req.ArtifactURI,
		Manifest:    manifest,
		Checksum:    req.Checksum,
		Status:      status,
		CreatedBy:   actorFromContext(r.Context()),
		UpdatedBy:   actorFromContext(r.Context()),
	}

	if err := s.store.CreateDeploymentVersion(r.Context(), deployment); err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to create deployment version")
		return
	}

	respondJSON(w, http.StatusCreated, deployment)
}

func (s *Server) handleListDeploymentVersions(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		respondError(w, r, http.StatusBadRequest, "project_id is required")
		return
	}
	environment := r.URL.Query().Get("environment")

	limit, cursor, err := parsePaginationParams(r)
	if err != nil {
		respondError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	versions, err := s.store.ListDeploymentVersions(r.Context(), projectID, environment, limit+1, cursor)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to list deployment versions")
		return
	}

	respondJSON(w, http.StatusOK, paginatedResult(versions, limit, func(v domain.DeploymentVersion) string {
		return v.CreatedAt.Format(time.RFC3339Nano)
	}))
}

func (s *Server) handleFinalizeDeploymentVersion(w http.ResponseWriter, r *http.Request) {
	deploymentID := chi.URLParam(r, "deploymentID")
	var req deploymentVersionMutationRequest
	if err := s.decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}
	if !s.validateRequest(w, r, &req) {
		return
	}

	deployment, err := s.store.FinalizeDeploymentVersion(
		r.Context(),
		deploymentID,
		req.ProjectID,
		actorFromContext(r.Context()),
	)
	if err != nil {
		if errors.Is(err, store.ErrDeploymentVersionNotFound) {
			respondError(w, r, http.StatusNotFound, "deployment version not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to finalize deployment version")
		return
	}

	respondJSON(w, http.StatusOK, deployment)
}

func (s *Server) handlePromoteDeploymentVersion(w http.ResponseWriter, r *http.Request) {
	s.handleDeploymentPromotion(w, r, false)
}

func (s *Server) handleRollbackDeploymentVersion(w http.ResponseWriter, r *http.Request) {
	s.handleDeploymentPromotion(w, r, true)
}

func (s *Server) handleDeploymentPromotion(w http.ResponseWriter, r *http.Request, rollback bool) {
	deploymentID := chi.URLParam(r, "deploymentID")
	var req deploymentVersionMutationRequest
	if err := s.decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}
	if !s.validateRequest(w, r, &req) {
		return
	}

	var (
		deployment *domain.DeploymentVersion
		err        error
	)

	if rollback {
		deployment, err = s.store.RollbackDeploymentVersion(
			r.Context(),
			deploymentID,
			req.ProjectID,
			req.Environment,
			actorFromContext(r.Context()),
		)
	} else {
		deployment, err = s.store.PromoteDeploymentVersion(
			r.Context(),
			deploymentID,
			req.ProjectID,
			req.Environment,
			actorFromContext(r.Context()),
		)
	}

	if err != nil {
		if errors.Is(err, store.ErrDeploymentVersionNotFound) {
			respondError(w, r, http.StatusNotFound, "deployment version not found")
			return
		}
		if rollback {
			respondError(w, r, http.StatusInternalServerError, "failed to rollback deployment version")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to promote deployment version")
		return
	}

	respondJSON(w, http.StatusOK, deployment)
}
