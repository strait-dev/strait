package tunnel

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"time"
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
// It waits up to 30 seconds for the tunnel URL to appear in cloudflared's output.
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

	// Parse tunnel URL from cloudflared output with a timeout to prevent
	// hanging indefinitely if cloudflared never outputs the URL.
	urlChan := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			select {
			case <-tunnelCtx.Done():
				return
			default:
			}
			line := scanner.Text()
			if match := tunnelURLPattern.FindString(line); match != "" {
				select {
				case urlChan <- match:
				case <-tunnelCtx.Done():
				}
				return
			}
		}
		// If scanner exits without finding URL (e.g., cloudflared crashes),
		// signal failure so the select below doesn't hang.
		select {
		case urlChan <- "":
		case <-tunnelCtx.Done():
		}
	}()

	const startupTimeout = 30 * time.Second
	timer := time.NewTimer(startupTimeout)
	defer timer.Stop()

	select {
	case url := <-urlChan:
		if url == "" {
			_ = tunnel.Close()
			return nil, fmt.Errorf("cloudflared exited without providing a tunnel URL")
		}
		tunnel.URL = url
		return tunnel, nil
	case <-timer.C:
		_ = tunnel.Close()
		return nil, fmt.Errorf("timed out waiting for tunnel URL after %s", startupTimeout)
	case <-tunnelCtx.Done():
		_ = tunnel.Close()
		return nil, fmt.Errorf("tunnel startup cancelled")
	}
}

// ParseTunnelURL extracts a trycloudflare.com URL from cloudflared output.
func ParseTunnelURL(line string) string {
	return tunnelURLPattern.FindString(line)
}
