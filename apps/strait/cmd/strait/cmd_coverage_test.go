package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"strait/internal/billing"
	"strait/internal/config"
	"strait/internal/domain"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// root.go

func TestNewRootCommand_Structure(t *testing.T) {
	t.Parallel()

	cmd := newRootCommand()
	require.Equal(
		t, "strait",
		cmd.Use)
	require.True(t,
		cmd.
			SilenceUsage)
	require.True(t,
		cmd.
			SilenceErrors)

	subs := map[string]bool{}
	for _, sub := range cmd.Commands() {
		subs[sub.Name()] = true
	}
	for _, name := range []string{"serve", "server", "migrate", "version", "health"} {
		require.True(t,
			subs[name])
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
			require.Equal(
				t, tc.
					want, got)
		})
	}
}

func TestNewServeCommand(t *testing.T) {
	t.Parallel()

	cmd := newServeCommand()
	require.Equal(
		t, "serve",
		cmd.Use)

	f := cmd.Flags().Lookup("mode")
	require.NotNil(t, f)
	require.Empty(
		t, f.DefValue)
}

func TestValidateBillingRedisDependency_FailsClosedWhenEnforcementEnabled(t *testing.T) {
	t.Parallel()

	err := validateBillingRedisDependency(&config.Config{BillingEnforcementEnabled: true}, nil)
	require.Error(
		t, err,
	)
	require.Contains(t,
		err.
			Error(), "billing enforcement requires Redis")
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
				require.NoError(t, err)

				return
			}
			require.Error(
				t, err,
			)
			assert.Contains(t, err.
				Error(), tt.want,
			)
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
				require.NoError(t, err)

				return
			}
			require.Error(
				t, err,
			)
			assert.Contains(t, err.
				Error(), tt.want,
			)
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
	require.True(t,
		checker.
			called)
}

func TestNewVersionCommand(t *testing.T) {
	t.Parallel()

	cmd := newVersionCommand()
	require.Equal(
		t, "version",
		cmd.Use)

	f := cmd.Flags().Lookup("short")
	require.NotNil(t, f)
	require.Equal(
		t, "false",
		f.DefValue)
}

func TestNewVersionCommand_Execute(t *testing.T) {
	t.Parallel()

	cmd := newVersionCommand()
	cmd.SetArgs([]string{"--short"})
	require.NoError(t, cmd.
		Execute())
}

func TestNewVersionCommand_ExecuteLong(t *testing.T) {
	t.Parallel()

	cmd := newVersionCommand()
	cmd.SetArgs(nil)
	require.NoError(t, cmd.
		Execute())
}

func TestNormalizeLegacyArgs_Empty(t *testing.T) {
	t.Parallel()
	got := normalizeLegacyArgs(nil)
	require.Nil(t, got)
}

func TestNormalizeLegacyArgs_AllSubcommands(t *testing.T) {
	t.Parallel()

	for _, sub := range []string{"serve", "server", "migrate", "version", "health", "help"} {
		args := []string{sub, "--verbose"}
		got := normalizeLegacyArgs(args)
		require.Equal(
			t, sub,
			got[0])
	}
}

func TestNormalizeLegacyArgs_UnknownNonFlag(t *testing.T) {
	t.Parallel()

	got := normalizeLegacyArgs([]string{"unknown-cmd"})
	require.False(
		t, len(got) != 1 || got[0] != "unknown-cmd",
	)
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
				require.Error(
					t, err,
				)

				return
			}
			require.NoError(t, err)
			require.Equal(
				t, tc.
					want, got)
		})
	}
}

func TestValidateMigrationDatabaseURLRejectsDisableSSLInProduction(t *testing.T) {
	t.Parallel()

	err := validateMigrationDatabaseURL("postgres://localhost/strait?sslmode=disable", "production")
	require.Error(
		t, err,
	)
	require.Contains(t,
		err.
			Error(), "sslmode=disable")
}

func TestValidateMigrationDatabaseURLAllowsDisableSSLInDevelopment(t *testing.T) {
	t.Parallel()
	require.NoError(t, validateMigrationDatabaseURL(
		"postgres://localhost/strait?sslmode=disable",

		"development"))
	require.Error(t, validateMigrationDatabaseURL(
		"postgres://localhost/strait?sslmode=disable",

		""))
}

// TestValidateMigrationDatabaseURLSharesConfigRules guards that the migration
// path uses the shared config validator: it accepts the "dev" alias and rejects
// an absent sslmode in production (which would default to libpq "prefer").
func TestValidateMigrationDatabaseURLSharesConfigRules(t *testing.T) {
	t.Parallel()
	require.NoError(t, validateMigrationDatabaseURL(
		"postgres://localhost/strait?sslmode=disable", "dev"))
	require.Error(t, validateMigrationDatabaseURL(
		"postgres://localhost/strait", "production"))
	require.NoError(t, validateMigrationDatabaseURL(
		"postgres://localhost/strait?sslmode=require", "production"))
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
			require.Equal(
				t, tc.
					want, got)
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
				require.NoError(t, os.
					WriteFile(filepath.
						Join(dir,
							f), []byte("-- test"), 0o600))
			}

			got, err := nextMigrationVersion(dir)
			require.NoError(t, err)
			require.Equal(
				t, tc.
					want, got)
		})
	}
}

func TestNextMigrationVersion_NonexistentDir(t *testing.T) {
	t.Parallel()

	_, err := nextMigrationVersion("/nonexistent/path/that/does/not/exist")
	require.Error(
		t, err,
	)
}

// migrate.go: command structure

func TestNewMigrateCommand_Structure(t *testing.T) {
	t.Parallel()

	cmd := newMigrateCommand()
	require.Equal(
		t, "migrate",
		cmd.Use)

	subs := map[string]bool{}
	for _, sub := range cmd.Commands() {
		subs[sub.Name()] = true
	}
	for _, name := range []string{"up", "down", "status", "create"} {
		require.True(t,
			subs[name])
	}
}

func TestNewMigrateDownCommand_YesFlag(t *testing.T) {
	t.Parallel()

	cmd := newMigrateCommand()
	down := findSubcommand(cmd, "down")
	require.NotNil(t, down)

	f := down.Flags().Lookup("yes")
	require.NotNil(t, f)
	require.Equal(
		t, "false",
		f.DefValue)
}

// server.go

func TestNewServerCommand_Structure(t *testing.T) {
	t.Parallel()

	cmd := newServerCommand()
	require.Equal(
		t, "server",
		cmd.Use)

	subs := map[string]bool{}
	for _, sub := range cmd.Commands() {
		subs[sub.Name()] = true
	}
	require.True(t,
		subs["start"])
}

func TestNewServerStartCommand_ModeFlag(t *testing.T) {
	t.Parallel()

	cmd := newServerStartCommand()
	require.Equal(
		t, "start",
		cmd.Use)

	f := cmd.Flags().Lookup("mode")
	require.NotNil(t, f)
	require.Empty(
		t, f.DefValue)
}

// services.go: retrySleep

func TestRetrySleep_ReturnsAfterDelay(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	start := time.Now()
	err := retrySleep(ctx, 0)
	elapsed := time.Since(start)
	require.NoError(t, err)
	require.GreaterOrEqual(t, elapsed, 900*
		time.Millisecond,
	)

	// attempt=0 means 1s delay; allow generous tolerance for CI
}

func TestRetrySleep_CancelledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := retrySleep(ctx, 0)
	require.Error(
		t, err,
	)
}

func TestRetrySleep_NegativeAttempt(t *testing.T) {
	t.Parallel()

	// Negative attempt should be clamped to 0 via max(attempt, 0),
	// resulting in a 1s delay. Just verify it does not panic.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := retrySleep(ctx, -5)
	require.NoError(t, err)
}

// services.go: nilSafeBillingEnforcer

func TestNilSafeBillingEnforcer_NilInput(t *testing.T) {
	t.Parallel()

	got := nilSafeBillingEnforcer(nil)
	require.Nil(t, got)
}

func TestNilSafeBillingEnforcer_NonNilInput(t *testing.T) {
	t.Parallel()

	// billing.NewEnforcer requires dependencies we cannot create here,
	// but we can verify the typed nil vs interface nil distinction.
	// A typed nil *billing.Enforcer assigned to the interface would be non-nil.
	var typed *billing.Enforcer
	got := nilSafeBillingEnforcer(typed)
	require.Nil(t, got)
}

func TestBuildUsageService_NilDependencies(t *testing.T) {
	t.Parallel()

	require.Nil(t, buildUsageService(nil, nil))
	require.Nil(t, buildUsageService(&billing.PgStore{}, nil))
}

func TestBuildUsageEnforcer_NilStore(t *testing.T) {
	t.Parallel()

	require.Nil(t, buildUsageEnforcer(nil, nil, nil, nil, nil))
}

func TestBuildUsageEnforcer_ReusesBillingEnforcer(t *testing.T) {
	t.Parallel()

	existing := &billing.Enforcer{}

	require.Same(t, existing, buildUsageEnforcer(nil, existing, nil, nil, nil))
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
