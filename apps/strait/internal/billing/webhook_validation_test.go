package billing

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateSubscriptionData_Valid(t *testing.T) {
	t.Parallel()
	err := validateStripeSubscription(testSubscriptionData{
		ID:         "sub_1",
		ProductID:  "prod_1",
		CustomerID: "cust_1",
	}.ToStripe())
	require.NoError(t,
		err)
}

func TestValidateSubscriptionData_EmptyID(t *testing.T) {
	t.Parallel()
	err := validateStripeSubscription(testSubscriptionData{
		ID:         "",
		ProductID:  "prod_1",
		CustomerID: "cust_1",
	}.ToStripe())
	require.Error(t, err)
}

func TestValidateSubscriptionData_EmptyProductID(t *testing.T) {
	t.Parallel()
	err := validateStripeSubscription(testSubscriptionData{
		ID:         "sub_1",
		ProductID:  "",
		CustomerID: "cust_1",
	}.ToStripe())
	require.Error(t, err)
}

func TestValidateSubscriptionData_ProductFromNested(t *testing.T) {
	t.Parallel()
	err := validateStripeSubscription(testSubscriptionData{
		ID:         "sub_1",
		ProductID:  "",
		Product:    &testProductData{ID: "prod_nested"},
		CustomerID: "cust_1",
	}.ToStripe())
	require.NoError(t,
		err)
}

func TestValidateSubscriptionData_EmptyCustomerID(t *testing.T) {
	t.Parallel()
	err := validateStripeSubscription(testSubscriptionData{
		ID:         "sub_1",
		ProductID:  "prod_1",
		CustomerID: "",
	}.ToStripe())
	require.Error(t, err)
}

func TestIsValidUUID_Valid(t *testing.T) {
	t.Parallel()
	require.True(t, isValidUUID("550e8400-e29b-41d4-a716-446655440000"))
}

func TestIsValidUUID_Invalid(t *testing.T) {
	t.Parallel()
	cases := []string{
		"",
		"not-a-uuid",
		"550e8400-e29b-41d4-a716",
		"'; DROP TABLE orgs; --",
		"550e8400-e29b-41d4-a716-44665544000g",
		"XXXXXXXX-XXXX-XXXX-XXXX-XXXXXXXXXXXX",
	}
	for _, c := range cases {
		assert.False(t, isValidUUID(c))
	}
}

func TestResolveOrgID_ValidUUID(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{}
	mapping := NewStripeMapping("s", "", "p", "")
	h := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil, WithDevBypassSignatureCheck())

	orgID := h.resolveOrgID(testSubscriptionData{
		Metadata: map[string]string{"org_id": "550e8400-e29b-41d4-a716-446655440000"},
	}.ToStripe())
	require.Equal(t, "550e8400-e29b-41d4-a716-446655440000",

		orgID)
}

func TestResolveOrgID_InvalidUUID_ReturnsEmpty(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{}
	mapping := NewStripeMapping("s", "", "p", "")
	h := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil, WithDevBypassSignatureCheck())

	orgID := h.resolveOrgID(testSubscriptionData{
		Metadata: map[string]string{"org_id": "not-a-uuid"},
	}.ToStripe())
	require.Empty(t, orgID)
}

func TestResolveOrgID_SQLInjection_ReturnsEmpty(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{}
	mapping := NewStripeMapping("s", "", "p", "")
	h := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil, WithDevBypassSignatureCheck())

	orgID := h.resolveOrgID(testSubscriptionData{
		Metadata: map[string]string{"org_id": "'; DROP TABLE organizations; --"},
	}.ToStripe())
	require.Empty(t, orgID)
}

func TestIsValidEmail_Valid(t *testing.T) {
	t.Parallel()
	cases := []string{
		"user@example.com",
		"test+tag@domain.co",
		"name@sub.domain.org",
	}
	for _, c := range cases {
		assert.True(t, isValidEmail(c))
	}
}

func TestIsValidEmail_Invalid(t *testing.T) {
	t.Parallel()
	cases := []string{
		"",
		"not-an-email",
		"@no-local.com",
	}
	for _, c := range cases {
		assert.False(t, isValidEmail(
			c),
		)
	}
}

func FuzzResolveOrgID(f *testing.F) {
	f.Add("550e8400-e29b-41d4-a716-446655440000")
	f.Add("")
	f.Add("not-a-uuid")
	f.Add("'; DROP TABLE orgs; --")

	store := &mockBillingStore{}
	mapping := NewStripeMapping("s", "", "p", "")
	h := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil, WithDevBypassSignatureCheck())

	f.Fuzz(func(t *testing.T, orgID string) {
		result := h.resolveOrgID(testSubscriptionData{
			Metadata: map[string]string{"org_id": orgID},
		}.ToStripe())
		assert.False(t, result !=
			"" &&

			!isValidUUID(result))
	})
}
