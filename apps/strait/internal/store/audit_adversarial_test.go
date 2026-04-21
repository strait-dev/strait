package store

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/sourcegraph/conc"
)

// --- Signature determinism and sensitivity. ---.

// TestComputeAuditSignature_StrictDeterminism verifies the same event always
// produces the exact same signature across 1000 calls.
func TestComputeAuditSignature_StrictDeterminism(t *testing.T) {
	t.Parallel()
	key, _ := DeriveAuditSigningKey("determinism-test")
	ev := testAuditEvent("ev-1", "proj-1", "create")
	expected := ComputeAuditSignature(ev, key)

	for range 1000 {
		got := ComputeAuditSignature(ev, key)
		if got != expected {
			t.Fatalf("signature changed: %s != %s", got, expected)
		}
	}
}

// TestComputeAuditSignature_ConcurrentSafe verifies signature computation
// is safe under concurrent access.
func TestComputeAuditSignature_ConcurrentSafe(t *testing.T) {
	t.Parallel()
	key, _ := DeriveAuditSigningKey("concurrent-test")
	ev := testAuditEvent("ev-1", "proj-1", "create")
	expected := ComputeAuditSignature(ev, key)

	var wg conc.WaitGroup
	errs := make(chan string, 100)
	for range 100 {
		wg.Go(func() {
			got := ComputeAuditSignature(ev, key)
			if got != expected {
				errs <- got
			}
		})
	}
	wg.Wait()
	close(errs)

	for bad := range errs {
		t.Fatalf("concurrent signature mismatch: %s != %s", bad, expected)
	}
}

// TestComputeAuditSignature_SingleBitChange verifies that changing a single
// character in any field produces a completely different signature.
func TestComputeAuditSignature_SingleBitChange(t *testing.T) {
	t.Parallel()
	key, _ := DeriveAuditSigningKey("bitflip-test")
	ev := testAuditEvent("ev-1", "proj-1", "create")
	baseSig := ComputeAuditSignature(ev, key)

	// Flip last char of each string field.
	fields := []struct {
		name   string
		modify func(*domain.AuditEvent)
	}{
		{"id", func(e *domain.AuditEvent) { e.ID = "ev-2" }},
		{"project_id", func(e *domain.AuditEvent) { e.ProjectID = "proj-2" }},
		{"actor_id", func(e *domain.AuditEvent) { e.ActorID = "actor-2" }},
		{"actor_type", func(e *domain.AuditEvent) { e.ActorType = "user" }},
		{"action", func(e *domain.AuditEvent) { e.Action = "delete" }},
		{"resource_type", func(e *domain.AuditEvent) { e.ResourceType = "workflow" }},
		{"resource_id", func(e *domain.AuditEvent) { e.ResourceID = "job-2" }},
		{"details", func(e *domain.AuditEvent) { e.Details = json.RawMessage(`{"key":"value2"}`) }},
		{"created_at_1ns", func(e *domain.AuditEvent) { e.CreatedAt = e.CreatedAt.Add(time.Nanosecond) }},
		{"previous_hash", func(e *domain.AuditEvent) { e.PreviousHash = "different" }},
	}

	for _, f := range fields {
		t.Run(f.name, func(t *testing.T) {
			t.Parallel()
			modified := testAuditEvent("ev-1", "proj-1", "create")
			f.modify(modified)
			modSig := ComputeAuditSignature(modified, key)
			if modSig == baseSig {
				t.Errorf("changing %s did not change signature", f.name)
			}
			// Verify signatures differ in at least 25% of characters (avalanche).
			diffCount := 0
			for i := range min(len(baseSig), len(modSig)) {
				if baseSig[i] != modSig[i] {
					diffCount++
				}
			}
			if diffCount < len(baseSig)/4 {
				t.Errorf("changing %s: only %d/%d chars differ (poor avalanche)", f.name, diffCount, len(baseSig))
			}
		})
	}
}

// --- Chain integrity verification. ---.

// TestAuditChain_LongChain verifies chain integrity across 100 events.
func TestAuditChain_LongChain(t *testing.T) {
	t.Parallel()
	key, _ := DeriveAuditSigningKey("long-chain-test")

	events := make([]*domain.AuditEvent, 100)
	prevHash := ZeroHash

	for i := range 100 {
		ev := &domain.AuditEvent{
			ID:           "ev-" + strings.Repeat("0", 3-len(string(rune('0'+i/100))))[0:0] + string(rune(i)),
			ProjectID:    "proj-1",
			ActorID:      "actor-1",
			ActorType:    "api_key",
			Action:       "update",
			ResourceType: "job",
			ResourceID:   "job-1",
			Details:      json.RawMessage(`{}`),
			PreviousHash: prevHash,
			CreatedAt:    time.Date(2026, 1, 1, 0, 0, i, 0, time.UTC),
		}
		ev.Signature = ComputeAuditSignature(ev, key)
		events[i] = ev
		prevHash = ev.Signature
	}

	// Verify entire chain.
	expectedPrev := ZeroHash
	for i, ev := range events {
		if ev.PreviousHash != expectedPrev {
			t.Fatalf("event %d: chain broken at previous_hash", i)
		}
		recomputed := ComputeAuditSignature(ev, key)
		if ev.Signature != recomputed {
			t.Fatalf("event %d: signature mismatch", i)
		}
		expectedPrev = ev.Signature
	}
}

// TestAuditChain_Adversarial_DeleteMiddleEvent verifies that deleting an event
// from the middle of the chain is detectable.
func TestAuditChain_Adversarial_DeleteMiddleEvent(t *testing.T) {
	t.Parallel()
	key, _ := DeriveAuditSigningKey("delete-middle-test")

	var events [3]*domain.AuditEvent
	prevHash := ZeroHash
	for i := range 3 {
		ev := &domain.AuditEvent{
			ID: string(rune('a' + i)), ProjectID: "proj-1", ActorID: "actor-1",
			ActorType: "api_key", Action: "create", ResourceType: "job", ResourceID: "job-1",
			Details: json.RawMessage(`{}`), PreviousHash: prevHash,
			CreatedAt: time.Date(2026, 1, 1, 0, 0, i, 0, time.UTC),
		}
		ev.Signature = ComputeAuditSignature(ev, key)
		events[i] = ev
		prevHash = ev.Signature
	}

	// "Delete" middle event -- verify event[2].PreviousHash doesn't match event[0].Signature.
	if events[2].PreviousHash == events[0].Signature {
		t.Fatal("event 2 should chain from event 1, not event 0")
	}
}

// TestAuditChain_Adversarial_ReorderEvents verifies that swapping two events
// breaks the chain.
func TestAuditChain_Adversarial_ReorderEvents(t *testing.T) {
	t.Parallel()
	key, _ := DeriveAuditSigningKey("reorder-test")

	var events [3]*domain.AuditEvent
	prevHash := ZeroHash
	for i := range 3 {
		ev := &domain.AuditEvent{
			ID: string(rune('a' + i)), ProjectID: "proj-1", ActorID: "actor-1",
			ActorType: "api_key", Action: "create", ResourceType: "job", ResourceID: "job-1",
			Details: json.RawMessage(`{}`), PreviousHash: prevHash,
			CreatedAt: time.Date(2026, 1, 1, 0, 0, i, 0, time.UTC),
		}
		ev.Signature = ComputeAuditSignature(ev, key)
		events[i] = ev
		prevHash = ev.Signature
	}

	// Swap events[1] and events[2].
	swapped := [3]*domain.AuditEvent{events[0], events[2], events[1]}

	// Chain verification should fail at position 1.
	expectedPrev := ZeroHash
	for i, ev := range swapped {
		if ev.PreviousHash != expectedPrev {
			// Expected to fail here.
			if i == 0 {
				t.Fatal("should not fail at event 0")
			}
			return // success: reorder detected
		}
		expectedPrev = ev.Signature
	}
	t.Fatal("reordered chain should have been detected")
}

// TestAuditChain_Adversarial_ModifyEventContent verifies that modifying event
// content after signing is detectable.
func TestAuditChain_Adversarial_ModifyEventContent(t *testing.T) {
	t.Parallel()
	key, _ := DeriveAuditSigningKey("modify-content-test")

	ev := testAuditEvent("ev-1", "proj-1", "create")
	ev.PreviousHash = ZeroHash
	ev.Signature = ComputeAuditSignature(ev, key)

	// Tamper: change action after signing.
	ev.Action = "delete"

	recomputed := ComputeAuditSignature(ev, key)
	if ev.Signature == recomputed {
		t.Fatal("tampered event should have different signature")
	}
}

// TestAuditChain_Adversarial_InsertForgedEvent verifies that inserting a
// forged event with a different key is detectable.
func TestAuditChain_Adversarial_InsertForgedEvent(t *testing.T) {
	t.Parallel()
	realKey, _ := DeriveAuditSigningKey("real-key")
	fakeKey, _ := DeriveAuditSigningKey("fake-key")

	ev1 := testAuditEvent("ev-1", "proj-1", "create")
	ev1.PreviousHash = ZeroHash
	ev1.Signature = ComputeAuditSignature(ev1, realKey)

	// Attacker inserts a forged event signed with a different key.
	forged := testAuditEvent("ev-forged", "proj-1", "delete")
	forged.PreviousHash = ev1.Signature
	forged.CreatedAt = ev1.CreatedAt.Add(time.Second)
	forged.Signature = ComputeAuditSignature(forged, fakeKey)

	// Verification with the real key should detect the forgery.
	recomputed := ComputeAuditSignature(forged, realKey)
	if forged.Signature == recomputed {
		t.Fatal("forged event signed with different key should not verify")
	}
}

// --- Key derivation adversarial tests. ---.

// TestDeriveAuditSigningKey_DifferentSecrets produces different keys.
func TestDeriveAuditSigningKey_DifferentSecrets(t *testing.T) {
	t.Parallel()
	key1, _ := DeriveAuditSigningKey("secret-one")
	key2, _ := DeriveAuditSigningKey("secret-two")

	if string(key1) == string(key2) {
		t.Fatal("different secrets should produce different keys")
	}
}

// TestDeriveAuditSigningKey_SimilarSecrets verifies that similar secrets
// produce completely different keys (HKDF avalanche).
func TestDeriveAuditSigningKey_SimilarSecrets(t *testing.T) {
	t.Parallel()
	key1, _ := DeriveAuditSigningKey("my-secret-key-a")
	key2, _ := DeriveAuditSigningKey("my-secret-key-b")

	diffBytes := 0
	for i := range 32 {
		if key1[i] != key2[i] {
			diffBytes++
		}
	}
	// HKDF should cause most bytes to differ.
	if diffBytes < 16 {
		t.Errorf("similar secrets: only %d/32 bytes differ (poor HKDF avalanche)", diffBytes)
	}
}

// --- Edge case tests. ---.

// TestComputeAuditSignature_EmptyFields verifies signature works with all
// empty string fields.
func TestComputeAuditSignature_EmptyFields(t *testing.T) {
	t.Parallel()
	key, _ := DeriveAuditSigningKey("empty-fields-test")
	ev := &domain.AuditEvent{
		ID:           "",
		ProjectID:    "",
		ActorID:      "",
		ActorType:    "",
		Action:       "",
		ResourceType: "",
		ResourceID:   "",
		Details:      json.RawMessage(`{}`),
		PreviousHash: "",
		CreatedAt:    time.Time{},
	}
	sig := ComputeAuditSignature(ev, key)
	if sig == "" {
		t.Fatal("signature should not be empty even for empty fields")
	}
	if len(sig) != 64 {
		t.Errorf("signature length = %d, want 64", len(sig))
	}
}

// TestComputeAuditSignature_NullBytesInFields verifies null bytes don't
// cause signature collisions or truncation.
func TestComputeAuditSignature_NullBytesInFields(t *testing.T) {
	t.Parallel()
	key, _ := DeriveAuditSigningKey("nullbytes-test")

	ev1 := testAuditEvent("ev-1", "proj\x00-1", "create")
	ev2 := testAuditEvent("ev-1", "proj\x00-2", "create")

	sig1 := ComputeAuditSignature(ev1, key)
	sig2 := ComputeAuditSignature(ev2, key)

	if sig1 == sig2 {
		t.Fatal("null bytes in different positions should produce different signatures")
	}
}

// TestComputeAuditSignature_LargeDetails verifies signature works with
// very large detail payloads.
func TestComputeAuditSignature_LargeDetails(t *testing.T) {
	t.Parallel()
	key, _ := DeriveAuditSigningKey("large-details-test")

	largeJSON := `{"data":"` + strings.Repeat("x", 100000) + `"}`
	ev := testAuditEvent("ev-1", "proj-1", "create")
	ev.Details = json.RawMessage(largeJSON)

	sig := ComputeAuditSignature(ev, key)
	if sig == "" || len(sig) != 64 {
		t.Fatalf("signature for large payload: len=%d, want 64", len(sig))
	}
}

// --- Fuzz tests. ---.

// FuzzComputeAuditSignature_Deterministic verifies that computing the same
// event signature twice always produces identical results.
func FuzzComputeAuditSignature_Deterministic(f *testing.F) {
	f.Add("ev-1", "proj-1", "create", "job", `{"k":"v"}`, "prev")
	f.Add("", "", "", "", "{}", "")
	f.Add("null\x00byte", "proj", "act", "res", `null`, "hash")

	key, _ := DeriveAuditSigningKey("fuzz-determinism")

	f.Fuzz(func(t *testing.T, id, projectID, action, resourceType, details, prevHash string) {
		ev := &domain.AuditEvent{
			ID: id, ProjectID: projectID, ActorID: "a", ActorType: "api_key",
			Action: action, ResourceType: resourceType, ResourceID: "r",
			Details: json.RawMessage(details), PreviousHash: prevHash,
			CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		}
		sig1 := ComputeAuditSignature(ev, key)
		sig2 := ComputeAuditSignature(ev, key)
		if sig1 != sig2 {
			t.Fatal("non-deterministic signature")
		}
	})
}

// FuzzDeriveAuditSigningKey_NoPanic verifies key derivation never panics.
func FuzzDeriveAuditSigningKey_NoPanic(f *testing.F) {
	f.Add("normal-secret")
	f.Add("")
	f.Add(strings.Repeat("x", 10000))
	f.Add("null\x00bytes")

	f.Fuzz(func(t *testing.T, secret string) {
		key, err := DeriveAuditSigningKey(secret)
		if secret == "" {
			if err == nil {
				t.Fatal("empty secret should error")
			}
			return
		}
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(key) != 32 {
			t.Fatalf("key length = %d, want 32", len(key))
		}
	})
}
