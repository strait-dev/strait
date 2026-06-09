package billing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/mail"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"strait/internal/clickhouse"
	"strait/internal/domain"

	"github.com/stripe/stripe-go/v82"
	stripeWebhook "github.com/stripe/stripe-go/v82/webhook"
)

// invoiceAuditFields normalises the per-invoice audit payload across every
// handler that emits one. Three rules:
//
//   - amount is recorded as amount_due_minor (string of minor units) so
//     downstream parsers never have to guess between cents and dollars and
//     never lose precision through float printing.
//   - currency is lowercased to match Stripe's documented canonical form
//     (their schema says lowercase but webhook payloads have shipped both
//     cases historically).
//   - stripe_invoice_num exposes invoice.Number — the human-readable
//     invoice ID Stripe surfaces in customer-facing PDFs — alongside the
//     internal invoice.ID. Audit consumers can correlate to billing emails
//     without round-tripping through the Stripe API.
//
// Empty fields are omitted so a malformed Stripe payload does not stamp
// blank cells into ClickHouse.
func invoiceAuditFields(invoice *stripe.Invoice) map[string]string {
	if invoice == nil {
		return map[string]string{}
	}
	out := map[string]string{
		"invoice_id":       invoice.ID,
		"amount_due_minor": strconv.FormatInt(invoice.AmountDue, 10),
		"currency":         strings.ToLower(string(invoice.Currency)),
	}
	if invoice.Number != "" {
		out["stripe_invoice_num"] = invoice.Number
	}
	return out
}

func stripeMinorUnitsToMicroUSD(amount int64) int64 {
	return amount * 10_000
}

var (
	ErrInvalidSignature = errors.New("invalid webhook signature")
	ErrUnknownPrice     = errors.New("unknown stripe price ID")

	errUnboundStripeObject = errors.New("stripe object is not bound to an organization")
)

// AuditStore is the subset of store operations needed for audit logging.
type AuditStore interface {
	CreateAuditEvent(ctx context.Context, event *domain.AuditEvent) error
}

// WelcomeEmailFunc sends a welcome email to a new paid subscriber.
// The email should mention spending limits and link to billing settings.
type WelcomeEmailFunc func(ctx context.Context, orgID string, planTier domain.PlanTier, customerEmail string) error

type webhookProcessingStore interface {
	ClaimWebhookForProcessing(ctx context.Context, msgID string, staleAfter time.Duration) (bool, error)
	MarkWebhookProcessed(ctx context.Context, msgID string) error
	ReleaseWebhookClaim(ctx context.Context, msgID string) error
}

type webhookProcessingStatusStore interface {
	GetWebhookProcessingStatus(ctx context.Context, msgID string) (string, error)
}

type webhookClaimState struct {
	eventID         string
	claimed         bool
	processingStore webhookProcessingStore
}

// WebhookHandler handles incoming Stripe webhook events.
type WebhookHandler struct {
	store             Store
	stripeMapping     *StripeMapping
	catalogResolver   *CatalogResolver
	secret            string
	logger            *slog.Logger
	enforcer          *Enforcer
	auditStore        AuditStore
	welcomeEmail      WelcomeEmailFunc
	posthog           *PostHogClient
	chExporter        billingEventEnqueuer
	billingEmails     *BillingEmailSender
	dunningStore      DunningStore
	edition           string
	devBypassSigCheck bool
	allowTestMetadata bool
	replayCache       sync.Map // eventID -> int64 (unix nanos), prevents replay within 10 minutes
}

// WebhookOption configures optional WebhookHandler behavior.
type WebhookOption func(*WebhookHandler)

// WithWelcomeEmail sets a function to send welcome emails on paid plan subscription.
func WithWelcomeEmail(fn WelcomeEmailFunc) WebhookOption {
	return func(h *WebhookHandler) { h.welcomeEmail = fn }
}

// WithPostHog sets the PostHog client for server-side revenue event tracking.
func WithPostHog(client *PostHogClient) WebhookOption {
	return func(h *WebhookHandler) { h.posthog = client }
}

// WithWebhookClickHouse attaches a ClickHouse exporter for billing events.
func WithWebhookClickHouse(exporter billingEventEnqueuer) WebhookOption {
	return func(h *WebhookHandler) { h.chExporter = exporter }
}

// WithBillingEmails sets the billing email sender for plan change notifications.
func WithBillingEmails(sender *BillingEmailSender) WebhookOption {
	return func(h *WebhookHandler) { h.billingEmails = sender }
}

// WithDunningStore wires the dunning state machine into Stripe webhook
// handlers. invoice.payment_failed enters dunning at step 1 and invoice.paid
// resolves the active cycle. When nil, dunning tracking is disabled and the
// flat grace-period path remains the only failure-mode handling.
func WithDunningStore(s DunningStore) WebhookOption {
	return func(h *WebhookHandler) { h.dunningStore = s }
}

// WithEdition sets the application edition for security mode decisions.
func WithEdition(edition string) WebhookOption {
	return func(h *WebhookHandler) { h.edition = edition }
}

// WithCatalogResolver overrides the default lookup-key resolver. The default
// (constructed from PlanCatalogs and AddonPacks) is sufficient for production;
// this option exists primarily for tests that need a narrower mapping.
func WithCatalogResolver(r *CatalogResolver) WebhookOption {
	return func(h *WebhookHandler) {
		if r != nil {
			h.catalogResolver = r
		}
	}
}

var (
	errEmptySubscriptionID = errors.New("subscription ID is empty")
	errEmptyPriceID        = errors.New("price ID is empty")
	errEmptyCustomerID     = errors.New("customer ID is empty")
)

// recordOrphanInvoiceDelivery is the centralized observability hook fired
// when a finalize/paid/payment_failed event arrives for a Stripe customer
// that is not mapped to any org in our database. Silently swallowing this
// is dangerous: it could mean (a) the customer was deleted in our DB but
// their Stripe subscription wasn't cancelled, (b) a misconfigured webhook
// is pointing at the wrong environment, or (c) Stripe is delivering events
// for a sandbox account into production. All three warrant operator
// attention. Counters keep this surfaceable on a Grafana panel.
func (h *WebhookHandler) recordOrphanInvoiceDelivery(ctx context.Context, eventType string, invoice *stripe.Invoice) {
	customerID := ""
	if invoice.Customer != nil {
		customerID = invoice.Customer.ID
	}
	h.logger.Warn("stripe webhook delivered for unknown customer",
		"event_type", eventType,
		"invoice_id", invoice.ID,
		"customer_id", customerID,
	)
	recordBillingWebhookOrphanDelivery(ctx, eventType)
}

const webhookProcessingClaimStaleAfter = 10 * time.Minute

// validateStripeSubscription checks that required fields are present on a Stripe subscription.
func validateStripeSubscription(sub *stripe.Subscription) error {
	if sub.ID == "" {
		return errEmptySubscriptionID
	}
	if sub.Customer == nil || sub.Customer.ID == "" {
		return errEmptyCustomerID
	}
	priceID := extractPriceID(sub)
	if priceID == "" {
		return errEmptyPriceID
	}
	return nil
}

// extractPriceID returns the Price ID from the first subscription item.
func extractPriceID(sub *stripe.Subscription) string {
	if sub.Items == nil || len(sub.Items.Data) == 0 {
		return ""
	}
	if sub.Items.Data[0].Price == nil {
		return ""
	}
	return sub.Items.Data[0].Price.ID
}

// extractLookupKey returns the Stripe lookup_key from the first subscription
// item's price, or empty when the price has no lookup key set. The lookup key
// is the cross-account stable identifier used by the catalog resolver.
func extractLookupKey(sub *stripe.Subscription) string {
	if sub.Items == nil || len(sub.Items.Data) == 0 {
		return ""
	}
	if sub.Items.Data[0].Price == nil {
		return ""
	}
	return sub.Items.Data[0].Price.LookupKey
}

// resolveTier returns the plan tier for a subscription, preferring lookup-key
// resolution (catalog-driven, account-agnostic) and falling back to per-account
// price ID mapping. Returns the lookup key actually used (empty if fallback).
func (h *WebhookHandler) resolveTier(sub *stripe.Subscription) (domain.PlanTier, string, bool) {
	if lk := extractLookupKey(sub); lk != "" {
		if t, ok := h.catalogResolver.TierForLookupKey(lk); ok {
			return t, lk, true
		}
	}
	t, ok := h.stripeMapping.TierForPrice(extractPriceID(sub))
	return t, "", ok
}

// resolveAddon returns the addon type for a subscription, preferring lookup-key
// resolution and falling back to per-account price ID mapping. Returns the
// lookup key actually used (empty if fallback).
func (h *WebhookHandler) resolveAddon(sub *stripe.Subscription) (AddonType, string, bool) {
	if lk := extractLookupKey(sub); lk != "" {
		if a, ok := h.catalogResolver.AddonForLookupKey(lk); ok {
			return a, lk, true
		}
	}
	a, ok := h.stripeMapping.AddonTypeForPrice(extractPriceID(sub))
	return a, "", ok
}

// isAddonSubscription reports whether the subscription resolves to an addon
// (via lookup key or fallback price ID mapping).
func (h *WebhookHandler) isAddonSubscription(sub *stripe.Subscription) bool {
	if lk := extractLookupKey(sub); lk != "" && h.catalogResolver.IsAddonLookupKey(lk) {
		return true
	}
	return h.stripeMapping.IsAddonPrice(extractPriceID(sub))
}

// extractCustomerEmail returns the email from a Stripe customer object.
func extractCustomerEmail(sub *stripe.Subscription) string {
	if sub.Customer == nil {
		return ""
	}
	return sub.Customer.Email
}

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func isValidUUID(s string) bool {
	return uuidPattern.MatchString(s)
}

// maskEmail returns a partially masked email for safe logging.
// "user@example.com" becomes "u***@example.com".
func maskEmail(email string) string {
	parts := strings.SplitN(email, "@", 2)
	if len(parts) != 2 || len(parts[0]) == 0 {
		return "***"
	}
	local := parts[0]
	if len(local) <= 1 {
		return local + "***@" + parts[1]
	}
	return string(local[0]) + "***@" + parts[1]
}

// isValidEmail performs basic email format validation using net/mail.
func isValidEmail(email string) bool {
	if email == "" {
		return false
	}
	_, err := mail.ParseAddress(email)
	return err == nil
}

func (h *WebhookHandler) emitBillingEvent(orgID, eventType, planTier string) {
	if h.chExporter == nil {
		return
	}
	h.chExporter.Enqueue(clickhouse.BillingEventRecord{
		Timestamp: time.Now(),
		OrgID:     orgID,
		EventType: eventType,
		PlanTier:  planTier,
	})
}

// NewWebhookHandler creates a new Stripe webhook handler.
// The enforcer is optional; when non-nil, org caches are invalidated on plan changes.
// The auditStore is optional; when non-nil, audit events are recorded for plan changes.
func NewWebhookHandler(store Store, mapping *StripeMapping, secret string, logger *slog.Logger, enforcer *Enforcer, auditStore AuditStore, opts ...WebhookOption) *WebhookHandler {
	h := &WebhookHandler{
		store:           store,
		stripeMapping:   mapping,
		catalogResolver: NewCatalogResolver(),
		secret:          secret,
		logger:          logger,
		enforcer:        enforcer,
		auditStore:      auditStore,
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// goAsync runs fn in a detached background goroutine with panic recovery. These
// are fire-and-forget side effects (transactional emails) that must not block or
// fail the webhook response. Recovering and logging the panic here is important:
// the previous conc.WaitGroup-without-Wait() pattern silently swallowed any panic
// because the stored panic was never re-raised.
func (h *WebhookHandler) goAsync(fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				h.logger.Error("panic in async billing webhook task", "panic", r)
			}
		}()
		fn()
	}()
}

// StartReplayCleanup periodically removes stale replay cache entries.
func (h *WebhookHandler) StartReplayCleanup(ctx context.Context) {
	h.goAsync(func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				now := time.Now().UnixNano()
				h.replayCache.Range(func(key, value any) bool {
					ts := value.(int64)
					if time.Duration(now-ts) > 10*time.Minute {
						h.replayCache.Delete(key)
					}
					return true
				})
			}
		}
	})
}

// ServeHTTP handles the Stripe webhook HTTP request.
func (h *WebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		recordBillingWebhookProcessed(r.Context(), "", "failure")
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}

	// Verify Stripe webhook signature.
	sigHeader := r.Header.Get("Stripe-Signature")
	if h.secret != "" {
		if _, err := stripeWebhook.ConstructEventWithOptions(body, sigHeader, h.secret, stripeWebhook.ConstructEventOptions{
			IgnoreAPIVersionMismatch: true,
		}); err != nil {
			h.logger.Warn("invalid stripe webhook signature", "error", err)
			recordBillingWebhookProcessed(r.Context(), "", "failure")
			http.Error(w, "invalid signature", http.StatusUnauthorized)
			return
		}
	} else if h.devBypassSigCheck {
		h.logger.Warn("stripe webhook signature verification bypassed via STRIPE_WEBHOOK_ALLOW_UNSIGNED")
	} else {
		h.logger.Error("stripe webhook secret not configured, rejecting request")
		recordBillingWebhookProcessed(r.Context(), "", "failure")
		http.Error(w, "webhook verification unavailable", http.StatusServiceUnavailable)
		return
	}

	// Parse the Stripe event.
	var event stripe.Event
	if err := json.Unmarshal(body, &event); err != nil {
		h.logger.Error("failed to parse stripe event", "error", err)
		recordBillingWebhookProcessed(r.Context(), "", "failure")
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	eventID := event.ID
	claim, duplicate, claimErr := h.claimWebhookForProcessing(r.Context(), eventID)
	if claimErr != nil {
		recordBillingWebhookProcessed(r.Context(), string(event.Type), "failure")
		if errors.Is(claimErr, errWebhookAlreadyProcessing) {
			w.Header().Set("Retry-After", "5")
			http.Error(w, "webhook already processing", http.StatusServiceUnavailable)
			return
		}
		http.Error(w, "webhook idempotency unavailable", http.StatusServiceUnavailable)
		return
	}
	if duplicate {
		recordBillingWebhookProcessed(r.Context(), string(event.Type), "duplicate")
		w.WriteHeader(http.StatusOK)
		return
	}

	err = h.dispatchStripeEvent(r.Context(), event)
	if errors.Is(err, errUnhandledStripeEvent) {
		h.logger.Debug("ignoring unhandled stripe event", "type", event.Type)
		h.markIgnoredWebhookProcessed(r.Context(), claim)
		recordBillingWebhookProcessed(r.Context(), string(event.Type), "ignored")
		w.WriteHeader(http.StatusOK)
		return
	}

	if err != nil {
		// Clear the in-memory replay cache so Stripe's retry can be processed.
		// Without this, a partially-failed webhook would be permanently rejected
		// by the replay cache even though it was never fully processed.
		h.releaseWebhookClaim(r.Context(), claim)
		h.logger.Error("failed to handle stripe webhook", "type", event.Type, "error", err)
		recordBillingWebhookProcessed(r.Context(), string(event.Type), "failure")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	h.markWebhookProcessed(r.Context(), claim)
	recordBillingWebhookProcessed(r.Context(), string(event.Type), "success")

	w.WriteHeader(http.StatusOK)
}

var errUnhandledStripeEvent = errors.New("unhandled stripe event")
var errWebhookAlreadyProcessing = errors.New("webhook already processing")

const (
	webhookStatusProcessing = "processing"
	webhookStatusProcessed  = "processed"
)

func (h *WebhookHandler) claimWebhookForProcessing(ctx context.Context, eventID string) (webhookClaimState, bool, error) {
	claim := webhookClaimState{eventID: eventID}
	if eventID == "" {
		return claim, false, nil
	}
	if prev, loaded := h.replayCache.Load(eventID); loaded {
		prevTime := prev.(int64)
		if time.Since(time.Unix(0, prevTime)) < 10*time.Minute {
			h.logger.Warn("duplicate stripe event ID", "event_id", eventID)
			return claim, true, nil
		}
		h.replayCache.Delete(eventID)
	}

	ps, ok := h.store.(webhookProcessingStore)
	if !ok {
		h.replayCache.Delete(eventID)
		return claim, false, fmt.Errorf("stripe webhook processing claims are not supported")
	}

	claim.processingStore = ps
	claimed, err := ps.ClaimWebhookForProcessing(ctx, eventID, webhookProcessingClaimStaleAfter)
	if err != nil {
		h.replayCache.Delete(eventID)
		h.logger.Error("failed to claim stripe webhook", "event_id", eventID, "error", err)
		return claim, false, err
	}
	if !claimed {
		if statusStore, statusOK := h.store.(webhookProcessingStatusStore); statusOK {
			status, statusErr := statusStore.GetWebhookProcessingStatus(ctx, eventID)
			if statusErr != nil {
				h.replayCache.Delete(eventID)
				h.logger.Error("failed to load stripe webhook status", "event_id", eventID, "error", statusErr)
				return claim, false, statusErr
			}
			if status == webhookStatusProcessed {
				h.replayCache.Store(eventID, time.Now().UnixNano())
				h.logger.Info("webhook already processed", "event_id", eventID)
				return claim, true, nil
			}
		}
		h.replayCache.Delete(eventID)
		h.logger.Info("webhook already processing", "event_id", eventID)
		return claim, false, errWebhookAlreadyProcessing
	}
	claim.claimed = true
	return claim, false, nil
}

func (h *WebhookHandler) dispatchStripeEvent(ctx context.Context, event stripe.Event) error {
	switch event.Type {
	case stripe.EventTypeCustomerSubscriptionCreated:
		return h.handleSubscriptionCreated(ctx, event.Data.Raw)
	case stripe.EventTypeCustomerSubscriptionUpdated:
		return h.handleSubscriptionUpdated(ctx, event.Data.Raw)
	case stripe.EventTypeCustomerSubscriptionDeleted:
		return h.handleSubscriptionDeleted(ctx, event.Data.Raw)
	case stripe.EventTypeInvoicePaid:
		return h.handlePaymentSucceeded(ctx, event.Data.Raw)
	case stripe.EventTypeInvoicePaymentFailed:
		return h.handlePaymentFailed(ctx, event.Data.Raw)
	case stripe.EventTypeCustomerSubscriptionPaused:
		return h.handleSubscriptionPaused(ctx, event.Data.Raw)
	case stripe.EventTypeCustomerSubscriptionResumed:
		return h.handleSubscriptionResumed(ctx, event.Data.Raw)
	case stripe.EventTypeCustomerSubscriptionTrialWillEnd:
		return h.handleTrialWillEnd(ctx, event.Data.Raw)
	case stripe.EventTypeChargeDisputeCreated:
		return h.handleChargeDisputeCreated(ctx, event.Data.Raw)
	case stripe.EventTypeInvoiceUpcoming:
		return h.handleInvoiceUpcoming(ctx, event.Data.Raw)
	case stripe.EventTypeInvoiceMarkedUncollectible:
		return h.handleInvoiceUncollectible(ctx, event.Data.Raw)
	case stripe.EventTypeInvoiceFinalized:
		return h.handleInvoiceFinalized(ctx, event.Data.Raw)
	case stripe.EventTypeInvoiceFinalizationFailed:
		return h.handleInvoiceFinalizationFailed(ctx, event.Data.Raw)
	default:
		return errUnhandledStripeEvent
	}
}

func (h *WebhookHandler) markIgnoredWebhookProcessed(ctx context.Context, claim webhookClaimState) {
	if claim.claimed && claim.processingStore != nil {
		if err := claim.processingStore.MarkWebhookProcessed(ctx, claim.eventID); err != nil {
			h.replayCache.Delete(claim.eventID)
			h.logger.Warn("failed to mark ignored webhook processed", "event_id", claim.eventID, "error", err)
		} else {
			h.replayCache.Store(claim.eventID, time.Now().UnixNano())
		}
	}
}

func (h *WebhookHandler) releaseWebhookClaim(ctx context.Context, claim webhookClaimState) {
	if claim.eventID == "" {
		return
	}
	h.replayCache.Delete(claim.eventID)
	if claim.claimed && claim.processingStore != nil {
		if err := claim.processingStore.ReleaseWebhookClaim(ctx, claim.eventID); err != nil {
			h.logger.Warn("failed to release stripe webhook claim", "event_id", claim.eventID, "error", err)
		}
	}
}

func (h *WebhookHandler) markWebhookProcessed(ctx context.Context, claim webhookClaimState) {
	if claim.eventID == "" {
		return
	}
	if claim.claimed && claim.processingStore != nil {
		if err := claim.processingStore.MarkWebhookProcessed(ctx, claim.eventID); err != nil {
			h.replayCache.Delete(claim.eventID)
			h.logger.Warn("failed to mark processed webhook", "event_id", claim.eventID, "error", err)
		} else {
			h.replayCache.Store(claim.eventID, time.Now().UnixNano())
		}
		return
	}
	if err := h.store.RecordProcessedWebhook(ctx, claim.eventID); err != nil {
		h.logger.Warn("failed to record processed webhook", "event_id", claim.eventID, "error", err)
	} else {
		h.replayCache.Store(claim.eventID, time.Now().UnixNano())
	}
}

func (h *WebhookHandler) handleSubscriptionCreated(ctx context.Context, data json.RawMessage) error {
	var sub stripe.Subscription
	if err := json.Unmarshal(data, &sub); err != nil {
		return fmt.Errorf("parsing subscription data: %w", err)
	}

	if err := validateStripeSubscription(&sub); err != nil {
		h.logger.Warn("invalid webhook subscription data", "error", err)
		return fmt.Errorf("invalid subscription data: %w", err)
	}

	priceID := extractPriceID(&sub)

	// Check if this is an addon (lookup-key first, then per-account price ID).
	if addonType, lookupKey, isAddon := h.resolveAddon(&sub); isAddon {
		return h.handleAddonSubscriptionCreated(ctx, &sub, addonType, lookupKey)
	}

	tier, lookupKey, ok := h.resolveTier(&sub)
	if !ok {
		h.logger.Warn("unknown stripe price", "price_id", priceID, "lookup_key", extractLookupKey(&sub))
		return ErrUnknownPrice
	}

	orgID, err := h.resolveOrgIDForNewSubscription(ctx, &sub, tier)
	if err != nil {
		return err
	}
	if orgID == "" {
		h.logger.Warn("cannot resolve org_id from subscription", "subscription_id", sub.ID)
		return fmt.Errorf("unable to resolve org_id from subscription %s metadata", sub.ID)
	}

	now := time.Now()
	periodStart, periodEnd := extractPeriod(&sub)
	customerID := sub.Customer.ID
	var lookupKeyPtr *string
	if lookupKey != "" {
		lookupKeyPtr = &lookupKey
	}
	orgSub := &OrgSubscription{
		ID:                    sub.ID,
		OrgID:                 orgID,
		PlanTier:              string(tier),
		StripeSubscriptionID:  &sub.ID,
		StripeCustomerID:      &customerID,
		StripeLookupKey:       lookupKeyPtr,
		Status:                "active",
		CurrentPeriodStart:    periodStart,
		CurrentPeriodEnd:      periodEnd,
		SpendingLimitMicrousd: -1,
		LimitAction:           "reject",
		MonthlyUsageEmail:     tier != domain.PlanFree, // opt-in for paid plans only
		CreatedAt:             now,
		UpdatedAt:             now,
	}

	// Capture previous tier before the upsert overwrites it.
	var previousTier string
	if tier == domain.PlanEnterprise {
		existing, existErr := h.store.GetOrgSubscription(ctx, orgID)
		if existErr == nil && existing != nil {
			previousTier = existing.PlanTier
		}
	}

	if err := h.store.UpsertOrgSubscription(ctx, orgSub); err != nil {
		return fmt.Errorf("upserting org subscription: %w", err)
	}

	if h.enforcer != nil {
		h.enforcer.InvalidateOrgCache(orgID)
	}

	// Detect plan transitions (e.g. Scale -> Enterprise) for audit logging.
	if tier == domain.PlanEnterprise && previousTier != "" && previousTier != string(tier) {
		h.logAuditEvent(ctx, "subscription.upgraded_to_enterprise", orgID, map[string]string{
			"previous_plan":          previousTier,
			"new_plan":               string(tier),
			"stripe_subscription_id": sub.ID,
		})
		h.logger.Info("plan upgraded to enterprise",
			"org_id", orgID,
			"previous_plan", previousTier,
		)
	}

	// For enterprise plans, create the enterprise contract based on the price's sub-tier.
	if tier == domain.PlanEnterprise {
		if entTier, ok := EnterpriseTierForPrice(priceID); ok {
			cfg := GetEnterpriseConfig(entTier)
			now := time.Now()
			contract := &EnterpriseContract{
				ID:                    sub.ID + "-contract",
				OrgID:                 orgID,
				EnterpriseTier:        entTier,
				AnnualCommitmentCents: cfg.AnnualCommitmentCents,
				OverageDiscountPct:    cfg.OverageDiscountPct,
				ContractStartDate:     now,
				ContractEndDate:       now.AddDate(1, 0, 0),
				AutoRenew:             true,
				BillingCadence:        "annual",
				StripeSubscriptionID:  &sub.ID,
				CreatedAt:             now,
				UpdatedAt:             now,
			}
			if err := h.store.UpsertEnterpriseContract(ctx, contract); err != nil {
				return fmt.Errorf("creating enterprise contract: %w", err)
			}
			h.logger.Info("enterprise contract created",
				"org_id", orgID, "enterprise_tier", entTier)
		}
	}

	h.logAuditEvent(ctx, "subscription.created", orgID, map[string]string{
		"plan_tier":              string(tier),
		"stripe_subscription_id": sub.ID,
	})

	h.emitBillingEvent(orgID, "plan_changed", string(tier))

	h.logger.Info("subscription created",
		"org_id", orgID,
		"plan_tier", tier,
		"stripe_subscription_id", sub.ID,
	)

	customerEmail := extractCustomerEmail(&sub)
	h.posthog.CaptureRevenueEvent(orgID, "subscription_created_server", map[string]any{
		"plan":                   string(tier),
		"customer_email":         maskEmail(customerEmail),
		"stripe_subscription_id": sub.ID,
	})

	// Send welcome email for paid plan subscriptions (async to avoid blocking webhook response).
	if h.welcomeEmail != nil && tier != domain.PlanFree {
		if isValidEmail(customerEmail) {
			welcomeFn := h.welcomeEmail
			h.goAsync(func() {
				emailCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				if err := welcomeFn(emailCtx, orgID, tier, customerEmail); err != nil {
					h.logger.Warn("failed to send welcome email",
						"org_id", orgID, "plan_tier", tier, "error", err)
				}
			})
		}
	}

	return nil
}

// timeFromUnix converts a Unix timestamp to *time.Time, returning nil for zero.
func timeFromUnix(ts int64) *time.Time {
	if ts == 0 {
		return nil
	}
	t := time.Unix(ts, 0)
	return &t
}

// extractPeriod returns the current period start/end from the first subscription item.
// In Stripe API v2025+, the period is on subscription items, not the subscription itself.
func extractPeriod(sub *stripe.Subscription) (*time.Time, *time.Time) {
	if sub.Items == nil || len(sub.Items.Data) == 0 {
		return nil, nil
	}
	item := sub.Items.Data[0]
	return timeFromUnix(item.CurrentPeriodStart), timeFromUnix(item.CurrentPeriodEnd)
}

// handleSubscriptionUpdated processes plan changes.
func (h *WebhookHandler) handleSubscriptionUpdated(ctx context.Context, data json.RawMessage) error {
	var sub stripe.Subscription
	if err := json.Unmarshal(data, &sub); err != nil {
		return fmt.Errorf("parsing subscription data: %w", err)
	}

	if err := validateStripeSubscription(&sub); err != nil {
		h.logger.Warn("invalid webhook subscription data", "error", err)
		return fmt.Errorf("invalid subscription data: %w", err)
	}

	tier, lookupKey, ok := h.resolveTier(&sub)
	if !ok {
		h.logger.Warn("unknown stripe price on update",
			"price_id", extractPriceID(&sub), "lookup_key", extractLookupKey(&sub))
		return nil
	}

	orgID, err := h.resolveBoundOrgID(ctx, &sub)
	if err != nil {
		return err
	}
	if orgID == "" {
		return nil
	}

	status := string(sub.Status)
	if status == "" {
		status = "active"
	}

	periodStart, periodEnd := extractPeriod(&sub)

	// Check if this is a downgrade by comparing plan limits.
	existing, existErr := h.store.GetOrgSubscription(ctx, orgID)
	if existErr != nil && !errors.Is(existErr, ErrSubscriptionNotFound) {
		return fmt.Errorf("getting existing subscription: %w", existErr)
	}

	if subscriptionUpdateIsDowngrade(existing, tier) {
		return h.deferSubscriptionDowngrade(ctx, subscriptionDowngrade{
			orgID:                orgID,
			existing:             existing,
			tier:                 tier,
			stripeSubscriptionID: sub.ID,
			periodStart:          periodStart,
			periodEnd:            periodEnd,
		})
	}

	previousTier := ""
	if existing != nil {
		previousTier = existing.PlanTier
	}

	if err := h.store.ClearPendingPlanTier(ctx, orgID); err != nil && !errors.Is(err, ErrSubscriptionNotFound) {
		return fmt.Errorf("clearing pending plan tier: %w", err)
	}

	if err := h.store.UpdateOrgSubscriptionFull(ctx, orgID, string(tier), status, periodStart, periodEnd); err != nil {
		if errors.Is(err, ErrSubscriptionNotFound) {
			return h.createSubscriptionFromUpdate(ctx, subscriptionUpdateCreate{
				orgID:       orgID,
				sub:         &sub,
				tier:        tier,
				lookupKey:   lookupKey,
				status:      status,
				periodStart: periodStart,
				periodEnd:   periodEnd,
			})
		}
		return fmt.Errorf("updating org subscription: %w", err)
	}

	if h.enforcer != nil {
		h.enforcer.InvalidateOrgCache(orgID)
	}
	if _, err := ReconcileActiveAddonsForPlan(ctx, h.store, orgID, GetPlanLimits(tier)); err != nil {
		return fmt.Errorf("reconciling add-ons after subscription update: %w", err)
	}

	h.unpauseHTTPJobsAfterUpgrade(ctx, orgID, previousTier, tier)

	auditDetails := map[string]string{
		"plan_tier":              string(tier),
		"stripe_subscription_id": sub.ID,
	}
	if previousTier != "" && previousTier != string(tier) {
		auditDetails["previous_tier"] = previousTier
	}
	h.logAuditEvent(ctx, "subscription.updated", orgID, auditDetails)

	h.sendPlanChangedEmailAsync(ctx, orgID, previousTier, string(tier))

	h.logger.Info("subscription updated",
		"org_id", orgID,
		"plan_tier", tier,
		"status", status,
	)
	return nil
}

func subscriptionUpdateIsDowngrade(existing *OrgSubscription, tier domain.PlanTier) bool {
	return existing != nil &&
		existing.PlanTier != string(tier) &&
		IsDowngrade(domain.PlanTier(existing.PlanTier), tier)
}

type subscriptionDowngrade struct {
	orgID                string
	existing             *OrgSubscription
	tier                 domain.PlanTier
	stripeSubscriptionID string
	periodStart          *time.Time
	periodEnd            *time.Time
}

func (h *WebhookHandler) deferSubscriptionDowngrade(ctx context.Context, downgrade subscriptionDowngrade) error {
	// Downgrades are deferred as a single store operation so the pending tier
	// and billing-period dates cannot drift apart.
	if err := h.store.SetPendingDowngrade(ctx, downgrade.orgID, string(downgrade.tier), downgrade.periodStart, downgrade.periodEnd); err != nil {
		return fmt.Errorf("setting pending downgrade: %w", err)
	}
	h.logAuditEvent(ctx, "subscription.updated", downgrade.orgID, map[string]string{
		"plan_tier":              downgrade.existing.PlanTier,
		"pending_plan_tier":      string(downgrade.tier),
		"previous_tier":          downgrade.existing.PlanTier,
		"stripe_subscription_id": downgrade.stripeSubscriptionID,
	})

	h.logger.Info("subscription downgrade deferred",
		"org_id", downgrade.orgID,
		"current_tier", downgrade.existing.PlanTier,
		"pending_tier", downgrade.tier,
	)

	h.maybeSendHTTPJobsDowngradeWarning(ctx, downgrade.orgID, downgrade.tier, downgrade.periodEnd)
	return nil
}

type subscriptionUpdateCreate struct {
	orgID       string
	sub         *stripe.Subscription
	tier        domain.PlanTier
	lookupKey   string
	status      string
	periodStart *time.Time
	periodEnd   *time.Time
}

func (h *WebhookHandler) createSubscriptionFromUpdate(ctx context.Context, update subscriptionUpdateCreate) error {
	now := time.Now()
	customerID := update.sub.Customer.ID
	var lookupKeyPtr *string
	if update.lookupKey != "" {
		lookupKeyPtr = &update.lookupKey
	}
	orgSub := &OrgSubscription{
		ID:                    update.sub.ID,
		OrgID:                 update.orgID,
		PlanTier:              string(update.tier),
		StripeSubscriptionID:  &update.sub.ID,
		StripeCustomerID:      &customerID,
		StripeLookupKey:       lookupKeyPtr,
		Status:                update.status,
		CurrentPeriodStart:    update.periodStart,
		CurrentPeriodEnd:      update.periodEnd,
		SpendingLimitMicrousd: -1,
		LimitAction:           "reject",
		CreatedAt:             now,
		UpdatedAt:             now,
	}
	if err := h.store.UpsertOrgSubscription(ctx, orgSub); err != nil {
		return err
	}
	if _, err := ReconcileActiveAddonsForPlan(ctx, h.store, update.orgID, GetPlanLimits(update.tier)); err != nil {
		return fmt.Errorf("reconciling add-ons after subscription fallback create: %w", err)
	}
	if h.enforcer != nil {
		h.enforcer.InvalidateOrgCache(update.orgID)
	}
	h.logAuditEvent(ctx, "subscription.updated", update.orgID, map[string]string{
		"plan_tier":              string(update.tier),
		"stripe_subscription_id": update.sub.ID,
	})
	h.logger.Info("subscription updated (created via fallback)",
		"org_id", update.orgID,
		"plan_tier", update.tier,
		"status", update.status,
	)
	return nil
}

func (h *WebhookHandler) unpauseHTTPJobsAfterUpgrade(ctx context.Context, orgID string, previousTier string, tier domain.PlanTier) {
	newAllowsHTTP := GetPlanLimits(tier).AllowsHTTPMode
	oldAllowsHTTP := previousTier != "" && GetPlanLimits(domain.PlanTier(previousTier)).AllowsHTTPMode
	if !newAllowsHTTP || oldAllowsHTTP {
		return
	}

	unpaused, err := h.store.UnpauseJobsByPauseReason(ctx, orgID, "plan_downgrade")
	if err != nil {
		h.logger.Warn("failed to unpause HTTP jobs on upgrade", "org_id", orgID, "error", err)
		return
	}
	if unpaused == 0 {
		return
	}
	h.logAuditEvent(ctx, "jobs.auto_unpaused", orgID, map[string]string{
		"count":  fmt.Sprintf("%d", unpaused),
		"reason": "plan_upgrade",
	})
	h.logger.Info("auto-unpaused HTTP jobs on upgrade",
		"org_id", orgID, "count", unpaused)
}

func (h *WebhookHandler) sendPlanChangedEmailAsync(ctx context.Context, orgID string, oldTier string, newTier string) {
	if h.billingEmails == nil || oldTier == "" || oldTier == newTier {
		return
	}

	emails, _ := h.store.ListOrgAdminEmails(ctx, orgID)
	h.goAsync(func() {
		emailCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		h.billingEmails.SendPlanChanged(emailCtx, emails, oldTier, newTier)
	})
}

// handleSubscriptionDeleted handles Stripe's customer.subscription.deleted event.
// Stripe fires this for both cancellations (cancel_at_period_end) and immediate revocations.
func (h *WebhookHandler) handleSubscriptionDeleted(ctx context.Context, data json.RawMessage) error {
	var sub stripe.Subscription
	if err := json.Unmarshal(data, &sub); err != nil {
		return fmt.Errorf("parsing subscription data: %w", err)
	}

	if err := validateStripeSubscription(&sub); err != nil {
		h.logger.Warn("invalid webhook subscription data", "error", err)
		return fmt.Errorf("invalid subscription data: %w", err)
	}

	// Handle addon subscription cancellation.
	if h.isAddonSubscription(&sub) {
		return h.handleAddonSubscriptionCanceled(ctx, &sub)
	}

	orgID, err := h.resolveBoundOrgID(ctx, &sub)
	if err != nil {
		return err
	}
	if orgID == "" {
		return nil
	}

	// If CancelAtPeriodEnd was set, treat as canceled (deferred); otherwise revoked (immediate).
	if sub.CancelAtPeriodEnd {
		return h.applySubscriptionCanceled(ctx, orgID, &sub)
	}
	return h.applySubscriptionRevoked(ctx, orgID, &sub)
}

func (h *WebhookHandler) applySubscriptionCanceled(ctx context.Context, orgID string, sub *stripe.Subscription) error {
	existing, err := h.store.GetOrgSubscription(ctx, orgID)
	if err != nil {
		if errors.Is(err, ErrSubscriptionNotFound) {
			return nil
		}
		return fmt.Errorf("getting org subscription: %w", err)
	}

	existing.Status = "canceled"
	canceledAt := timeFromUnix(sub.CanceledAt)
	existing.CanceledAt = canceledAt
	existing.UpdatedAt = time.Now()

	if err := h.store.UpsertOrgSubscription(ctx, existing); err != nil {
		return fmt.Errorf("updating canceled subscription: %w", err)
	}

	// Queue a downgrade to free at period end so paid quotas don't persist indefinitely.
	if existing.PlanTier != string(domain.PlanFree) {
		if err := h.store.SetPendingPlanTier(ctx, orgID, string(domain.PlanFree)); err != nil {
			if !errors.Is(err, ErrSubscriptionNotFound) {
				return fmt.Errorf("setting pending free tier on cancellation: %w", err)
			}
		}
	}

	if h.enforcer != nil {
		h.enforcer.InvalidateOrgCache(orgID)
	}

	h.logAuditEvent(ctx, "subscription.canceled", orgID, map[string]string{
		"plan_tier":              existing.PlanTier,
		"stripe_subscription_id": sub.ID,
	})

	h.posthog.CaptureRevenueEvent(orgID, "subscription_canceled_server", map[string]any{
		"plan":                   existing.PlanTier,
		"stripe_subscription_id": sub.ID,
	})

	h.logger.Info("subscription canceled",
		"org_id", orgID,
		"plan_tier", existing.PlanTier,
	)
	return nil
}

func (h *WebhookHandler) applySubscriptionRevoked(ctx context.Context, orgID string, sub *stripe.Subscription) error {
	if err := h.store.UpdateOrgSubscriptionPlan(ctx, orgID, string(domain.PlanFree), "revoked"); err != nil {
		if errors.Is(err, ErrSubscriptionNotFound) {
			return nil
		}
		return fmt.Errorf("revoking subscription: %w", err)
	}
	if err := h.store.ClearPendingPlanTier(ctx, orgID); err != nil && !errors.Is(err, ErrSubscriptionNotFound) {
		return fmt.Errorf("clearing pending downgrade on revoke: %w", err)
	}

	if h.enforcer != nil {
		h.enforcer.InvalidateOrgCache(orgID)
	}

	h.logAuditEvent(ctx, "subscription.revoked", orgID, map[string]string{
		"plan_tier":              string(domain.PlanFree),
		"stripe_subscription_id": sub.ID,
	})

	h.posthog.CaptureRevenueEvent(orgID, "subscription_revoked_server", map[string]any{
		"stripe_subscription_id": sub.ID,
	})

	if h.enforcer != nil {
		h.enforcer.DispatchBilling(ctx, orgID, domain.PlanFree,
			domain.WebhookEventBillingSuspended, map[string]any{
				"stripe_subscription_id": sub.ID,
				"reason":                 "revoked",
			})
	}

	h.logger.Info("subscription revoked, downgraded to free",
		"org_id", orgID,
	)
	return nil
}

// invoiceSubscription extracts the subscription from a Stripe invoice.
// In Stripe API v2025+, the subscription is nested under parent.subscription_details.
func invoiceSubscription(inv *stripe.Invoice) *stripe.Subscription {
	if inv.Parent != nil && inv.Parent.SubscriptionDetails != nil && inv.Parent.SubscriptionDetails.Subscription != nil {
		return inv.Parent.SubscriptionDetails.Subscription
	}
	return nil
}

// resolveOrgIDFromInvoice resolves invoices only through persisted Stripe
// bindings. Metadata is used solely as a conflict check, never as authority.
func (h *WebhookHandler) resolveOrgIDFromInvoice(ctx context.Context, inv *stripe.Invoice) (string, error) {
	sub := invoiceSubscription(inv)
	metadataOrgID := invoiceMetadataOrgID(inv)
	if sub != nil && sub.ID != "" {
		existing, err := h.store.GetOrgSubscriptionByStripeSubscriptionID(ctx, sub.ID)
		if err != nil && !errors.Is(err, ErrSubscriptionNotFound) {
			return "", fmt.Errorf("lookup invoice subscription binding: %w", err)
		}
		if existing != nil {
			return validateStripeBindingOrg(metadataOrgID, existing.OrgID, "subscription", sub.ID)
		}
	}
	if inv.Customer != nil && inv.Customer.ID != "" {
		existing, err := h.store.GetOrgSubscriptionByStripeCustomerID(ctx, inv.Customer.ID)
		if err != nil && !errors.Is(err, ErrSubscriptionNotFound) {
			return "", fmt.Errorf("lookup invoice customer binding: %w", err)
		}
		if existing != nil {
			return validateStripeBindingOrg(metadataOrgID, existing.OrgID, "customer", inv.Customer.ID)
		}
	}
	if h.allowTestMetadata {
		return metadataOrgID, nil
	}
	if metadataOrgID != "" {
		return "", errUnboundStripeObject
	}
	return "", nil
}

func invoiceMetadataOrgID(inv *stripe.Invoice) string {
	sub := invoiceSubscription(inv)
	if sub != nil && sub.Metadata != nil {
		if id, ok := sub.Metadata["org_id"]; ok && isValidUUID(id) {
			return id
		}
	}
	if inv.Parent != nil && inv.Parent.SubscriptionDetails != nil && inv.Parent.SubscriptionDetails.Metadata != nil {
		if id, ok := inv.Parent.SubscriptionDetails.Metadata["org_id"]; ok && isValidUUID(id) {
			return id
		}
	}
	if inv.Customer != nil && inv.Customer.Metadata != nil {
		if id, ok := inv.Customer.Metadata["org_id"]; ok && isValidUUID(id) {
			return id
		}
	}
	return ""
}

// handlePaymentSucceeded handles invoice.paid events from Stripe.
func (h *WebhookHandler) handlePaymentSucceeded(ctx context.Context, data json.RawMessage) error {
	var invoice stripe.Invoice
	if err := json.Unmarshal(data, &invoice); err != nil {
		return fmt.Errorf("parsing invoice data: %w", err)
	}

	sub := invoiceSubscription(&invoice)
	if sub == nil || invoice.Customer == nil {
		return nil
	}

	orgID, err := h.resolveOrgIDFromInvoice(ctx, &invoice)
	if err != nil {
		return err
	}
	if orgID == "" {
		h.recordOrphanInvoiceDelivery(ctx, "invoice.paid", &invoice)
		return nil
	}

	existing, err := h.store.GetOrgSubscription(ctx, orgID)
	if err != nil {
		if errors.Is(err, ErrSubscriptionNotFound) {
			return nil
		}
		return fmt.Errorf("getting subscription for payment success: %w", err)
	}

	// Only clear grace if the org is actually in grace or restricted state.
	recovered := existing.PaymentStatus == "grace" || existing.PaymentStatus == "restricted"
	if recovered {
		if err := h.store.UpdatePaymentStatus(ctx, orgID, "ok", nil); err != nil {
			return fmt.Errorf("clearing grace on payment success: %w", err)
		}
		if h.enforcer != nil {
			h.enforcer.InvalidateOrgCache(orgID)
		}
		h.logger.Info("payment succeeded, grace period cleared",
			"org_id", orgID,
		)
	}

	if h.dunningStore != nil {
		if err := h.dunningStore.ResolveDunning(ctx, orgID, time.Now().UTC()); err != nil {
			h.logger.Warn("resolve dunning on payment success failed", "org_id", orgID, "err", err)
		}
	}

	// Only dispatch billing.payment_succeeded on actual recovery from
	// grace/restricted. Routine renewal payments don't fire — they aren't
	// state changes a subscriber needs to react to.
	if recovered && h.enforcer != nil {
		h.enforcer.DispatchBilling(ctx, orgID, domain.PlanTier(existing.PlanTier),
			domain.WebhookEventBillingPaymentSucceeded, map[string]any{
				"stripe_invoice_id":      invoice.ID,
				"stripe_subscription_id": sub.ID,
				"plan_tier":              existing.PlanTier,
				"amount_paid_microusd":   stripeMinorUnitsToMicroUSD(invoice.AmountPaid),
				"paid_at":                time.Now().UTC().Format(time.RFC3339Nano),
			})
	}

	h.posthog.CaptureRevenueEvent(orgID, "payment_received", map[string]any{
		"plan":                   existing.PlanTier,
		"stripe_subscription_id": sub.ID,
	})

	return nil
}

// handlePaymentFailed handles invoice.payment_failed events from Stripe.
func (h *WebhookHandler) handlePaymentFailed(ctx context.Context, data json.RawMessage) error {
	var invoice stripe.Invoice
	if err := json.Unmarshal(data, &invoice); err != nil {
		return fmt.Errorf("parsing invoice data: %w", err)
	}

	sub := invoiceSubscription(&invoice)
	if sub == nil || invoice.Customer == nil {
		return nil
	}

	orgID, err := h.resolveOrgIDFromInvoice(ctx, &invoice)
	if err != nil {
		return err
	}
	if orgID == "" {
		h.recordOrphanInvoiceDelivery(ctx, "invoice.payment_failed", &invoice)
		return nil
	}

	existing, err := h.store.GetOrgSubscription(ctx, orgID)
	if err != nil {
		if errors.Is(err, ErrSubscriptionNotFound) {
			return nil
		}
		return fmt.Errorf("getting subscription for payment failure: %w", err)
	}

	// Only set grace period for paid plans.
	if existing.PlanTier == string(domain.PlanFree) {
		return nil
	}
	if existing.PaymentStatus == "restricted" {
		h.logger.Info("payment failed for already restricted org, leaving restriction in place",
			"org_id", orgID,
		)
		return nil
	}

	graceEnd := time.Now().Add(72 * time.Hour)
	if err := h.store.UpdatePaymentStatus(ctx, orgID, "grace", &graceEnd); err != nil && !errors.Is(err, ErrSubscriptionNotFound) {
		return fmt.Errorf("setting grace period on payment failure: %w", err)
	}
	if h.enforcer != nil {
		h.enforcer.InvalidateOrgCache(orgID)
	}
	h.logger.Info("payment failed, grace period set",
		"org_id", orgID,
		"grace_period_end", graceEnd,
	)

	// Send payment failed email.
	if h.billingEmails != nil {
		adminEmails, _ := h.store.ListOrgAdminEmails(ctx, orgID)
		localGraceEnd := graceEnd
		planTier := existing.PlanTier
		h.goAsync(func() {
			emailCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			h.billingEmails.SendPaymentFailed(emailCtx, adminEmails, planTier, localGraceEnd)
		})
	}

	if h.enforcer != nil {
		h.enforcer.DispatchBilling(ctx, orgID, domain.PlanTier(existing.PlanTier),
			domain.WebhookEventBillingDelinquent, map[string]any{
				"stripe_invoice_id":   invoice.ID,
				"grace_period_end":    graceEnd.UTC().Format(time.RFC3339Nano),
				"amount_due_microusd": stripeMinorUnitsToMicroUSD(invoice.AmountDue),
				"attempt_count":       invoice.AttemptCount,
			})
	}

	if h.dunningStore != nil {
		if err := h.dunningStore.StartDunning(ctx, orgID, time.Now().UTC()); err != nil && !errors.Is(err, ErrSubscriptionNotFound) {
			h.logger.Warn("start dunning on payment failure failed", "org_id", orgID, "err", err)
		}
	}

	return nil
}

// handleSubscriptionPaused handles customer.subscription.paused events.
// Sets the subscription status to "paused" and restricts access. The plan_tier
// is preserved so the org can be resumed onto its prior plan without re-reading
// the Stripe price-to-tier mapping.
func (h *WebhookHandler) handleSubscriptionPaused(ctx context.Context, data json.RawMessage) error {
	var sub stripe.Subscription
	if err := json.Unmarshal(data, &sub); err != nil {
		return fmt.Errorf("parsing subscription data: %w", err)
	}

	if err := validateStripeSubscription(&sub); err != nil {
		return fmt.Errorf("invalid subscription data: %w", err)
	}

	orgID, err := h.resolveBoundOrgID(ctx, &sub)
	if err != nil {
		return err
	}
	if orgID == "" {
		return nil
	}

	var previousPlanTier string
	if existing, err := h.store.GetOrgSubscription(ctx, orgID); err == nil && existing != nil {
		previousPlanTier = existing.PlanTier
	}

	if err := h.store.UpdateOrgSubscriptionStatus(ctx, orgID, "paused"); err != nil {
		if errors.Is(err, ErrSubscriptionNotFound) {
			return nil
		}
		return fmt.Errorf("pausing subscription: %w", err)
	}
	if err := h.store.UpdatePaymentStatus(ctx, orgID, "restricted", nil); err != nil && !errors.Is(err, ErrSubscriptionNotFound) {
		return fmt.Errorf("restricting paused subscription: %w", err)
	}

	if h.enforcer != nil {
		h.enforcer.InvalidateOrgCache(orgID)
	}

	details := map[string]string{
		"stripe_subscription_id": sub.ID,
	}
	if previousPlanTier != "" {
		details["previous_plan_tier"] = previousPlanTier
	}
	h.logAuditEvent(ctx, "subscription.paused", orgID, details)

	if h.enforcer != nil {
		h.enforcer.DispatchBilling(ctx, orgID, domain.PlanTier(previousPlanTier),
			domain.WebhookEventBillingSuspended, map[string]any{
				"stripe_subscription_id": sub.ID,
				"reason":                 "paused",
				"previous_plan_tier":     previousPlanTier,
			})
	}

	h.logger.Info("subscription paused", "org_id", orgID, "previous_plan_tier", previousPlanTier)
	return nil
}

// handleSubscriptionResumed handles customer.subscription.resumed events.
// Restores the subscription to active status.
func (h *WebhookHandler) handleSubscriptionResumed(ctx context.Context, data json.RawMessage) error {
	var sub stripe.Subscription
	if err := json.Unmarshal(data, &sub); err != nil {
		return fmt.Errorf("parsing subscription data: %w", err)
	}

	if err := validateStripeSubscription(&sub); err != nil {
		return fmt.Errorf("invalid subscription data: %w", err)
	}

	orgID, err := h.resolveBoundOrgID(ctx, &sub)
	if err != nil {
		return err
	}
	if orgID == "" {
		return nil
	}

	tier, _, ok := h.resolveTier(&sub)
	if !ok {
		return nil
	}

	periodStart, periodEnd := extractPeriod(&sub)
	if err := h.store.UpdateOrgSubscriptionFull(ctx, orgID, string(tier), "active", periodStart, periodEnd); err != nil {
		if errors.Is(err, ErrSubscriptionNotFound) {
			return nil
		}
		return fmt.Errorf("resuming subscription: %w", err)
	}
	if err := h.store.UpdatePaymentStatus(ctx, orgID, "ok", nil); err != nil && !errors.Is(err, ErrSubscriptionNotFound) {
		return fmt.Errorf("clearing payment restriction on subscription resume: %w", err)
	}
	if _, err := ReconcileActiveAddonsForPlan(ctx, h.store, orgID, GetPlanLimits(tier)); err != nil {
		return fmt.Errorf("reconciling add-ons after subscription resume: %w", err)
	}

	if h.enforcer != nil {
		h.enforcer.InvalidateOrgCache(orgID)
	}

	h.logAuditEvent(ctx, "subscription.resumed", orgID, map[string]string{
		"plan_tier":              string(tier),
		"stripe_subscription_id": sub.ID,
	})

	h.logger.Info("subscription resumed", "org_id", orgID, "plan_tier", tier)
	return nil
}

// handleTrialWillEnd handles Stripe's trial-ending lifecycle event for legacy
// or contract-specific temporary access.
func (h *WebhookHandler) handleTrialWillEnd(ctx context.Context, data json.RawMessage) error {
	var sub stripe.Subscription
	if err := json.Unmarshal(data, &sub); err != nil {
		return fmt.Errorf("parsing subscription data: %w", err)
	}

	orgID, err := h.resolveBoundOrgID(ctx, &sub)
	if err != nil {
		return err
	}
	if orgID == "" {
		return nil
	}

	trialEnd := timeFromUnix(sub.TrialEnd)
	daysRemaining := 3
	trialEndStr := "soon"
	if trialEnd != nil {
		daysRemaining = max(0, int(time.Until(*trialEnd).Hours()/24))
		trialEndStr = trialEnd.Format("January 2, 2006")
	}

	h.logAuditEvent(ctx, "subscription.trial_will_end", orgID, map[string]string{
		"stripe_subscription_id": sub.ID,
		"trial_end":              trialEndStr,
	})

	if h.billingEmails != nil {
		adminEmails, _ := h.store.ListOrgAdminEmails(ctx, orgID)
		localEnd := trialEndStr
		localDays := daysRemaining
		h.goAsync(func() {
			emailCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			h.billingEmails.SendTrialEndingSoon(emailCtx, adminEmails, localEnd, localDays)
		})
	}

	h.logger.Info("temporary access ending soon", "org_id", orgID, "days_remaining", daysRemaining)
	return nil
}

// handleChargeDisputeCreated fires when a customer disputes a charge (chargeback).
// Records an audit event and notifies admins.
func (h *WebhookHandler) handleChargeDisputeCreated(ctx context.Context, data json.RawMessage) error {
	var dispute stripe.Dispute
	if err := json.Unmarshal(data, &dispute); err != nil {
		return fmt.Errorf("parsing dispute data: %w", err)
	}

	orgID, err := h.resolveOrgIDFromDispute(ctx, &dispute)
	if err != nil {
		return err
	}
	if orgID == "" {
		h.logger.Warn("cannot resolve org_id from dispute", "dispute_id", dispute.ID)
		return nil
	}

	amountStr := fmt.Sprintf("$%.2f", float64(dispute.Amount)/100)

	h.logAuditEvent(ctx, "charge.dispute.created", orgID, map[string]string{
		"dispute_id": dispute.ID,
		"amount":     amountStr,
		"reason":     string(dispute.Reason),
	})

	if h.billingEmails != nil {
		adminEmails, _ := h.store.ListOrgAdminEmails(ctx, orgID)
		localAmount := amountStr
		h.goAsync(func() {
			emailCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			h.billingEmails.SendDisputeAlert(emailCtx, adminEmails, localAmount)
		})
	}

	h.logger.Warn("charge disputed",
		"org_id", orgID,
		"dispute_id", dispute.ID,
		"amount", amountStr,
		"reason", dispute.Reason,
	)
	return nil
}

func (h *WebhookHandler) resolveOrgIDFromDispute(ctx context.Context, dispute *stripe.Dispute) (string, error) {
	if dispute.Charge == nil || dispute.Charge.Customer == nil {
		return "", nil
	}
	customer := dispute.Charge.Customer
	metadataOrgID := ""
	if customer.Metadata != nil {
		if id, ok := customer.Metadata["org_id"]; ok && isValidUUID(id) {
			metadataOrgID = id
		}
	}
	if customer.ID == "" {
		if metadataOrgID != "" {
			return "", errUnboundStripeObject
		}
		return "", nil
	}
	existing, err := h.store.GetOrgSubscriptionByStripeCustomerID(ctx, customer.ID)
	if err != nil && !errors.Is(err, ErrSubscriptionNotFound) {
		return "", fmt.Errorf("lookup dispute customer binding: %w", err)
	}
	if existing == nil {
		if h.allowTestMetadata {
			return metadataOrgID, nil
		}
		if metadataOrgID != "" {
			return "", errUnboundStripeObject
		}
		return "", nil
	}
	return validateStripeBindingOrg(metadataOrgID, existing.OrgID, "customer", customer.ID)
}

// handleInvoiceUpcoming fires ~72 hours before an invoice is finalized.
// Sends a heads-up email so the org knows about the upcoming charge.
func (h *WebhookHandler) handleInvoiceUpcoming(ctx context.Context, data json.RawMessage) error {
	var invoice stripe.Invoice
	if err := json.Unmarshal(data, &invoice); err != nil {
		return fmt.Errorf("parsing invoice data: %w", err)
	}

	orgID, err := h.resolveOrgIDFromInvoice(ctx, &invoice)
	if err != nil {
		return err
	}
	if orgID == "" {
		return nil
	}

	amountDue := fmt.Sprintf("$%.2f", float64(invoice.AmountDue)/100)
	dueDate := "upcoming"
	if invoice.DueDate > 0 {
		dueDate = time.Unix(invoice.DueDate, 0).Format("January 2, 2006")
	} else if invoice.NextPaymentAttempt > 0 {
		dueDate = time.Unix(invoice.NextPaymentAttempt, 0).Format("January 2, 2006")
	}

	h.logAuditEvent(ctx, "invoice.upcoming", orgID, map[string]string{
		"amount_due": amountDue,
		"due_date":   dueDate,
	})

	if h.billingEmails != nil {
		adminEmails, _ := h.store.ListOrgAdminEmails(ctx, orgID)
		localAmount := amountDue
		localDate := dueDate
		h.goAsync(func() {
			emailCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			h.billingEmails.SendInvoiceUpcoming(emailCtx, adminEmails, localAmount, localDate)
		})
	}

	h.logger.Info("invoice upcoming", "org_id", orgID, "amount_due", amountDue)
	return nil
}

// handleInvoiceUncollectible fires when Stripe marks an invoice as uncollectible
// (all payment retries exhausted). Restricts the org similar to payment failure.
func (h *WebhookHandler) handleInvoiceUncollectible(ctx context.Context, data json.RawMessage) error {
	var invoice stripe.Invoice
	if err := json.Unmarshal(data, &invoice); err != nil {
		return fmt.Errorf("parsing invoice data: %w", err)
	}

	orgID, err := h.resolveOrgIDFromInvoice(ctx, &invoice)
	if err != nil {
		return err
	}
	if orgID == "" {
		return nil
	}

	existing, err := h.store.GetOrgSubscription(ctx, orgID)
	if err != nil {
		if errors.Is(err, ErrSubscriptionNotFound) {
			return nil
		}
		return fmt.Errorf("getting subscription for uncollectible: %w", err)
	}

	if existing.PlanTier == string(domain.PlanFree) {
		return nil
	}

	if err := h.store.UpdatePaymentStatus(ctx, orgID, "restricted", nil); err != nil && !errors.Is(err, ErrSubscriptionNotFound) {
		return fmt.Errorf("setting restricted on uncollectible: %w", err)
	}
	if h.enforcer != nil {
		h.enforcer.InvalidateOrgCache(orgID)
	}

	h.logAuditEvent(ctx, "invoice.uncollectible", orgID, invoiceAuditFields(&invoice))

	h.logger.Warn("invoice marked uncollectible, org restricted",
		"org_id", orgID,
		"invoice_id", invoice.ID,
	)
	return nil
}

// handleInvoiceFinalized fires when Stripe finalizes an invoice — the canonical
// signal that the amount due is locked in. We record an audit breadcrumb so the
// dashboard can correlate finalize → paid/payment_failed transitions without
// recomputing from raw Stripe data.
func (h *WebhookHandler) handleInvoiceFinalized(ctx context.Context, data json.RawMessage) error {
	var invoice stripe.Invoice
	if err := json.Unmarshal(data, &invoice); err != nil {
		return fmt.Errorf("parsing invoice data: %w", err)
	}

	orgID, err := h.resolveOrgIDFromInvoice(ctx, &invoice)
	if err != nil {
		return err
	}
	if orgID == "" {
		h.recordOrphanInvoiceDelivery(ctx, "invoice.finalized", &invoice)
		return nil
	}

	h.logAuditEvent(ctx, "invoice.finalized", orgID, invoiceAuditFields(&invoice))

	h.logger.Info("invoice finalized",
		"org_id", orgID,
		"invoice_id", invoice.ID,
		"amount_due", invoice.AmountDue,
	)
	return nil
}

// handleInvoiceFinalizationFailed fires when Stripe cannot finalize an invoice.
// This is unusual and indicates a billing configuration issue.
func (h *WebhookHandler) handleInvoiceFinalizationFailed(ctx context.Context, data json.RawMessage) error {
	var invoice stripe.Invoice
	if err := json.Unmarshal(data, &invoice); err != nil {
		return fmt.Errorf("parsing invoice data: %w", err)
	}

	orgID, err := h.resolveOrgIDFromInvoice(ctx, &invoice)
	if err != nil {
		return err
	}
	if orgID == "" {
		h.recordOrphanInvoiceDelivery(ctx, "invoice.finalization_failed", &invoice)
		return nil
	}

	h.logAuditEvent(ctx, "invoice.finalization_failed", orgID, invoiceAuditFields(&invoice))

	h.logger.Error("invoice finalization failed",
		"org_id", orgID,
		"invoice_id", invoice.ID,
	)
	return nil
}

// maybeSendHTTPJobsDowngradeWarning sends an email warning if the pending
// downgrade will cause HTTP-mode jobs to be paused.
func (h *WebhookHandler) maybeSendHTTPJobsDowngradeWarning(ctx context.Context, orgID string, pendingTier domain.PlanTier, periodEnd *time.Time) {
	targetLimits := GetPlanLimits(pendingTier)
	if targetLimits.AllowsHTTPMode || h.billingEmails == nil {
		return
	}

	httpCount, _ := h.store.CountHTTPJobsByOrg(ctx, orgID)
	if httpCount == 0 {
		return
	}

	adminEmails, _ := h.store.ListOrgAdminEmails(ctx, orgID)
	periodEndStr := "your next billing date"
	if periodEnd != nil {
		periodEndStr = periodEnd.Format("January 2, 2006")
	}

	localEnd := periodEndStr
	localCount := httpCount
	h.goAsync(func() {
		emailCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		h.billingEmails.SendDowngradeHTTPJobsWarning(emailCtx, adminEmails, localEnd, localCount)
	})
}

// resolveOrgID extracts the org_id from Stripe subscription metadata or customer metadata.
func (h *WebhookHandler) resolveOrgID(sub *stripe.Subscription) string {
	if sub.Metadata != nil {
		if orgID, ok := sub.Metadata["org_id"]; ok && isValidUUID(orgID) {
			return orgID
		}
	}
	if sub.Customer != nil && sub.Customer.Metadata != nil {
		if orgID, ok := sub.Customer.Metadata["org_id"]; ok && isValidUUID(orgID) {
			return orgID
		}
	}
	return ""
}

func (h *WebhookHandler) resolveBoundOrgID(ctx context.Context, sub *stripe.Subscription) (string, error) {
	metadataOrgID := h.resolveOrgID(sub)
	if sub.ID != "" {
		existing, err := h.store.GetOrgSubscriptionByStripeSubscriptionID(ctx, sub.ID)
		if err != nil && !errors.Is(err, ErrSubscriptionNotFound) {
			return "", fmt.Errorf("lookup subscription binding: %w", err)
		}
		if existing != nil {
			return validateStripeBindingOrg(metadataOrgID, existing.OrgID, "subscription", sub.ID)
		}
	}
	if sub.Customer != nil && sub.Customer.ID != "" {
		existing, err := h.store.GetOrgSubscriptionByStripeCustomerID(ctx, sub.Customer.ID)
		if err != nil && !errors.Is(err, ErrSubscriptionNotFound) {
			return "", fmt.Errorf("lookup customer binding: %w", err)
		}
		if existing != nil {
			return validateStripeBindingOrg(metadataOrgID, existing.OrgID, "customer", sub.Customer.ID)
		}
	}
	if h.allowTestMetadata {
		return metadataOrgID, nil
	}
	if metadataOrgID != "" {
		return "", errUnboundStripeObject
	}
	return "", nil
}

func validateStripeBindingOrg(metadataOrgID, boundOrgID, bindingType, bindingID string) (string, error) {
	if boundOrgID == "" {
		return metadataOrgID, nil
	}
	if metadataOrgID != "" && metadataOrgID != boundOrgID {
		return "", fmt.Errorf("stripe %s %s is already bound to org %s", bindingType, bindingID, boundOrgID)
	}
	return boundOrgID, nil
}

func (h *WebhookHandler) resolveOrgIDForNewSubscription(ctx context.Context, sub *stripe.Subscription, tier domain.PlanTier) (string, error) {
	orgID, err := h.resolveBoundOrgID(ctx, sub)
	if err == nil && orgID != "" {
		return orgID, nil
	}
	if err != nil && !errors.Is(err, errUnboundStripeObject) {
		return "", err
	}

	metadataOrgID := h.resolveOrgID(sub)
	if metadataOrgID == "" {
		return "", errUnboundStripeObject
	}
	if h.allowTestMetadata {
		return metadataOrgID, nil
	}
	existing, err := h.store.GetOrgSubscription(ctx, metadataOrgID)
	if err != nil {
		if errors.Is(err, ErrSubscriptionNotFound) {
			return "", errUnboundStripeObject
		}
		return "", fmt.Errorf("lookup pending subscription intent: %w", err)
	}
	if existing.StripeSubscriptionID != nil || existing.StripeCustomerID != nil {
		return "", errUnboundStripeObject
	}
	if existing.PendingPlanTier == nil || *existing.PendingPlanTier != string(tier) {
		return "", errUnboundStripeObject
	}
	return metadataOrgID, nil
}

// logAuditEvent records an audit event if the audit store is configured.
// Errors are logged but do not fail the webhook handler.
func (h *WebhookHandler) logAuditEvent(ctx context.Context, action, orgID string, details map[string]string) {
	if h.auditStore == nil {
		return
	}

	detailsJSON, err := json.Marshal(details)
	if err != nil {
		h.logger.Error("failed to marshal audit details", "error", err)
		return
	}

	ev := &domain.AuditEvent{
		ActorType:    "system",
		ActorID:      "stripe-webhook",
		Action:       action,
		ResourceType: "subscription",
		ResourceID:   orgID,
		Details:      detailsJSON,
	}

	if err := h.auditStore.CreateAuditEvent(ctx, ev); err != nil {
		h.logger.Error("failed to create audit event", "action", action, "org_id", orgID, "error", err)
	}
}

// handleAddonSubscriptionCreated creates an addon record when a Stripe addon
// subscription is created. lookupKey is the Stripe lookup_key that resolved to
// this addon (empty when resolution fell back to per-account price ID).
func (h *WebhookHandler) handleAddonSubscriptionCreated(ctx context.Context, sub *stripe.Subscription, addonType AddonType, lookupKey string) error {
	if !IsLaunchActiveAddonType(addonType) {
		h.logger.Warn("roadmap addon not sellable at launch, ignoring addon webhook",
			"subscription_id", sub.ID, "addon_type", addonType, "lookup_key", lookupKey)
		return nil
	}

	orgID, err := h.resolveBoundOrgID(ctx, sub)
	if err != nil {
		return err
	}
	if orgID == "" {
		h.logger.Warn("cannot resolve org_id for addon subscription", "subscription_id", sub.ID)
		return nil
	}

	// Check addon quantity cap for this org's plan tier.
	// A nil MaxAddonPacks map means addons are not allowed (e.g. Free tier).
	if h.enforcer != nil {
		limits, limErr := h.enforcer.GetOrgPlanLimits(ctx, orgID)
		if limErr != nil {
			return fmt.Errorf("get org plan limits for addon subscription: %w", limErr)
		}
		if limits.MaxAddonPacks == nil {
			h.logger.Warn("addons not allowed on plan, ignoring addon webhook",
				"org_id", orgID, "plan_tier", limits.PlanTier, "addon_type", addonType)
			return nil
		}
		maxPacks, hasCap := limits.MaxAddonPacks[addonType]
		if !hasCap {
			h.logger.Warn("addon not available on plan, ignoring addon webhook",
				"org_id", orgID, "plan_tier", limits.PlanTier, "addon_type", addonType)
			return nil
		}
		if maxPacks >= 0 {
			existing, countErr := h.store.CountActiveAddonsByType(ctx, orgID, addonType)
			if countErr != nil {
				return fmt.Errorf("count active addons for addon subscription: %w", countErr)
			}
			if existing >= maxPacks {
				h.logger.Warn("addon cap exceeded, ignoring addon webhook",
					"org_id", orgID, "addon_type", addonType, "cap", maxPacks, "existing", existing)
				return nil
			}
		}
	}

	_, periodEnd := extractPeriod(sub)
	var lookupKeyPtr *string
	if lookupKey != "" {
		lookupKeyPtr = &lookupKey
	}
	addon := &Addon{
		ID:                   sub.ID,
		OrgID:                orgID,
		AddonType:            addonType,
		Quantity:             1,
		StripeSubscriptionID: &sub.ID,
		StripeLookupKey:      lookupKeyPtr,
		Active:               true,
		ExpiresAt:            periodEnd,
	}

	if err := h.store.CreateAddon(ctx, addon); err != nil {
		return fmt.Errorf("creating addon record: %w", err)
	}

	if h.enforcer != nil {
		h.enforcer.InvalidateOrgCache(orgID)
	}

	h.logger.Info("addon subscription created",
		"org_id", orgID,
		"addon_type", addonType,
		"subscription_id", sub.ID,
	)
	return nil
}

// handleAddonSubscriptionCanceled deactivates an addon record when a Stripe
// addon subscription is canceled or deleted.
func (h *WebhookHandler) handleAddonSubscriptionCanceled(ctx context.Context, sub *stripe.Subscription) error {
	orgID, err := h.resolveBoundOrgID(ctx, sub)
	if err != nil {
		return err
	}
	if orgID == "" {
		return nil
	}

	if err := h.store.DeactivateAddon(ctx, sub.ID); err != nil {
		return fmt.Errorf("deactivating addon: %w", err)
	}

	if h.enforcer != nil {
		h.enforcer.InvalidateOrgCache(orgID)
	}

	h.logger.Info("addon subscription canceled",
		"org_id", orgID,
		"subscription_id", sub.ID,
	)
	return nil
}
