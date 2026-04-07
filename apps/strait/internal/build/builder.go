package build

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
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
	"strait/internal/registry"
)

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
//  6. Returns the image URI + digest
type Builder struct {
	buildkitAddr string
	objectStore  objectstore.ObjectStore
	registry     registry.ContainerRegistry
	cacheEnabled bool
	timeout      time.Duration
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

// Build runs the full build pipeline for a single deployment and returns the result.
// Logs contain stdout+stderr captured from BuildKit vertex logs.
func (b *Builder) Build(ctx context.Context, d *domain.CodeDeployment) (*BuildResult, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "build.Builder.Build")
	defer span.End()

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
	repoName := fmt.Sprintf("strait-jobs/%s", d.JobID)
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
	bk, err := bkclient.New(buildCtx, b.buildkitAddr)
	if err != nil {
		return nil, fmt.Errorf("connect to buildkit at %s: %w", b.buildkitAddr, err)
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
		Session: buildkitAuthSession(registryHost, authToken),
	}

	if b.cacheEnabled {
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

	// 6. Stream build status to capture logs.
	var logsBuf strings.Builder
	statusCh := make(chan *bkclient.SolveStatus)
	doneCh := make(chan struct{})
	go func() {
		defer close(doneCh)
		for status := range statusCh {
			for _, log := range status.Logs {
				logsBuf.Write(log.Data)
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

	if buildErr != nil {
		return nil, fmt.Errorf("buildkit solve: %w", buildErr)
	}

	// 7. Extract the image digest from the solve response.
	digest := resp.ExporterResponse["containerimage.digest"]

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
// credentials to BuildKit during image push.
func buildkitAuthSession(registryHost, bearerToken string) []session.Attachable {
	cfg := authprovider.DockerAuthProviderConfig{
		AuthConfigProvider: func(_ context.Context, host string, _ []string, _ authprovider.ExpireCachedAuthCheck) (dockerconfigtypes.AuthConfig, error) {
			if host == registryHost {
				return dockerconfigtypes.AuthConfig{
					RegistryToken: bearerToken,
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
