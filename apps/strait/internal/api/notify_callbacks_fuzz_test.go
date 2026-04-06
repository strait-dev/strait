package api

import (
	"testing"
	"time"

	"strait/internal/domain"
)

func FuzzExtractNotifyResendMessageID(f *testing.F) {
	f.Add("msg_1", "")
	f.Add("", "msg_tag")
	f.Add("", "")

	f.Fuzz(func(t *testing.T, directMessageID, taggedMessageID string) {
		payload := map[string]any{}
		if directMessageID != "" {
			payload["message_id"] = directMessageID
		}

		data := map[string]any{}
		if taggedMessageID != "" {
			data["tags"] = []any{map[string]any{"name": "strait_message_id", "value": taggedMessageID}}
		}
		payload["data"] = data

		got := extractNotifyResendMessageID(payload)
		switch {
		case directMessageID != "":
			if got != directMessageID {
				t.Fatalf("extractNotifyResendMessageID() = %q, want direct %q", got, directMessageID)
			}
		case taggedMessageID != "":
			if got != taggedMessageID {
				t.Fatalf("extractNotifyResendMessageID() = %q, want tag %q", got, taggedMessageID)
			}
		default:
			if got != "" {
				t.Fatalf("extractNotifyResendMessageID() = %q, want empty", got)
			}
		}
	})
}

func FuzzResolveNotifyResendCallbackOutcome(f *testing.F) {
	f.Add("email.delivered")
	f.Add("email.bounced")
	f.Add("email.complained")
	f.Add("unknown")

	f.Fuzz(func(t *testing.T, eventType string) {
		status, fields, suppress := resolveNotifyResendCallbackOutcome(eventType, time.Unix(0, 0).UTC())

		if status == "" {
			if fields != nil {
				t.Fatalf("fields = %v, want nil for unsupported status", fields)
			}
			if suppress {
				t.Fatal("suppress = true for unsupported status")
			}
			return
		}

		if fields == nil {
			t.Fatal("fields = nil for supported status")
		}
		if status == domain.NotifyMessageStatusDelivered {
			if suppress {
				t.Fatal("delivered status should not suppress")
			}
		}
	})
}

func FuzzShouldApplyNotifyProviderCallbackTransition(f *testing.F) {
	statuses := []string{
		domain.NotifyMessageStatusRendering,
		domain.NotifyMessageStatusScheduled,
		domain.NotifyMessageStatusPending,
		domain.NotifyMessageStatusProcessing,
		domain.NotifyMessageStatusDelivered,
		domain.NotifyMessageStatusFailed,
		domain.NotifyMessageStatusBounced,
		domain.NotifyMessageStatusCancelled,
		"",
	}

	for _, current := range statuses {
		for _, next := range statuses {
			f.Add(current, next)
		}
	}

	f.Fuzz(func(t *testing.T, currentStatus, nextStatus string) {
		got := shouldApplyNotifyProviderCallbackTransition(currentStatus, nextStatus)

		if nextStatus == "" && got {
			t.Fatalf("transition %q -> %q = true, want false", currentStatus, nextStatus)
		}
		if currentStatus == nextStatus && got {
			t.Fatalf("transition %q -> %q = true, want false", currentStatus, nextStatus)
		}
		if isTerminalNotifyMessageStatus(currentStatus) && got {
			t.Fatalf("transition %q -> %q = true, want false for terminal current", currentStatus, nextStatus)
		}

		if !isTerminalNotifyMessageStatus(currentStatus) && nextStatus != "" && currentStatus != nextStatus && !got {
			t.Fatalf("transition %q -> %q = false, want true", currentStatus, nextStatus)
		}
	})
}

func FuzzHashNotifyProviderCallbackPayload(f *testing.F) {
	f.Add("{}")
	f.Add("{\"type\":\"email.delivered\"}")

	f.Fuzz(func(t *testing.T, payload string) {
		h1 := hashNotifyProviderCallbackPayload([]byte(payload))
		h2 := hashNotifyProviderCallbackPayload([]byte(payload))
		if h1 != h2 {
			t.Fatalf("hash mismatch for identical payload: %q != %q", h1, h2)
		}
		if len(h1) != 64 {
			t.Fatalf("hash length = %d, want 64", len(h1))
		}
	})
}
