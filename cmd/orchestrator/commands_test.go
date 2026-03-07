package main

import "testing"

func TestFieldsToJSON(t *testing.T) {
	t.Parallel()

	fields := []string{"name=test", "priority=10", "enabled=true", "meta={\"k\":\"v\"}"}
	got, err := fieldsToJSON(fields)
	if err != nil {
		t.Fatalf("fieldsToJSON err: %v", err)
	}
	if got["name"] != "test" {
		t.Fatalf("name mismatch")
	}
	if got["priority"].(float64) != 10 {
		t.Fatalf("priority mismatch")
	}
}

func TestParseWaitCondition(t *testing.T) {
	t.Parallel()

	status, err := parseWaitCondition("status=completed")
	if err != nil {
		t.Fatalf("parse err: %v", err)
	}
	if status != "completed" {
		t.Fatalf("status mismatch: %s", status)
	}
}
