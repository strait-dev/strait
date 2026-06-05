package dbscan

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNilIfEmptyString_Empty verifies empty string returns nil.
func TestNilIfEmptyString_Empty(t *testing.T) {
	t.Parallel()

	got := NilIfEmptyString("")
	assert.Nil(t, got)
}

// TestNilIfEmptyString_NonEmpty verifies non-empty string returns the string.
func TestNilIfEmptyString_NonEmpty(t *testing.T) {
	t.Parallel()

	got := NilIfEmptyString("abc")
	assert.Equal(t, "abc", got)
}

// TestNilIfEmptyString_NullByte verifies a string with only a null byte is not nil.
func TestNilIfEmptyString_NullByte(t *testing.T) {
	t.Parallel()

	got := NilIfEmptyString("\x00")
	require.NotNil(t, got)
	s, ok := got.(string)
	require.True(t, ok, "expected string, got %T", got)
	assert.Equal(t, "\x00", s)
}

// TestNilIfEmptyRawMessage_Empty verifies empty or nil RawMessage returns nil.
func TestNilIfEmptyRawMessage_Empty(t *testing.T) {
	t.Parallel()

	assert.Nil(t, NilIfEmptyRawMessage(nil))
	assert.Nil(t, NilIfEmptyRawMessage(json.RawMessage{}))
}

// TestNilIfEmptyRawMessage_Valid verifies valid JSON is returned as-is.
func TestNilIfEmptyRawMessage_Valid(t *testing.T) {
	t.Parallel()

	input := json.RawMessage(`{"key":"value"}`)
	got := NilIfEmptyRawMessage(input)
	require.NotNil(t, got)

	raw, ok := got.(json.RawMessage)
	require.True(t, ok, "expected json.RawMessage, got %T", got)
	assert.JSONEq(t, `{"key":"value"}`, string(raw))
}

// TestNilIfZeroInt_Zero verifies zero returns nil.
func TestNilIfZeroInt_Zero(t *testing.T) {
	t.Parallel()

	got := NilIfZeroInt(0)
	assert.Nil(t, got)
}

// TestNilIfZeroInt_Negative verifies negative values are returned as-is.
func TestNilIfZeroInt_Negative(t *testing.T) {
	t.Parallel()

	got := NilIfZeroInt(-1)
	require.NotNil(t, got)
	assert.Equal(t, -1, got)
}

// TestNilIfZeroInt64_MaxInt verifies MaxInt64 is returned as-is.
func TestNilIfZeroInt64_MaxInt(t *testing.T) {
	t.Parallel()

	got := NilIfZeroInt64(math.MaxInt64)
	require.NotNil(t, got)
	val, ok := got.(int64)
	require.True(t, ok, "expected int64, got %T", got)
	assert.Equal(t, int64(math.MaxInt64), val)
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
		assert.Equal(t, s == "", gotStr == nil)

		// NilIfZeroInt64: nil iff i64 == 0.
		gotI64 := NilIfZeroInt64(i64)
		assert.Equal(t, i64 == 0, gotI64 == nil)

		// NilIfZeroInt: nil iff i == 0.
		gotI := NilIfZeroInt(i)
		assert.Equal(t, i == 0, gotI == nil)
	})
}
