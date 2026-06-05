package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"strait/internal/billing"
	"strait/internal/config"
	"strait/internal/domain"

	"github.com/spf13/cobra"
)

// root.go

func TestNewRootCommand_Structure(t *testing.T) {
	t.Parallel()

	cmd := newRootCommand()
	if cmd.Use != "strait" {
		t.Fatalf("root command Use = %q, want %q", cmd.Use, "strait")
	}
	if !cmd.SilenceUsage {
		t.Fatal("expected SilenceUsage to be true")
	}
	if !cmd.SilenceErrors {
		t.Fatal("expected SilenceErrors to be true")
	}

	subs := map[string]bool{}
	for _, sub := range cmd.Commands() {
		subs[sub.Name()] = true
	}
	for _, name := range []string{"serve", "server", "migrate", "version", "health"} {
		if !subs[name] {
			t.Fatalf("expected subcommand %q to be registered", name)
		}
	}
}

func TestContainsModeFlag(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want bool
	}{
		{name: "empty args", args: nil, want: false},
		{name: "no mode flag", args: []string{"--verbose", "--port", "8080"}, want: false},
		{name: "standalone mode flag", args: []string{"--mode", "all"}, want: true},
		{name: "mode flag with equals", args: []string{"--mode=worker"}, want: true},
		{name: "mode flag in the middle", args: []string{"--verbose", "--mode", "api", "--port", "8080"}, want: true},
		{name: "mode flag with equals in the middle", args: []string{"--verbose", "--mode=api"}, want: true},
		{name: "partial match should not match", args: []string{"--modex"}, want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := containsModeFlag(tc.args)
			if got != tc.want {
				t.Fatalf("containsModeFlag(%v) = %v, want %v", tc.args, got, tc.want)
			}
		})
	}
}

func TestNewServeCommand(t *testing.T) {
	t.Parallel()

	cmd := newServeCommand()
	if cmd.Use != "serve" {
		t.Fatalf("serve command Use = %q, want %q", cmd.Use, "serve")
	}

	f := cmd.Flags().Lookup("mode")
	if f == nil {
		t.Fatal("expected --mode flag to be registered on serve command")
		return
	}
	if f.DefValue != "" {
		t.Fatalf("--mode default = %q, want empty string", f.DefValue)
	}
}

func TestValidateBillingRedisDependency_FailsClosedWhenEnforcementEnabled(t *testing.T) {
	t.Parallel()

	err := validateBillingRedisDependency(&config.Config{BillingEnforcementEnabled: true}, nil)
	if err == nil {
		t.Fatal("expected billing enforcement without Redis to fail")
	}
	if !strings.Contains(err.Error(), "billing enforcement requires Redis") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateBillingEnforcerDependency(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		cfg      *config.Config
		enforcer *billing.Enforcer
		want     string
	}{
		{
			name: "nil config allowed",
		},
		{
			name: "billing enforcement disabled allows nil enforcer",
			cfg:  &config.Config{},
		},
		{
			name:     "billing enforcement enabled with enforcer allowed",
			cfg:      &config.Config{BillingEnforcementEnabled: true},
			enforcer: &billing.Enforcer{},
		},
		{
			name: "billing enforcement enabled fails without enforcer",
			cfg:  &config.Config{BillingEnforcementEnabled: true},
			want: "billing enforcement requires billing enforcer",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateBillingEnforcerDependency(tt.cfg, tt.enforcer)
			if tt.want == "" {
				if err != nil {
					t.Fatalf("validateBillingEnforcerDependency() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("validateBillingEnforcerDependency() error = %v, want %s", err, tt.want)
			}
		})
	}
}

func TestValidateCloudBillingConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		edition domain.Edition
		cfg     *config.Config
		want    string
	}{
		{
			name:    "community production without billing allowed",
			edition: domain.EditionCommunity,
			cfg:     &config.Config{SentryEnvironment: "production"},
		},
		{
			name:    "cloud development without billing allowed",
			edition: domain.EditionCloud,
			cfg:     &config.Config{SentryEnvironment: "development"},
		},
		{
			name:    "cloud test without billing allowed",
			edition: domain.EditionCloud,
			cfg:     &config.Config{SentryEnvironment: "test"},
		},
		{
			name:    "cloud production requires billing enforcement flag",
			edition: domain.EditionCloud,
			cfg:     &config.Config{SentryEnvironment: "production"},
			want:    "BILLING_ENFORCEMENT_ENABLED",
		},
		{
			name:    "cloud production requires stripe webhook secret",
			edition: domain.EditionCloud,
			cfg: &config.Config{
				SentryEnvironment:         "production",
				BillingEnforcementEnabled: true,
			},
			want: "STRIPE_WEBHOOK_SECRET",
		},
		{
			name:    "cloud production with billing enforcement configured",
			edition: domain.EditionCloud,
			cfg: &config.Config{
				SentryEnvironment:         "production",
				BillingEnforcementEnabled: true,
				StripeWebhookSecret:       "whsec_test",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateCloudBillingConfig(tt.edition, tt.cfg)
			if tt.want == "" {
				if err != nil {
					t.Fatalf("validateCloudBillingConfig() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("validateCloudBillingConfig() error = %v, want %s", err, tt.want)
			}
		})
	}
}

type auditDMLStartupChecker struct {
	called bool
}

func (c *auditDMLStartupChecker) AuditEventsDMLRestricted(context.Context) (bool, error) {
	c.called = true
	return true, nil
}

func TestLogAuditDMLGuardStartup_UsesDMLRestrictedInterface(t *testing.T) {
	t.Parallel()

	checker := &auditDMLStartupChecker{}
	logAuditDMLGuardStartup(context.Background(), checker, nil)
	if !checker.called {
		t.Fatal("expected startup audit DML guard to call AuditEventsDMLRestricted")
	}
}

func TestNewVersionCommand(t *testing.T) {
	t.Parallel()

	cmd := newVersionCommand()
	if cmd.Use != "version" {
		t.Fatalf("version command Use = %q, want %q", cmd.Use, "version")
	}

	f := cmd.Flags().Lookup("short")
	if f == nil {
		t.Fatal("expected --short flag to be registered on version command")
		return
	}
	if f.DefValue != "false" {
		t.Fatalf("--short default = %q, want %q", f.DefValue, "false")
	}
}

func TestNewVersionCommand_Execute(t *testing.T) {
	t.Parallel()

	cmd := newVersionCommand()
	cmd.SetArgs([]string{"--short"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("version --short returned error: %v", err)
	}
}

func TestNewVersionCommand_ExecuteLong(t *testing.T) {
	t.Parallel()

	cmd := newVersionCommand()
	cmd.SetArgs(nil)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("version (long) returned error: %v", err)
	}
}

func TestNormalizeLegacyArgs_Empty(t *testing.T) {
	t.Parallel()
	got := normalizeLegacyArgs(nil)
	if got != nil {
		t.Fatalf("normalizeLegacyArgs(nil) = %v, want nil", got)
	}
}

func TestNormalizeLegacyArgs_AllSubcommands(t *testing.T) {
	t.Parallel()

	for _, sub := range []string{"serve", "server", "migrate", "version", "health", "help"} {
		args := []string{sub, "--verbose"}
		got := normalizeLegacyArgs(args)
		if got[0] != sub {
			t.Fatalf("normalizeLegacyArgs(%v)[0] = %q, want %q", args, got[0], sub)
		}
	}
}

func TestNormalizeLegacyArgs_UnknownNonFlag(t *testing.T) {
	t.Parallel()

	got := normalizeLegacyArgs([]string{"unknown-cmd"})
	if len(got) != 1 || got[0] != "unknown-cmd" {
		t.Fatalf("normalizeLegacyArgs([unknown-cmd]) = %v, want [unknown-cmd]", got)
	}
}

// migrate.go: parsePositiveInt

func TestParsePositiveInt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    int
		wantErr bool
	}{
		{name: "valid small number", input: "1", want: 1},
		{name: "valid larger number", input: "42", want: 42},
		{name: "valid large number", input: "9999", want: 9999},
		{name: "zero", input: "0", wantErr: true},
		{name: "negative", input: "-3", wantErr: true},
		{name: "non-numeric", input: "abc", wantErr: true},
		{name: "empty string", input: "", wantErr: true},
		{name: "float string", input: "1.5", wantErr: true},
		{name: "trailing suffix", input: "1xyz", wantErr: true},
		{name: "leading spaces with number", input: " 5", want: 5},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := parsePositiveInt(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parsePositiveInt(%q) = %d, nil; want error", tc.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parsePositiveInt(%q) returned unexpected error: %v", tc.input, err)
			}
			if got != tc.want {
				t.Fatalf("parsePositiveInt(%q) = %d, want %d", tc.input, got, tc.want)
			}
		})
	}
}

func TestValidateMigrationDatabaseURLRejectsDisableSSLInProduction(t *testing.T) {
	t.Parallel()

	err := validateMigrationDatabaseURL("postgres://localhost/strait?sslmode=disable", "production")
	if err == nil {
		t.Fatal("expected production sslmode=disable to be rejected")
	}
	if !strings.Contains(err.Error(), "sslmode=disable") {
		t.Fatalf("error = %v, want sslmode=disable", err)
	}
}

func TestValidateMigrationDatabaseURLAllowsDisableSSLInDevelopment(t *testing.T) {
	t.Parallel()

	if err := validateMigrationDatabaseURL("postgres://localhost/strait?sslmode=disable", "development"); err != nil {
		t.Fatalf("development sslmode=disable should be allowed: %v", err)
	}
	if err := validateMigrationDatabaseURL("postgres://localhost/strait?sslmode=disable", ""); err != nil {
		t.Fatalf("empty environment should default to development: %v", err)
	}
}

// migrate.go: sanitizeMigrationName

func TestSanitizeMigrationName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "simple name", input: "add_users", want: "add_users"},
		{name: "spaces become underscores", input: "add users table", want: "add_users_table"},
		{name: "special chars removed", input: "add-users!@#table", want: "add_users_table"},
		{name: "unicode removed", input: "add_tabl\u00e9", want: "add_tabl"},
		{name: "multiple consecutive specials", input: "add---users", want: "add_users"},
		{name: "leading special chars stripped", input: "---add_users", want: "add_users"},
		{name: "trailing special chars stripped", input: "add_users---", want: "add_users"},
		{name: "empty input", input: "", want: ""},
		{name: "only special chars", input: "!@#$%^&*()", want: ""},
		{name: "mixed case preserved", input: "AddUsersTable", want: "AddUsersTable"},
		{name: "numbers preserved", input: "migration_001", want: "migration_001"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := sanitizeMigrationName(tc.input)
			if got != tc.want {
				t.Fatalf("sanitizeMigrationName(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// migrate.go: nextMigrationVersion

func TestNextMigrationVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		files []string
		want  int
	}{
		{
			name:  "empty directory",
			files: nil,
			want:  1,
		},
		{
			name:  "single migration pair",
			files: []string{"000001_init.up.sql", "000001_init.down.sql"},
			want:  2,
		},
		{
			name: "multiple migrations",
			files: []string{
				"000001_init.up.sql", "000001_init.down.sql",
				"000002_add_users.up.sql", "000002_add_users.down.sql",
				"000005_add_jobs.up.sql", "000005_add_jobs.down.sql",
			},
			want: 6,
		},
		{
			name:  "ignores non-migration files",
			files: []string{"000003_init.up.sql", "README.md", ".gitkeep"},
			want:  4,
		},
		{
			name:  "ignores malformed names",
			files: []string{"000003_init.up.sql", "bad_name.up.sql", "12345_short.up.sql"},
			want:  4,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			for _, f := range tc.files {
				if err := os.WriteFile(filepath.Join(dir, f), []byte("-- test"), 0o600); err != nil {
					t.Fatalf("failed to create test file %s: %v", f, err)
				}
			}

			got, err := nextMigrationVersion(dir)
			if err != nil {
				t.Fatalf("nextMigrationVersion() returned error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("nextMigrationVersion() = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestNextMigrationVersion_NonexistentDir(t *testing.T) {
	t.Parallel()

	_, err := nextMigrationVersion("/nonexistent/path/that/does/not/exist")
	if err == nil {
		t.Fatal("expected error for nonexistent directory, got nil")
	}
}

// migrate.go: command structure

func TestNewMigrateCommand_Structure(t *testing.T) {
	t.Parallel()

	cmd := newMigrateCommand()
	if cmd.Use != "migrate" {
		t.Fatalf("migrate command Use = %q, want %q", cmd.Use, "migrate")
	}

	subs := map[string]bool{}
	for _, sub := range cmd.Commands() {
		subs[sub.Name()] = true
	}
	for _, name := range []string{"up", "down", "status", "create"} {
		if !subs[name] {
			t.Fatalf("expected subcommand %q on migrate", name)
		}
	}
}

func TestNewMigrateDownCommand_YesFlag(t *testing.T) {
	t.Parallel()

	cmd := newMigrateCommand()
	down := findSubcommand(cmd, "down")
	if down == nil {
		t.Fatal("down subcommand not found")
	}

	f := down.Flags().Lookup("yes")
	if f == nil {
		t.Fatal("expected --yes flag on migrate down command")
		return
	}
	if f.DefValue != "false" {
		t.Fatalf("--yes default = %q, want %q", f.DefValue, "false")
	}
}

// server.go

func TestNewServerCommand_Structure(t *testing.T) {
	t.Parallel()

	cmd := newServerCommand()
	if cmd.Use != "server" {
		t.Fatalf("server command Use = %q, want %q", cmd.Use, "server")
	}

	subs := map[string]bool{}
	for _, sub := range cmd.Commands() {
		subs[sub.Name()] = true
	}
	if !subs["start"] {
		t.Fatal("expected start subcommand on server command")
	}
}

func TestNewServerStartCommand_ModeFlag(t *testing.T) {
	t.Parallel()

	cmd := newServerStartCommand()
	if cmd.Use != "start" {
		t.Fatalf("server start Use = %q, want %q", cmd.Use, "start")
	}

	f := cmd.Flags().Lookup("mode")
	if f == nil {
		t.Fatal("expected --mode flag on server start command")
		return
	}
	if f.DefValue != "" {
		t.Fatalf("--mode default = %q, want empty string", f.DefValue)
	}
}

// services.go: retrySleep

func TestRetrySleep_ReturnsAfterDelay(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	start := time.Now()
	err := retrySleep(ctx, 0)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("retrySleep returned error: %v", err)
	}
	// attempt=0 means 1s delay; allow generous tolerance for CI
	if elapsed < 900*time.Millisecond {
		t.Fatalf("retrySleep(0) returned too quickly: %v", elapsed)
	}
}

func TestRetrySleep_CancelledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := retrySleep(ctx, 0)
	if err == nil {
		t.Fatal("expected error from cancelled context, got nil")
	}
}

func TestRetrySleep_NegativeAttempt(t *testing.T) {
	t.Parallel()

	// Negative attempt should be clamped to 0 via max(attempt, 0),
	// resulting in a 1s delay. Just verify it does not panic.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := retrySleep(ctx, -5)
	if err != nil {
		t.Fatalf("retrySleep with negative attempt returned error: %v", err)
	}
}

// services.go: nilSafeBillingEnforcer

func TestNilSafeBillingEnforcer_NilInput(t *testing.T) {
	t.Parallel()

	got := nilSafeBillingEnforcer(nil)
	if got != nil {
		t.Fatalf("nilSafeBillingEnforcer(nil) returned non-nil: %v", got)
	}
}

func TestNilSafeBillingEnforcer_NonNilInput(t *testing.T) {
	t.Parallel()

	// billing.NewEnforcer requires dependencies we cannot create here,
	// but we can verify the typed nil vs interface nil distinction.
	// A typed nil *billing.Enforcer assigned to the interface would be non-nil.
	var typed *billing.Enforcer
	got := nilSafeBillingEnforcer(typed)
	if got != nil {
		t.Fatal("nilSafeBillingEnforcer(typed nil) should return nil interface")
	}
}

// services.go: logWorkerShutdownStart / logWorkerShutdownComplete

func TestLogWorkerShutdownStart_NilLogger(t *testing.T) {
	t.Parallel()

	// Should not panic with nil logger -- it falls back to slog.Default().
	logWorkerShutdownStart(nil, time.Now(), 0, 5*time.Second)
}

func TestLogWorkerShutdownComplete_NilLogger(t *testing.T) {
	t.Parallel()

	logWorkerShutdownComplete(nil, nil, time.Now(), 0, "", nil)
}

func TestLogWorkerShutdownComplete_WithError(t *testing.T) {
	t.Parallel()

	// Should not panic when logging an error path with nil metrics.
	logWorkerShutdownComplete(nil, nil, time.Now(), 0, "", context.DeadlineExceeded)
}

func TestLogWorkerShutdownComplete_EmptyReasonDerivation(t *testing.T) {
	t.Parallel()

	// When reason is empty, the function derives it from the error.
	// Just verify no panic.
	logWorkerShutdownComplete(nil, nil, time.Now(), 3, "", nil)
}

// helpers

func findSubcommand(parent interface{ Commands() []*cobra.Command }, name string) *cobra.Command {
	for _, sub := range parent.Commands() {
		if sub.Name() == name {
			return sub
		}
	}
	return nil
}
