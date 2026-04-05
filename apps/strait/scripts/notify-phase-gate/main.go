package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"strait/internal/notifytest"
)

type arrayFlags []string

func (a *arrayFlags) String() string {
	return strings.Join(*a, ",")
}

func (a *arrayFlags) Set(value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed != "" {
		*a = append(*a, trimmed)
	}
	return nil
}

func main() {
	var (
		phase string
		mode  int
		cmds  arrayFlags
	)

	flag.StringVar(&phase, "phase", "", "Phase label for logs")
	flag.IntVar(&mode, "mode", 2, "Validation mode. 2 allows known external failures for non-notify commands")
	flag.Var(&cmds, "cmd", "Command to execute. Repeat --cmd for multiple commands")
	flag.Parse()

	if len(cmds) == 0 {
		exitf("at least one --cmd is required")
	}

	fmt.Printf("notify phase gate\n")
	if phase != "" {
		fmt.Printf("phase: %s\n", phase)
	}
	fmt.Printf("mode: %d\n\n", mode)

	for _, command := range cmds {
		fmt.Printf("running: %s\n", command)
		output, err := runCommand(command)
		if output != "" {
			fmt.Println(output)
		}
		if err == nil {
			fmt.Println("result: ok")
			fmt.Println()
			continue
		}

		if mode == 2 && !notifytest.IsNotifyScopedCommand(command) && notifytest.IsKnownExternalFailure(output) {
			fmt.Printf("result: non-blocking external failure (%v)\n\n", err)
			continue
		}

		exitf("result: failed (%v)", err)
	}

	fmt.Println("phase gate: passed")
}

func runCommand(command string) (string, error) {
	cmd := exec.Command("bash", "-lc", command)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return strings.TrimSpace(out.String()), err
}

func exitf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
