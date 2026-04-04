package billing

import (
	"encoding/json"
	"time"

	"github.com/stripe/stripe-go/v82"
)

// testSubscriptionData is a test-only compatibility type.
// It maps subscription fields to Stripe-compatible JSON
// so that existing test data constructions continue to work.
type testSubscriptionData struct {
	ID                 string            `json:"-"`
	ProductID          string            `json:"-"`
	CustomerID         string            `json:"-"`
	Status             string            `json:"-"`
	Metadata           map[string]string `json:"-"`
	Product            *testProductData  `json:"-"`
	Customer           *testCustomerData `json:"-"`
	CurrentPeriodStart *time.Time        `json:"-"`
	CurrentPeriodEnd   *time.Time        `json:"-"`
	CanceledAt         *time.Time        `json:"-"`
	CancelAtPeriodEnd  bool              `json:"-"`
}

// testCustomerData is a test-only compatibility type for customer data.
type testCustomerData struct {
	ID       string            `json:"id"`
	Email    string            `json:"email,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// testProductData is a test-only compatibility type for product/price data.
type testProductData struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

// MarshalJSON produces JSON compatible with stripe.Subscription.
func (p testSubscriptionData) MarshalJSON() ([]byte, error) {
	sub := p.ToStripe()
	return json.Marshal(sub)
}

// ToStripe converts testSubscriptionData to a *stripe.Subscription.
func (p testSubscriptionData) ToStripe() *stripe.Subscription {
	sub := &stripe.Subscription{
		ID:       p.ID,
		Status:   stripe.SubscriptionStatus(p.Status),
		Metadata: p.Metadata,
	}

	// Map CustomerID to Customer object.
	if p.CustomerID != "" || p.Customer != nil {
		cust := &stripe.Customer{}
		if p.Customer != nil {
			cust.ID = p.Customer.ID
			cust.Email = p.Customer.Email
			cust.Metadata = p.Customer.Metadata
		}
		if p.CustomerID != "" && cust.ID == "" {
			cust.ID = p.CustomerID
		}
		sub.Customer = cust
	}

	// Map ProductID (or Product.ID) to Items[0].Price.ID.
	priceID := p.ProductID
	if priceID == "" && p.Product != nil {
		priceID = p.Product.ID
	}
	if priceID != "" {
		sub.Items = &stripe.SubscriptionItemList{
			Data: []*stripe.SubscriptionItem{
				{
					Price: &stripe.Price{
						ID: priceID,
					},
				},
			},
		}
	}

	// Map period timestamps to the first subscription item (Stripe v2025+ format).
	if p.CurrentPeriodStart != nil || p.CurrentPeriodEnd != nil {
		if sub.Items == nil {
			sub.Items = &stripe.SubscriptionItemList{
				Data: []*stripe.SubscriptionItem{{}},
			}
		}
		if len(sub.Items.Data) == 0 {
			sub.Items.Data = []*stripe.SubscriptionItem{{}}
		}
		item := sub.Items.Data[0]
		if p.CurrentPeriodStart != nil {
			item.CurrentPeriodStart = p.CurrentPeriodStart.Unix()
		}
		if p.CurrentPeriodEnd != nil {
			item.CurrentPeriodEnd = p.CurrentPeriodEnd.Unix()
		}
	}
	if p.CanceledAt != nil {
		sub.CanceledAt = p.CanceledAt.Unix()
	}
	sub.CancelAtPeriodEnd = p.CancelAtPeriodEnd

	return sub
}

// StripeWebhookPayload is a test-only type that produces JSON compatible
// with stripe.Event so the webhook handler can parse it.
type StripeWebhookPayload struct {
	ID   string          `json:"id,omitempty"`
	Type string          `json:"type"`
	Data json.RawMessage `json:"-"`
}

// testInvoiceData is a test-only type that produces JSON compatible with stripe.Invoice.
// It uses the Stripe v2025+ nested parent.subscription_details structure.
type testInvoiceData struct {
	ID         string            `json:"-"`
	CustomerID string            `json:"-"`
	SubID      string            `json:"-"`
	Metadata   map[string]string `json:"-"`
}

// MarshalJSON produces JSON compatible with stripe.Invoice including parent.subscription_details.
func (d testInvoiceData) MarshalJSON() ([]byte, error) {
	type subDetail struct {
		Subscription *stripe.Subscription `json:"subscription"`
		Metadata     map[string]string    `json:"metadata,omitempty"`
	}
	type parent struct {
		SubscriptionDetails *subDetail `json:"subscription_details"`
	}
	type invoiceJSON struct {
		ID       string           `json:"id"`
		Customer *stripe.Customer `json:"customer,omitempty"`
		Parent   *parent          `json:"parent,omitempty"`
	}

	inv := invoiceJSON{ID: d.ID}
	if d.CustomerID != "" {
		inv.Customer = &stripe.Customer{ID: d.CustomerID}
	}
	if d.SubID != "" || d.Metadata != nil {
		sub := &stripe.Subscription{ID: d.SubID, Metadata: d.Metadata}
		inv.Parent = &parent{
			SubscriptionDetails: &subDetail{
				Subscription: sub,
				Metadata:     d.Metadata,
			},
		}
	}
	return json.Marshal(inv)
}

// MarshalJSON produces JSON compatible with stripe.Event.
// The Data field is placed under {"data": {"object": ...}}.
func (p StripeWebhookPayload) MarshalJSON() ([]byte, error) {
	type dataWrapper struct {
		Object json.RawMessage `json:"object"`
	}
	type eventJSON struct {
		ID   string      `json:"id,omitempty"`
		Type string      `json:"type"`
		Data dataWrapper `json:"data"`
	}
	return json.Marshal(eventJSON{
		ID:   p.ID,
		Type: p.Type,
		Data: dataWrapper{Object: p.Data},
	})
}
