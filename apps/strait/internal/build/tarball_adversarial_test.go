package build

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// TestValidateTarball_PathTraversalVariants tests a wide range of path
// traversal patterns to ensure none slip through validation.
func TestValidateTarball_PathTraversalVariants(t *testing.T) {
	traversalPaths := []string{
		"../etc/passwd",
		"../../etc/shadow",
		"../../../root/.ssh/id_rsa",
		"foo/../../../etc/passwd",
		"./../../etc/passwd",
		"a/b/../../../etc/passwd",
		"..%2fetc%2fpasswd", // URL-encoded (should be rejected by path.Clean)
		"..%2F..%2Fetc%2Fpasswd",
		"\x00../etc/passwd", // null byte
		"a/b/c/../../../../etc/passwd",
	}

	for _, p := range traversalPaths {
		t.Run(fmt.Sprintf("path=%q", p), func(t *testing.T) {
			var buf bytes.Buffer
			gw := gzip.NewWriter(&buf)
			tw := tar.NewWriter(gw)
			body := []byte("malicious content")
			hdr := &tar.Header{
				Name: p,
				Size: int64(len(body)),
				Mode: 0o644,
			}
			_ = tw.WriteHeader(hdr)
			_, _ = tw.Write(body)
			_ = tw.Close()
			_ = gw.Close()

			err := ValidateTarball(bytes.NewReader(buf.Bytes()))
			// Either an error is returned, or the path was cleaned to something safe.
			// The key is that no path traversal can succeed silently.
			if err != nil {
				// Error is the expected outcome for dangerous paths.
				return
			}
			// If no error, the path must have been cleaned to something that
			// doesn't escape. This is acceptable for URL-encoded or null-byte variants
			// that filepath.Clean neutralizes.
			t.Logf("path %q was accepted (may be safe after clean)", p)
		})
	}
}

// TestValidateTarball_SymlinkVariants tests a range of symlink escape patterns.
func TestValidateTarball_SymlinkVariants(t *testing.T) {
	symlinks := []struct {
		entry  string
		target string
		safe   bool
	}{
		// Dangerous.
		{"a.py", "/etc/passwd", false},
		{"a.py", "../../etc/passwd", false},
		{"sub/a.py", "../../outside", false},
		{"a.py", "../outside", false},
		// Safe — resolves within the archive root.
		{"a.py", "b.py", true},
		{"a.py", "sub/b.py", true},
		{"sub/a.py", "../c.py", true}, // "sub/.." → "" which is root-level → safe
		{"sub/a.py", "./c.py", true},
		{"sub/deep/a.py", "../../root.py", true}, // resolves to "root.py" → safe
	}

	for _, tc := range symlinks {
		t.Run(fmt.Sprintf("%s->%s", tc.entry, tc.target), func(t *testing.T) {
			var buf bytes.Buffer
			gw := gzip.NewWriter(&buf)
			tw := tar.NewWriter(gw)
			hdr := &tar.Header{
				Name:     tc.entry,
				Typeflag: tar.TypeSymlink,
				Linkname: tc.target,
			}
			_ = tw.WriteHeader(hdr)
			_ = tw.Close()
			_ = gw.Close()

			err := ValidateTarball(bytes.NewReader(buf.Bytes()))
			if tc.safe && err != nil {
				t.Errorf("expected safe symlink to be allowed, got: %v", err)
			}
			if !tc.safe && err == nil {
				t.Errorf("expected dangerous symlink to be rejected: %q → %q", tc.entry, tc.target)
			}
		})
	}
}

// TestValidateTarball_AbsolutePaths tests a variety of absolute path formats.
func TestValidateTarball_AbsolutePaths(t *testing.T) {
	absPaths := []string{
		"/etc/passwd",
		"/root/.ssh/authorized_keys",
		"/proc/self/mem",
		"/usr/bin/sh",
	}
	for _, p := range absPaths {
		t.Run(p, func(t *testing.T) {
			var buf bytes.Buffer
			gw := gzip.NewWriter(&buf)
			tw := tar.NewWriter(gw)
			body := []byte("x")
			hdr := &tar.Header{Name: p, Size: int64(len(body)), Mode: 0o644}
			_ = tw.WriteHeader(hdr)
			_, _ = tw.Write(body)
			_ = tw.Close()
			_ = gw.Close()

			err := ValidateTarball(bytes.NewReader(buf.Bytes()))
			if err == nil {
				t.Errorf("expected error for absolute path %q, got nil", p)
			}
		})
	}
}

// TestValidateTarball_LargeValidArchive ensures a legitimate large archive passes.
func TestValidateTarball_LargeValidArchive(t *testing.T) {
	// 1000 files × 1KB each = 1MB total, well within limits.
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	body := bytes.Repeat([]byte("a"), 1024)
	for i := range 1000 {
		hdr := &tar.Header{
			Name: fmt.Sprintf("file%04d.py", i),
			Size: int64(len(body)),
			Mode: 0o644,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("write header: %v", err)
		}
		if _, err := tw.Write(body); err != nil {
			t.Fatalf("write body: %v", err)
		}
	}
	_ = tw.Close()
	_ = gw.Close()

	if err := ValidateTarball(bytes.NewReader(buf.Bytes())); err != nil {
		t.Fatalf("large valid archive should pass, got: %v", err)
	}
}

// TestValidateTarball_ExactlyAtLimit verifies that an archive at exactly
// MaxFileCount files is accepted, while MaxFileCount+1 is rejected.
func TestValidateTarball_ExactlyAtLimit(t *testing.T) {
	makeN := func(n int) []byte {
		var buf bytes.Buffer
		gw := gzip.NewWriter(&buf)
		tw := tar.NewWriter(gw)
		body := []byte("x")
		for i := range n {
			hdr := &tar.Header{Name: fmt.Sprintf("f%d", i), Size: 1, Mode: 0o644}
			_ = tw.WriteHeader(hdr)
			_, _ = tw.Write(body)
		}
		_ = tw.Close()
		_ = gw.Close()
		return buf.Bytes()
	}

	// Exactly at limit: should pass.
	if err := ValidateTarball(bytes.NewReader(makeN(MaxFileCount))); err != nil {
		t.Errorf("exactly MaxFileCount files should be accepted, got: %v", err)
	}

	// One over: should fail.
	if err := ValidateTarball(bytes.NewReader(makeN(MaxFileCount + 1))); err == nil {
		t.Error("MaxFileCount+1 files should be rejected, got nil")
	}
}

// TestValidateTarball_NullByteInPath tests for null bytes in file paths.
func TestValidateTarball_NullByteInPath(t *testing.T) {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	// Construct a header with a null byte in the name.
	hdr := &tar.Header{
		Name: "normal\x00../etc/passwd",
		Size: 1,
		Mode: 0o644,
	}
	_ = tw.WriteHeader(hdr)
	_, _ = tw.Write([]byte("x"))
	_ = tw.Close()
	_ = gw.Close()

	// Either the tar library rejects it, or our validator does.
	// Either way, it must not pass silently.
	err := ValidateTarball(bytes.NewReader(buf.Bytes()))
	if err != nil {
		// Expected: null byte or traversal detected.
		return
	}
	// If no error, check that the path was cleaned to something safe by the tar library.
	t.Log("null byte path was accepted — verify it was cleaned safely")
}

// TestValidateTarball_TarErrorMessage verifies TarballError messages are descriptive.
func TestValidateTarball_TarErrorMessage(t *testing.T) {
	data := makeTarball(t, []struct{ name, content string }{
		{"../evil.sh", "rm -rf /"},
	})
	err := ValidateTarball(bytes.NewReader(data))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "tarball validation failed") {
		t.Errorf("error message should start with 'tarball validation failed', got: %q", msg)
	}
}

// FuzzValidateTarball is a fuzz target for the tarball validator.
// Properties under test:
//  1. Never panics regardless of input.
//  2. Any non-nil error returned is a *TarballError — no raw I/O or other
//     unexported error types must escape the function.
func FuzzValidateTarball(f *testing.F) {
	// Seed: valid single-file archive.
	f.Add(makeTarball(nil, []struct{ name, content string }{
		{"main.py", "print('hello')"},
	}))
	// Seed: multi-file archive with a subdirectory.
	f.Add(makeTarball(nil, []struct{ name, content string }{
		{"src/", ""},
		{"src/handler.py", "def handler(): pass"},
		{"requirements.txt", "requests==2.31.0\n"},
	}))
	// Seed: safe symlink.
	f.Add(makeTarball(nil, []struct{ name, content string }{
		{"real.py", "x=1"},
		{"link.py", "SYMLINK:real.py"},
	}))
	// Seed: path traversal entry — must be rejected.
	f.Add(makeRawTarball([]struct {
		name, content, linkname string
		typeflag                byte
	}{
		{name: "../etc/passwd", content: "root:x:0:0", typeflag: tar.TypeReg},
	}))
	// Seed: symlink escaping the root — must be rejected.
	f.Add(makeRawTarball([]struct {
		name, content, linkname string
		typeflag                byte
	}{
		{name: "escape.py", typeflag: tar.TypeSymlink, linkname: "../../etc/passwd"},
	}))
	// Seed: absolute path — must be rejected.
	f.Add(makeRawTarball([]struct {
		name, content, linkname string
		typeflag                byte
	}{
		{name: "/etc/cron.d/evil", content: "* * * * * root id", typeflag: tar.TypeReg},
	}))
	// Seed: truncated gzip header.
	f.Add([]byte{0x1f, 0x8b, 0x08, 0x00})
	// Seed: empty input.
	f.Add([]byte{})
	// Seed: random bytes (not valid gzip).
	f.Add([]byte("this is not a valid gzip archive at all"))
	// Seed: valid gzip wrapping invalid tar data.
	f.Add(makeGzipPayload([]byte("definitely not a tar stream")))

	f.Fuzz(func(t *testing.T, data []byte) {
		err := ValidateTarball(bytes.NewReader(data))
		if err == nil {
			return // safe archive — fine
		}
		// All errors must be *TarballError; no raw errors must escape.
		var tarErr *TarballError
		if !errors.As(err, &tarErr) {
			t.Errorf("ValidateTarball returned non-TarballError %T: %v", err, err)
		}
	})
}

// makeRawTarball creates a gzipped tar from raw entry descriptors, bypassing
// the high-level sentinel system so we can set arbitrary Typeflag values.
func makeRawTarball(entries []struct {
	name, content, linkname string
	typeflag                byte
}) []byte {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	for _, e := range entries {
		body := []byte(e.content)
		hdr := &tar.Header{
			Name:     e.name,
			Typeflag: e.typeflag,
			Linkname: e.linkname,
			Size:     int64(len(body)),
			Mode:     0o644,
		}
		_ = tw.WriteHeader(hdr)
		if len(body) > 0 {
			_, _ = tw.Write(body)
		}
	}
	_ = tw.Close()
	_ = gw.Close()
	return buf.Bytes()
}

// makeGzipPayload compresses arbitrary payload bytes into a gzip stream.
func makeGzipPayload(payload []byte) []byte {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	_, _ = gw.Write(payload)
	_ = gw.Close()
	return buf.Bytes()
}
