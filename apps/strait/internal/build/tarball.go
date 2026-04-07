// Package build provides utilities for code-first deployment builds:
// tarball security validation, per-runtime Dockerfile generation, and
// deployment manifest parsing.
package build

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

const (
	// MaxTarballBytes is the maximum compressed tarball size allowed on upload.
	MaxTarballBytes = 256 * 1024 * 1024 // 256 MB

	// MaxUncompressedBytes is the maximum total uncompressed size of all entries.
	// Prevents decompression bombs.
	MaxUncompressedBytes = 1 * 1024 * 1024 * 1024 // 1 GB

	// MaxFileCount is the maximum number of entries in the tarball.
	MaxFileCount = 50_000

	// MaxSingleFileBytes is the maximum size of any single entry.
	MaxSingleFileBytes = 100 * 1024 * 1024 // 100 MB

)

// TarballError is returned when the tarball fails a security or size check.
type TarballError struct {
	Reason string
	Entry  string
}

func (e *TarballError) Error() string {
	if e.Entry != "" {
		return fmt.Sprintf("tarball validation failed: %s (entry: %q)", e.Reason, e.Entry)
	}
	return "tarball validation failed: " + e.Reason
}

// ValidateTarball reads a gzipped tar stream and checks for:
//   - Path traversal (../ in entry names)
//   - Absolute paths
//   - Symlink escapes (link targets that escape the archive root)
//   - Zip bombs (total uncompressed size > MaxUncompressedBytes)
//   - Too many files (> MaxFileCount)
//   - Oversized individual entries (> MaxSingleFileBytes)
//
// The reader is consumed in a single pass; the caller does not need to seek.
// Returns nil if the tarball is safe.
func ValidateTarball(r io.Reader) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return &TarballError{Reason: "invalid gzip archive: " + err.Error()}
	}
	defer gz.Close()

	tr := tar.NewReader(gz)

	var totalUncompressed int64
	var fileCount int

	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return &TarballError{Reason: "corrupted archive: " + err.Error()}
		}

		fileCount++
		if fileCount > MaxFileCount {
			return &TarballError{
				Reason: fmt.Sprintf("too many entries (max %d); possible zip bomb", MaxFileCount),
			}
		}

		name := filepath.Clean(hdr.Name)

		// Reject absolute paths.
		if filepath.IsAbs(name) {
			return &TarballError{Reason: "absolute path not allowed", Entry: hdr.Name}
		}

		// Reject path traversal.
		if strings.HasPrefix(name, "..") || strings.Contains(name, "/..") || strings.Contains(name, "../") {
			return &TarballError{Reason: "path traversal detected", Entry: hdr.Name}
		}

		// Reject symlinks that point outside the archive root.
		if hdr.Typeflag == tar.TypeSymlink || hdr.Typeflag == tar.TypeLink {
			if err := validateLinkTarget(hdr.Name, hdr.Linkname); err != nil {
				return err
			}
		}

		// Enforce per-entry size limit declared in the header.
		if hdr.Size > MaxSingleFileBytes {
			return &TarballError{
				Reason: fmt.Sprintf("entry exceeds max single-file size (%d MB)", MaxSingleFileBytes/1024/1024),
				Entry:  hdr.Name,
			}
		}

		// Read the entry body to count actual bytes. We limit to
		// MaxUncompressedBytes+1 so we detect the bomb in a single Read.
		remaining := MaxUncompressedBytes - totalUncompressed + 1
		n, err := io.Copy(io.Discard, io.LimitReader(tr, remaining))
		if err != nil {
			return &TarballError{Reason: "failed to read entry: " + err.Error(), Entry: hdr.Name}
		}
		totalUncompressed += n

		if totalUncompressed > MaxUncompressedBytes {
			return &TarballError{
				Reason: fmt.Sprintf("uncompressed size exceeds limit (%d GB); possible zip bomb", MaxUncompressedBytes/1024/1024/1024),
			}
		}
	}

	return nil
}

// validateLinkTarget ensures a symlink or hard link target cannot escape the
// archive root. Only the fully-resolved path (relative to the entry's directory)
// is checked — a target of "../sibling.py" from "sub/link" resolves to
// "sibling.py" which is within the root and must be allowed.
func validateLinkTarget(entryName, target string) error {
	if filepath.IsAbs(target) {
		return &TarballError{Reason: "symlink to absolute path not allowed", Entry: entryName}
	}

	// Resolve the target relative to the directory that contains the link entry.
	entryDir := filepath.Dir(entryName)
	resolved := filepath.Clean(filepath.Join(entryDir, target))

	// If the resolved path starts with "..", it has escaped the archive root.
	if strings.HasPrefix(resolved, "..") {
		return &TarballError{
			Reason: fmt.Sprintf("symlink escape detected: %q → %q (resolves outside archive root)", entryName, target),
		}
	}

	return nil
}
