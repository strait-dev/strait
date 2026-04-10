# Build stage runs on the native runner architecture so Go cross-compiles
# natively instead of running under QEMU emulation, which is 10-30x slower.
# Base image pinned to the multi-arch manifest-list digest. Dependabot's
# docker ecosystem keeps this bumped weekly.
FROM --platform=$BUILDPLATFORM golang:1.26-alpine@sha256:c2a1f7b2095d046ae14b286b18413a05bb82c9bca9b25fe7ff5efef0f0826166 AS builder

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

# Runtime stage. Pinned to the multi-arch manifest-list digest; Dependabot
# keeps this bumped weekly. alpine 3.22 ships openssl 3.3.7-r0 which fixes
# CVE-2026-28390; alpine 3.21 is still on 3.3.6 and fails Trivy's HIGH gate.
FROM alpine:3.22@sha256:55ae5d250caebc548793f321534bc6a8ef1d116f334f18f4ada1b2daad3251b2

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
ARG SOCI_VERSION=0.13.0
ARG SOCI_SHA256_AMD64=8951d87d6d17d719f7155d89f25ce08e139c3b48ef3323ee8e347de2d8c2d555
ARG SOCI_SHA256_ARM64=dab5117cc50b0343521ae9f6dc691b4d3ab5f7eb19404d1f7c35524b48bcdad5
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
