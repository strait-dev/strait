package domain

import "testing"

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
			if got := tt.fn(); got != tt.want {
				t.Errorf("%s.%s() = %v, want %v", tt.edition, tt.method, got, tt.want)
			}
		})
	}
}
