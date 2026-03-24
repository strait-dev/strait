package api

import (
	"context"
	"errors"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/danielgtaylor/huma/v2"
)

type ListJobVersionsInput struct {
	JobID  string `path:"jobID"`
	Limit  string `query:"limit"`
	Cursor string `query:"cursor"`
}
type ListJobVersionsOutput struct{ Body PaginatedResponse }

func (s *Server) handleListJobVersions(ctx context.Context, input *ListJobVersionsInput) (*ListJobVersionsOutput, error) {
	limit, cursor, err := parsePaginationFromStrings(input.Limit, input.Cursor)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	versions, err := s.store.ListJobVersionsByJob(ctx, input.JobID, limit+1, cursor)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list job versions")
	}
	return &ListJobVersionsOutput{Body: paginatedResult(versions, limit, func(v domain.JobVersion) string { return v.CreatedAt.Format(time.RFC3339Nano) })}, nil
}

type GetJobVersionInput struct {
	JobID     string `path:"jobID"`
	VersionID string `path:"versionID"`
}
type GetJobVersionOutput struct{ Body *domain.JobVersion }

func (s *Server) handleGetJobVersion(ctx context.Context, input *GetJobVersionInput) (*GetJobVersionOutput, error) {
	version, err := s.store.GetJobVersionByVersionID(ctx, input.VersionID)
	if err != nil {
		if errors.Is(err, store.ErrJobNotFound) {
			return nil, huma.Error404NotFound("version not found")
		}
		return nil, huma.Error500InternalServerError("failed to get job version")
	}
	if version.JobID != input.JobID {
		return nil, huma.Error404NotFound("version not found")
	}
	return &GetJobVersionOutput{Body: version}, nil
}
