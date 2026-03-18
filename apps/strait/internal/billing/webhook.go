package billing

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
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
}

// NewWebhookHandler creates a new Polar webhook handler.
func NewWebhookHandler(store Store, mapping *PolarMapping, secret string, logger *slog.Logger) *WebhookHandler {
	return &WebhookHandler{
		store:        store,
		polarMapping: mapping,
		secret:       secret,
		logger:       logger,
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
		sig := r.Header.Get("X-Polar-Signature")
		if !h.verifySignature(body, sig) {
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

func (h *WebhookHandler) verifySignature(body []byte, signature string) bool {
	mac := hmac.New(sha256.New, []byte(h.secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
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

	if err := h.store.UpdateOrgSubscriptionPlan(ctx, orgID, string(tier), status); err != nil {
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
			return h.store.UpsertOrgSubscription(ctx, orgSub)
		}
		return fmt.Errorf("updating org subscription: %w", err)
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
