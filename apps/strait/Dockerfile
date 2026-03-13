# Build stage
FROM golang:1.26-alpine AS builder

RUN apk add --no-cache ca-certificates

WORKDIR /build/apps/strait

COPY apps/strait/go.mod apps/strait/go.sum ./
RUN go mod download

COPY apps/strait ./

ARG VERSION=dev
ARG COMMIT=none
ARG DATE=unknown
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE}" \
    -o /strait ./cmd/strait

# Runtime stage
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S app \
    && adduser -S app -G app

COPY --from=builder /strait /usr/local/bin/strait

USER app

EXPOSE 8080

HEALTHCHECK --interval=15s --timeout=5s --start-period=10s --retries=3 \
    CMD ["strait", "health", "--quiet"]

ENTRYPOINT ["strait"]
