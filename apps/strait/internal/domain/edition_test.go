package domain

import "testing"

func TestEditionCapabilities(t *testing.T) {
	tests := []struct {
		edition Edition
		method  string
		fn      func() bool
		want    bool
	}{
		{EditionCommunity, "AllowsManagedExecution", EditionCommunity.AllowsManagedExecution, false},
		{EditionCommunity, "AllowsMultiRegion", EditionCommunity.AllowsMultiRegion, false},
		{EditionCommunity, "AllowsAdvancedAnalytics", EditionCommunity.AllowsAdvancedAnalytics, false},
		{EditionCommunity, "AllowsWarmPool", EditionCommunity.AllowsWarmPool, false},
		{EditionCloud, "AllowsManagedExecution", EditionCloud.AllowsManagedExecution, true},
		{EditionCloud, "AllowsMultiRegion", EditionCloud.AllowsMultiRegion, true},
		{EditionCloud, "AllowsAdvancedAnalytics", EditionCloud.AllowsAdvancedAnalytics, true},
		{EditionCloud, "AllowsWarmPool", EditionCloud.AllowsWarmPool, true},
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
