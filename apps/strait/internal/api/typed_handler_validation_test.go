package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
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
		require.False(t, ran)
		require.Equal(t, http.StatusUnprocessableEntity,

			rec.Code,
		)
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
		require.False(t, ran)
		require.Equal(t, http.StatusUnprocessableEntity,

			rec.Code,
		)
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
		require.Equal(t, http.StatusOK,
			rec.Code)
		require.Equal(t, "ok", got)
	})
}
