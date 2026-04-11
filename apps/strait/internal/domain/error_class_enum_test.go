package domain

import (
	"errors"
	"testing"
)

func TestErrorClassEnum_IsValid(t *testing.T) {
	valid := []string{
		ErrorClassRateLimited, ErrorClassAuth, ErrorClassClient, ErrorClassServer,
		ErrorClassTransient, ErrorClassTimeout, ErrorClassOOM, ErrorClassConnection,
		ErrorClassBudget, ErrorClassUnknown,
	}
	for _, e := range valid {
		if !ErrorClassEnum(e).IsValid() {
			t.Errorf("%q should be valid", e)
		}
	}
	invalid := []string{"", "FATAL", "retry", "http_500"}
	for _, e := range invalid {
		if ErrorClassEnum(e).IsValid() && e != "" {
			t.Errorf("%q should not be valid", e)
		}
	}
}

func TestErrorClassEnum_IsRetryable(t *testing.T) {
	nonRetryable := []string{ErrorClassClient, ErrorClassAuth, ErrorClassBudget, ErrorClassOOM}
	for _, e := range nonRetryable {
		if ErrorClassEnum(e).IsRetryable() {
			t.Errorf("%q should not be retryable", e)
		}
	}
	retryable := []string{
		ErrorClassServer, ErrorClassTransient, ErrorClassTimeout,
		ErrorClassConnection, ErrorClassRateLimited, ErrorClassUnknown,
	}
	for _, e := range retryable {
		if !ErrorClassEnum(e).IsRetryable() {
			t.Errorf("%q should be retryable", e)
		}
	}
}

func TestErrorClassEnum_IsTransient(t *testing.T) {
	transient := []string{
		ErrorClassRateLimited, ErrorClassTransient, ErrorClassConnection, ErrorClassTimeout,
	}
	for _, e := range transient {
		if !ErrorClassEnum(e).IsTransient() {
			t.Errorf("%q should be transient", e)
		}
	}
	nonTransient := []string{
		ErrorClassClient, ErrorClassAuth, ErrorClassBudget, ErrorClassOOM, ErrorClassServer,
	}
	for _, e := range nonTransient {
		if ErrorClassEnum(e).IsTransient() {
			t.Errorf("%q should not be transient", e)
		}
	}
}

func TestErrorClassEnum_Scan(t *testing.T) {
	var e ErrorClassEnum
	if err := e.Scan("timeout"); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if e != ErrorClassEnum(ErrorClassTimeout) {
		t.Errorf("e = %q", e)
	}

	// nil resets.
	if err := e.Scan(nil); err != nil {
		t.Fatalf("scan nil: %v", err)
	}
	if e != "" {
		t.Errorf("expected empty, got %q", e)
	}

	// unknown errors.
	if err := e.Scan("garbage"); err == nil {
		t.Error("expected error for garbage")
	}
}

func TestErrorClassEnum_Value(t *testing.T) {
	v, err := ErrorClassEnum(ErrorClassAuth).Value()
	if err != nil || v != "auth" {
		t.Errorf("value = %v %v", v, err)
	}
	empty, err := ErrorClassEnum("").Value()
	if empty != nil || err != nil {
		t.Errorf("empty = %v %v", empty, err)
	}
}

func TestParseErrorClass(t *testing.T) {
	e, err := ParseErrorClass("server")
	if err != nil || e != ErrorClassEnum(ErrorClassServer) {
		t.Errorf("got %q %v", e, err)
	}
	e, err = ParseErrorClass("")
	if err != nil || e != "" {
		t.Errorf("empty passthrough got %q %v", e, err)
	}
	if _, err := ParseErrorClass("nonsense"); !errors.Is(err, ErrUnknownErrorClass) {
		t.Errorf("want ErrUnknownErrorClass, got %v", err)
	}
}

func FuzzErrorClassScan(f *testing.F) {
	f.Add("timeout")
	f.Add("")
	f.Add("; DROP TABLE")
	f.Fuzz(func(t *testing.T, raw string) {
		var e ErrorClassEnum
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("Scan panic on %q: %v", raw, r)
			}
		}()
		err := e.Scan(raw)
		if err == nil && raw != "" && !e.IsValid() {
			t.Errorf("Scan accepted invalid %q → %q", raw, e)
		}
	})
}
