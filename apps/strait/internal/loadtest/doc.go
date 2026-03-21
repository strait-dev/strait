// Package loadtest provides a load testing framework for Strait.
//
// It includes:
//   - Multi-tenant traffic simulation with realistic patterns
//   - Throughput and concurrency ramp engines
//   - In-process metrics collection (Go runtime, Postgres, Redis)
//   - Test HTTP server for HTTP-mode job execution
//   - Pre-defined test scenarios for each tier
//
// All tests run locally using Docker containers via testcontainers-go.
// Build tag: loadtest.
package loadtest
