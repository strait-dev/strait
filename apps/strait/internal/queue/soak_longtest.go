//go:build longtest

package queue

// This file intentionally ships under the longtest build tag so the
// soak/bloat/scale tests never run in the default `go test ./...`
// pipeline. Enable with `go test -tags=longtest,integration ./internal/queue/...`.
