package eventfilter

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEval(t *testing.T) {
	validPayload := json.RawMessage(`{"type":"deploy","env":"prod","user":{"name":"alice","role":"admin"}}`)

	tests := []struct {
		name       string
		filter     json.RawMessage
		payload    json.RawMessage
		want       bool
		wantErr    bool
		errContain string
	}{
		{
			name:    "nil filter matches everything",
			filter:  nil,
			payload: validPayload,
			want:    true,
		},
		{
			name:    "empty filter matches everything",
			filter:  json.RawMessage(``),
			payload: validPayload,
			want:    true,
		},
		{
			name:    "null filter matches everything",
			filter:  json.RawMessage(`null`),
			payload: validPayload,
			want:    true,
		},
		{
			name:    "eq condition match",
			filter:  json.RawMessage(`{"eq":[["type","deploy"]]}`),
			payload: validPayload,
			want:    true,
		},
		{
			name:    "eq condition no match",
			filter:  json.RawMessage(`{"eq":[["type","build"]]}`),
			payload: validPayload,
			want:    false,
		},
		{
			name:    "ne condition match",
			filter:  json.RawMessage(`{"ne":[["type","build"]]}`),
			payload: validPayload,
			want:    true,
		},
		{
			name:    "ne condition no match",
			filter:  json.RawMessage(`{"ne":[["type","deploy"]]}`),
			payload: validPayload,
			want:    false,
		},
		{
			name:    "has condition field exists",
			filter:  json.RawMessage(`{"has":["env"]}`),
			payload: validPayload,
			want:    true,
		},
		{
			name:    "has condition field missing",
			filter:  json.RawMessage(`{"has":["region"]}`),
			payload: validPayload,
			want:    false,
		},
		{
			name:    "nested field access via dot notation",
			filter:  json.RawMessage(`{"eq":[["user.name","alice"]]}`),
			payload: validPayload,
			want:    true,
		},
		{
			name:    "nested field dot notation no match",
			filter:  json.RawMessage(`{"eq":[["user.name","bob"]]}`),
			payload: validPayload,
			want:    false,
		},
		{
			name:    "nested has condition",
			filter:  json.RawMessage(`{"has":["user.role"]}`),
			payload: validPayload,
			want:    true,
		},
		{
			name:    "nested has condition missing",
			filter:  json.RawMessage(`{"has":["user.email"]}`),
			payload: validPayload,
			want:    false,
		},
		{
			name:    "combined conditions all pass",
			filter:  json.RawMessage(`{"eq":[["type","deploy"]],"has":["env"]}`),
			payload: validPayload,
			want:    true,
		},
		{
			name:    "combined conditions eq fails",
			filter:  json.RawMessage(`{"eq":[["type","build"]],"has":["env"]}`),
			payload: validPayload,
			want:    false,
		},
		{
			name:    "combined conditions has fails",
			filter:  json.RawMessage(`{"eq":[["type","deploy"]],"has":["region"]}`),
			payload: validPayload,
			want:    false,
		},
		{
			name:    "combined eq ne has all pass",
			filter:  json.RawMessage(`{"eq":[["type","deploy"]],"ne":[["env","staging"]],"has":["user.name"]}`),
			payload: validPayload,
			want:    true,
		},
		{
			name:    "combined eq ne has ne fails",
			filter:  json.RawMessage(`{"eq":[["type","deploy"]],"ne":[["env","prod"]],"has":["user.name"]}`),
			payload: validPayload,
			want:    false,
		},
		{
			name:       "invalid filter expression",
			filter:     json.RawMessage(`{not json`),
			payload:    validPayload,
			wantErr:    true,
			errContain: "invalid filter expression",
		},
		{
			name:       "invalid payload",
			filter:     json.RawMessage(`{"eq":[["type","deploy"]]}`),
			payload:    json.RawMessage(`{not json`),
			wantErr:    true,
			errContain: "invalid payload",
		},
		{
			name:    "eq on missing field does not match",
			filter:  json.RawMessage(`{"eq":[["missing","value"]]}`),
			payload: validPayload,
			want:    false,
		},
		{
			name:    "ne on missing field matches",
			filter:  json.RawMessage(`{"ne":[["missing","value"]]}`),
			payload: validPayload,
			want:    true,
		},
		{
			name:    "multiple eq conditions all match",
			filter:  json.RawMessage(`{"eq":[["type","deploy"],["env","prod"]]}`),
			payload: validPayload,
			want:    true,
		},
		{
			name:    "multiple eq conditions second fails",
			filter:  json.RawMessage(`{"eq":[["type","deploy"],["env","staging"]]}`),
			payload: validPayload,
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Eval(tt.filter, tt.payload)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContain != "" {
					assert.Contains(t, err.Error(), tt.errContain)
				}
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
