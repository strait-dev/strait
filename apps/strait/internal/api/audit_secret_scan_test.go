package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// scanForSecrets is the test-facing helper that surfaces shape hits as
// labeled strings for easier debug output. It delegates to the production
// scanner (auditSecretShapes + scanAndRedact) so regressions in either
// the pattern set or the walker surface here.
func scanForSecrets(value any) []string {
	var hits []string
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case string:
			for _, shape := range auditSecretShapes {
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
// complements the forbidden-key test by catching leaks by shape rather than by
// key name.
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
	require.Len(t,
		captured, len(domain.
			KnownAuditActions()),
	)

	for _, ev := range captured {
		var details any
		if len(ev.Details) > 0 {
			require.NoError(t, json.Unmarshal(ev.
				Details, &details,
			))

		}
		if hits := scanForSecrets(details); len(hits) > 0 {
			assert.Failf(t, "test failure",

				"action %q leaked secret shapes: %v", ev.Action, hits)
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
		t.Run(tc.name, func(t *testing.T) {
			hits := scanForSecrets(map[string]any{"leak": tc.value})
			if tc.want == "" {
				assert.LessOrEqual(t, len(hits), 0)

				return
			}
			found := false
			for _, h := range hits {
				if len(h) >= len(tc.want) && h[:len(tc.want)] == tc.want {
					found = true
					break
				}
			}
			assert.True(t,
				found)

		})
	}

	// Build the strait key case properly (40 hex chars).
	hex := "abcdef0123456789abcdef0123456789abcdef01"
	hits := scanForSecrets(fmt.Sprintf("strait_%s", hex))
	assert.NotEmpty(t, hits)

}

// TestAuditDetails_ScannerIgnoresBenign asserts the scanner does not flag
// well-known safe identifier shapes. Zero false positives on UUIDs, ULIDs,
// project-id slugs, and hex hashes.
func TestAuditDetails_ScannerIgnoresBenign(t *testing.T) {
	t.Parallel()

	benign := []any{
		"01HXABCXYZ1234567890ABCDEF",           // ULID
		"550e8400-e29b-41d4-a716-446655440000", // UUID
		"proj_abc123",                          // project id
		"my-job-slug",                          // slug
		"2026-04-11T12:00:00Z",                 // timestamp
		"abc123def456abc123def456abc123de",     // 32-char hex — not a key shape
		"/v1/jobs/job-123",                     // url path
		"https://example.com/webhook",          // url
		"user@example.com",                     // email
		"hello world",                          // arbitrary text
		float64(42),                            // number
		true,                                   // bool
		nil,                                    // nil
		map[string]any{"k": "v"},               // map
		[]any{"a", "b", "c"},                   // slice
	}

	for i, v := range benign {
		if hits := scanForSecrets(v); len(hits) > 0 {
			assert.Failf(t, "test failure",

				"case %d (%v): unexpected hits: %v", i, v, hits)
		}
	}
}

// TestScanAndRedact_RedactsStripeKey asserts a Stripe secret key planted
// in a value position is replaced with a labeled marker.
func TestScanAndRedact_RedactsStripeKey(t *testing.T) {
	t.Parallel()
	in := map[string]any{"leak": "sk_live_ABCDEFGHIJKLMNOPQRSTUVWX"}
	out, shapes := scanAndRedact(in)
	m, ok := out.(map[string]any)
	require.True(
		t, ok)

	if got, _ := m["leak"].(string); got != "[redacted:stripe_secret_key]" {
		assert.Failf(t, "test failure",

			"leak = %q, want [redacted:stripe_secret_key]", got)
	}
	assert.False(
		t, len(shapes) !=
			1 || shapes[0] !=
			"stripe_secret_key",
	)

}

// TestScanAndRedact_RedactsJWT asserts a JWT-shaped token is redacted.
func TestScanAndRedact_RedactsJWT(t *testing.T) {
	t.Parallel()
	in := map[string]any{
		"token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.abc123def456",
	}
	out, shapes := scanAndRedact(in)
	m := out.(map[string]any)
	if got, _ := m["token"].(string); got != "[redacted:jwt_like]" {
		assert.Failf(t, "test failure",

			"token = %q, want [redacted:jwt_like]", got)
	}
	assert.False(
		t, len(shapes) !=
			1 || shapes[0] !=
			"jwt_like",
	)

}

// TestScanAndRedact_RedactsPEM asserts a PEM private-key header substring
// is redacted even when embedded in a larger string.
func TestScanAndRedact_RedactsPEM(t *testing.T) {
	t.Parallel()
	in := map[string]any{
		"note": "leaked key: -----BEGIN RSA PRIVATE KEY-----\nfoo\n-----END RSA PRIVATE KEY-----",
	}
	out, shapes := scanAndRedact(in)
	m := out.(map[string]any)
	got, _ := m["note"].(string)
	assert.NotEqual(t, in["note"],
		got)
	assert.False(
		t, len(shapes) !=
			1 || shapes[0] !=
			"private_key_block",
	)

}

func TestScanAndRedact_RedactsNestedWithoutMutatingInput(t *testing.T) {
	t.Parallel()

	secret := "Bearer abcdef1234567890ABCDEF"
	in := map[string]any{
		"meta": map[string]any{
			"nested": []any{"safe", secret},
		},
	}

	out, shapes := scanAndRedact(in)
	require.False(t, len(shapes) !=
		1 ||
		shapes[0] !=
			"bearer_token",
	)
	require.Equal(t, secret, in["meta"].(map[string]any)["nested"].([]any)[1])

	got := out.(map[string]any)["meta"].(map[string]any)["nested"].([]any)[1]
	require.Equal(t, "[redacted:bearer_token]",

		got)

}

// TestScanAndRedact_NoFalsePositivesOnBenignContent asserts realistic
// identifier shapes (ULIDs, UUIDs, slugs, urls, emails, timestamps) never
// trigger a redaction.
func TestScanAndRedact_NoFalsePositivesOnBenignContent(t *testing.T) {
	t.Parallel()
	in := map[string]any{
		"ulid":      "01HXABCXYZ1234567890ABCDEF",
		"uuid":      "550e8400-e29b-41d4-a716-446655440000",
		"slug":      "my-job-slug",
		"ts":        "2026-04-11T12:00:00Z",
		"url":       "https://example.com/webhook",
		"email":     "user@example.com",
		"hex32":     "abc123def456abc123def456abc123de",
		"nums":      []any{float64(1), float64(2), float64(3)},
		"nested":    map[string]any{"k": "v"},
		"proj_slug": "proj_abc123",
	}
	_, shapes := scanAndRedact(in)
	assert.LessOrEqual(t, len(shapes), 0)

}

// TestMarshalAndCapDetails_EmitsRedactedMetric asserts the production emit
// path passes details through the scanner, replaces secret-shaped
// substrings with redaction markers, and records a "_redacted" shape list
// on the resulting details blob. The Stripe-shaped value must not appear
// in the marshaled JSON.
func TestMarshalAndCapDetails_EmitsRedactedMetric(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, nil, nil)

	details := map[string]any{
		"note": "oops stripe=sk_live_ABCDEFGHIJKLMNOPQRSTUVWX trailing",
		"meta": map[string]any{
			"token": "Bearer abcdef1234567890ABCDEF",
		},
	}

	raw, err := srv.marshalAndCapDetails(context.Background(), domain.AuditActionRoleCreated, details)
	require.NoError(t, err)

	s := string(raw)
	assert.False(
		t, strings.Contains(s, "sk_live_ABCDEFGHIJKLMNOPQRSTUVWX"))
	assert.False(
		t, strings.Contains(s, "Bearer abcdef1234567890ABCDEF"),
	)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw,
		&decoded),
	)

	shapes, ok := decoded["_redacted"].([]any)
	require.False(t, !ok || len(shapes) ==
		0)

	// Must include both shapes, deduped and sorted alphabetically
	// (bearer_token, stripe_secret_key).
	wantShapes := map[string]bool{"bearer_token": true, "stripe_secret_key": true}
	for _, sh := range shapes {
		delete(wantShapes, sh.(string))
	}
	assert.LessOrEqual(t, len(wantShapes),
		0)

}
