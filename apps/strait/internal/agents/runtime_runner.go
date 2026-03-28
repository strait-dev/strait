package agents

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type RuntimeEventHandler func(ctx context.Context, event RuntimeEvent) error

type RuntimeRunner interface {
	Run(ctx context.Context, envelope RuntimeDispatchEnvelope, handler RuntimeEventHandler) error
}

type CommandRuntimeOptions struct {
	Command []string
	Workdir string
}

type CommandRuntimeRunner struct {
	command []string
	workdir string
}

func NewCommandRuntimeRunner(opts CommandRuntimeOptions) *CommandRuntimeRunner {
	command := append([]string(nil), opts.Command...)
	if len(command) == 0 {
		command = []string{"bun", "run", "dev"}
	}
	workdir := opts.Workdir
	if strings.TrimSpace(workdir) == "" {
		workdir = defaultAgentsWorkdir()
	}

	return &CommandRuntimeRunner{
		command: command,
		workdir: workdir,
	}
}

func defaultAgentsWorkdir() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return ""
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "../../../../apps/agents"))
}

func (r *CommandRuntimeRunner) Run(ctx context.Context, envelope RuntimeDispatchEnvelope, handler RuntimeEventHandler) error {
	if len(r.command) == 0 {
		return fmt.Errorf("runtime command is not configured")
	}

	payload, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("marshal runtime dispatch envelope: %w", err)
	}

	//nolint:gosec // The runtime command is configured locally by the server process for local-only execution.
	cmd := exec.CommandContext(ctx, r.command[0], r.command[1:]...)
	if r.workdir != "" {
		cmd.Dir = r.workdir
	}
	cmd.Stdin = bytes.NewReader(payload)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("runtime stdout pipe: %w", err)
	}

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start runtime command: %w", err)
	}

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var event RuntimeEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			return fmt.Errorf("decode runtime event: %w", err)
		}
		if err := handler(ctx, event); err != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return fmt.Errorf("read runtime event stream: %w", err)
	}

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("runtime command failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	return nil
}
