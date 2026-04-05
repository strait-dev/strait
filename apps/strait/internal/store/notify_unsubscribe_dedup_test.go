package store

import "testing"

func TestHashUnsubscribeToken_Deterministic(t *testing.T) {
	t.Parallel()

	got1 := hashUnsubscribeToken("tok_123")
	got2 := hashUnsubscribeToken("tok_123")
	got3 := hashUnsubscribeToken("tok_456")

	if got1 == "" {
		t.Fatal("hashUnsubscribeToken() returned empty hash")
	}
	if got1 != got2 {
		t.Fatalf("hash mismatch for same token: %q vs %q", got1, got2)
	}
	if got1 == got3 {
		t.Fatalf("expected different hashes for different tokens, got %q", got1)
	}
}
