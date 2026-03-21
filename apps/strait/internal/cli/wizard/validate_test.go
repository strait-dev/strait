package wizard

import (
	"strings"
	"testing"
)

func TestValidateProjectName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{name: "valid simple", input: "my-api", wantErr: ""},
		{name: "valid short", input: "a", wantErr: ""},
		{name: "valid numbers", input: "api123", wantErr: ""},
		{name: "valid with hyphens", input: "my-cool-api", wantErr: ""},
		{name: "empty", input: "", wantErr: "required"},
		{name: "too long", input: strings.Repeat("a", 129), wantErr: "at most 128"},
		{name: "max length", input: strings.Repeat("a", 128), wantErr: ""},
		{name: "leading hyphen", input: "-bad", wantErr: "cannot start or end"},
		{name: "trailing hyphen", input: "bad-", wantErr: "cannot start or end"},
		{name: "uppercase", input: "MyAPI", wantErr: "lowercase"},
		{name: "spaces", input: "my api", wantErr: "lowercase"},
		{name: "special chars", input: "my_api!", wantErr: "lowercase"},
		{name: "underscore", input: "my_api", wantErr: "lowercase"},
		{name: "consecutive hyphens", input: "my--api", wantErr: ""}, // allowed by regex, generateSlug collapses them
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateProjectName(tc.input)
			if tc.wantErr == "" && err != nil {
				t.Fatalf("expected no error, got: %v", err)
			}
			if tc.wantErr != "" && err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if tc.wantErr != "" && err != nil && !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got: %v", tc.wantErr, err)
			}
		})
	}
}

func TestValidateSlug(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{name: "valid", input: "process-payment", wantErr: ""},
		{name: "valid single char", input: "a", wantErr: ""},
		{name: "empty", input: "", wantErr: "required"},
		{name: "too long", input: strings.Repeat("a", 129), wantErr: "at most 128"},
		{name: "uppercase", input: "BadSlug", wantErr: "lowercase"},
		{name: "spaces", input: "bad slug", wantErr: "lowercase"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateSlug(tc.input)
			if tc.wantErr == "" && err != nil {
				t.Fatalf("expected no error, got: %v", err)
			}
			if tc.wantErr != "" && err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if tc.wantErr != "" && err != nil && !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got: %v", tc.wantErr, err)
			}
		})
	}
}

func TestValidateEndpoint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{name: "valid http", input: "http://localhost:3000", wantErr: ""},
		{name: "valid https", input: "https://api.example.com/jobs/process", wantErr: ""},
		{name: "valid with path", input: "http://localhost:3000/jobs/payment", wantErr: ""},
		{name: "empty", input: "", wantErr: "required"},
		{name: "no scheme", input: "example.com", wantErr: "http or https"},
		{name: "ftp scheme", input: "ftp://example.com", wantErr: "http or https"},
		{name: "javascript injection", input: "javascript:alert(1)", wantErr: "http or https"},
		{name: "too long", input: "https://example.com/" + strings.Repeat("a", 2030), wantErr: "at most 2048"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateEndpoint(tc.input)
			if tc.wantErr == "" && err != nil {
				t.Fatalf("expected no error, got: %v", err)
			}
			if tc.wantErr != "" && err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if tc.wantErr != "" && err != nil && !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got: %v", tc.wantErr, err)
			}
		})
	}
}

func TestValidateRuntime(t *testing.T) {
	t.Parallel()

	valid := []string{"node", "bun", "python", "go", "docker"}
	for _, rt := range valid {
		t.Run("valid_"+rt, func(t *testing.T) {
			t.Parallel()
			if err := ValidateRuntime(rt); err != nil {
				t.Fatalf("expected valid, got: %v", err)
			}
		})
	}

	invalid := []string{"java", "rust", ""}
	for _, rt := range invalid {
		t.Run("invalid_"+rt, func(t *testing.T) {
			t.Parallel()
			if err := ValidateRuntime(rt); err == nil {
				t.Fatalf("expected error for runtime %q", rt)
			}
		})
	}
}

func TestValidateCron(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{name: "valid 5 field", input: "*/5 * * * *", wantErr: ""},
		{name: "valid complex", input: "0 12 * * 1-5", wantErr: ""},
		{name: "valid 6 field", input: "0 */5 * * * *", wantErr: ""},
		{name: "empty is optional", input: "", wantErr: ""},
		{name: "alias hourly", input: "@hourly", wantErr: ""},
		{name: "alias daily", input: "@daily", wantErr: ""},
		{name: "alias weekly", input: "@weekly", wantErr: ""},
		{name: "alias monthly", input: "@monthly", wantErr: ""},
		{name: "alias yearly", input: "@yearly", wantErr: ""},
		{name: "too few fields", input: "* * *", wantErr: "5 or 6 fields"},
		{name: "not cron", input: "not-cron-at-all", wantErr: "5 or 6 fields"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateCron(tc.input)
			if tc.wantErr == "" && err != nil {
				t.Fatalf("expected no error, got: %v", err)
			}
			if tc.wantErr != "" && err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if tc.wantErr != "" && err != nil && !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got: %v", tc.wantErr, err)
			}
		})
	}
}

func TestValidateTimeout(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		secs    int
		wantErr string
	}{
		{name: "valid 60", secs: 60, wantErr: ""},
		{name: "valid 1", secs: 1, wantErr: ""},
		{name: "valid max", secs: 86400, wantErr: ""},
		{name: "zero", secs: 0, wantErr: "at least 1"},
		{name: "negative", secs: -1, wantErr: "at least 1"},
		{name: "too large", secs: 86401, wantErr: "at most 86400"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateTimeout(tc.secs)
			if tc.wantErr == "" && err != nil {
				t.Fatalf("expected no error, got: %v", err)
			}
			if tc.wantErr != "" && err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if tc.wantErr != "" && err != nil && !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got: %v", tc.wantErr, err)
			}
		})
	}
}

func TestValidateMaxAttempts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		n       int
		wantErr string
	}{
		{name: "valid 3", n: 3, wantErr: ""},
		{name: "valid 1", n: 1, wantErr: ""},
		{name: "valid max", n: 100, wantErr: ""},
		{name: "zero", n: 0, wantErr: "at least 1"},
		{name: "negative", n: -1, wantErr: "at least 1"},
		{name: "too large", n: 101, wantErr: "at most 100"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateMaxAttempts(tc.n)
			if tc.wantErr == "" && err != nil {
				t.Fatalf("expected no error, got: %v", err)
			}
			if tc.wantErr != "" && err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if tc.wantErr != "" && err != nil && !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got: %v", tc.wantErr, err)
			}
		})
	}
}

func TestRuntimes(t *testing.T) {
	t.Parallel()

	runtimes := Runtimes()
	if len(runtimes) != 5 {
		t.Fatalf("expected 5 runtimes, got %d", len(runtimes))
	}
	expected := map[string]bool{"node": true, "bun": true, "python": true, "go": true, "docker": true}
	for _, rt := range runtimes {
		if !expected[rt] {
			t.Fatalf("unexpected runtime: %q", rt)
		}
	}
}
