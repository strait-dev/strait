package build

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"strings"
	"testing"
)

// makeTarball builds an in-memory gzipped tar archive from a list of (name, content) pairs.
//
// Special sentinel values for content:
//   - "SYMLINK:target" → creates a symlink entry pointing to target
//   - "HARDLINK:target" → creates a hard link entry pointing to target
//
// t may be nil when creating seed data for fuzz targets.
func makeTarball(t *testing.T, entries []struct{ name, content string }) []byte {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	for _, e := range entries {
		switch {
		case strings.HasPrefix(e.content, "SYMLINK:"):
			target := strings.TrimPrefix(e.content, "SYMLINK:")
			hdr := &tar.Header{Name: e.name, Typeflag: tar.TypeSymlink, Linkname: target}
			if err := tw.WriteHeader(hdr); err != nil && t != nil {
				t.Fatalf("write symlink header %q: %v", e.name, err)
			}

		case strings.HasPrefix(e.content, "HARDLINK:"):
			target := strings.TrimPrefix(e.content, "HARDLINK:")
			hdr := &tar.Header{Name: e.name, Typeflag: tar.TypeLink, Linkname: target}
			if err := tw.WriteHeader(hdr); err != nil && t != nil {
				t.Fatalf("write hardlink header %q: %v", e.name, err)
			}

		default:
			body := []byte(e.content)
			hdr := &tar.Header{Name: e.name, Size: int64(len(body)), Mode: 0o644}
			if err := tw.WriteHeader(hdr); err != nil && t != nil {
				t.Fatalf("write header %q: %v", e.name, err)
			}
			if _, err := tw.Write(body); err != nil && t != nil {
				t.Fatalf("write body %q: %v", e.name, err)
			}
		}
	}

	_ = tw.Close()
	_ = gw.Close()
	return buf.Bytes()
}
