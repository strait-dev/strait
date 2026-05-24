# Build stage runs on the native runner architecture so Go cross-compiles
# natively instead of running under QEMU emulation, which is 10-30x slower.
# Base image pinned to the multi-arch manifest-list digest. Dependabot's
# docker ecosystem keeps this bumped weekly.
FROM --platform=$BUILDPLATFORM golang:1.26.3-alpine@sha256:91eda9776261207ea25fd06b5b7fed8d397dd2c0a283e77f2ab6e91bfa71079d AS builder

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

# SOCI fetch stage. Alpine is still used here as a throwaway builder to run
# wget/sha256sum/tar, but the resulting runtime image never includes anything
# from alpine — only the extracted `soci` binary. Trivy scans the final image,
# not intermediate stages, so alpine's distro-lag CVEs do not appear in the
# published image. The binary itself is statically linked Go and authenticates
# to ECR via the standard AWS SDK credential chain at runtime.
#
# Supply chain security: the tarball checksum is verified against the upstream
# sha256sum.txt before extraction. Update both SOCI_VERSION and the expected
# checksums whenever upgrading.
FROM alpine:3.23@sha256:5b10f432ef3da1b8d4c7eb6c487f2f5a8f096bc91145e68878dd4a5019afde11 AS soci-fetch
ARG SOCI_VERSION=0.13.0
ARG SOCI_SHA256_AMD64=8951d87d6d17d719f7155d89f25ce08e139c3b48ef3323ee8e347de2d8c2d555
ARG SOCI_SHA256_ARM64=dab5117cc50b0343521ae9f6dc691b4d3ab5f7eb19404d1f7c35524b48bcdad5
ARG TARGETARCH
RUN if [ "$TARGETARCH" = "amd64" ]; then EXPECTED_SHA256="${SOCI_SHA256_AMD64}"; \
    else EXPECTED_SHA256="${SOCI_SHA256_ARM64}"; fi && \
    wget -qO /tmp/soci.tar.gz \
        "https://github.com/awslabs/soci-snapshotter/releases/download/v${SOCI_VERSION}/soci-snapshotter-${SOCI_VERSION}-linux-${TARGETARCH}.tar.gz" && \
    echo "${EXPECTED_SHA256}  /tmp/soci.tar.gz" | sha256sum -c - && \
    tar -xzf /tmp/soci.tar.gz -C /tmp soci && \
    chmod 0755 /tmp/soci

# Runtime stage. Distroless static-debian12 ships only ca-certificates, tzdata,
# /etc/passwd with a `nonroot` user, and a CA bundle — no shell, no package
# manager, no libssl/libcrypto, no userspace utilities. Since the strait and
# soci binaries are both statically-linked Go, they need nothing else at
# runtime. This permanently removes the entire class of base-image distro-lag
# CVEs that were blocking the v0.1.3 release (alpine 3.21 → 3.22 → 3.23 all
# carried the same CVE-2026-28390 in libcrypto3/libssl3).
FROM gcr.io/distroless/static-debian12:nonroot@sha256:a9329520abc449e3b14d5bc3a6ffae065bdde0f02667fa10880c49b35c109fd1

COPY --from=soci-fetch /tmp/soci /usr/local/bin/soci
COPY --from=builder /strait /usr/local/bin/strait

# `nonroot` (UID 65532) is provided by the distroless image.
USER nonroot:nonroot

EXPOSE 8080

HEALTHCHECK --interval=15s --timeout=5s --start-period=10s --retries=3 \
    CMD ["strait", "health", "--quiet"]

ENTRYPOINT ["strait"]
