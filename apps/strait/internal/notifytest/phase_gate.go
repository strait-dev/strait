package notifytest

import "strings"

// KnownExternalFailures is a curated list of known environment issues that are
// considered non-blocking in phase-gate mode 2.
var KnownExternalFailures = []string{
	"rootless Docker not found",
	"connect: connection refused",
	"dial tcp 127.0.0.1",
	"k8s",
}

// IsKnownExternalFailure reports whether the command output contains one of the
// known non-blocking environment failure signatures.
func IsKnownExternalFailure(output string) bool {
	if strings.TrimSpace(output) == "" {
		return false
	}
	lower := strings.ToLower(output)
	for _, signature := range KnownExternalFailures {
		if strings.Contains(lower, strings.ToLower(signature)) {
			return true
		}
	}
	return false
}

// IsNotifyScopedCommand reports whether a command is considered part of notify
// scoped validation.
func IsNotifyScopedCommand(command string) bool {
	lower := strings.ToLower(strings.TrimSpace(command))
	if lower == "" {
		return false
	}

	scopedFragments := []string{
		"internal/api",
		"internal/scheduler",
		"internal/store",
		"notify",
		"packages/inbox",
	}
	for _, fragment := range scopedFragments {
		if strings.Contains(lower, fragment) {
			return true
		}
	}
	return false
}
