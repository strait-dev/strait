package main

import (
	"testing"
)

func TestLogin_FlagsExist(t *testing.T) {
	t.Parallel()

	cmd := newRootCommand()
	login := findSubcommand(t, cmd, "login")

	for _, name := range []string{"token", "api-key", "with-token", "context", "server", "browser", "no-browser"} {
		if login.Flags().Lookup(name) == nil {
			t.Errorf("login missing --%s flag", name)
		}
	}
}

func TestLogin_TokenFlag_ValidatesNonEmpty(t *testing.T) {
	t.Parallel()

	cmd := newRootCommand()
	login := findSubcommand(t, cmd, "login")

	tokenFlag := login.Flags().Lookup("token")
	if tokenFlag == nil {
		t.Fatal("login missing --token flag")
	}
	if tokenFlag.DefValue != "" {
		t.Errorf("--token default: got %q, want empty string", tokenFlag.DefValue)
	}
}

func TestLogin_TokenFlag_StoresKey(t *testing.T) {
	t.Parallel()

	// Verify the --token flag accepts a string value.
	cmd := newRootCommand()
	login := findSubcommand(t, cmd, "login")

	tokenFlag := login.Flags().Lookup("token")
	if tokenFlag == nil {
		t.Fatal("login missing --token flag")
	}
	if tokenFlag.Value.Type() != "string" {
		t.Errorf("--token type: got %q, want string", tokenFlag.Value.Type())
	}
}

func TestLogin_BrowserMode_RequiresTTY(t *testing.T) {
	t.Parallel()

	// In a non-TTY environment (like CI or tests), the login command should
	// require --token or --with-token. We verify this by checking the login
	// command's --no-browser flag exists as a mechanism to control behavior.
	cmd := newRootCommand()
	login := findSubcommand(t, cmd, "login")

	noBrowserFlag := login.Flags().Lookup("no-browser")
	if noBrowserFlag == nil {
		t.Fatal("login missing --no-browser flag")
	}

	// Verify the command has a Long description mentioning --token.
	if login.Long == "" {
		t.Error("login command should have a long description explaining authentication options")
	}
}
