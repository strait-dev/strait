package main

import (
	"testing"
	"time"
)

func TestMatchesLogRow_NoFilters(t *testing.T) {
	t.Parallel()

	row := map[string]any{
		"run_id":    "run-1",
		"timestamp": time.Now(),
		"level":     "info",
		"type":      "log",
		"message":   "hello world",
	}
	if !matchesLogRow(row, "", "", "", time.Time{}) {
		t.Fatal("expected match with no filters")
	}
}

func TestMatchesLogRow_SinceFilter(t *testing.T) {
	t.Parallel()

	sinceTime := time.Now().Add(-1 * time.Hour)

	old := map[string]any{
		"timestamp": time.Now().Add(-2 * time.Hour),
		"level":     "info",
		"type":      "log",
		"message":   "old",
	}
	if matchesLogRow(old, "", "", "", sinceTime) {
		t.Fatal("expected old row to be filtered out")
	}

	recent := map[string]any{
		"timestamp": time.Now().Add(-30 * time.Minute),
		"level":     "info",
		"type":      "log",
		"message":   "recent",
	}
	if !matchesLogRow(recent, "", "", "", sinceTime) {
		t.Fatal("expected recent row to match")
	}
}

func TestMatchesLogRow_SearchFilter(t *testing.T) {
	t.Parallel()

	row := map[string]any{
		"timestamp": time.Now(),
		"level":     "error",
		"type":      "log",
		"message":   "Payment Error occurred",
	}
	if !matchesLogRow(row, "", "", "error", time.Time{}) {
		t.Fatal("expected case-insensitive search match")
	}

	row2 := map[string]any{
		"timestamp": time.Now(),
		"level":     "info",
		"type":      "log",
		"message":   "Success",
	}
	if matchesLogRow(row2, "", "", "error", time.Time{}) {
		t.Fatal("expected no match for non-matching search")
	}
}

func TestMatchesLogRow_LevelFilter(t *testing.T) {
	t.Parallel()

	row := map[string]any{
		"timestamp": time.Now(),
		"level":     "info",
		"type":      "log",
		"message":   "test",
	}
	if !matchesLogRow(row, "info", "", "", time.Time{}) {
		t.Fatal("expected match for matching level")
	}
	if matchesLogRow(row, "warn", "", "", time.Time{}) {
		t.Fatal("expected no match for non-matching level")
	}
}

func TestMatchesLogRow_LevelFilter_CaseInsensitive(t *testing.T) {
	t.Parallel()

	row := map[string]any{
		"timestamp": time.Now(),
		"level":     "ERROR",
		"type":      "log",
		"message":   "something broke",
	}
	if !matchesLogRow(row, "error", "", "", time.Time{}) {
		t.Fatal("expected case-insensitive level match: ERROR should match --level=error")
	}
	if !matchesLogRow(row, "Error", "", "", time.Time{}) {
		t.Fatal("expected case-insensitive level match: ERROR should match --level=Error")
	}

	row2 := map[string]any{
		"timestamp": time.Now(),
		"level":     "info",
		"type":      "log",
		"message":   "all good",
	}
	if !matchesLogRow(row2, "INFO", "", "", time.Time{}) {
		t.Fatal("expected case-insensitive level match: info should match --level=INFO")
	}
}

func TestMatchesLogRow_CombinedFilters(t *testing.T) {
	t.Parallel()

	sinceTime := time.Now().Add(-1 * time.Hour)

	row := map[string]any{
		"timestamp": time.Now(),
		"level":     "error",
		"type":      "log",
		"message":   "Payment failed",
	}
	if !matchesLogRow(row, "error", "", "payment", sinceTime) {
		t.Fatal("expected match for all filters")
	}

	row2 := map[string]any{
		"timestamp": time.Now(),
		"level":     "info",
		"type":      "log",
		"message":   "Payment failed",
	}
	if matchesLogRow(row2, "error", "", "payment", sinceTime) {
		t.Fatal("expected no match when level filter fails")
	}
}

func TestPrintGroupedLogs_GroupsBySlug(t *testing.T) {
	t.Parallel()

	rows := []map[string]any{
		{"job_slug": "job-a", "level": "info", "message": "one"},
		{"job_slug": "job-a", "level": "info", "message": "two"},
		{"job_slug": "job-b", "level": "error", "message": "three"},
	}

	state := &appState{opts: &rootOptions{outputFormat: "json"}}
	err := printGroupedLogs(state, rows)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPrintGroupedLogs_UnknownSlug(t *testing.T) {
	t.Parallel()

	rows := []map[string]any{
		{"level": "info", "message": "no slug"},
	}

	state := &appState{opts: &rootOptions{outputFormat: "json"}}
	err := printGroupedLogs(state, rows)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPrintGroupedLogs_LevelCounts(t *testing.T) {
	t.Parallel()

	rows := []map[string]any{
		{"job_slug": "job-a", "level": "info", "message": "one"},
		{"job_slug": "job-a", "level": "warn", "message": "two"},
		{"job_slug": "job-a", "level": "error", "message": "three"},
	}

	state := &appState{opts: &rootOptions{outputFormat: "json"}}
	err := printGroupedLogs(state, rows)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSortLogRows(t *testing.T) {
	t.Parallel()

	now := time.Now()
	rows := []map[string]any{
		{"timestamp": now.Add(2 * time.Minute), "message": "third"},
		{"timestamp": now, "message": "first"},
		{"timestamp": now.Add(1 * time.Minute), "message": "second"},
	}

	sortLogRows(rows)

	msg0, _ := rows[0]["message"].(string)
	msg1, _ := rows[1]["message"].(string)
	msg2, _ := rows[2]["message"].(string)

	if msg0 != "first" || msg1 != "second" || msg2 != "third" {
		t.Fatalf("expected sorted order first/second/third, got %s/%s/%s", msg0, msg1, msg2)
	}
}

func TestLogRowTimestamp_Valid(t *testing.T) {
	t.Parallel()

	now := time.Now()
	row := map[string]any{"timestamp": now}
	got := logRowTimestamp(row)
	if !got.Equal(now) {
		t.Fatalf("expected %v, got %v", now, got)
	}
}

func TestLogRowTimestamp_Missing(t *testing.T) {
	t.Parallel()

	row := map[string]any{"message": "no timestamp"}
	got := logRowTimestamp(row)
	if !got.IsZero() {
		t.Fatalf("expected zero time, got %v", got)
	}
}
