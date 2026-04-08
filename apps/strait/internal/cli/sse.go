package cli

import (
	"bufio"
	"context"
	"io"
	"strings"
)

// ReadEvents reads a text/event-stream response, calling fn for every "data:"
// line. It returns when r is closed or ctx is cancelled.
// Per the SSE spec each event may consist of multiple "data:" lines — this
// implementation calls fn once per data line (sufficient for our build-log
// streaming where each event is a single JSON object on one line).
func ReadEvents(ctx context.Context, r io.Reader, fn func(data []byte)) error {
	scanner := bufio.NewScanner(r)
	// Expand the default 64 KB buffer to handle large log lines.
	scanner.Buffer(make([]byte, 256*1024), 256*1024)

	done := make(chan error, 1)
	go func() {
		for scanner.Scan() {
			line := scanner.Text()
			if after, ok := strings.CutPrefix(line, "data:"); ok {
				data := strings.TrimSpace(after)
				if data != "" {
					fn([]byte(data))
				}
			}
		}
		done <- scanner.Err()
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-done:
		return err
	}
}
