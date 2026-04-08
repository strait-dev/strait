package cli

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/moby/patternmatcher"
)

// defaultIgnorePatterns are excluded from every tarball regardless of
// .straitignore / .gitignore contents.
var defaultIgnorePatterns = []string{
	".git",
	".git/**",
	"node_modules",
	"node_modules/**",
	"__pycache__",
	"__pycache__/**",
	"*.pyc",
	"*.pyo",
	".DS_Store",
	".env",
	".env.*",
	"*.log",
	"*.tmp",
	".idea",
	".vscode",
	"dist",
	"build",
	".cache",
	"coverage",
	".pytest_cache",
	".mypy_cache",
	".tox",
	"vendor",
}

// runtimeIgnorePatterns contains additional ignore patterns per runtime.
var runtimeIgnorePatterns = map[string][]string{
	"typescript": {"*.js.map", "*.d.ts", ".next", ".nuxt", "out"},
	"python":     {"*.egg-info", "*.egg", ".eggs", "venv", ".venv", "env"},
	"ruby":       {".bundle", "tmp", "log"},
	"go":         {"*.test", "*.out"},
	"rust":       {"target"},
}

// PackResult holds the SHA-256 hex and byte count for a packed tarball.
type PackResult struct {
	SHA256Hex string
	SizeBytes int64
}

// Pack writes a deterministic gzipped tar of dir to w, honouring patterns from
// .straitignore (or .gitignore as fallback) and defaultIgnorePatterns.
// It returns the SHA-256 hash and byte count of the written tarball.
func Pack(w io.Writer, dir string, extraPatterns []string) (*PackResult, error) {
	dir = filepath.Clean(dir)

	ignore, err := buildMatcher(dir, extraPatterns)
	if err != nil {
		return nil, err
	}

	// Tee through a SHA-256 hasher and a byte counter.
	hasher := sha256.New()
	counter := &byteCounter{}
	mw := io.MultiWriter(w, hasher, counter)

	gz := gzip.NewWriter(mw)
	tw := tar.NewWriter(gz)

	err = filepath.Walk(dir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		// Compute path relative to the root for tar entry names and matching.
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil // skip the root directory itself
		}

		// Normalise to forward slashes for cross-platform consistency.
		relSlash := filepath.ToSlash(rel)

		if ignore != nil {
			matched, matchErr := ignore.MatchesOrParentMatches(relSlash)
			if matchErr != nil {
				return fmt.Errorf("match pattern for %q: %w", relSlash, matchErr)
			}
			if matched {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}

		if info.IsDir() {
			return nil // directories are implicit in a tar
		}

		if !info.Mode().IsRegular() {
			return nil // skip symlinks, devices, etc.
		}

		hdr := &tar.Header{
			Name:     relSlash,
			Size:     info.Size(),
			Mode:     int64(info.Mode().Perm()),
			ModTime:  info.ModTime(),
			Typeflag: tar.TypeReg,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return fmt.Errorf("write tar header for %q: %w", relSlash, err)
		}

		f, err := os.Open(path) //nolint:gosec // G122: symlinks are skipped above; path is always a regular file under dir
		if err != nil {
			return fmt.Errorf("open %q: %w", path, err)
		}
		defer f.Close()

		if _, err := io.Copy(tw, f); err != nil {
			return fmt.Errorf("write tar entry %q: %w", relSlash, err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("close tar: %w", err)
	}
	if err := gz.Close(); err != nil {
		return nil, fmt.Errorf("close gzip: %w", err)
	}

	return &PackResult{
		SHA256Hex: hex.EncodeToString(hasher.Sum(nil)),
		SizeBytes: counter.n,
	}, nil
}

// buildMatcher constructs a patternmatcher from default patterns, runtime
// patterns (if detected), extra caller-supplied patterns, and the first of
// .straitignore / .gitignore found in dir.
func buildMatcher(dir string, extra []string) (*patternmatcher.PatternMatcher, error) {
	patterns := make([]string, 0, len(defaultIgnorePatterns)+len(extra)+32)
	patterns = append(patterns, defaultIgnorePatterns...)

	// Auto-detect runtime and add runtime-specific ignores.
	rt := detectRuntime(dir)
	if rt != "" {
		if rp, ok := runtimeIgnorePatterns[rt]; ok {
			patterns = append(patterns, rp...)
		}
	}

	patterns = append(patterns, extra...)

	// Read .straitignore or fall back to .gitignore.
	filePatterns, err := loadIgnoreFile(dir)
	if err != nil {
		return nil, err
	}
	patterns = append(patterns, filePatterns...)

	if len(patterns) == 0 {
		return nil, nil
	}
	return patternmatcher.New(patterns)
}

// loadIgnoreFile reads .straitignore in dir, falling back to .gitignore.
// Returns nil patterns (no error) if neither file exists.
func loadIgnoreFile(dir string) ([]string, error) {
	for _, name := range []string{".straitignore", ".gitignore"} {
		path := filepath.Join(dir, name)
		f, err := os.Open(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("open %s: %w", name, err)
		}
		defer f.Close()
		return parseIgnoreFile(f), nil
	}
	return nil, nil
}

// parseIgnoreFile reads a gitignore-style file and returns non-empty,
// non-comment lines.
func parseIgnoreFile(r io.Reader) []string {
	var patterns []string
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, line)
	}
	return patterns
}

// detectRuntime guesses the runtime from files present in dir.
func detectRuntime(dir string) string {
	checks := []struct {
		file    string
		runtime string
	}{
		{"package.json", "typescript"},
		{"requirements.txt", "python"},
		{"pyproject.toml", "python"},
		{"go.mod", "go"},
		{"Gemfile", "ruby"},
		{"Cargo.toml", "rust"},
	}
	for _, c := range checks {
		if _, err := os.Stat(filepath.Join(dir, c.file)); err == nil {
			return c.runtime
		}
	}
	return ""
}

// byteCounter counts bytes written through it.
type byteCounter struct{ n int64 }

func (c *byteCounter) Write(p []byte) (int, error) {
	c.n += int64(len(p))
	return len(p), nil
}
