package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// -- JSON decoding adversarial tests.

func TestDecodeJSON_DeeplyNestedObject(t *testing.T) {
	t.Parallel()
	depth := 1000
	buf := strings.Repeat(`{"a":`, depth) + `1` + strings.Repeat(`}`, depth)
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(buf))
	r.Header.Set("Content-Type", "application/json")
	srv := &Server{maxRequestBodySize: 1048576}
	var target map[string]any
	_ = srv.decodeJSON(r, &target)
}

func TestDecodeJSON_InvalidUTF8(t *testing.T) {
	t.Parallel()
	data := []byte{0xff, 0xfe, 0xfd}
	r := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(data))
	r.Header.Set("Content-Type", "application/json")
	srv := &Server{maxRequestBodySize: 1048576}
	var target map[string]any
	err := srv.decodeJSON(r, &target)
	if err == nil {
		t.Fatal("expected error for invalid UTF-8")
	}
}

func TestDecodeJSON_NullBytes(t *testing.T) {
	t.Parallel()
	data := []byte(`{"key` + "\x00" + `":"val"}`)
	r := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(data))
	r.Header.Set("Content-Type", "application/json")
	srv := &Server{maxRequestBodySize: 1048576}
	var target map[string]any
	_ = srv.decodeJSON(r, &target)
}

func TestDecodeJSON_HugeArray(t *testing.T) {
	t.Parallel()
	var sb strings.Builder
	sb.WriteString("[")
	for i := range 100000 {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString("1")
	}
	sb.WriteString("]")
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(sb.String()))
	r.Header.Set("Content-Type", "application/json")
	srv := &Server{maxRequestBodySize: 10 * 1024 * 1024}
	var target []any
	_ = srv.decodeJSON(r, &target)
}

func TestDecodeJSON_EmptyBody(t *testing.T) {
	t.Parallel()
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(""))
	r.Header.Set("Content-Type", "application/json")
	srv := &Server{maxRequestBodySize: 1048576}
	var target map[string]any
	_ = srv.decodeJSON(r, &target)
}

func TestDecodeJSON_BodyExceedsMaxSize(t *testing.T) {
	t.Parallel()
	data := strings.Repeat("x", 2*1024*1024)
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(data))
	r.Header.Set("Content-Type", "application/json")
	srv := &Server{maxRequestBodySize: 1024}
	var target map[string]any
	err := srv.decodeJSON(r, &target)
	if err == nil {
		t.Fatal("expected error for body exceeding max size")
	}
}

func FuzzDecodeJSONAdversarial(f *testing.F) {
	f.Add([]byte("\x00\x01\x02\x03"))
	f.Add([]byte(`{"a":"\u0000"}`))
	f.Add([]byte(`{"` + strings.Repeat("a", 1000) + `":1}`))
	f.Add([]byte(`[[[[[[[[[[[[[[[[[[[[[`))
	f.Add([]byte{0xff, 0xfe})

	f.Fuzz(func(t *testing.T, data []byte) {
		r := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(data))
		r.Header.Set("Content-Type", "application/json")
		srv := &Server{maxRequestBodySize: 1048576}
		var target map[string]any
		_ = srv.decodeJSON(r, &target)
	})
}

// -- Pagination adversarial tests.

func TestPagination_NegativeLimit(t *testing.T) {
	t.Parallel()
	_, _, err := parsePaginationFromStrings("-1", "")
	if err == nil {
		t.Fatal("expected error for negative limit")
	}
}

func TestPagination_OverflowLimit(t *testing.T) {
	t.Parallel()
	_, _, err := parsePaginationFromStrings(fmt.Sprintf("%d", math.MaxInt64), "")
	if err != nil {
		t.Fatalf("overflow limit should be silently capped, got error: %v", err)
	}
}

func TestPagination_ExtremeDateCursor(t *testing.T) {
	t.Parallel()
	_, cursor, err := parsePaginationFromStrings("10", "9999-12-31T23:59:59.999999999Z")
	if err != nil {
		t.Fatalf("extreme date cursor should parse: %v", err)
	}
	if cursor == nil {
		t.Fatal("cursor should not be nil")
	}
}

func TestPagination_EmptyCursor(t *testing.T) {
	t.Parallel()
	limit, cursor, err := parsePaginationFromStrings("", "")
	if err != nil {
		t.Fatalf("empty cursor should succeed: %v", err)
	}
	if cursor != nil {
		t.Fatal("empty cursor should be nil")
	}
	if limit != defaultPageLimit {
		t.Fatalf("expected default limit %d, got %d", defaultPageLimit, limit)
	}
}

func TestPagination_MalformedCursor(t *testing.T) {
	t.Parallel()
	_, _, err := parsePaginationFromStrings("10", "not-a-date")
	if err == nil {
		t.Fatal("expected error for malformed cursor")
	}
}

func FuzzPaginationParams(f *testing.F) {
	f.Add("10", "2024-01-01T00:00:00Z")
	f.Add("-1", "")
	f.Add("0", "invalid")
	f.Add("999999999999", "9999-12-31T23:59:59Z")
	f.Add("", "")

	f.Fuzz(func(t *testing.T, limitStr, cursorStr string) {
		_, _, _ = parsePaginationFromStrings(limitStr, cursorStr)
	})
}

// -- Tag validation adversarial tests.

func TestValidateTags_ControlCharacters(t *testing.T) {
	t.Parallel()
	tags := map[string]string{"\x01\x02\x03": "val"}
	// Control characters are currently accepted by validateTags.
	err := validateTags(tags)
	if err != nil {
		t.Fatalf("control chars in tag key should be accepted (no char validation): %v", err)
	}
}

func TestValidateTags_NullByteInKey(t *testing.T) {
	t.Parallel()
	tags := map[string]string{"key\x00injected": "val"}
	err := validateTags(tags)
	if err != nil {
		t.Fatalf("null byte in key should be accepted (no char validation): %v", err)
	}
}

func TestValidateTags_CombiningChars(t *testing.T) {
	t.Parallel()
	// Combining character: e + combining acute accent.
	tags := map[string]string{"e\u0301": "val"}
	err := validateTags(tags)
	if err != nil {
		t.Fatalf("combining chars should be accepted: %v", err)
	}
}

func TestValidateTags_BoundaryKeyLength(t *testing.T) {
	t.Parallel()
	t.Run("exactly_64", func(t *testing.T) {
		t.Parallel()
		key := strings.Repeat("a", 64)
		err := validateTags(map[string]string{key: "val"})
		if err != nil {
			t.Fatalf("64-char key should be accepted: %v", err)
		}
	})
	t.Run("65_chars", func(t *testing.T) {
		t.Parallel()
		key := strings.Repeat("a", 65)
		err := validateTags(map[string]string{key: "val"})
		if err == nil {
			t.Fatal("65-char key should be rejected")
		}
	})
}

func TestValidateTags_BoundaryValueLength(t *testing.T) {
	t.Parallel()
	t.Run("exactly_256", func(t *testing.T) {
		t.Parallel()
		val := strings.Repeat("b", 256)
		err := validateTags(map[string]string{"key": val})
		if err != nil {
			t.Fatalf("256-char value should be accepted: %v", err)
		}
	})
	t.Run("257_chars", func(t *testing.T) {
		t.Parallel()
		val := strings.Repeat("b", 257)
		err := validateTags(map[string]string{"key": val})
		if err == nil {
			t.Fatal("257-char value should be rejected")
		}
	})
}

func FuzzValidateTagsAdversarial(f *testing.F) {
	f.Add("key", "value")
	f.Add("\x00", "\x00")
	f.Add(strings.Repeat("x", 100), strings.Repeat("y", 300))
	f.Add("e\u0301", "val")
	f.Add("", "")

	f.Fuzz(func(t *testing.T, key, value string) {
		tags := map[string]string{key: value}
		_ = validateTags(tags)
	})
}

// -- Payload schema depth limit tests.

func buildNestedSchema(depth int) json.RawMessage {
	if depth <= 0 {
		return json.RawMessage(`{"type":"string"}`)
	}
	inner := buildNestedSchema(depth - 1)
	return json.RawMessage(fmt.Sprintf(`{"type":"object","properties":{"a":%s}}`, string(inner)))
}

func buildNestedPayload(depth int) json.RawMessage {
	if depth <= 0 {
		return json.RawMessage(`"leaf"`)
	}
	inner := buildNestedPayload(depth - 1)
	return json.RawMessage(fmt.Sprintf(`{"a":%s}`, string(inner)))
}

func TestPayloadSchema_RecursionDepthLimit(t *testing.T) {
	t.Parallel()
	schema := buildNestedSchema(50)
	payload := buildNestedPayload(50)
	err := validatePayloadAgainstSchema(payload, schema)
	if err == nil {
		t.Fatal("expected error for 50-level schema (exceeds maxSchemaDepth)")
	}
	if !strings.Contains(err.Error(), "maximum schema nesting depth") {
		t.Fatalf("expected depth-limit error, got: %v", err)
	}
}

func TestPayloadSchema_AtExactLimit(t *testing.T) {
	t.Parallel()
	// maxSchemaDepth levels should pass (depth goes from 0 to maxSchemaDepth, check is >).
	schema := buildNestedSchema(maxSchemaDepth)
	payload := buildNestedPayload(maxSchemaDepth)
	err := validatePayloadAgainstSchema(payload, schema)
	if err != nil {
		t.Fatalf("schema at maxSchemaDepth should pass: %v", err)
	}
}

func TestPayloadSchema_OneOverLimit(t *testing.T) {
	t.Parallel()
	schema := buildNestedSchema(maxSchemaDepth + 1)
	payload := buildNestedPayload(maxSchemaDepth + 1)
	err := validatePayloadAgainstSchema(payload, schema)
	if err == nil {
		t.Fatal("schema one over maxSchemaDepth should be rejected")
	}
	if !strings.Contains(err.Error(), "maximum schema nesting depth") {
		t.Fatalf("expected depth-limit error, got: %v", err)
	}
}

func TestPayloadSchema_LargeArray(t *testing.T) {
	t.Parallel()
	var items []int
	for i := range 10000 {
		items = append(items, i)
	}
	payload, _ := json.Marshal(items)
	schema := json.RawMessage(`{"type":"array","items":{"type":"number"}}`)
	err := validatePayloadAgainstSchema(payload, schema)
	if err != nil {
		t.Fatalf("large array should validate: %v", err)
	}
}

func FuzzPayloadSchemaDepth(f *testing.F) {
	f.Add(5)
	f.Add(0)
	f.Add(32)
	f.Add(50)
	f.Add(100)

	f.Fuzz(func(t *testing.T, depth int) {
		if depth < 0 || depth > 200 {
			return
		}
		schema := buildNestedSchema(depth)
		payload := buildNestedPayload(depth)
		_ = validatePayloadAgainstSchema(payload, schema)
	})
}

// -- ID format validation tests.

func TestValidateIDFormat_PathTraversal(t *testing.T) {
	t.Parallel()
	err := validateIDFormat("../../etc/passwd")
	if err == nil {
		t.Fatal("path traversal ID should be rejected")
	}
}

func TestValidateIDFormat_EmptyID(t *testing.T) {
	t.Parallel()
	err := validateIDFormat("")
	if err == nil {
		t.Fatal("empty ID should be rejected")
	}
}

func TestValidateIDFormat_ExtremelyLong(t *testing.T) {
	t.Parallel()
	err := validateIDFormat(strings.Repeat("a", 10000))
	if err == nil {
		t.Fatal("extremely long ID should be rejected")
	}
}

func TestValidateIDFormat_NullByte(t *testing.T) {
	t.Parallel()
	err := validateIDFormat("job\x00-123")
	if err == nil {
		t.Fatal("ID with null byte should be rejected")
	}
}

func TestValidateIDFormat_ValidNanoid(t *testing.T) {
	t.Parallel()
	err := validateIDFormat("abc123def456")
	if err != nil {
		t.Fatalf("valid nanoid should be accepted: %v", err)
	}
}

func TestValidateIDFormat_SlashInID(t *testing.T) {
	t.Parallel()
	err := validateIDFormat("job/123")
	if err == nil {
		t.Fatal("ID with slash should be rejected")
	}
}

func TestValidateIDFormat_ExactMaxLength(t *testing.T) {
	t.Parallel()
	err := validateIDFormat(strings.Repeat("a", maxIDLength))
	if err != nil {
		t.Fatalf("ID at max length should be accepted: %v", err)
	}
}

func TestValidateIDFormat_OneOverMaxLength(t *testing.T) {
	t.Parallel()
	err := validateIDFormat(strings.Repeat("a", maxIDLength+1))
	if err == nil {
		t.Fatal("ID over max length should be rejected")
	}
}

// -- decodeJSON with various time.Time parsing edge cases (via cursor).

func TestPagination_UnixEpochCursor(t *testing.T) {
	t.Parallel()
	_, cursor, err := parsePaginationFromStrings("10", "1970-01-01T00:00:00Z")
	if err != nil {
		t.Fatalf("Unix epoch cursor should parse: %v", err)
	}
	if cursor == nil || !cursor.Equal(time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("unexpected cursor: %v", cursor)
	}
}

func TestPagination_ZeroLimit(t *testing.T) {
	t.Parallel()
	_, _, err := parsePaginationFromStrings("0", "")
	if err == nil {
		t.Fatal("zero limit should be rejected")
	}
}
