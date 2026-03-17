package compute

import "testing"

func TestClassifyExitCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		code       int
		errorClass string
		signal     string
		isOOM      bool
		wantMsg    string
	}{
		{code: 0, errorClass: "server", signal: "", isOOM: false, wantMsg: "container exited with code 0"},
		{code: 1, errorClass: "application_error", signal: "", isOOM: false, wantMsg: "container exited with code 1"},
		{code: 2, errorClass: "application_error", signal: "", isOOM: false, wantMsg: "container exited with code 2"},
		{code: 126, errorClass: "application_error", signal: "", isOOM: false, wantMsg: "container exited with code 126"},
		{code: 127, errorClass: "application_error", signal: "", isOOM: false, wantMsg: "container exited with code 127"},
		{code: 128, errorClass: "application_error", signal: "", isOOM: false, wantMsg: "container exited with code 128"},
		{code: 137, errorClass: "out_of_memory", signal: "SIGKILL", isOOM: true, wantMsg: "container killed by OOM (SIGKILL)"},
		{code: 139, errorClass: "segfault", signal: "SIGSEGV", isOOM: false, wantMsg: "container crashed with segmentation fault"},
		{code: 143, errorClass: "graceful_shutdown", signal: "SIGTERM", isOOM: false, wantMsg: "container received shutdown signal (SIGTERM)"},
		{code: 129, errorClass: "server", signal: "SIGHUP", isOOM: false, wantMsg: "container killed by signal SIGHUP"},
		{code: 130, errorClass: "server", signal: "SIGINT", isOOM: false, wantMsg: "container killed by signal SIGINT"},
		{code: 131, errorClass: "server", signal: "SIGQUIT", isOOM: false, wantMsg: "container killed by signal SIGQUIT"},
		{code: 132, errorClass: "server", signal: "SIGILL", isOOM: false, wantMsg: "container killed by signal SIGILL"},
		{code: 134, errorClass: "server", signal: "SIGABRT", isOOM: false, wantMsg: "container killed by signal SIGABRT"},
		{code: 136, errorClass: "server", signal: "SIGFPE", isOOM: false, wantMsg: "container killed by signal SIGFPE"},
		{code: 138, errorClass: "server", signal: "SIGUSR1", isOOM: false, wantMsg: "container killed by signal SIGUSR1"},
		{code: 140, errorClass: "server", signal: "SIGUSR2", isOOM: false, wantMsg: "container killed by signal SIGUSR2"},
		{code: 141, errorClass: "server", signal: "SIGPIPE", isOOM: false, wantMsg: "container killed by signal SIGPIPE"},
		{code: 142, errorClass: "server", signal: "SIGALRM", isOOM: false, wantMsg: "container killed by signal SIGALRM"},
		{code: 255, errorClass: "server", signal: "SIG127", isOOM: false, wantMsg: "container killed by signal SIG127"},
		{code: -1, errorClass: "server", signal: "", isOOM: false, wantMsg: "container exited with code -1"},
	}

	for _, tt := range tests {
		c := ClassifyExitCode(tt.code)
		if c.ErrorClass != tt.errorClass {
			t.Errorf("code %d: ErrorClass = %q, want %q", tt.code, c.ErrorClass, tt.errorClass)
		}
		if c.Signal != tt.signal {
			t.Errorf("code %d: Signal = %q, want %q", tt.code, c.Signal, tt.signal)
		}
		if c.IsOOM != tt.isOOM {
			t.Errorf("code %d: IsOOM = %v, want %v", tt.code, c.IsOOM, tt.isOOM)
		}
		if c.HumanMessage != tt.wantMsg {
			t.Errorf("code %d: HumanMessage = %q, want %q", tt.code, c.HumanMessage, tt.wantMsg)
		}
	}
}

func TestClassifyExitCode_AllSignalCodes(t *testing.T) {
	t.Parallel()

	// All signal-based exit codes (129-255) should have ErrorClass "server" or a specific override.
	for code := 129; code <= 255; code++ {
		c := ClassifyExitCode(code)
		switch code {
		case 137:
			if c.ErrorClass != "out_of_memory" {
				t.Errorf("code %d: expected out_of_memory, got %s", code, c.ErrorClass)
			}
		case 139:
			if c.ErrorClass != "segfault" {
				t.Errorf("code %d: expected segfault, got %s", code, c.ErrorClass)
			}
		case 143:
			if c.ErrorClass != "graceful_shutdown" {
				t.Errorf("code %d: expected graceful_shutdown, got %s", code, c.ErrorClass)
			}
		default:
			if c.ErrorClass != "server" {
				t.Errorf("code %d: expected server, got %s", code, c.ErrorClass)
			}
		}
		if c.Signal == "" {
			t.Errorf("code %d: expected non-empty Signal", code)
		}
	}
}

func TestClassifyExitCode_AppErrorRange(t *testing.T) {
	t.Parallel()

	// All codes 1-128 should be application_error.
	for code := 1; code <= 128; code++ {
		c := ClassifyExitCode(code)
		if c.ErrorClass != "application_error" {
			t.Errorf("code %d: expected application_error, got %s", code, c.ErrorClass)
		}
		if c.Signal != "" {
			t.Errorf("code %d: expected empty signal, got %s", code, c.Signal)
		}
		if c.IsOOM {
			t.Errorf("code %d: expected IsOOM=false", code)
		}
	}
}
