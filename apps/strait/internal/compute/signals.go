package compute

import "fmt"

// ExitClassification describes the nature of a container exit based on its exit code.
type ExitClassification struct {
	ErrorClass   string // "out_of_memory", "graceful_shutdown", "segfault", "application_error", "server"
	Signal       string // "SIGKILL", "SIGTERM", "SIGSEGV", or ""
	IsOOM        bool
	HumanMessage string
}

// signalNames maps signal numbers (exit code - 128) to their names.
var signalNames = map[int]string{
	1:  "SIGHUP",
	2:  "SIGINT",
	3:  "SIGQUIT",
	4:  "SIGILL",
	5:  "SIGTRAP",
	6:  "SIGABRT",
	7:  "SIGBUS",
	8:  "SIGFPE",
	9:  "SIGKILL",
	10: "SIGUSR1",
	11: "SIGSEGV",
	12: "SIGUSR2",
	13: "SIGPIPE",
	14: "SIGALRM",
	15: "SIGTERM",
}

// ClassifyExitCode maps a container exit code to an ExitClassification.
func ClassifyExitCode(code int) ExitClassification {
	switch {
	case code == 137: // 128 + 9 (SIGKILL) — typically OOM killer
		return ExitClassification{
			ErrorClass:   "out_of_memory",
			Signal:       "SIGKILL",
			IsOOM:        true,
			HumanMessage: "container killed by OOM (SIGKILL)",
		}
	case code == 143: // 128 + 15 (SIGTERM)
		return ExitClassification{
			ErrorClass:   "graceful_shutdown",
			Signal:       "SIGTERM",
			IsOOM:        false,
			HumanMessage: "container received shutdown signal (SIGTERM)",
		}
	case code == 139: // 128 + 11 (SIGSEGV)
		return ExitClassification{
			ErrorClass:   "segfault",
			Signal:       "SIGSEGV",
			IsOOM:        false,
			HumanMessage: "container crashed with segmentation fault",
		}
	case code >= 1 && code <= 128:
		return ExitClassification{
			ErrorClass:   "application_error",
			Signal:       "",
			IsOOM:        false,
			HumanMessage: fmt.Sprintf("container exited with code %d", code),
		}
	case code > 128:
		sigNum := code - 128
		sigName := signalNames[sigNum]
		if sigName == "" {
			sigName = fmt.Sprintf("SIG%d", sigNum)
		}
		return ExitClassification{
			ErrorClass:   "server",
			Signal:       sigName,
			IsOOM:        false,
			HumanMessage: fmt.Sprintf("container killed by signal %s", sigName),
		}
	default:
		// code <= 0 (including -1 for "no exit event")
		return ExitClassification{
			ErrorClass:   "server",
			Signal:       "",
			IsOOM:        false,
			HumanMessage: fmt.Sprintf("container exited with code %d", code),
		}
	}
}
