package api

import (
	"context"
	"time"

	"strait/internal/domain"

	"github.com/danielgtaylor/huma/v2"
)

type ListBatchOperationsInput struct {
	Limit  string `query:"limit"`
	Cursor string `query:"cursor"`
}
type ListBatchOperationsOutput struct{ Body PaginatedResponse }

func (s *Server) handleListBatchOperations(ctx context.Context, input *ListBatchOperationsInput) (*ListBatchOperationsOutput, error) {
	projectID := projectIDFromContext(ctx)
	limit, cursor, err := parsePaginationFromStrings(input.Limit, input.Cursor)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	ops, err := s.store.ListBatchOperations(ctx, projectID, limit+1, cursor)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list batch operations")
	}
	return &ListBatchOperationsOutput{Body: paginatedResult(ops, limit, func(op domain.BatchOperation) string { return op.CreatedAt.Format(time.RFC3339Nano) })}, nil
}

type GetBatchOperationInput struct {
	BatchID string `path:"batchID"`
}
type GetBatchOperationOutput struct{ Body *domain.BatchOperation }

func (s *Server) handleGetBatchOperation(ctx context.Context, input *GetBatchOperationInput) (*GetBatchOperationOutput, error) {
	op, err := s.store.GetBatchOperation(ctx, input.BatchID, projectIDFromContext(ctx))
	if err != nil {
		return nil, huma.Error404NotFound("batch operation not found")
	}
	return &GetBatchOperationOutput{Body: op}, nil
}
