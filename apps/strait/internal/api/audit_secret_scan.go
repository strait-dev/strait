package api

import (
	"regexp"
	"sort"
)

// secretShapePattern is the compiled regex for a known secret shape and
// the short label emitted in redaction markers and metric labels.
//
// Redaction operates by shape, not by key name: the Phase 3 forbidden-key
// guard catches leaks whose *key* looks secret-ish ("password", "token"),
// and this scanner catches leaks whose *value* matches a known real-world
// secret format regardless of what key it landed under. Both run — a
// handler that stuffs a Stripe key into a field called "note" still gets
// scrubbed.
//
// Patterns use prefix anchors and minimum-length constraints to keep
// false positives near zero on realistic identifier shapes (UUIDs, hex
// hashes, project-id slugs).
type secretShapePattern struct {
	name    string
	pattern *regexp.Regexp
}

// auditSecretShapes is the production scanner's pattern set. Kept in sync
// with the list exercised by audit_secret_scan_test.go and the fuzzer in
// audit_secret_fuzz_test.go.
var auditSecretShapes = []secretShapePattern{
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

// scanAndRedact walks any JSON-decoded value (the shape produced by
// unmarshalling the details map via json.Marshal → json.Unmarshal), and
// for every string leaf that matches a known secret shape, replaces the
// matched substring with "[redacted:<shape>]". Returns the redacted
// value plus the deduplicated sorted list of shape names that matched.
//
// Non-string leaves (numbers, bools, nulls) are left alone. Map keys are
// never scanned (they are not the attack surface for secret-shaped
// leaks; the value is).
//
// The function never panics on arbitrary input — verified by the
// package's fuzzer.
func scanAndRedact(value any) (redacted any, matches []string) {
	seen := map[string]struct{}{}
	red := walkAndRedact(value, seen)
	if len(seen) == 0 {
		return red, nil
	}
	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	sort.Strings(out)
	return red, out
}

func walkAndRedact(v any, seen map[string]struct{}) any {
	switch x := v.(type) {
	case string:
		redacted := x
		for _, shape := range auditSecretShapes {
			if shape.pattern.MatchString(redacted) {
				seen[shape.name] = struct{}{}
				redacted = shape.pattern.ReplaceAllString(redacted, "[redacted:"+shape.name+"]")
			}
		}
		return redacted
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, vv := range x {
			out[k] = walkAndRedact(vv, seen)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, vv := range x {
			out[i] = walkAndRedact(vv, seen)
		}
		return out
	default:
		return v
	}
}
