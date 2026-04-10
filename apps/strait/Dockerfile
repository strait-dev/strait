# Build stage
FROM golang:1.26-alpine AS builder

RUN apk add --no-cache ca-certificates

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
ARG COMMIT=none
ARG DATE=unknown
ARG BUILD_TAGS=""
RUN CGO_ENABLED=0 GOOS=linux go build \
    -tags "${BUILD_TAGS}" \
    -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE}" \
    -o /strait ./cmd/strait

# Runtime stage
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S app \
    && adduser -S app -G app

# Install the SOCI CLI for lazy image loading (used when SOCI_ENABLED=true).
# The binary is statically linked and authenticates to ECR via the AWS SDK
# credential chain — no additional configuration needed at build time.
ARG SOCI_VERSION=0.9.0
RUN ARCH=$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/') && \
    wget -qO /tmp/soci.tar.gz \
        "https://github.com/awslabs/soci-snapshotter/releases/download/v${SOCI_VERSION}/soci-snapshotter-${SOCI_VERSION}-linux-${ARCH}.tar.gz" && \
    tar -xzf /tmp/soci.tar.gz -C /tmp soci && \
    install -m 0755 /tmp/soci /usr/local/bin/soci && \
    rm /tmp/soci.tar.gz /tmp/soci

COPY --from=builder /strait /usr/local/bin/strait

USER app

EXPOSE 8080

HEALTHCHECK --interval=15s --timeout=5s --start-period=10s --retries=3 \
    CMD ["strait", "health", "--quiet"]

ENTRYPOINT ["strait"]
