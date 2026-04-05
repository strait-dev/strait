package notifytest

import "testing"

func TestIsKnownExternalFailure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		output string
		want   bool
	}{
		{name: "docker missing", output: "panic: rootless Docker not found", want: true},
		{name: "connection refused", output: "dial tcp 127.0.0.1:55739: connect: connection refused", want: true},
		{name: "empty", output: "", want: false},
		{name: "normal failure", output: "--- FAIL: TestFoo", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := IsKnownExternalFailure(tt.output); got != tt.want {
				t.Fatalf("IsKnownExternalFailure() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsNotifyScopedCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		command string
		want    bool
	}{
		{name: "notify api package", command: "cd apps/strait && go test ./internal/api -run TestNotify", want: true},
		{name: "scheduler package", command: "go test ./internal/scheduler", want: true},
		{name: "store package", command: "go test ./internal/store", want: true},
		{name: "inbox package", command: "cd packages/inbox && bun run test", want: true},
		{name: "unrelated app", command: "cd apps/website && bun run typecheck", want: false},
		{name: "blank", command: "  ", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := IsNotifyScopedCommand(tt.command); got != tt.want {
				t.Fatalf("IsNotifyScopedCommand() = %v, want %v", got, tt.want)
			}
		})
	}
}

func FuzzIsKnownExternalFailure(f *testing.F) {
	f.Add("panic: rootless Docker not found")
	f.Add("dial tcp 127.0.0.1:1234: connect: connection refused")
	f.Add("random output")

	f.Fuzz(func(t *testing.T, output string) {
		_ = IsKnownExternalFailure(output)
	})
}
