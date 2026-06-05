package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestErrorClassEnum_IsValid(t *testing.T) {
	valid := []string{
		ErrorClassRateLimited, ErrorClassAuth, ErrorClassClient, ErrorClassServer,
		ErrorClassTransient, ErrorClassTimeout, ErrorClassOOM, ErrorClassConnection,
		ErrorClassBudget, ErrorClassUnknown,
	}
	for _, e := range valid {
		assert.True(t,
			ErrorClassEnum(e).IsValid())

	}
	invalid := []string{"", "FATAL", "retry", "http_500"}
	for _, e := range invalid {
		assert.False(t,
			ErrorClassEnum(e).IsValid() && e != "")

	}
}

func TestErrorClassEnum_IsRetryable(t *testing.T) {
	nonRetryable := []string{ErrorClassClient, ErrorClassAuth, ErrorClassBudget, ErrorClassOOM}
	for _, e := range nonRetryable {
		assert.False(t,
			ErrorClassEnum(e).IsRetryable())

	}
	retryable := []string{
		ErrorClassServer, ErrorClassTransient, ErrorClassTimeout,
		ErrorClassConnection, ErrorClassRateLimited, ErrorClassUnknown,
	}
	for _, e := range retryable {
		assert.True(t,
			ErrorClassEnum(e).IsRetryable())

	}
}

func TestErrorClassEnum_IsTransient(t *testing.T) {
	transient := []string{
		ErrorClassRateLimited, ErrorClassTransient, ErrorClassConnection, ErrorClassTimeout,
	}
	for _, e := range transient {
		assert.True(t,
			ErrorClassEnum(e).IsTransient())

	}
	nonTransient := []string{
		ErrorClassClient, ErrorClassAuth, ErrorClassBudget, ErrorClassOOM, ErrorClassServer,
	}
	for _, e := range nonTransient {
		assert.False(t,
			ErrorClassEnum(e).IsTransient())

	}
}

func TestErrorClassEnum_Scan(t *testing.T) {
	var e ErrorClassEnum
	require.NoError(t, e.Scan(
		"timeout",
	))
	assert.Equal(t,
		ErrorClassEnum(ErrorClassTimeout), e)
	require.NoError(t, e.Scan(
		nil,
	))
	assert.Equal(t,
		ErrorClassEnum(""), e)
	assert.Error(t,
		e.Scan("garbage"))

	// nil resets.

	// unknown errors.

}

func TestErrorClassEnum_Value(t *testing.T) {
	v, err := ErrorClassEnum(ErrorClassAuth).Value()
	assert.False(t,
		err != nil ||

			v != "auth")

	empty, err := ErrorClassEnum("").Value()
	assert.False(t,
		empty != nil ||
			err != nil)

}

func TestParseErrorClass(t *testing.T) {
	e, err := ParseErrorClass("server")
	assert.False(t,
		err != nil ||

			e != ErrorClassEnum(ErrorClassServer))

	e, err = ParseErrorClass("")
	assert.False(t,
		err != nil ||

			e != "")

	_, err = ParseErrorClass("nonsense")
	assert.ErrorIs(t, err, ErrUnknownErrorClass)
}

func FuzzErrorClassScan(f *testing.F) {
	f.Add("timeout")
	f.Add("")
	f.Add("; DROP TABLE")
	f.Fuzz(func(t *testing.T, raw string) {
		var e ErrorClassEnum
		defer func() {
			if r := recover(); r != nil {
				require.Failf(t, "Scan panic", "raw=%q panic=%v", raw, r)
			}
		}()
		err := e.Scan(raw)
		assert.False(t,
			err == nil &&

				raw != "" && !e.IsValid())

	})
}
