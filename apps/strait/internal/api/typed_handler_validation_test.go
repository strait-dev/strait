package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestTypedHandler_ValidatesInput locks in that TypedHandler enforces the input
// struct's validate tags centrally, before the handler runs. Previously the
// bridge only decoded the body and relied on each handler remembering to call
// s.validate.Struct, so a handler that omitted it shipped with zero validation
// despite the framework advertising it.
func TestTypedHandler_ValidatesInput(t *testing.T) {
	t.Parallel()

	type body struct {
		Name string `json:"name" validate:"required,max=5"`
	}
	type input struct {
		Body body
	}

	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	t.Run("invalid body is rejected before handler runs", func(t *testing.T) {
		t.Parallel()
		var ran bool
		h := TypedHandler(srv, http.StatusOK, func(_ context.Context, _ *input) (*struct{}, error) {
			ran = true
			return &struct{}{}, nil
		})

		rec := httptest.NewRecorder()
		req := authedRequest(http.MethodPost, "/x", `{"name":"toolongvalue"}`)
		h(rec, req)

		if ran {
			t.Fatal("handler ran despite invalid input")
		}
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
		}
	})

	t.Run("missing required field is rejected", func(t *testing.T) {
		t.Parallel()
		var ran bool
		h := TypedHandler(srv, http.StatusOK, func(_ context.Context, _ *input) (*struct{}, error) {
			ran = true
			return &struct{}{}, nil
		})

		rec := httptest.NewRecorder()
		req := authedRequest(http.MethodPost, "/x", `{}`)
		h(rec, req)

		if ran {
			t.Fatal("handler ran despite missing required field")
		}
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
		}
	})

	t.Run("valid body passes to handler", func(t *testing.T) {
		t.Parallel()
		var got string
		h := TypedHandler(srv, http.StatusOK, func(_ context.Context, in *input) (*struct{}, error) {
			got = in.Body.Name
			return &struct{}{}, nil
		})

		rec := httptest.NewRecorder()
		req := authedRequest(http.MethodPost, "/x", `{"name":"ok"}`)
		h(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, strings.TrimSpace(rec.Body.String()))
		}
		if got != "ok" {
			t.Fatalf("handler received name = %q, want %q", got, "ok")
		}
	})
}
