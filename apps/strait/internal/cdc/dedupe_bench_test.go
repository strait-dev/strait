package cdc

import (
	"fmt"
	"testing"
)

func TestRecentDedupe_EvictsOldestAndKeepsRecent(t *testing.T) {
	t.Parallel()

	d := newRecentDedupe(3)
	if !d.Remember("a") || !d.Remember("b") || !d.Remember("c") {
		t.Fatal("initial keys should be accepted")
	}
	if d.Remember("a") {
		t.Fatal("recent key should be suppressed before eviction")
	}
	if !d.Remember("d") {
		t.Fatal("new key should be accepted")
	}
	if !d.Remember("a") {
		t.Fatal("oldest key should be accepted after eviction")
	}
	if d.Remember("c") {
		t.Fatal("non-evicted key should still be suppressed")
	}
}

func BenchmarkRecentDedupeRememberSteadyState(b *testing.B) {
	const limit = 16_384

	keys := make([]string, limit+b.N)
	for i := range keys {
		keys[i] = fmt.Sprintf("cdc:key:%08d", i)
	}
	d := newRecentDedupe(limit)
	for i := range limit {
		_ = d.Remember(keys[i])
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		if !d.Remember(keys[limit+i]) {
			b.Fatal("new key should be accepted")
		}
	}
}
