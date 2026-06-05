package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEditionCapabilities(t *testing.T) {
	tests := []struct {
		edition Edition
		method  string
		fn      func() bool
		want    bool
	}{
		{EditionCommunity, "AllowsAdvancedAnalytics", EditionCommunity.AllowsAdvancedAnalytics, false},
		{EditionCloud, "AllowsAdvancedAnalytics", EditionCloud.AllowsAdvancedAnalytics, true},
		{EditionCommunity, "RequiresHTTPModeGating", EditionCommunity.RequiresHTTPModeGating, false},
		{EditionCloud, "RequiresHTTPModeGating", EditionCloud.RequiresHTTPModeGating, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.edition)+"/"+tt.method, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.fn())
		})
	}
}
