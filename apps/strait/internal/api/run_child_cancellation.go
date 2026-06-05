package api

import (
	"context"
	"log/slog"
	"time"
)

// maxCancelDepth limits recursive child cancellation to prevent runaway traversal.
const maxCancelDepth = 20

const childCancelPageLimit = 100

// cancelChildRunsRecursive uses CancelChildRunsByParentIDs to bulk-cancel
// children at each depth level, avoiding N+1 individual UpdateRunStatus calls.
func (s *Server) cancelChildRunsRecursive(ctx context.Context, parentRunID string) int64 {
	now := time.Now()
	parentIDs := []string{parentRunID}
	var total int64

	for depth := range maxCancelDepth {
		select {
		case <-ctx.Done():
			return total
		default:
		}

		if len(parentIDs) == 0 {
			break
		}

		canceled, err := s.store.CancelChildRunsByParentIDs(ctx, parentIDs, now, "parent run canceled")
		if err != nil {
			slog.Error("failed to bulk cancel child runs", "depth", depth, "parent_count", len(parentIDs), "error", err)
			break
		}
		if canceled == 0 {
			break
		}
		total += canceled

		parentIDs = s.nextChildCancellationParents(ctx, parentIDs)
	}

	return total
}

func (s *Server) nextChildCancellationParents(ctx context.Context, parentIDs []string) []string {
	nextParentIDs := make([]string, 0)
	for _, parentID := range parentIDs {
		var cursor *time.Time
		for {
			children, err := s.store.ListChildRuns(ctx, parentID, childCancelPageLimit, cursor)
			if err != nil || len(children) == 0 {
				break
			}
			for _, child := range children {
				nextParentIDs = append(nextParentIDs, child.ID)
			}
			lastCreatedAt := children[len(children)-1].CreatedAt
			cursor = &lastCreatedAt
		}
	}
	return nextParentIDs
}
