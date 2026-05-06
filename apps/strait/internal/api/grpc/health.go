// Package grpc registers the standard gRPC health service.
// The actual registration happens in server.go inside buildServer().
// This file documents the health check semantics.
package grpc

// Health check notes:
//
// Two services are registered:
//   - "" (empty string) — overall server health, always SERVING while the
//     process is up. Used by load balancer and process liveness checks.
//   - "strait.worker.v1.WorkerService" — mirrors the overall status.
//
// When the server begins shutting down (GracefulStop called), the health
// server automatically transitions to NOT_SERVING so load balancers stop
// routing new connections before the drain window expires.
