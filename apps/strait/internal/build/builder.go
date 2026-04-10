package build

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	dockerconfigtypes "github.com/docker/cli/cli/config/types"
	bkclient "github.com/moby/buildkit/client"
	"github.com/moby/buildkit/session"
	"github.com/moby/buildkit/session/auth/authprovider"
	"go.opentelemetry.io/otel"

	"strait/internal/domain"
	"strait/internal/objectstore"
	"strait/internal/pubsub"
	"strait/internal/registry"
)

// BuildLogChannel returns the pub/sub channel name for streaming build logs
// for the given deployment ID.
func BuildLogChannel(deploymentID string) string {
	return "deploy:" + deploymentID + ":logs"
}

// buildLogChunk is the JSON shape published on the build log channel.
type buildLogChunk struct {
	Chunk string `json:"chunk,omitempty"`
	Done  bool   `json:"done,omitempty"`
}

// BuildResult holds the output of a successful container image build.
type BuildResult struct {
	ImageURI   string
	Digest     string
	BuildLogs  string
	FinishedAt time.Time
}

// Builder executes a single code deployment build:
//  1. Downloads the source tarball from object storage
//  2. Extracts it to a temp directory
//  3. Generates a runtime-specific Dockerfile
//  4. Submits the build to BuildKit
//  5. Pushes the image to the container registry
//  6. Optionally generates a SOCI index for lazy image loading
//  7. Returns the image URI + digest
type Builder struct {
	buildkitAddr string
	objectStore  objectstore.ObjectStore
	registry     registry.ContainerRegistry
	cacheEnabled bool
	timeout      time.Duration
	logPublisher pubsub.Publisher
	extraAuths   map[string]string
	// SOCI lazy loading — generates a seekable OCI index in the registry after
	// a successful build so containerd can start containers before the full image
	// is downloaded. sociRunner is overridable in tests.
	sociEnabled bool
	sociRunner  func(ctx context.Context, imageRef string) error
	sociTimeout time.Duration // defaults to 2 minutes; overridable in tests
}

// NewBuilder creates a Builder configured to talk to BuildKit at addr.
func NewBuilder(
	buildkitAddr string,
	objectStore objectstore.ObjectStore,
	reg registry.ContainerRegistry,
	cacheEnabled bool,
	timeout time.Duration,
) *Builder {
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	return &Builder{
		buildkitAddr: buildkitAddr,
		objectStore:  objectStore,
		registry:     reg,
		cacheEnabled: cacheEnabled,
		timeout:      timeout,
	}
}

// WithLogPublisher configures the builder to stream log chunks to a pub/sub
// channel during the build. Each log chunk is published as JSON {"chunk":"..."}
// and a final {"done":true} sentinel is published when the build finishes.
func (b *Builder) WithLogPublisher(p pubsub.Publisher) *Builder {
	b.logPublisher = p
	return b
}

// WithExtraRegistryAuths provides bearer tokens for private base-image
// registries used in Dockerfile FROM instructions. The map key is the
// registry hostname (e.g. "ghcr.io") and the value is the bearer token.
func (b *Builder) WithExtraRegistryAuths(auths map[string]string) *Builder {
	b.extraAuths = auths
	return b
}

// WithSOCI enables or disables SOCI (Seekable OCI) index generation after each
// successful build. When enabled, a SOCI index is pushed to the same registry
// alongside the image so the containerd SOCI snapshotter can lazily stream image
// layers, reducing cold-start latency. Requires the `soci` CLI to be in PATH
// and the SOCI snapshotter to be installed on cluster nodes.
//
// SOCI failure is non-fatal: the build is still considered successful and the
// image can be pulled normally. The only downside is no lazy loading for that
// image.
func (b *Builder) WithSOCI(enabled bool) *Builder {
	b.sociEnabled = enabled
	if enabled {
		b.sociRunner = runSOCICLI
	}
	return b
}

// withSOCITimeout overrides the internal SOCI context deadline. For testing only.
func (b *Builder) withSOCITimeout(d time.Duration) *Builder {
	b.sociTimeout = d
	return b
}

// runSOCICLI invokes the `soci` CLI to create a SOCI index for imageRef and push
// it to the registry. The soci binary authenticates to ECR via the standard AWS
// SDK credential chain (AWS_REGION, AWS_ACCESS_KEY_ID, etc.) — no explicit auth
// is needed beyond what the process environment already provides.
func runSOCICLI(ctx context.Context, imageRef string) error {
	path, err := exec.LookPath("soci")
	if err != nil {
		return fmt.Errorf("soci binary not in PATH: %w", err)
	}
	cmd := exec.CommandContext(ctx, path, "create", imageRef)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("soci create: %w (output: %s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// generateSOCIIndex generates a SOCI index for imageRef after a successful build.
// Failures are logged as warnings and do not fail the build — SOCI is a
// performance optimisation, not a correctness requirement.
func (b *Builder) generateSOCIIndex(ctx context.Context, imageRef string) {
	if !b.sociEnabled || b.sociRunner == nil {
		return
	}
	timeout := b.sociTimeout
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	sociCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := b.sociRunner(sociCtx, imageRef); err != nil {
		slog.Warn("soci index generation failed (build still succeeded)",
			"image", imageRef,
			"error", err,
		)
		return
	}
	slog.Info("soci index generated", "image", imageRef)
}

// Build runs the full build pipeline for a single deployment and returns the result.
// Logs contain stdout+stderr captured from BuildKit vertex logs.
// addr overrides the configured BuildKit address for this build; pass "" to use
// the address the Builder was constructed with (single-node path).
func (b *Builder) Build(ctx context.Context, d *domain.CodeDeployment, addr string) (*BuildResult, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "build.Builder.Build")
	defer span.End()

	if addr == "" {
		addr = b.buildkitAddr
	}

	buildCtx, cancel := context.WithTimeout(ctx, b.timeout)
	defer cancel()

	// 1. Download and extract the source tarball to a temp directory.
	contextDir, cleanup, err := b.extractTarball(buildCtx, d.SourceURI)
	if err != nil {
		return nil, fmt.Errorf("extract tarball: %w", err)
	}
	defer cleanup()

	// 2. Generate the runtime Dockerfile into the context directory.
	dockerfileContent, err := GenerateDockerfile(DockerfileSpec{
		Runtime:   d.Runtime,
		BaseImage: DefaultBaseImage(d.Runtime),
		JobSlug:   d.JobID,
		DepsFile:  DefaultDepsFile(d.Runtime),
	})
	if err != nil {
		return nil, fmt.Errorf("generate dockerfile: %w", err)
	}
	if err := os.WriteFile(filepath.Join(contextDir, "Dockerfile"), []byte(dockerfileContent), 0o600); err != nil {
		return nil, fmt.Errorf("write dockerfile: %w", err)
	}

	// 3. Ensure the container repository exists and get credentials.
	//
	// Repository path includes project_id to enforce strict per-tenant isolation:
	// two jobs with the same job_id in different projects get separate repositories
	// and separate BuildKit layer caches. Without the project_id prefix a crafted
	// job_id could collide with another tenant's path and share cached layers.
	repoName := registry.JobRepositoryName("", d.ProjectID, d.JobID)
	repositoryURI, err := b.registry.EnsureRepository(buildCtx, repoName)
	if err != nil {
		return nil, fmt.Errorf("ensure repository: %w", err)
	}

	authToken, _, err := b.registry.GetAuthToken(buildCtx)
	if err != nil {
		return nil, fmt.Errorf("get registry auth token: %w", err)
	}

	imageTag := fmt.Sprintf("%s:%s", repositoryURI, d.ID)

	// 4. Connect to BuildKit.
	bk, err := bkclient.New(buildCtx, addr)
	if err != nil {
		return nil, fmt.Errorf("connect to buildkit at %s: %w", addr, err)
	}
	defer bk.Close()

	// 5. Assemble solve options.
	registryHost := registryHostFromURI(repositoryURI)
	solveOpt := bkclient.SolveOpt{
		Frontend: "dockerfile.v0",
		FrontendAttrs: map[string]string{
			"filename": "Dockerfile",
		},
		// LocalDirs is deprecated in favour of LocalMounts but avoids the
		// fsutil.FS import complexity for a simple directory-backed context.
		LocalDirs: map[string]string{
			"context":    contextDir,
			"dockerfile": contextDir,
		},
		Exports: []bkclient.ExportEntry{
			{
				Type: bkclient.ExporterImage,
				Attrs: map[string]string{
					"name": imageTag,
					"push": "true",
				},
			},
		},
		Session: buildkitAuthSession(registryHost, authToken, b.extraAuths),
	}

	if b.cacheEnabled {
		// Cache key is scoped to the repository path (which already includes
		// project_id/job_id) so no cross-tenant cache sharing is possible.
		// Using "mode=max" exports all intermediate layers for maximum cache hit
		// rate on subsequent builds of the same job.
		cacheRef := fmt.Sprintf("%s:buildcache", repositoryURI)
		solveOpt.CacheImports = []bkclient.CacheOptionsEntry{
			{Type: "registry", Attrs: map[string]string{"ref": cacheRef}},
		}
		solveOpt.CacheExports = []bkclient.CacheOptionsEntry{
			{Type: "registry", Attrs: map[string]string{
				"ref":  cacheRef,
				"mode": "max",
			}},
		}
	}

	// 6. Stream build status to capture logs (and optionally publish them live).
	var logsBuf strings.Builder
	statusCh := make(chan *bkclient.SolveStatus)
	doneCh := make(chan struct{})
	go func() {
		defer close(doneCh)
		for status := range statusCh {
			for _, log := range status.Logs {
				logsBuf.Write(log.Data)
				if b.logPublisher != nil && len(log.Data) > 0 {
					chunk, _ := json.Marshal(buildLogChunk{Chunk: string(log.Data)})
					_ = b.logPublisher.Publish(buildCtx, BuildLogChannel(d.ID), chunk)
				}
			}
			for _, v := range status.Vertexes {
				if v.Error != "" {
					slog.Warn("buildkit vertex error",
						"vertex", v.Name,
						"error", v.Error,
						"deployment_id", d.ID,
					)
				}
			}
		}
	}()

	resp, buildErr := bk.Solve(buildCtx, nil, solveOpt, statusCh)
	<-doneCh

	// Publish the done sentinel regardless of build outcome so SSE consumers know
	// the stream has ended. Use a detached context so a server shutdown (which
	// cancels ctx) cannot prevent the sentinel from reaching connected clients.
	if b.logPublisher != nil {
		sentinel, _ := json.Marshal(buildLogChunk{Done: true})
		sentinelCtx, sentinelCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer sentinelCancel()
		_ = b.logPublisher.Publish(sentinelCtx, BuildLogChannel(d.ID), sentinel)
	}

	if buildErr != nil {
		return nil, fmt.Errorf("buildkit solve: %w", buildErr)
	}

	// 7. Extract the image digest from the solve response.
	digest := resp.ExporterResponse["containerimage.digest"]

	// 8. Optionally generate a SOCI index for lazy image loading. This is a
	// best-effort step: failure is logged but does not fail the build.
	b.generateSOCIIndex(ctx, imageTag)

	return &BuildResult{
		ImageURI:   imageTag,
		Digest:     digest,
		BuildLogs:  logsBuf.String(),
		FinishedAt: time.Now().UTC(),
	}, nil
}

// extractTarball downloads the tarball from object storage and extracts it into a
// temporary directory. Returns the directory path and a cleanup function.
func (b *Builder) extractTarball(ctx context.Context, sourceURI string) (dir string, cleanup func(), err error) {
	rc, err := b.objectStore.GetObject(ctx, sourceURI)
	if err != nil {
		return "", nil, fmt.Errorf("get object %s: %w", sourceURI, err)
	}
	defer rc.Close()

	data, err := io.ReadAll(io.LimitReader(rc, MaxTarballBytes+1))
	if err != nil {
		return "", nil, fmt.Errorf("read tarball: %w", err)
	}
	if int64(len(data)) > MaxTarballBytes {
		return "", nil, &TarballError{Reason: fmt.Sprintf("compressed tarball exceeds maximum size (%d MB)", MaxTarballBytes/1024/1024)}
	}

	// Security: validate before extracting.
	if err := ValidateTarball(bytes.NewReader(data)); err != nil {
		return "", nil, fmt.Errorf("tarball security validation: %w", err)
	}

	tmpDir, err := os.MkdirTemp("", "strait-build-*")
	if err != nil {
		return "", nil, fmt.Errorf("create temp dir: %w", err)
	}

	cleanup = func() { _ = os.RemoveAll(tmpDir) }

	if err := extractGzipTar(bytes.NewReader(data), tmpDir); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("extract tarball: %w", err)
	}

	return tmpDir, cleanup, nil
}

// extractGzipTar extracts a gzipped tar archive into dir.
// ValidateTarball must have been called on the same data before this function.
func extractGzipTar(r io.Reader, dir string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("gzip: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar: %w", err)
		}

		// filepath.Join + Clean is safe: ValidateTarball already rejected all
		// path traversal before this function is called.
		target := filepath.Join(dir, filepath.Clean(hdr.Name))

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o750); err != nil {
				return fmt.Errorf("mkdir %s: %w", target, err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
				return fmt.Errorf("mkdir parent of %s: %w", target, err)
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode)&0o755) //nolint:gosec // mode from trusted archive
			if err != nil {
				return fmt.Errorf("create %s: %w", target, err)
			}
			if _, copyErr := io.Copy(f, io.LimitReader(tr, MaxSingleFileBytes+1)); copyErr != nil {
				_ = f.Close()
				return fmt.Errorf("write %s: %w", target, copyErr)
			}
			if err := f.Close(); err != nil {
				return fmt.Errorf("close %s: %w", target, err)
			}
		case tar.TypeSymlink:
			if err := os.Symlink(hdr.Linkname, target); err != nil {
				return fmt.Errorf("symlink %s → %s: %w", target, hdr.Linkname, err)
			}
		}
	}
	return nil
}

// buildkitAuthSession returns session attachables that provide registry
// credentials to BuildKit for both the push registry and any private base
// image registries declared in extraAuths.
func buildkitAuthSession(registryHost, bearerToken string, extraAuths map[string]string) []session.Attachable {
	cfg := authprovider.DockerAuthProviderConfig{
		AuthConfigProvider: func(_ context.Context, host string, _ []string, _ authprovider.ExpireCachedAuthCheck) (dockerconfigtypes.AuthConfig, error) {
			if host == registryHost {
				return dockerconfigtypes.AuthConfig{
					RegistryToken: bearerToken,
				}, nil
			}
			if tok, ok := extraAuths[host]; ok {
				return dockerconfigtypes.AuthConfig{
					RegistryToken: tok,
				}, nil
			}
			return dockerconfigtypes.AuthConfig{}, nil
		},
	}
	return []session.Attachable{authprovider.NewDockerAuthProvider(cfg)}
}

// registryHostFromURI extracts the hostname from a container registry URI.
// Example: "123456789.dkr.ecr.us-east-1.amazonaws.com/repo:tag" returns "123456789.dkr.ecr.us-east-1.amazonaws.com".
func registryHostFromURI(uri string) string {
	if host, _, found := strings.Cut(uri, "/"); found {
		return host
	}
	return uri
}
