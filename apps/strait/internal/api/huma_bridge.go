package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
	validator "github.com/go-playground/validator/v10"

	"strait/internal/billing"
)

type ctxKey int

const (
	ctxKeyResponseWriter ctxKey = iota
	ctxKeyRequest
)

// responseWriterFromContext retrieves the http.ResponseWriter stored by TypedHandler.
func responseWriterFromContext(ctx context.Context) http.ResponseWriter {
	if w, ok := ctx.Value(ctxKeyResponseWriter).(http.ResponseWriter); ok {
		return w
	}
	return nil
}

// requestFromContext retrieves the *http.Request stored by TypedHandler.
func requestFromContext(ctx context.Context) *http.Request {
	if r, ok := ctx.Value(ctxKeyRequest).(*http.Request); ok {
		return r
	}
	return nil
}

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

		// Enforce the input struct's validate tags centrally so every typed handler
		// gets the framework-advertised validation even if it omits an explicit
		// s.validate.Struct call. Per-handler validation remains as harmless
		// redundancy. nil validator (test servers) skips this.
		if s.validate != nil {
			if err := s.validate.Struct(&input); err != nil {
				writeTypedError(w, r, newValidationError(err))
				return
			}
		}

		// Store w and r in context for streaming/export handlers that need raw access.
		ctx := context.WithValue(r.Context(), ctxKeyResponseWriter, w)
		ctx = context.WithValue(ctx, ctxKeyRequest, r)
		if strings.HasPrefix(r.URL.Path, "/sdk/") {
			if err := s.revalidateRunTokenState(ctx); err != nil {
				writeTypedError(w, r, err)
				return
			}
		}

		output, err := handler(ctx, &input)
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
	if v.Kind() == reflect.Pointer {
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
			// Support []string fields for multi-value query params (e.g. statuses[]=a&statuses[]=b).
			if fv.Kind() == reflect.Slice && fv.Type().Elem().Kind() == reflect.String {
				vals := r.URL.Query()[tag]
				if len(vals) == 0 {
					vals = r.URL.Query()[tag+"[]"]
				}
				if len(vals) > 0 {
					fv.Set(reflect.ValueOf(vals))
				}
			} else if val := r.URL.Query().Get(tag); val != "" {
				if err := setStringField(fv, val); err != nil {
					return fmt.Errorf("query param %q: %w", tag, err)
				}
			}
		}
		if tag := field.Tag.Get("header"); tag != "" {
			if val := r.Header.Get(tag); val != "" {
				if err := setStringField(fv, val); err != nil {
					return fmt.Errorf("header %q: %w", tag, err)
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
	case reflect.Float64:
		f, err := strconv.ParseFloat(val, 64)
		if err != nil {
			return err
		}
		fv.SetFloat(f)
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
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	return v.Kind() == reflect.Struct && v.FieldByName("Body").IsValid()
}

// nullByteStrippingReader wraps an io.Reader and replaces null bytes (\x00)
// with spaces to prevent Postgres "invalid byte sequence" errors.
type nullByteStrippingReader struct {
	r io.Reader
}

func (n *nullByteStrippingReader) Read(p []byte) (int, error) {
	nr, err := n.r.Read(p)
	for i := range nr {
		if p[i] == 0 {
			p[i] = ' '
		}
	}
	return nr, err
}

func decodeBody(s *Server, r *http.Request, input any) error {
	v := reflect.ValueOf(input)
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	bodyField := v.FieldByName("Body")
	if !bodyField.IsValid() || !bodyField.CanAddr() {
		return nil
	}
	defer r.Body.Close()
	dec := json.NewDecoder(&nullByteStrippingReader{r: io.LimitReader(r.Body, s.maxRequestBodySize)})
	err := dec.Decode(bodyField.Addr().Interface())
	if errors.Is(err, io.EOF) {
		return nil // empty body is OK -- fields stay at zero values
	}
	if err == nil {
		// Strip null bytes from decoded string fields. JSON \u0000 escapes
		// are decoded by encoding/json into real \x00 bytes which Postgres
		// rejects with "invalid byte sequence".
		stripNullBytesFromStruct(bodyField)
	}
	return err
}

// stripNullBytesFromStruct recursively removes \x00 bytes from all string values
// reachable from v, preventing Postgres "invalid byte sequence for encoding UTF8:
// 0x00" errors. It traverses structs, pointers, slices, arrays, maps, and
// interfaces so that NUL bytes decoded from JSON \x00 escapes inside collection
// fields (tags, labels, metadata, nested step structs) are stripped, not just
// those in top-level string fields. Map values and interface contents are not
// addressable, so those are rebuilt through addressable copies.
func stripNullBytesFromStruct(v reflect.Value) {
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return
		}
		v = v.Elem()
	}
	switch v.Kind() {
	case reflect.Struct:
		for i := range v.NumField() { //nolint:modernize // Fields() returns StructField not Value
			stripNullBytesFromStruct(v.Field(i))
		}
	case reflect.Slice, reflect.Array:
		for i := range v.Len() {
			stripNullBytesFromStruct(v.Index(i))
		}
	case reflect.Map:
		stripNullBytesFromMap(v)
	case reflect.Interface:
		if v.IsNil() || !v.CanSet() {
			return
		}
		inner := v.Elem()
		sanitized := reflect.New(inner.Type()).Elem()
		sanitized.Set(inner)
		stripNullBytesFromStruct(sanitized)
		v.Set(sanitized)
	case reflect.String:
		if v.CanSet() {
			s := v.String()
			if strings.ContainsRune(s, 0) {
				v.SetString(strings.ReplaceAll(s, "\x00", ""))
			}
		}
	default:
		return
	}
}

// stripNullBytesFromMap rebuilds a map's string-bearing keys and values with NUL
// bytes removed. Map entries are not addressable, so each key and value is copied
// into an addressable temporary, sanitized recursively, and written back.
func stripNullBytesFromMap(v reflect.Value) {
	// CanInterface is false for values reached through unexported fields; their
	// keys/values cannot be read or written via reflection, so skip them rather
	// than panic. Request input structs expose their collection fields, so this
	// only guards against incidental unexported maps in nested types.
	if v.IsNil() || !v.CanInterface() {
		return
	}
	for _, k := range v.MapKeys() {
		val := v.MapIndex(k)
		sanitizedVal := reflect.New(val.Type()).Elem()
		sanitizedVal.Set(val)
		stripNullBytesFromStruct(sanitizedVal)

		sanitizedKey := reflect.New(k.Type()).Elem()
		sanitizedKey.Set(k)
		stripNullBytesFromStruct(sanitizedKey)

		if sanitizedKey.Interface() != k.Interface() {
			v.SetMapIndex(k, reflect.Value{})
		}
		v.SetMapIndex(sanitizedKey, sanitizedVal)
	}
}

func extractBodyField(output any) any {
	v := reflect.ValueOf(output)
	if v.Kind() == reflect.Pointer {
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
	// Check for raw JSON status errors (SDK handlers return custom JSON bodies).
	var rse *rawStatusError
	if errors.As(err, &rse) {
		respondJSON(w, rse.status, rse.body)
		return
	}
	// Check for typed API errors that carry a full APIError body.
	var tae *typedAPIError
	if errors.As(err, &tae) {
		for key, value := range tae.headers {
			w.Header().Set(key, value)
		}
		respondError(w, r, tae.status, tae.apiError)
		return
	}
	// Check for huma status errors (e.g., huma.Error404NotFound).
	var se huma.StatusError
	if errors.As(err, &se) {
		status := se.GetStatus()
		if status >= http.StatusInternalServerError {
			slog.Error("typed handler returned 5xx status error", "status", status, "error", err, "path", r.URL.Path)
			respondError(w, r, status, "internal server error")
			return
		}
		respondError(w, r, status, se.Error())
		return
	}
	// Check for billing limit errors. Convert to the canonical 402
	// quota_exceeded body (or 503 for fail-open service_degraded) so
	// handlers that surface a raw *billing.LimitError still get the
	// structured response shape.
	var le *billing.LimitError
	if errors.As(err, &le) {
		if converted := newQuotaExceeded(le, ""); converted != nil {
			var rse *rawStatusError
			if errors.As(converted, &rse) {
				respondJSON(w, rse.status, rse.body)
				return
			}
		}
		respondError(w, r, http.StatusPaymentRequired, le)
		return
	}
	slog.Error("unhandled error in typed handler", "error", err, "path", r.URL.Path)
	respondError(w, r, http.StatusInternalServerError, "internal server error")
}

// typedAPIError wraps an APIError with an HTTP status code for use in typed handlers.
// It is checked first in writeTypedError so the full APIError (with Code, Message,
// Details) reaches the client.
type typedAPIError struct {
	status   int
	apiError APIError
	headers  map[string]string
}

func (e *typedAPIError) Error() string {
	return e.apiError.Message
}

func (e *typedAPIError) GetStatus() int {
	return e.status
}

// rawStatusError writes a raw JSON body with a specific HTTP status code.
// It is used by SDK handlers that return structured error bodies (e.g.,
// {"error": "token_budget_exceeded", "current": 100, "limit": 50}) that
// must not be wrapped in the standard ErrorResponse envelope.
type rawStatusError struct {
	status int
	body   any
}

func (e *rawStatusError) Error() string {
	return fmt.Sprintf("raw status error %d", e.status)
}

func (e *rawStatusError) GetStatus() int {
	return e.status
}

// newValidationError creates a typedAPIError for struct validation failures.
// Field-level validation failures are surfaced as 422 Unprocessable Entity
// with the canonical validation_failed code; non-validation errors fall back
// to 400 bad_request.
func newValidationError(err error) error {
	var ve validator.ValidationErrors
	if errors.As(err, &ve) {
		messages := make([]string, 0, len(ve))
		for _, fe := range ve {
			messages = append(messages, fmt.Sprintf("%s: failed on '%s'", fe.Field(), fe.Tag()))
		}
		return &typedAPIError{
			status: http.StatusUnprocessableEntity,
			apiError: APIError{
				Code:    ErrorCodeValidationFailed,
				Message: "validation failed",
				Details: messages,
			},
		}
	}
	return &typedAPIError{
		status: http.StatusBadRequest,
		apiError: APIError{
			Code:    ErrorCodeBadRequest,
			Message: "invalid request",
		},
	}
}

// OpMeta holds OpenAPI operation metadata used by RegisterTypedOp.
type OpMeta struct {
	ID          string
	Method      string
	Path        string
	Summary     string
	Description string
	Tags        []string
	Security    []map[string][]string
	Errors      []int
}

var bearerSecurity = []map[string][]string{{"bearerAuth": {}}}

// RegisterTypedOp registers a Huma doc-only operation using the real handler's
// Input/Output types. This eliminates the need for separate stub types in
// huma_operations.go -- the OpenAPI spec is generated directly from the types
// the actual handler uses.
func RegisterTypedOp[I any, O any](api huma.API, meta OpMeta, _ func(context.Context, *I) (*O, error)) {
	huma.Register(api, huma.Operation{
		OperationID: meta.ID,
		Method:      meta.Method,
		Path:        meta.Path,
		Summary:     meta.Summary,
		Description: meta.Description,
		Tags:        meta.Tags,
		Security:    meta.Security,
		Errors:      meta.Errors,
	}, func(_ context.Context, _ *I) (*O, error) {
		return nil, nil //nolint:nilnil // doc-only stub for OpenAPI generation
	})
}
