package grpc

import (
	"log/slog"

	workerv1 "strait/internal/api/grpc/proto/workerv1"
	"strait/internal/config"
	"strait/internal/pubsub"
	"strait/internal/store"
)

// workerService implements workerv1.WorkerServiceServer.
type workerService struct {
	queries  *store.Queries
	pub      pubsub.Publisher
	registry *ConnectionRegistry
	cfg      *config.Config
}

// StreamTasks is the bidirectional streaming RPC between the server and a worker SDK.
// Full implementation: see Phase 6.4.
func (s *workerService) StreamTasks(stream workerv1.WorkerService_StreamTasksServer) error {
	ctx := stream.Context()

	// Authenticate the connecting worker via the Bearer API key in gRPC metadata.
	apiKey, err := resolveAPIKeyFromContext(ctx, s.queries)
	if err != nil {
		return err
	}
	ctx = withAPIKeyContext(ctx, apiKey)
	_ = ctx // context enrichment used in Phase 6.4 handler

	// TODO(Phase 6.4): full recv/send loop, registration, slot accounting, log drain.
	slog.Info("grpc worker stream connected (stub)", "project_id", apiKey.ProjectID)

	// Block until the stream is closed.
	for {
		_, err := stream.Recv()
		if err != nil {
			return err
		}
	}
}
