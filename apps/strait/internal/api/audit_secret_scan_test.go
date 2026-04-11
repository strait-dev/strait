package api

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sync"
	"testing"

	"strait/internal/domain"
)

// secretShapes are regexes matching known real-world secret shapes. A value
// in audit details that matches any of these is almost certainly a leak and
// the test fails. The list is intentionally conservative — regexes use
// prefix anchors and minimum-length constraints to keep false positives near
// zero on realistic identifier shapes (UUIDs, hex hashes, slugs).
//
// Adding a new shape: prefer adding it here over relying on forbidden-key
// names in Phase 3. This test catches leaks by *shape*, not by key name.
var secretShapes = []struct {
	name    string
	pattern *regexp.Regexp
}{
	{"stripe_secret_key", regexp.MustCompile(`\bsk_(live|test)_[A-Za-z0-9]{16,}\b`)},
	{"webhook_signing_secret", regexp.MustCompile(`\bwhsec_[A-Za-z0-9]{16,}\b`)},
	{"github_personal_token", regexp.MustCompile(`\bghp_[A-Za-z0-9]{20,}\b`)},
	{"slack_bot_token", regexp.MustCompile(`\bxox[bpas]-[A-Za-z0-9-]{20,}\b`)},
	{"jwt_like", regexp.MustCompile(`\bey[A-Za-z0-9_-]{20,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\b`)},
	{"aws_access_key", regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`)},
	{"google_api_key", regexp.MustCompile(`\bAIza[0-9A-Za-z_-]{35}\b`)},
	{"bearer_token", regexp.MustCompile(`(?i)\bBearer [A-Za-z0-9._~+/=-]{16,}\b`)},
	{"private_key_block", regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`)},
	{"strait_api_key_raw", regexp.MustCompile(`\bstrait_[a-f0-9]{40,}\b`)},
}

// scanForSecrets walks any JSON value and returns the list of shape names
// that matched. Empty slice means clean.
func scanForSecrets(value any) []string {
	var hits []string
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case string:
			for _, shape := range secretShapes {
				if shape.pattern.MatchString(x) {
					hits = append(hits, shape.name+"="+truncateForLog(x))
				}
			}
		case map[string]any:
			for _, vv := range x {
				walk(vv)
			}
		case []any:
			for _, vv := range x {
				walk(vv)
			}
		}
	}
	walk(value)
	return hits
}

func truncateForLog(s string) string {
	if len(s) > 64 {
		return s[:64] + "..."
	}
	return s
}

// TestAuditDetails_NoSecretLeakage exercises the emit path with realistic
// details payloads for every registered audit action and asserts that no
// captured event contains any string matching a known secret shape. It
// complements Phase 3's forbidden-key test by catching leaks by shape
// rather than by key name.
func TestAuditDetails_NoSecretLeakage(t *testing.T) {
	t.Parallel()

	var (
		mu       sync.Mutex
		captured []*domain.AuditEvent
	)
	ms := &APIStoreMock{
		CreateAuditEventFunc: func(_ context.Context, ev *domain.AuditEvent) error {
			mu.Lock()
			defer mu.Unlock()
			clone := *ev
			captured = append(captured, &clone)
			return nil
		},
	}

	srv := newTestServer(t, ms, nil, nil)

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-scan")
	ctx = context.WithValue(ctx, ctxActorIDKey, "actor-scan")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "user")

	// Benign example detail values: realistic identifiers and short strings.
	benign := map[string]any{
		"id":           "01HXABCXYZ1234567890ABCDEF",
		"slug":         "my-job-slug",
		"name":         "Daily Digest",
		"count":        42,
		"enabled":      true,
		"scheduled_at": "2026-04-11T12:00:00Z",
		"tag_keys":     []string{"env", "team"},
		"changes": map[string]any{
			"before": map[string]any{"name": "old"},
			"after":  map[string]any{"name": "new"},
		},
	}

	for _, action := range domain.KnownAuditActions() {
		srv.emitAuditEvent(ctx, action, "scan_probe", "probe-1", benign)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(captured) != len(domain.KnownAuditActions()) {
		t.Fatalf("captured %d events, want %d", len(captured), len(domain.KnownAuditActions()))
	}

	for _, ev := range captured {
		var details any
		if len(ev.Details) > 0 {
			if err := json.Unmarshal(ev.Details, &details); err != nil {
				t.Fatalf("action %q: details unmarshal: %v", ev.Action, err)
			}
		}
		if hits := scanForSecrets(details); len(hits) > 0 {
			t.Errorf("action %q leaked secret shapes: %v", ev.Action, hits)
		}
	}
}

// TestAuditDetails_SecretScannerCatchesInjected asserts the scanner works:
// injecting known-secret-shaped strings into details makes the scanner fire.
// This guards against a regex regression that would make the scanner a
// silent no-op.
func TestAuditDetails_SecretScannerCatchesInjected(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		value any
		want  string
	}{
		{"stripe", "sk_live_ABCDEFGHIJKLMNOPQRSTUVWX", "stripe_secret_key"},
		{"whsec", "whsec_aaaaaaaaaaaaaaaaaaaaaaaaaa", "webhook_signing_secret"},
		{"jwt", "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.abc123def456", "jwt_like"},
		{"aws", "AKIAIOSFODNN7EXAMPLE", "aws_access_key"},
		{"bearer", "Bearer abcdef1234567890ABCDEF", "bearer_token"},
		{"private_key", "-----BEGIN RSA PRIVATE KEY-----\nfoo\n-----END RSA PRIVATE KEY-----", "private_key_block"},
		{"strait", "strait_" + string(make([]byte, 40)), ""}, // zero bytes aren't hex
	}

	// Build a value containing the string and assert the scanner flags it
	// (or correctly passes the benign zero-byte case above).
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			hits := scanForSecrets(map[string]any{"leak": tc.value})
			if tc.want == "" {
				if len(hits) > 0 {
					t.Errorf("expected clean, got hits: %v", hits)
				}
				return
			}
			found := false
			for _, h := range hits {
				if len(h) >= len(tc.want) && h[:len(tc.want)] == tc.want {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected hit with prefix %q, got %v", tc.want, hits)
			}
		})
	}

	// Build the strait key case properly (40 hex chars).
	hex := "abcdef0123456789abcdef0123456789abcdef01"
	hits := scanForSecrets(fmt.Sprintf("strait_%s", hex))
	if len(hits) == 0 {
		t.Error("expected strait_<hex> to be detected")
	}
}

// TestAuditDetails_ScannerIgnoresBenign asserts the scanner does not flag
// well-known safe identifier shapes. Zero false positives on UUIDs, ULIDs,
// project-id slugs, and hex hashes.
func TestAuditDetails_ScannerIgnoresBenign(t *testing.T) {
	t.Parallel()

	benign := []any{
		"01HXABCXYZ1234567890ABCDEF",                   // ULID
		"550e8400-e29b-41d4-a716-446655440000",         // UUID
		"proj_abc123",                                  // project id
		"my-job-slug",                                  // slug
		"2026-04-11T12:00:00Z",                         // timestamp
		"abc123def456abc123def456abc123de",             // 32-char hex — not a key shape
		"/v1/jobs/job-123",                             // url path
		"https://example.com/webhook",                  // url
		"user@example.com",                             // email
		"hello world",                                  // arbitrary text
		float64(42),                                    // number
		true,                                           // bool
		nil,                                            // nil
		map[string]any{"k": "v"},                       // map
		[]any{"a", "b", "c"},                           // slice
	}

	for i, v := range benign {
		if hits := scanForSecrets(v); len(hits) > 0 {
			t.Errorf("case %d (%v): unexpected hits: %v", i, v, hits)
		}
	}
}
