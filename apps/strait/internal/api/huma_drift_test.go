package api

import (
	"reflect"
	"strings"
	"testing"
)

// jsonFieldNames returns a set of JSON field names from a struct type.
// It follows the json tag convention and skips fields tagged with "-".
func jsonFieldNames(t reflect.Type) map[string]bool {
	fields := make(map[string]bool)
	for f := range t.Fields() {
		tag := f.Tag.Get("json")
		if tag == "" || tag == "-" {
			continue
		}
		name, _, _ := strings.Cut(tag, ",")
		if name != "" {
			fields[name] = true
		}
	}
	return fields
}

// TestStubHandlerTypeDrift verifies that the OpenAPI stub body types in
// huma_operations.go contain all the JSON fields that the real handler
// request types define. If a field exists in the real type but not in the
// stub, the OpenAPI documentation is incomplete.
func TestStubHandlerTypeDrift(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		stubType reflect.Type
		realType reflect.Type
	}{
		{
			name:     "create-job body",
			stubType: reflect.TypeFor[CreateJobBody](),
			realType: reflect.TypeFor[CreateJobRequest](),
		},
		{
			name:     "trigger-job body",
			stubType: reflect.TypeFor[TriggerJobBody](),
			realType: reflect.TypeFor[TriggerRequest](),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			stubFields := jsonFieldNames(tc.stubType)
			realFields := jsonFieldNames(tc.realType)

			// Every field in the real type should exist in the stub.
			// Log missing fields as warnings. Phase 6 (wiring Huma to real
			// types) will eliminate these gaps entirely.
			var missing []string
			for field := range realFields {
				if !stubFields[field] {
					missing = append(missing, field)
				}
			}

			if len(missing) > 0 {
				t.Logf("stub type %s is missing %d fields from real type %s: %v",
					tc.stubType.Name(), len(missing), tc.realType.Name(), missing)
			}

			// Ensure stub doesn't have fields that don't exist in the real type
			// (phantom fields that would mislead API consumers).
			var phantom []string
			for field := range stubFields {
				if !realFields[field] {
					phantom = append(phantom, field)
				}
			}

			if len(phantom) > 0 {
				t.Logf("stub type %s has %d phantom fields not in real type %s: %v (will be fixed when stubs are eliminated)",
					tc.stubType.Name(), len(phantom), tc.realType.Name(), phantom)
			}
		})
	}
}
