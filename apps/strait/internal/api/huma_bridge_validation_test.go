package api

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestNewValidationError_UsesJSONFieldNames is the regression guard for the
// internal-field-name disclosure: validation messages must reference the public
// JSON field name, not the Go struct field name.
func TestNewValidationError_UsesJSONFieldNames(t *testing.T) {
	t.Parallel()
	type reqBody struct {
		ProjectID string `json:"project_id" validate:"required"`
	}
	v := newRequestValidator()
	verr := v.Struct(&reqBody{})
	require.Error(t, verr)

	var te *typedAPIError
	require.ErrorAs(t, newValidationError(verr), &te)
	require.NotEmpty(t, te.apiError.Details)
	joined := strings.Join(te.apiError.Details, " ")
	require.Contains(t, joined, "project_id")
	require.NotContains(t, joined, "ProjectID", "must not leak the Go struct field name")
}
