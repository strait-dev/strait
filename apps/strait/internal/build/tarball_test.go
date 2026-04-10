package build

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"strings"
	"testing"
)

// Note: makeTarball is defined in testhelper_test.go.

func TestValidateTarball_ValidArchive(t *testing.T) {
	data := makeTarball(t, []struct{ name, content string }{
		{"main.py", "print('hello')"},
		{"requirements.txt", "requests==2.31.0\n"},
		{"lib/helper.py", "def helper(): pass"},
	})
	if err := ValidateTarball(bytes.NewReader(data)); err != nil {
		t.Fatalf("expected valid archive, got error: %v", err)
	}
}

func TestValidateTarball_PathTraversal(t *testing.T) {
	cases := []struct {
		name  string
		entry string
	}{
		{"dotdot_prefix", "../etc/passwd"},
		{"dotdot_middle", "foo/../../../etc/shadow"},
		{"dotdot_suffix", "lib/../../secret"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data := makeTarball(t, []struct{ name, content string }{
				{tc.entry, "bad content"},
			})
			err := ValidateTarball(bytes.NewReader(data))
			if err == nil {
				t.Fatal("expected error for path traversal, got nil")
			}
			var tarErr *TarballError
			if !isTargetError(err, &tarErr) {
				t.Fatalf("expected TarballError, got %T: %v", err, err)
			}
			if !strings.Contains(tarErr.Reason, "traversal") && !strings.Contains(tarErr.Reason, "absolute") {
				t.Errorf("expected traversal or absolute error, got: %s", tarErr.Reason)
			}
		})
	}
}

func TestValidateTarball_AbsolutePath(t *testing.T) {
	data := makeTarball(t, []struct{ name, content string }{
		{"/etc/passwd", "root:x:0:0"},
	})
	err := ValidateTarball(bytes.NewReader(data))
	if err == nil {
		t.Fatal("expected error for absolute path, got nil")
	}
}

func TestValidateTarball_SymlinkEscape(t *testing.T) {
	cases := []struct {
		name       string
		entryName  string
		linkTarget string
	}{
		{"absolute_target", "link", "/etc/passwd"},
		{"dotdot_target", "link", "../../etc/shadow"},
		{"relative_escape_from_subdir", "subdir/link", "../../outside"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data := makeTarball(t, []struct{ name, content string }{
				{tc.entryName, "SYMLINK:" + tc.linkTarget},
			})
			err := ValidateTarball(bytes.NewReader(data))
			if err == nil {
				t.Fatalf("expected error for symlink escape %q → %q, got nil", tc.entryName, tc.linkTarget)
			}
		})
	}
}

func TestValidateTarball_SymlinkWithinArchive(t *testing.T) {
	// A symlink that stays within the archive root should be allowed.
	data := makeTarball(t, []struct{ name, content string }{
		{"real.py", "x=1"},
		{"alias.py", "SYMLINK:real.py"},
		{"subdir/ref.py", "SYMLINK:../real.py"},
	})
	if err := ValidateTarball(bytes.NewReader(data)); err != nil {
		t.Fatalf("expected valid symlink to be allowed, got: %v", err)
	}
}

func TestValidateTarball_HardlinkEscape(t *testing.T) {
	data := makeTarball(t, []struct{ name, content string }{
		{"link", "HARDLINK:../../outside"},
	})
	err := ValidateTarball(bytes.NewReader(data))
	if err == nil {
		t.Fatal("expected error for hardlink escape, got nil")
	}
}

func TestValidateTarball_TooManyFiles(t *testing.T) {
	entries := make([]struct{ name, content string }, MaxFileCount+1)
	for i := range entries {
		entries[i] = struct{ name, content string }{
			name:    fmt.Sprintf("file%d.txt", i),
			content: "x",
		}
	}
	data := makeTarball(t, entries)
	err := ValidateTarball(bytes.NewReader(data))
	if err == nil {
		t.Fatal("expected error for too many files, got nil")
	}
	var tarErr *TarballError
	if !isTargetError(err, &tarErr) {
		t.Fatalf("expected TarballError, got %T", err)
	}
	if !strings.Contains(tarErr.Reason, "too many") {
		t.Errorf("expected 'too many' in error, got: %s", tarErr.Reason)
	}
}

func TestValidateTarball_ZipBombSingleFile(t *testing.T) {
	// Construct a tarball whose header claims a file is bigger than MaxSingleFileBytes.
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	hdr := &tar.Header{
		Name: "bomb.bin",
		Size: MaxSingleFileBytes + 1,
		Mode: 0o644,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("write header: %v", err)
	}
	// Write only 1 byte; the header size is what we're testing.
	if _, err := tw.Write([]byte("x")); err != nil {
		t.Fatalf("write body: %v", err)
	}
	// Don't close properly since the body is intentionally truncated.
	_ = tw.Flush()
	_ = gw.Close()

	err := ValidateTarball(bytes.NewReader(buf.Bytes()))
	if err == nil {
		t.Fatal("expected error for oversized single file, got nil")
	}
}

func TestValidateTarball_EmptyArchive(t *testing.T) {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	_ = tw.Close()
	_ = gw.Close()

	if err := ValidateTarball(bytes.NewReader(buf.Bytes())); err != nil {
		t.Fatalf("expected empty archive to be valid, got: %v", err)
	}
}

func TestValidateTarball_NotGzip(t *testing.T) {
	err := ValidateTarball(strings.NewReader("not a gzip archive"))
	if err == nil {
		t.Fatal("expected error for non-gzip input, got nil")
	}
}

func TestValidateTarball_CorruptedTar(t *testing.T) {
	// Valid gzip wrapping corrupted tar content.
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	_, _ = gw.Write([]byte("corrupted tar content that is not valid"))
	_ = gw.Close()

	err := ValidateTarball(bytes.NewReader(buf.Bytes()))
	if err == nil {
		t.Fatal("expected error for corrupted tar, got nil")
	}
}

func TestValidateTarball_DotDotInMiddleOfName(t *testing.T) {
	// A filename containing ".." but not at the start — still dangerous.
	data := makeTarball(t, []struct{ name, content string }{
		{"foo/../../etc/passwd", "root"},
	})
	err := ValidateTarball(bytes.NewReader(data))
	if err == nil {
		t.Fatal("expected error for dotdot in middle of path, got nil")
	}
}

// isTargetError unwraps err into *TarballError if possible.
func isTargetError(err error, target **TarballError) bool {
	var te *TarballError
	if strings.Contains(err.Error(), "tarball validation failed") {
		// Manually assign since errors.As on pointer-to-interface has edge cases.
		te = &TarballError{Reason: err.Error()}
		*target = te
		return true
	}
	return false
}
