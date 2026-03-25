package dbscan

import (
	"encoding/json"
	"math"
	"testing"
)

// TestNilIfEmptyString_Empty verifies empty string returns nil.
func TestNilIfEmptyString_Empty(t *testing.T) {
	t.Parallel()

	got := NilIfEmptyString("")
	if got != nil {
		t.Errorf("NilIfEmptyString(\"\") = %v, want nil", got)
	}
}

// TestNilIfEmptyString_NonEmpty verifies non-empty string returns the string.
func TestNilIfEmptyString_NonEmpty(t *testing.T) {
	t.Parallel()

	got := NilIfEmptyString("abc")
	if got != "abc" {
		t.Errorf("NilIfEmptyString(\"abc\") = %v, want \"abc\"", got)
	}
}

// TestNilIfEmptyString_NullByte verifies a string with only a null byte is not nil.
func TestNilIfEmptyString_NullByte(t *testing.T) {
	t.Parallel()

	got := NilIfEmptyString("\x00")
	if got == nil {
		t.Fatal("NilIfEmptyString(\"\\x00\") should not be nil")
	}
	s, ok := got.(string)
	if !ok {
		t.Fatalf("expected string, got %T", got)
	}
	if s != "\x00" {
		t.Errorf("expected \"\\x00\", got %q", s)
	}
}

// TestNilIfEmptyRawMessage_Empty verifies empty or nil RawMessage returns nil.
func TestNilIfEmptyRawMessage_Empty(t *testing.T) {
	t.Parallel()

	if NilIfEmptyRawMessage(nil) != nil {
		t.Error("NilIfEmptyRawMessage(nil) should be nil")
	}
	if NilIfEmptyRawMessage(json.RawMessage{}) != nil {
		t.Error("NilIfEmptyRawMessage(empty) should be nil")
	}
}

// TestNilIfEmptyRawMessage_Valid verifies valid JSON is returned as-is.
func TestNilIfEmptyRawMessage_Valid(t *testing.T) {
	t.Parallel()

	input := json.RawMessage(`{"key":"value"}`)
	got := NilIfEmptyRawMessage(input)
	if got == nil {
		t.Fatal("expected non-nil for valid JSON")
	}

	raw, ok := got.(json.RawMessage)
	if !ok {
		t.Fatalf("expected json.RawMessage, got %T", got)
	}
	if string(raw) != `{"key":"value"}` {
		t.Errorf("got %q, want %q", string(raw), `{"key":"value"}`)
	}
}

// TestNilIfZeroInt_Zero verifies zero returns nil.
func TestNilIfZeroInt_Zero(t *testing.T) {
	t.Parallel()

	got := NilIfZeroInt(0)
	if got != nil {
		t.Errorf("NilIfZeroInt(0) = %v, want nil", got)
	}
}

// TestNilIfZeroInt_Negative verifies negative values are returned as-is.
func TestNilIfZeroInt_Negative(t *testing.T) {
	t.Parallel()

	got := NilIfZeroInt(-1)
	if got == nil {
		t.Fatal("NilIfZeroInt(-1) should not be nil")
	}
	if got != -1 {
		t.Errorf("NilIfZeroInt(-1) = %v, want -1", got)
	}
}

// TestNilIfZeroInt64_MaxInt verifies MaxInt64 is returned as-is.
func TestNilIfZeroInt64_MaxInt(t *testing.T) {
	t.Parallel()

	got := NilIfZeroInt64(math.MaxInt64)
	if got == nil {
		t.Fatal("NilIfZeroInt64(MaxInt64) should not be nil")
	}
	val, ok := got.(int64)
	if !ok {
		t.Fatalf("expected int64, got %T", got)
	}
	if val != math.MaxInt64 {
		t.Errorf("got %d, want %d", val, int64(math.MaxInt64))
	}
}

// FuzzNilIfEmpty fuzzes all NilIf* functions with various inputs.
func FuzzNilIfEmpty(f *testing.F) {
	f.Add("", int64(0), 0)
	f.Add("hello", int64(42), 5)
	f.Add("\x00", int64(-1), -1)
	f.Add("abc", int64(math.MaxInt64), math.MaxInt)

	f.Fuzz(func(t *testing.T, s string, i64 int64, i int) {
		// NilIfEmptyString: nil iff s == "".
		gotStr := NilIfEmptyString(s)
		if s == "" && gotStr != nil {
			t.Errorf("NilIfEmptyString(%q) should be nil", s)
		}
		if s != "" && gotStr == nil {
			t.Errorf("NilIfEmptyString(%q) should not be nil", s)
		}

		// NilIfZeroInt64: nil iff i64 == 0.
		gotI64 := NilIfZeroInt64(i64)
		if i64 == 0 && gotI64 != nil {
			t.Errorf("NilIfZeroInt64(%d) should be nil", i64)
		}
		if i64 != 0 && gotI64 == nil {
			t.Errorf("NilIfZeroInt64(%d) should not be nil", i64)
		}

		// NilIfZeroInt: nil iff i == 0.
		gotI := NilIfZeroInt(i)
		if i == 0 && gotI != nil {
			t.Errorf("NilIfZeroInt(%d) should be nil", i)
		}
		if i != 0 && gotI == nil {
			t.Errorf("NilIfZeroInt(%d) should not be nil", i)
		}
	})
}
