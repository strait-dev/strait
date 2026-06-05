//go:build !cloud

package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseEdition_Community(t *testing.T) {
	// In the community build, ParseEdition always returns EditionCommunity
	// regardless of input. This prevents self-hosters from enabling cloud
	// features by setting STRAIT_EDITION=cloud.
	tests := []struct {
		input string
	}{
		{"community"},
		{"cloud"},
		{""},
		{"unknown"},
		{"Cloud"},
		{"CLOUD"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ParseEdition(tt.input)
			assert.Equal(t,

				EditionCommunity,

				got)
		})
	}
}
