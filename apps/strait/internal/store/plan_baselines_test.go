package store

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsExplainableSelect(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		sql  string
		want bool
	}{
		{name: "select", sql: "SELECT * FROM job_runs", want: true},
		{name: "lowercase select", sql: "select * from job_runs", want: true},
		{name: "blank", sql: "", want: false},
		{name: "whitespace", sql: "   ", want: false},
		{name: "insert", sql: "INSERT INTO job_runs DEFAULT VALUES", want: false},
		{name: "semicolon", sql: "SELECT * FROM job_runs;", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.want, isExplainableSelect(strings.TrimSpace(tt.sql)))
		})
	}
}
