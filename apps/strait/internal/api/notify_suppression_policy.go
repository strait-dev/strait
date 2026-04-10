package api

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/danielgtaylor/huma/v2"
)

func (s *Server) enforceNotifyUnsuppressPolicy(
	ctx context.Context,
	ns notifyStore,
	projectID,
	recipientID,
	channel string,
	force,
	selfService bool,
) error {
	if !strings.EqualFold(channel, "email") {
		return nil
	}

	event, err := ns.GetLatestNotifySuppressionEvent(
		ctx,
		projectID,
		domain.NotifyRecipientTypeSubscriber,
		recipientID,
		"global",
		"email",
	)
	if err != nil {
		if errors.Is(err, store.ErrNotifySuppressionEventNotFound) {
			return nil
		}
		return huma.Error500InternalServerError("failed to evaluate suppression policy")
	}

	if event.Action != domain.NotifySuppressionActionSuppressed {
		return nil
	}
	if !domain.NotifySuppressionReasonRequiresManualOverride(event.Reason) {
		return nil
	}
	if force {
		return nil
	}

	if selfService {
		return huma.Error409Conflict("self-service unsuppress is blocked for provider complaint/bounce suppressions")
	}
	return huma.Error409Conflict("unsuppress requires force=true for provider complaint/bounce suppressions")
}

func notifyChannelPrefExplicitEnableEmail(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}

	prefs := map[string]any{}
	if err := json.Unmarshal(raw, &prefs); err != nil {
		return false
	}

	value, ok := prefs["email"]
	if !ok {
		return false
	}
	enabled, ok := value.(bool)
	if !ok {
		return false
	}

	return enabled
}
