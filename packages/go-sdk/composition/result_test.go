package composition

import (
	"errors"
	"testing"
)

func TestOk(t *testing.T) {
	r := Ok(42)
	if !r.IsOk() {
		t.Error("expected IsOk")
	}
	if r.IsErr() {
		t.Error("expected not IsErr")
	}
	if r.Unwrap() != 42 {
		t.Errorf("expected 42, got %d", r.Unwrap())
	}
}

func TestErr(t *testing.T) {
	r := Err[int](errors.New("fail"))
	if r.IsOk() {
		t.Error("expected not IsOk")
	}
	if !r.IsErr() {
		t.Error("expected IsErr")
	}
}

func TestErr_UnwrapPanics(t *testing.T) {
	r := Err[int](errors.New("fail"))
	defer func() {
		if recover() == nil {
			t.Error("expected panic on Unwrap")
		}
	}()
	r.Unwrap()
}

func TestUnwrapErr(t *testing.T) {
	r := Ok(42)
	val, err := r.UnwrapErr()
	if err != nil {
		t.Error("expected no error")
	}
	if val != 42 {
		t.Errorf("expected 42, got %d", val)
	}

	r2 := Err[int](errors.New("fail"))
	_, err = r2.UnwrapErr()
	if err == nil {
		t.Error("expected error")
	}
}

func TestMatch_Ok(t *testing.T) {
	r := Ok("hello")
	var matched string
	r.Match(func(v string) { matched = v }, func(_ error) { t.Error("unexpected error") })
	if matched != "hello" {
		t.Errorf("expected 'hello', got %q", matched)
	}
}

func TestMatch_Err(t *testing.T) {
	r := Err[string](errors.New("fail"))
	var matched error
	r.Match(func(_ string) { t.Error("unexpected ok") }, func(e error) { matched = e })
	if matched == nil {
		t.Error("expected error")
	}
}

func TestFromFunc_Success(t *testing.T) {
	r := FromFunc(func() (int, error) { return 42, nil })
	if !r.IsOk() || r.Unwrap() != 42 {
		t.Error("expected Ok(42)")
	}
}

func TestFromFunc_Error(t *testing.T) {
	r := FromFunc(func() (int, error) { return 0, errors.New("fail") })
	if !r.IsErr() {
		t.Error("expected Err")
	}
}
