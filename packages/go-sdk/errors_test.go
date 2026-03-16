package strait

import (
	"errors"
	"testing"
)

func TestTransportError(t *testing.T) {
	cause := errors.New("connection refused")
	err := &TransportError{Message: "request failed", Cause: cause}

	if err.Error() != "request failed" {
		t.Errorf("expected 'request failed', got %q", err.Error())
	}
	if !errors.Is(err, cause) {
		t.Error("expected Unwrap to return cause")
	}
}

func TestDecodeError(t *testing.T) {
	cause := errors.New("invalid json")
	err := &DecodeError{Message: "decode failed", Body: `{"bad":`, Cause: cause}

	if err.Error() != "decode failed" {
		t.Errorf("expected 'decode failed', got %q", err.Error())
	}
	if !errors.Is(err, cause) {
		t.Error("expected Unwrap to return cause")
	}
	if err.Body != `{"bad":` {
		t.Errorf("expected body to be preserved")
	}
}

func TestValidationError(t *testing.T) {
	err := &ValidationError{Message: "invalid config", Issues: []string{"missing field"}}

	if err.Error() != "invalid config" {
		t.Errorf("expected 'invalid config', got %q", err.Error())
	}
	if len(err.Issues) != 1 || err.Issues[0] != "missing field" {
		t.Error("expected issues to be preserved")
	}
}

func TestUnauthorizedError(t *testing.T) {
	err := &UnauthorizedError{Status: 401, Message: "unauthorized"}
	if err.Error() != "unauthorized" {
		t.Errorf("expected 'unauthorized', got %q", err.Error())
	}
	if err.Status != 401 {
		t.Errorf("expected status 401, got %d", err.Status)
	}
}

func TestNotFoundError(t *testing.T) {
	err := &NotFoundError{Status: 404, Message: "not found"}
	if err.Error() != "not found" {
		t.Errorf("expected 'not found', got %q", err.Error())
	}
}

func TestConflictError(t *testing.T) {
	err := &ConflictError{Status: 409, Message: "conflict"}
	if err.Error() != "conflict" {
		t.Errorf("expected 'conflict', got %q", err.Error())
	}
}

func TestRateLimitedError(t *testing.T) {
	err := &RateLimitedError{Status: 429, Message: "too many requests"}
	if err.Error() != "too many requests" {
		t.Errorf("expected 'too many requests', got %q", err.Error())
	}
}

func TestApiError(t *testing.T) {
	err := &ApiError{Status: 500, Message: "internal error"}
	if err.Error() != "internal error" {
		t.Errorf("expected 'internal error', got %q", err.Error())
	}
}

func TestTimeoutError(t *testing.T) {
	err := &TimeoutError{Message: "timed out", RunID: "run_1", ElapsedMs: 5000}
	if err.Error() != "timed out" {
		t.Errorf("expected 'timed out', got %q", err.Error())
	}
	if err.RunID != "run_1" {
		t.Errorf("expected RunID 'run_1', got %q", err.RunID)
	}
}

func TestDagValidationError(t *testing.T) {
	err := &DagValidationError{
		Message:       "invalid dag",
		Cycles:        []string{"a", "b"},
		MissingRefs:   []string{"c"},
		DuplicateRefs: []string{"d"},
	}
	if err.Error() != "invalid dag" {
		t.Errorf("expected 'invalid dag', got %q", err.Error())
	}
}

func TestMapHttpError_401(t *testing.T) {
	err := MapHttpError(401, "unauthorized", nil)
	var target *UnauthorizedError
	if !errors.As(err, &target) {
		t.Error("expected UnauthorizedError for 401")
	}
}

func TestMapHttpError_403(t *testing.T) {
	err := MapHttpError(403, "forbidden", nil)
	var target *UnauthorizedError
	if !errors.As(err, &target) {
		t.Error("expected UnauthorizedError for 403")
	}
}

func TestMapHttpError_404(t *testing.T) {
	err := MapHttpError(404, "not found", nil)
	var target *NotFoundError
	if !errors.As(err, &target) {
		t.Error("expected NotFoundError for 404")
	}
}

func TestMapHttpError_409(t *testing.T) {
	err := MapHttpError(409, "conflict", nil)
	var target *ConflictError
	if !errors.As(err, &target) {
		t.Error("expected ConflictError for 409")
	}
}

func TestMapHttpError_429(t *testing.T) {
	err := MapHttpError(429, "rate limited", nil)
	var target *RateLimitedError
	if !errors.As(err, &target) {
		t.Error("expected RateLimitedError for 429")
	}
}

func TestMapHttpError_500(t *testing.T) {
	err := MapHttpError(500, "server error", nil)
	var target *ApiError
	if !errors.As(err, &target) {
		t.Error("expected ApiError for 500")
	}
}

func TestMapHttpError_EmptyMessage(t *testing.T) {
	err := MapHttpError(500, "", nil)
	if err.Error() != "HTTP 500" {
		t.Errorf("expected 'HTTP 500', got %q", err.Error())
	}
}

func TestMapHttpError_PreservesBody(t *testing.T) {
	body := map[string]string{"detail": "something went wrong"}
	err := MapHttpError(404, "not found", body)
	var target *NotFoundError
	if !errors.As(err, &target) {
		t.Fatal("expected NotFoundError")
	}
	bodyMap, ok := target.Body.(map[string]string)
	if !ok {
		t.Fatal("expected body to be map[string]string")
	}
	if bodyMap["detail"] != "something went wrong" {
		t.Error("expected body detail to be preserved")
	}
}
