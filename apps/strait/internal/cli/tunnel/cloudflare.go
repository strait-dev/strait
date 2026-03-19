package tunnel

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"regexp"
)

var tunnelURLPattern = regexp.MustCompile(`https://[a-zA-Z0-9-]+\.trycloudflare\.com`)

// Tunnel represents a running Cloudflare tunnel.
type Tunnel struct {
	URL    string
	cmd    *exec.Cmd
	cancel context.CancelFunc
}

// Close gracefully shuts down the tunnel process.
func (t *Tunnel) Close() error {
	if t.cancel != nil {
		t.cancel()
	}
	if t.cmd != nil && t.cmd.Process != nil {
		return t.cmd.Process.Kill()
	}
	return nil
}

// StartTunnel starts a cloudflared tunnel pointing to localhost on the given port.
func StartTunnel(ctx context.Context, localPort int) (*Tunnel, error) {
	cloudflaredPath, err := exec.LookPath("cloudflared")
	if err != nil {
		return nil, fmt.Errorf("cloudflared not found in PATH; install it:\n  macOS:   brew install cloudflare/cloudflare/cloudflared\n  Linux:   https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/downloads/\n  Windows: https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/downloads/")
	}

	tunnelCtx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(tunnelCtx, cloudflaredPath, "tunnel", "--url", fmt.Sprintf("http://localhost:%d", localPort)) //nolint:gosec // port is from trusted CLI flag

	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("start cloudflared: %w", err)
	}

	tunnel := &Tunnel{cmd: cmd, cancel: cancel}

	// Parse tunnel URL from cloudflared output
	urlChan := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			line := scanner.Text()
			if match := tunnelURLPattern.FindString(line); match != "" {
				urlChan <- match
				return
			}
		}
	}()

	select {
	case url := <-urlChan:
		tunnel.URL = url
		return tunnel, nil
	case <-tunnelCtx.Done():
		_ = tunnel.Close()
		return nil, fmt.Errorf("tunnel startup cancelled")
	}
}

// ParseTunnelURL extracts a trycloudflare.com URL from cloudflared output.
func ParseTunnelURL(line string) string {
	return tunnelURLPattern.FindString(line)
}
