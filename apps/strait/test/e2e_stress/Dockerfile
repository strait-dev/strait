# Strait Loadtest Image
# Packages the E2E stress test harness and echo server into a single image.
#
# Build (from apps/strait/):
#   docker build -t ghcr.io/strait-dev/strait-loadtest -f test/e2e_stress/Dockerfile .
#
# Run:
#   docker run --rm \
#     -e STRAIT_URL=http://host.docker.internal:8080 \
#     -e INTERNAL_SECRET=your-secret \
#     -e ITERATIONS=1000 \
#     ghcr.io/strait-dev/strait-loadtest

# Build stage
FROM golang:1.26-alpine AS builder

RUN apk add --no-cache ca-certificates

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w" \
    -o /loadtest ./test/e2e_stress/

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w" \
    -o /echoserver ./test/e2e_stress/echoserver/

# Runtime stage
FROM alpine:3.21

RUN apk add --no-cache ca-certificates

COPY --from=builder /loadtest /usr/local/bin/loadtest
COPY --from=builder /echoserver /usr/local/bin/echoserver
COPY test/e2e_stress/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]
