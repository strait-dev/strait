# Build stage runs on the native runner architecture so Go cross-compiles
# natively instead of running under QEMU emulation, which is 10-30x slower.
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS builder

RUN apk add --no-cache ca-certificates

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
ARG COMMIT=none
ARG DATE=unknown
ARG BUILD_TAGS=""
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
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
#
# Supply chain security: the tarball checksum is verified against the upstream
# sha256sum.txt before extraction. Update both SOCI_VERSION and the expected
# checksums whenever upgrading.
ARG SOCI_VERSION=0.9.0
ARG SOCI_SHA256_AMD64=deff4c4d5aeaf88cf658f487dbde800e4073f4714d55c36d20bd8097350173c9
ARG SOCI_SHA256_ARM64=836922af9284923934bcbd0e721fe6218d3acbe1c3ab150f5adca1f89496a276
RUN ARCH=$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/') && \
    if [ "$ARCH" = "amd64" ]; then EXPECTED_SHA256="${SOCI_SHA256_AMD64}"; \
    else EXPECTED_SHA256="${SOCI_SHA256_ARM64}"; fi && \
    wget -qO /tmp/soci.tar.gz \
        "https://github.com/awslabs/soci-snapshotter/releases/download/v${SOCI_VERSION}/soci-snapshotter-${SOCI_VERSION}-linux-${ARCH}.tar.gz" && \
    echo "${EXPECTED_SHA256}  /tmp/soci.tar.gz" | sha256sum -c - && \
    tar -xzf /tmp/soci.tar.gz -C /tmp soci && \
    install -m 0755 /tmp/soci /usr/local/bin/soci && \
    rm /tmp/soci.tar.gz /tmp/soci

COPY --from=builder /strait /usr/local/bin/strait

USER app

EXPOSE 8080

HEALTHCHECK --interval=15s --timeout=5s --start-period=10s --retries=3 \
    CMD ["strait", "health", "--quiet"]

ENTRYPOINT ["strait"]
