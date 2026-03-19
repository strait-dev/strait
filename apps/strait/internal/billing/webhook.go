package billing

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"strait/internal/domain"
)

var (
	ErrInvalidSignature = errors.New("invalid webhook signature")
	ErrUnknownProduct   = errors.New("unknown polar product ID")
)

// WebhookHandler handles incoming Polar webhook events.
type WebhookHandler struct {
	store        Store
	polarMapping *PolarMapping
	secret       string
	logger       *slog.Logger
	enforcer     *Enforcer
}

// NewWebhookHandler creates a new Polar webhook handler.
// The enforcer is optional; when non-nil, org caches are invalidated on plan changes.
func NewWebhookHandler(store Store, mapping *PolarMapping, secret string, logger *slog.Logger, enforcer *Enforcer) *WebhookHandler {
	return &WebhookHandler{
		store:        store,
		polarMapping: mapping,
		secret:       secret,
		logger:       logger,
		enforcer:     enforcer,
	}
}

// PolarWebhookPayload represents the top-level Polar webhook envelope.
type PolarWebhookPayload struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// PolarSubscriptionData represents the subscription data in a Polar webhook.
type PolarSubscriptionData struct {
	ID                 string             `json:"id"`
	Status             string             `json:"status"`
	CurrentPeriodStart *time.Time         `json:"current_period_start"`
	CurrentPeriodEnd   *time.Time         `json:"current_period_end"`
	CanceledAt         *time.Time         `json:"canceled_at"`
	CustomerID         string             `json:"customer_id"`
	Customer           *PolarCustomerData `json:"customer"`
	Product            *PolarProductData  `json:"product"`
	ProductID          string             `json:"product_id"`
	Metadata           map[string]string  `json:"metadata"`
}

// PolarCustomerData represents customer info from Polar.
type PolarCustomerData struct {
	ID       string            `json:"id"`
	Email    string            `json:"email"`
	Metadata map[string]string `json:"metadata"`
}

// PolarProductData represents product info from Polar.
type PolarProductData struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ServeHTTP handles the Polar webhook HTTP request.
func (h *WebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}

	if h.secret != "" {
		if !h.verifySignature(body, r) {
			h.logger.Warn("invalid polar webhook signature")
			http.Error(w, "invalid signature", http.StatusUnauthorized)
			return
		}
	}

	var payload PolarWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		h.logger.Error("failed to parse webhook payload", "error", err)
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	switch payload.Type {
	case "subscription.created":
		err = h.handleSubscriptionCreated(ctx, payload.Data)
	case "subscription.updated":
		err = h.handleSubscriptionUpdated(ctx, payload.Data)
	case "subscription.canceled":
		err = h.handleSubscriptionCanceled(ctx, payload.Data)
	case "subscription.revoked":
		err = h.handleSubscriptionRevoked(ctx, payload.Data)
	default:
		h.logger.Debug("ignoring unhandled webhook event", "type", payload.Type)
		w.WriteHeader(http.StatusOK)
		return
	}

	if err != nil {
		h.logger.Error("failed to handle webhook", "type", payload.Type, "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// verifySignature implements Standard Webhooks signature verification.
// Polar uses the Standard Webhooks spec: base64-encoded secret (prefixed with "whsec_"),
// signature header "webhook-signature" containing "v1,<base64-hmac>",
// message = "${webhook-id}.${webhook-timestamp}.${body}".
func (h *WebhookHandler) verifySignature(body []byte, r *http.Request) bool {
	msgID := r.Header.Get("webhook-id")
	timestamp := r.Header.Get("webhook-timestamp")
	sigHeader := r.Header.Get("webhook-signature")

	if msgID == "" || timestamp == "" || sigHeader == "" {
		return false
	}

	// Validate timestamp within 5-minute tolerance.
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return false
	}
	diff := time.Since(time.Unix(ts, 0))
	if diff < -5*time.Minute || diff > 5*time.Minute {
		return false
	}

	// Decode secret: strip "whsec_" prefix and base64-decode.
	secretStr := strings.TrimPrefix(h.secret, "whsec_")
	key, err := base64.StdEncoding.DecodeString(secretStr)
	if err != nil {
		return false
	}

	// Construct signed content and compute HMAC.
	signedContent := fmt.Sprintf("%s.%s.%s", msgID, timestamp, string(body))
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(signedContent))
	expected := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	// The header may contain multiple signatures separated by spaces.
	for entry := range strings.SplitSeq(sigHeader, " ") {
		parts := strings.SplitN(entry, ",", 2)
		if len(parts) != 2 || parts[0] != "v1" {
			continue
		}
		if hmac.Equal([]byte(expected), []byte(parts[1])) {
			return true
		}
	}
	return false
}

func (h *WebhookHandler) handleSubscriptionCreated(ctx context.Context, data json.RawMessage) error {
	var sub PolarSubscriptionData
	if err := json.Unmarshal(data, &sub); err != nil {
		return fmt.Errorf("parsing subscription data: %w", err)
	}

	productID := sub.ProductID
	if sub.Product != nil {
		productID = sub.Product.ID
	}

	tier, ok := h.polarMapping.TierForProduct(productID)
	if !ok {
		h.logger.Warn("unknown polar product ID", "product_id", productID)
		return ErrUnknownProduct
	}

	orgID := h.resolveOrgID(sub)
	if orgID == "" {
		h.logger.Warn("cannot resolve org_id from subscription", "subscription_id", sub.ID)
		return nil
	}

	now := time.Now()
	orgSub := &OrgSubscription{
		ID:                    sub.ID,
		OrgID:                 orgID,
		PlanTier:              string(tier),
		PolarSubscriptionID:   &sub.ID,
		PolarCustomerID:       &sub.CustomerID,
		Status:                "active",
		CurrentPeriodStart:    sub.CurrentPeriodStart,
		CurrentPeriodEnd:      sub.CurrentPeriodEnd,
		SpendingLimitMicrousd: -1,
		LimitAction:           "reject",
		CreatedAt:             now,
		UpdatedAt:             now,
	}

	if err := h.store.UpsertOrgSubscription(ctx, orgSub); err != nil {
		return fmt.Errorf("upserting org subscription: %w", err)
	}

	if h.enforcer != nil {
		h.enforcer.InvalidateOrgCache(orgID)
	}

	h.logger.Info("subscription created",
		"org_id", orgID,
		"plan_tier", tier,
		"polar_subscription_id", sub.ID,
	)
	return nil
}

func (h *WebhookHandler) handleSubscriptionUpdated(ctx context.Context, data json.RawMessage) error {
	var sub PolarSubscriptionData
	if err := json.Unmarshal(data, &sub); err != nil {
		return fmt.Errorf("parsing subscription data: %w", err)
	}

	productID := sub.ProductID
	if sub.Product != nil {
		productID = sub.Product.ID
	}

	tier, ok := h.polarMapping.TierForProduct(productID)
	if !ok {
		h.logger.Warn("unknown polar product ID on update", "product_id", productID)
		return nil
	}

	orgID := h.resolveOrgID(sub)
	if orgID == "" {
		return nil
	}

	status := sub.Status
	if status == "" {
		status = "active"
	}

	// Check if this is a downgrade by comparing plan limits.
	existing, existErr := h.store.GetOrgSubscription(ctx, orgID)
	if existErr != nil && !errors.Is(existErr, ErrSubscriptionNotFound) {
		return fmt.Errorf("getting existing subscription: %w", existErr)
	}

	isDowngrade := false
	if existing != nil && existing.PlanTier != string(tier) {
		currentLimits := GetPlanLimits(domain.PlanTier(existing.PlanTier))
		newLimits := GetPlanLimits(tier)
		isDowngrade = newLimits.MaxRunsPerDay < currentLimits.MaxRunsPerDay ||
			newLimits.MaxProjectsPerOrg < currentLimits.MaxProjectsPerOrg ||
			newLimits.ComputeCreditMicrousd < currentLimits.ComputeCreditMicrousd
	}

	if isDowngrade {
		// Defer the downgrade: store the pending tier for end-of-period application.
		if err := h.store.SetPendingPlanTier(ctx, orgID, string(tier)); err != nil {
			return fmt.Errorf("setting pending plan tier: %w", err)
		}
		// Still update period dates and status.
		if err := h.store.UpdateOrgSubscriptionFull(ctx, orgID, existing.PlanTier, status, sub.CurrentPeriodStart, sub.CurrentPeriodEnd); err != nil {
			if !errors.Is(err, ErrSubscriptionNotFound) {
				return fmt.Errorf("updating subscription period: %w", err)
			}
		}
		h.logger.Info("subscription downgrade deferred",
			"org_id", orgID,
			"current_tier", existing.PlanTier,
			"pending_tier", tier,
		)
		return nil
	}

	// Upgrade or same-tier update: apply immediately with period dates.
	if err := h.store.UpdateOrgSubscriptionFull(ctx, orgID, string(tier), status, sub.CurrentPeriodStart, sub.CurrentPeriodEnd); err != nil {
		if errors.Is(err, ErrSubscriptionNotFound) {
			now := time.Now()
			orgSub := &OrgSubscription{
				ID:                    sub.ID,
				OrgID:                 orgID,
				PlanTier:              string(tier),
				PolarSubscriptionID:   &sub.ID,
				PolarCustomerID:       &sub.CustomerID,
				Status:                status,
				CurrentPeriodStart:    sub.CurrentPeriodStart,
				CurrentPeriodEnd:      sub.CurrentPeriodEnd,
				SpendingLimitMicrousd: -1,
				LimitAction:           "reject",
				CreatedAt:             now,
				UpdatedAt:             now,
			}
			if upsertErr := h.store.UpsertOrgSubscription(ctx, orgSub); upsertErr != nil {
				return upsertErr
			}
			if h.enforcer != nil {
				h.enforcer.InvalidateOrgCache(orgID)
			}
			h.logger.Info("subscription updated (created via fallback)",
				"org_id", orgID,
				"plan_tier", tier,
				"status", status,
			)
			return nil
		}
		return fmt.Errorf("updating org subscription: %w", err)
	}

	if h.enforcer != nil {
		h.enforcer.InvalidateOrgCache(orgID)
	}

	h.logger.Info("subscription updated",
		"org_id", orgID,
		"plan_tier", tier,
		"status", status,
	)
	return nil
}

func (h *WebhookHandler) handleSubscriptionCanceled(ctx context.Context, data json.RawMessage) error {
	var sub PolarSubscriptionData
	if err := json.Unmarshal(data, &sub); err != nil {
		return fmt.Errorf("parsing subscription data: %w", err)
	}

	orgID := h.resolveOrgID(sub)
	if orgID == "" {
		return nil
	}

	existing, err := h.store.GetOrgSubscription(ctx, orgID)
	if err != nil {
		if errors.Is(err, ErrSubscriptionNotFound) {
			return nil
		}
		return fmt.Errorf("getting org subscription: %w", err)
	}

	existing.Status = "canceled"
	existing.CanceledAt = sub.CanceledAt
	existing.UpdatedAt = time.Now()

	if err := h.store.UpsertOrgSubscription(ctx, existing); err != nil {
		return fmt.Errorf("updating canceled subscription: %w", err)
	}

	// Queue a downgrade to free at period end so paid quotas don't persist indefinitely.
	if err := h.store.SetPendingPlanTier(ctx, orgID, string(domain.PlanFree)); err != nil {
		if !errors.Is(err, ErrSubscriptionNotFound) {
			return fmt.Errorf("setting pending free tier on cancellation: %w", err)
		}
	}

	if h.enforcer != nil {
		h.enforcer.InvalidateOrgCache(orgID)
	}

	h.logger.Info("subscription canceled",
		"org_id", orgID,
		"plan_tier", existing.PlanTier,
	)
	return nil
}

func (h *WebhookHandler) handleSubscriptionRevoked(ctx context.Context, data json.RawMessage) error {
	var sub PolarSubscriptionData
	if err := json.Unmarshal(data, &sub); err != nil {
		return fmt.Errorf("parsing subscription data: %w", err)
	}

	orgID := h.resolveOrgID(sub)
	if orgID == "" {
		return nil
	}

	if err := h.store.UpdateOrgSubscriptionPlan(ctx, orgID, string(domain.PlanFree), "revoked"); err != nil {
		if errors.Is(err, ErrSubscriptionNotFound) {
			return nil
		}
		return fmt.Errorf("revoking subscription: %w", err)
	}

	if h.enforcer != nil {
		h.enforcer.InvalidateOrgCache(orgID)
	}

	h.logger.Info("subscription revoked, downgraded to free",
		"org_id", orgID,
	)
	return nil
}

// resolveOrgID extracts the org_id from subscription metadata or customer metadata.
func (h *WebhookHandler) resolveOrgID(sub PolarSubscriptionData) string {
	if orgID, ok := sub.Metadata["org_id"]; ok && orgID != "" {
		return orgID
	}
	if sub.Customer != nil {
		if orgID, ok := sub.Customer.Metadata["org_id"]; ok && orgID != "" {
			return orgID
		}
	}
	return ""
}
