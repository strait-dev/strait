package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	emailverifier "github.com/AfterShip/email-verifier"
	"github.com/danielgtaylor/huma/v2"
	normalizer "github.com/dimuska139/go-email-normalizer/v5"
)

var notifyEmailNormalizer = normalizer.NewNormalizer()

func (s *Server) sanitizeNotifySubscriberEmail(rawEmail string, attributes json.RawMessage) (string, json.RawMessage, error) {
	email := strings.TrimSpace(rawEmail)
	if email == "" {
		return "", attributes, nil
	}

	if s.config.NotifyEmailNormalizeEnabled {
		email = strings.TrimSpace(notifyEmailNormalizer.Normalize(email))
	}

	verificationMeta := map[string]any{
		"normalized": email,
		"checked_at": time.Now().UTC().Format(time.RFC3339Nano),
	}

	if s.config.NotifyEmailVerifyEnabled {
		result, err := emailverifier.NewVerifier().Verify(email)
		if err != nil {
			slog.Warn("notify email verification failed", "email", email, "error", err)
			return "", nil, huma.Error400BadRequest("unable to verify email address")
		}
		if !result.Syntax.Valid {
			return "", nil, huma.Error400BadRequest("invalid email syntax")
		}
		if s.config.NotifyEmailVerifyMX && !result.HasMxRecords {
			return "", nil, huma.Error400BadRequest("email domain has no MX records")
		}

		verificationMeta["has_mx_records"] = result.HasMxRecords
		verificationMeta["reachable"] = result.Reachable
		verificationMeta["disposable"] = result.Disposable
		verificationMeta["free"] = result.Free
		verificationMeta["role_account"] = result.RoleAccount
	}

	mergedAttributes, err := mergeNotifySubscriberAttributes(attributes, verificationMeta)
	if err != nil {
		return "", nil, huma.Error400BadRequest("attributes must be a valid JSON object")
	}

	return email, mergedAttributes, nil
}

func mergeNotifySubscriberAttributes(base json.RawMessage, emailMeta map[string]any) (json.RawMessage, error) {
	attrs := map[string]any{}
	if len(base) > 0 {
		if err := json.Unmarshal(base, &attrs); err != nil {
			return nil, fmt.Errorf("unmarshal subscriber attributes: %w", err)
		}
	}

	attrs["email_verification"] = emailMeta

	encoded, err := json.Marshal(attrs)
	if err != nil {
		return nil, fmt.Errorf("marshal subscriber attributes: %w", err)
	}

	return encoded, nil
}
