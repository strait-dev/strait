package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strconv"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
	validator "github.com/go-playground/validator/v10"

	"strait/internal/billing"
)

// TypedHandler creates a chi-compatible http.HandlerFunc from a typed handler function.
// It extracts path/query params into the input struct, decodes JSON body if present,
// validates using the server's validator, and returns the output as JSON.
//
// Input struct field tags:
//   - `path:"name"` for chi URL params
//   - `query:"name"` for query string params
//   - A `Body` field (struct) for JSON request body
//
// Output struct should have a `Body` field containing the response data.
// If handler returns a huma.StatusError, it is mapped to the appropriate HTTP status.
func TypedHandler[I any, O any](s *Server, status int, handler func(ctx context.Context, input *I) (*O, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input I
		if err := extractParams(r, &input); err != nil {
			respondError(w, r, http.StatusBadRequest, err.Error())
			return
		}
		if hasBodyField(&input) {
			if err := decodeBody(s, r, &input); err != nil {
				respondError(w, r, http.StatusBadRequest, "invalid request body: "+err.Error())
				return
			}
		}

		output, err := handler(r.Context(), &input)
		if err != nil {
			writeTypedError(w, r, err)
			return
		}
		if output == nil {
			w.WriteHeader(status)
			return
		}
		respondJSON(w, status, extractBodyField(output))
	}
}

// extractParams fills struct fields tagged with `path` or `query` from the request.
func extractParams(r *http.Request, input any) error {
	v := reflect.ValueOf(input)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil
	}
	t := v.Type()
	for i := range t.NumField() {
		field := t.Field(i)
		fv := v.Field(i)
		if !fv.CanSet() {
			continue
		}
		if tag := field.Tag.Get("path"); tag != "" {
			if val := chi.URLParam(r, tag); val != "" {
				if err := setStringField(fv, val); err != nil {
					return fmt.Errorf("path param %q: %w", tag, err)
				}
			}
		}
		if tag := field.Tag.Get("query"); tag != "" {
			if val := r.URL.Query().Get(tag); val != "" {
				if err := setStringField(fv, val); err != nil {
					return fmt.Errorf("query param %q: %w", tag, err)
				}
			}
		}
	}
	return nil
}

func setStringField(fv reflect.Value, val string) error {
	switch fv.Kind() {
	case reflect.String:
		fv.SetString(val)
	case reflect.Int, reflect.Int64:
		n, err := strconv.ParseInt(val, 10, 64)
		if err != nil {
			return err
		}
		fv.SetInt(n)
	case reflect.Bool:
		b, err := strconv.ParseBool(val)
		if err != nil {
			return err
		}
		fv.SetBool(b)
	default:
		return fmt.Errorf("unsupported param type %s", fv.Kind())
	}
	return nil
}

func hasBodyField(input any) bool {
	v := reflect.ValueOf(input)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	return v.Kind() == reflect.Struct && v.FieldByName("Body").IsValid()
}

func decodeBody(s *Server, r *http.Request, input any) error {
	v := reflect.ValueOf(input)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	bodyField := v.FieldByName("Body")
	if !bodyField.IsValid() || !bodyField.CanAddr() {
		return nil
	}
	defer r.Body.Close()
	dec := json.NewDecoder(io.LimitReader(r.Body, s.maxRequestBodySize))
	err := dec.Decode(bodyField.Addr().Interface())
	if errors.Is(err, io.EOF) {
		return nil // empty body is OK -- fields stay at zero values
	}
	return err
}

func extractBodyField(output any) any {
	v := reflect.ValueOf(output)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() == reflect.Struct {
		if bodyField := v.FieldByName("Body"); bodyField.IsValid() {
			return bodyField.Interface()
		}
	}
	return output
}

func writeTypedError(w http.ResponseWriter, r *http.Request, err error) {
	// Check for typed API errors that carry a full APIError body.
	var tae *typedAPIError
	if errors.As(err, &tae) {
		respondError(w, r, tae.status, tae.apiError)
		return
	}
	// Check for huma status errors (e.g., huma.Error404NotFound).
	var se huma.StatusError
	if errors.As(err, &se) {
		respondError(w, r, se.GetStatus(), se.Error())
		return
	}
	// Check for billing limit errors.
	var le *billing.LimitError
	if errors.As(err, &le) {
		respondError(w, r, http.StatusForbidden, le)
		return
	}
	respondError(w, r, http.StatusInternalServerError, err.Error())
}

// typedAPIError wraps an APIError with an HTTP status code for use in typed handlers.
// It is checked first in writeTypedError so the full APIError (with Code, Message,
// Details) reaches the client.
type typedAPIError struct {
	status   int
	apiError APIError
}

func (e *typedAPIError) Error() string {
	return e.apiError.Message
}

func (e *typedAPIError) GetStatus() int {
	return e.status
}

// newValidationError creates a typedAPIError for struct validation failures,
// preserving the same response shape as the old s.validateRequest helper.
func newValidationError(err error) error {
	var ve validator.ValidationErrors
	if errors.As(err, &ve) {
		messages := make([]string, 0, len(ve))
		for _, fe := range ve {
			messages = append(messages, fmt.Sprintf("%s: failed on '%s'", fe.Field(), fe.Tag()))
		}
		return &typedAPIError{
			status: http.StatusBadRequest,
			apiError: APIError{
				Code:    ErrorCodeValidationError,
				Message: "validation failed",
				Details: messages,
			},
		}
	}
	return &typedAPIError{
		status: http.StatusBadRequest,
		apiError: APIError{
			Code:    ErrorCodeValidationError,
			Message: "invalid request",
		},
	}
}
