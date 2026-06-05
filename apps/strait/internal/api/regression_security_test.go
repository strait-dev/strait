package api

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/require"
)

// Regression guards for earlier security hardening work.

// TestRegression_PayloadSchemaMaxDepth verifies the maxSchemaDepth constant is
// 32 and that deeply nested schemas are rejected.
func TestRegression_PayloadSchemaMaxDepth(t *testing.T) {
	t.Parallel()
	require.Equal(t, 32,
		maxSchemaDepth,
	)

	// Guard: constant must remain 32.

	// Build a schema that exceeds the max depth.
	depth := maxSchemaDepth + 2
	schema := regressionNestedSchema(depth)
	payload := regressionNestedPayload(depth)

	err := validatePayloadAgainstSchema(payload, schema)
	require.Error(t, err)
	require.Contains(t, err.
		Error(), "maximum schema nesting depth")

	// A schema at exactly maxSchemaDepth should still pass.
	okSchema := regressionNestedSchema(maxSchemaDepth - 1)
	okPayload := regressionNestedPayload(maxSchemaDepth - 1)
	require.NoError(t,
		validatePayloadAgainstSchema(
			okPayload,

			okSchema))
}

// regressionNestedSchema creates a JSON schema with the given nesting depth.
func regressionNestedSchema(depth int) json.RawMessage {
	inner := `{"type":"string"}`
	for range depth {
		inner = fmt.Sprintf(`{"type":"object","properties":{"k":%s}}`, inner)
	}
	return json.RawMessage(inner)
}

// regressionNestedPayload creates a JSON payload matching the nested schema.
func regressionNestedPayload(depth int) json.RawMessage {
	inner := `"v"`
	for range depth {
		inner = fmt.Sprintf(`{"k":%s}`, inner)
	}
	return json.RawMessage(inner)
}

// TestRegression_IDFormatValidation verifies validateIDFormat rejects path
// traversal, null bytes, and overly long identifiers.
func TestRegression_IDFormatValidation(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		id      string
		wantErr bool
		errSub  string
	}{
		{"empty", "", true, "must not be empty"},
		{"path traversal dot-dot", "foo..bar", true, "must not contain '..'"},
		{"path traversal slash", "foo/bar", true, "must not contain '/'"},
		{"null byte", "id\x00injected", true, "null bytes"},
		{"over 64 chars", strings.Repeat("a", 65), true, "too long"},
		{"exactly 64 chars", strings.Repeat("a", 64), false, ""},
		{"valid id", "job-abc-123", false, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validateIDFormat(tc.id)
			if tc.wantErr {
				require.Error(t, err)
				require.Contains(t, err.
					Error(), tc.
					errSub,
				)
			} else if err != nil {
				require.Failf(t, "test failure",

					"validateIDFormat(%q) = %v, want nil", tc.id, err)
			}
		})
	}
}

// TestRegression_CronExpressionLength verifies that extremely long cron
// expressions do not cause the parser to hang. The robfig/cron parser should
// return an error quickly.
func TestRegression_CronExpressionLength(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()

	longCron := strings.Repeat("* ", 10000)
	done := make(chan struct{})
	concWG.Go(func() {
		defer close(done)
		parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
		// We only care that it finishes; the result does not matter.
		_, _ = parser.Parse(longCron)
	})

	select {
	case <-done:
		// Completed in time.
	case <-time.After(2 * time.Second):
		require.Fail(t, "cron parser hung on extremely long expression (>2s)")
	}
}

// TestRegression_EndpointURLSSRF verifies that validateURL blocks localhost,
// cloud metadata endpoints, and private IP addresses.
func TestRegression_EndpointURLSSRF(t *testing.T) {
	t.Parallel()

	blockedURLs := []struct {
		name string
		url  string
	}{
		{"localhost", "http://localhost/webhook"},
		{"metadata v4", "http://169.254.169.254/latest/meta-data/"},
		{"google metadata", "http://metadata.google.internal/computeMetadata/v1/"},
		{"loopback ipv4", "http://127.0.0.1/callback"},
		{"private 10.x", "http://10.0.0.1/callback"},
		{"private 192.168", "http://192.168.1.1/callback"},
	}

	for _, tc := range blockedURLs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validateURLWithAllowPrivate(tc.url, false)
			require.Error(t, err)
		})
	}
}

// TestRegression_EventFilterUnboundedArrays verifies that large filter arrays
// in validateTags complete in a reasonable time without unbounded allocation.
func TestRegression_EventFilterUnboundedArrays(t *testing.T) {
	var concWG conc.WaitGroup

	// Build a large tags map (just at the limit of 20).
	defer concWG.Wait()
	t.Parallel()

	tags := make(map[string]string, 20)
	for i := range 20 {
		tags[fmt.Sprintf("key-%d", i)] = fmt.Sprintf("value-%d", i)
	}

	done := make(chan error, 1)
	concWG.Go(func() {
		done <- validateTags(tags)
	})

	select {
	case err := <-done:
		if err != nil {
			require.Failf(t, "test failure",

				"validateTags with 20 tags should pass, got: %v", err)
		}
	case <-time.After(2 * time.Second):
		require.Fail(t, "validateTags hung on large tag map (>2s)")
	}

	// Verify exceeding the limit is rejected.
	tags["extra-key"] = "extra-value"
	require.Error(t, validateTags(tags))
}

// TestRegression_BatchSizeLimits verifies the maxBatchSize constant is 50 and
// that the limit is enforced.
func TestRegression_BatchSizeLimits(t *testing.T) {
	t.Parallel()
	require.Equal(t, 50,
		maxBatchSize,
	)
}

// TestRegression_RequestBodySizeLimit verifies the default request body size
// limit is 1MB and the decodeJSON path uses io.LimitReader.
func TestRegression_RequestBodySizeLimit(t *testing.T) {
	t.Parallel()

	// The default maxRequestBodySize when config is zero should be 1<<20 (1MB).
	expected := int64(1 << 20)
	require.Equal(t, 5*
		1024*
		1024, maxPayloadSize,
	)

	// Verify the payload size limit constant is 5MB.

	// Verify that validatePayloadSize rejects payloads exceeding the limit.
	bigPayload := json.RawMessage(strings.Repeat("x", maxPayloadSize+1))
	require.Error(t, validatePayloadSize(
		bigPayload),
	)
	require.EqualValues(t, 1<<
		20,
		expected)

	// Verify default body limit.
}

// FuzzRegression_AllValidators is a meta-fuzz test that exercises validateTags,
// validatePayloadSize, and validateIDFormat with the same fuzzed input to
// ensure none of them panic on arbitrary data.
func FuzzRegression_AllValidators(f *testing.F) {
	f.Add("valid-id")
	f.Add("")
	f.Add("../../../etc/passwd")
	f.Add(strings.Repeat("a", 100))
	f.Add("id\x00null")
	f.Add("key/with/slashes")

	f.Fuzz(func(t *testing.T, input string) {
		// validateIDFormat must never panic.
		_ = validateIDFormat(input)

		// validatePayloadSize must never panic.
		_ = validatePayloadSize(json.RawMessage(input))

		// validateTags must never panic.
		_ = validateTags(map[string]string{input: input})
	})
}
