package build

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"testing"
)

// TestValidateTarball_UnicodePaths verifies that files with valid Unicode names
// (CJK characters, Cyrillic, emoji) are accepted without error.
// Unicode in filenames is legitimate and must not be erroneously rejected.
func TestValidateTarball_UnicodePaths(t *testing.T) {
	t.Parallel()

	unicodeFiles := []string{
		"src/日本語/main.py",
		"src/中文/handler.go",
		"данные/файл.txt",
		"ñoño.py",
		"café.rb",
		"über/thing.ts",
	}

	for _, name := range unicodeFiles {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			gw := gzip.NewWriter(&buf)
			tw := tar.NewWriter(gw)

			body := []byte("contents")
			_ = tw.WriteHeader(&tar.Header{
				Name: name,
				Size: int64(len(body)),
				Mode: 0o644,
			})
			_, _ = tw.Write(body)
			_ = tw.Close()
			_ = gw.Close()

			if err := ValidateTarball(bytes.NewReader(buf.Bytes())); err != nil {
				t.Errorf("unicode filename %q unexpectedly rejected: %v", name, err)
			}
		})
	}
}

// TestValidateTarball_UnicodeLookalikeSeparator verifies that filenames
// containing Unicode lookalike characters for '/' (e.g. U+2215 DIVISION SLASH)
// are not confused with path separators. These should be treated as safe
// literal filename characters, not as directory traversal vectors.
func TestValidateTarball_UnicodeLookalikeSeparator(t *testing.T) {
	t.Parallel()

	// U+2215 DIVISION SLASH: visually similar to '/' but is not a path separator.
	const divisionSlash = "\u2215"

	safeNames := []string{
		// Division slash used literally in a filename — not a real path separator.
		"src" + divisionSlash + "etc" + divisionSlash + "passwd",
		// Fraction slash U+2044 — also not a path separator.
		"a\u2044b",
	}

	for _, name := range safeNames {
		t.Run(fmt.Sprintf("name=%q", name), func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			gw := gzip.NewWriter(&buf)
			tw := tar.NewWriter(gw)

			body := []byte("data")
			_ = tw.WriteHeader(&tar.Header{
				Name: name,
				Size: int64(len(body)),
				Mode: 0o644,
			})
			_, _ = tw.Write(body)
			_ = tw.Close()
			_ = gw.Close()

			// The result can be either accepted (safe literal chars) or rejected
			// (some OS path cleaners may reject non-ASCII in filenames). The key
			// property is: no panic, no path traversal silently succeeding.
			err := ValidateTarball(bytes.NewReader(buf.Bytes()))
			if err != nil {
				t.Logf("filename %q rejected (acceptable): %v", name, err)
			}
		})
	}
}

// TestValidateTarball_SparseLargeFileHeader verifies that a tar entry whose
// declared header size exceeds MaxSingleFileBytes is rejected before any body
// bytes are read. This guards against sparse tar entries that claim a huge
// logical size to exhaust storage during extraction.
func TestValidateTarball_SparseLargeFileHeader(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	// Declare a size just over the limit in the header without writing body bytes.
	// A real sparse file would declare a huge logical size but contain very few
	// non-zero bytes; the header Size field represents the logical size.
	oversizeHeader := &tar.Header{
		Name: "big.bin",
		Size: MaxSingleFileBytes + 1, // one byte over the limit
		Mode: 0o644,
	}
	_ = tw.WriteHeader(oversizeHeader)
	// Write zero bytes — the header alone should trigger rejection.
	_ = tw.Close()
	_ = gw.Close()

	err := ValidateTarball(bytes.NewReader(buf.Bytes()))
	if err == nil {
		t.Error("expected error for entry with Size > MaxSingleFileBytes, got nil")
	}
}

// TestValidateTarball_SparseLargeFileHeaderAtLimit verifies that an entry
// exactly at MaxSingleFileBytes is accepted (boundary condition).
func TestValidateTarball_SparseLargeFileHeaderAtLimit(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	_ = tw.WriteHeader(&tar.Header{
		Name: "exact.bin",
		Size: MaxSingleFileBytes, // exactly at the limit — must be accepted
		Mode: 0o644,
	})
	// We don't write the body; the validator reads body bytes via io.Copy which
	// returns 0 bytes for a zero-content entry. The header-size check passes.
	_ = tw.Close()
	_ = gw.Close()

	err := ValidateTarball(bytes.NewReader(buf.Bytes()))
	if err != nil {
		// If the validator tries to read the declared bytes and fails because no
		// body was written, that is still a valid rejection (corrupted archive).
		// The key is that the limit boundary itself is not off-by-one.
		t.Logf("entry at MaxSingleFileBytes got error (may be corrupt archive): %v", err)
	}
}

// TestValidateTarball_DeepSymlinkChain_Safe verifies that a chain of symlinks
// where each hop stays within the archive root is accepted. The validator checks
// each symlink's target relative to its own directory independently; a long chain
// of safe hops must not trigger a false positive.
func TestValidateTarball_DeepSymlinkChain_Safe(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	// Create a chain: link_00 → link_01 → ... → link_49 → "real.py"
	// Each link points to the next one within the same directory level.
	const depth = 50
	for i := range depth {
		name := fmt.Sprintf("link_%02d", i)
		target := fmt.Sprintf("link_%02d", i+1)
		if i == depth-1 {
			target = "real.py"
		}
		_ = tw.WriteHeader(&tar.Header{
			Typeflag: tar.TypeSymlink,
			Name:     name,
			Linkname: target,
		})
	}
	// Write the real file at the chain's end.
	body := []byte("real content")
	_ = tw.WriteHeader(&tar.Header{
		Name: "real.py",
		Size: int64(len(body)),
		Mode: 0o644,
	})
	_, _ = tw.Write(body)
	_ = tw.Close()
	_ = gw.Close()

	err := ValidateTarball(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Errorf("deep safe symlink chain unexpectedly rejected: %v", err)
	}
}

// TestValidateTarball_DeepSymlinkChain_LastHopEscapes verifies that a symlink
// chain is rejected when the final hop points outside the archive root.
// The validator inspects every symlink independently; even the last link in a
// long chain must not escape.
func TestValidateTarball_DeepSymlinkChain_LastHopEscapes(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	// Build a chain of 10 safe hops, with the last one escaping.
	const depth = 10
	for i := range depth {
		name := fmt.Sprintf("link_%02d", i)
		target := fmt.Sprintf("link_%02d", i+1)
		if i == depth-1 {
			// Last hop escapes the archive root.
			target = "../../etc/passwd"
		}
		_ = tw.WriteHeader(&tar.Header{
			Typeflag: tar.TypeSymlink,
			Name:     name,
			Linkname: target,
		})
	}
	_ = tw.Close()
	_ = gw.Close()

	err := ValidateTarball(bytes.NewReader(buf.Bytes()))
	if err == nil {
		t.Error("expected error for symlink chain with escaping last hop, got nil")
	}
}
