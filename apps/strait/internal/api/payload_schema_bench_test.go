package api

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidatePayloadAgainstSchemaReportsNestedArrayPaths(t *testing.T) {
	t.Parallel()

	schema := json.RawMessage(`{
		"type":"object",
		"properties":{
			"items":{
				"type":"array",
				"items":{
					"type":"object",
					"required":["id"],
					"properties":{"id":{"type":"string"}}
				}
			}
		}
	}`)

	err := validatePayloadAgainstSchema(json.RawMessage(`{"items":[{"id":"ok"},{}]}`), schema)
	require.EqualError(t, err, "$.items[1].id is required")

	err = validatePayloadAgainstSchema(json.RawMessage(`{"items":[{"id":"ok"},{"id":42}]}`), schema)
	require.EqualError(t, err, "$.items[1].id must be string")
}

func BenchmarkValidatePayloadAgainstSchemaArray(b *testing.B) {
	payload := json.RawMessage(`{
		"items":[
			{"id":"run-001","attempt":1,"enabled":true},
			{"id":"run-002","attempt":2,"enabled":false},
			{"id":"run-003","attempt":3,"enabled":true},
			{"id":"run-004","attempt":4,"enabled":false},
			{"id":"run-005","attempt":5,"enabled":true},
			{"id":"run-006","attempt":6,"enabled":false},
			{"id":"run-007","attempt":7,"enabled":true},
			{"id":"run-008","attempt":8,"enabled":false}
		]
	}`)
	schema := json.RawMessage(`{
		"type":"object",
		"required":["items"],
		"properties":{
			"items":{
				"type":"array",
				"items":{
					"type":"object",
					"required":["id","attempt","enabled"],
					"properties":{
						"id":{"type":"string"},
						"attempt":{"type":"integer"},
						"enabled":{"type":"boolean"}
					}
				}
			}
		}
	}`)

	b.ReportAllocs()
	for b.Loop() {
		if err := validatePayloadAgainstSchema(payload, schema); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkValidateArraySchemaNode(b *testing.B) {
	var value any
	require.NoError(b, json.Unmarshal(json.RawMessage(`[
		{"id":"run-001","attempt":1,"enabled":true},
		{"id":"run-002","attempt":2,"enabled":false},
		{"id":"run-003","attempt":3,"enabled":true},
		{"id":"run-004","attempt":4,"enabled":false},
		{"id":"run-005","attempt":5,"enabled":true},
		{"id":"run-006","attempt":6,"enabled":false},
		{"id":"run-007","attempt":7,"enabled":true},
		{"id":"run-008","attempt":8,"enabled":false}
	]`), &value))

	var schema map[string]any
	require.NoError(b, json.Unmarshal(json.RawMessage(`{
		"type":"array",
		"items":{
			"type":"object",
			"required":["id","attempt","enabled"],
			"properties":{
				"id":{"type":"string"},
				"attempt":{"type":"integer"},
				"enabled":{"type":"boolean"}
			}
		}
	}`), &schema))

	path := newPayloadSchemaPath().withKey("items")

	b.ReportAllocs()
	for b.Loop() {
		if err := validateArraySchemaNode(value, schema, path, 1); err != nil {
			b.Fatal(err)
		}
	}
}
