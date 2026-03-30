package billing

import (
	"log/slog"
	"testing"
)

func TestValidateSubscriptionData_Valid(t *testing.T) {
	t.Parallel()
	err := validateSubscriptionData(PolarSubscriptionData{
		ID:         "sub_1",
		ProductID:  "prod_1",
		CustomerID: "cust_1",
	})
	if err != nil {
		t.Fatalf("expected nil, got: %v", err)
	}
}

func TestValidateSubscriptionData_EmptyID(t *testing.T) {
	t.Parallel()
	err := validateSubscriptionData(PolarSubscriptionData{
		ID:         "",
		ProductID:  "prod_1",
		CustomerID: "cust_1",
	})
	if err == nil {
		t.Fatal("expected error for empty ID")
	}
}

func TestValidateSubscriptionData_EmptyProductID(t *testing.T) {
	t.Parallel()
	err := validateSubscriptionData(PolarSubscriptionData{
		ID:         "sub_1",
		ProductID:  "",
		CustomerID: "cust_1",
	})
	if err == nil {
		t.Fatal("expected error for empty product ID")
	}
}

func TestValidateSubscriptionData_ProductFromNested(t *testing.T) {
	t.Parallel()
	err := validateSubscriptionData(PolarSubscriptionData{
		ID:         "sub_1",
		ProductID:  "",
		Product:    &PolarProductData{ID: "prod_nested"},
		CustomerID: "cust_1",
	})
	if err != nil {
		t.Fatalf("expected nil with nested product, got: %v", err)
	}
}

func TestValidateSubscriptionData_EmptyCustomerID(t *testing.T) {
	t.Parallel()
	err := validateSubscriptionData(PolarSubscriptionData{
		ID:         "sub_1",
		ProductID:  "prod_1",
		CustomerID: "",
	})
	if err == nil {
		t.Fatal("expected error for empty customer ID")
	}
}

func TestIsValidUUID_Valid(t *testing.T) {
	t.Parallel()
	if !isValidUUID("550e8400-e29b-41d4-a716-446655440000") {
		t.Fatal("expected valid UUID")
	}
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
		if isValidUUID(c) {
			t.Errorf("expected invalid for %q", c)
		}
	}
}

func TestResolveOrgID_ValidUUID(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{}
	mapping := NewPolarMapping("s", "", "p", "")
	h := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil)

	orgID := h.resolveOrgID(PolarSubscriptionData{
		Metadata: map[string]string{"org_id": "550e8400-e29b-41d4-a716-446655440000"},
	})
	if orgID != "550e8400-e29b-41d4-a716-446655440000" {
		t.Fatalf("expected valid UUID, got %q", orgID)
	}
}

func TestResolveOrgID_InvalidUUID_ReturnsEmpty(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{}
	mapping := NewPolarMapping("s", "", "p", "")
	h := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil)

	orgID := h.resolveOrgID(PolarSubscriptionData{
		Metadata: map[string]string{"org_id": "not-a-uuid"},
	})
	if orgID != "" {
		t.Fatalf("expected empty for invalid UUID, got %q", orgID)
	}
}

func TestResolveOrgID_SQLInjection_ReturnsEmpty(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{}
	mapping := NewPolarMapping("s", "", "p", "")
	h := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil)

	orgID := h.resolveOrgID(PolarSubscriptionData{
		Metadata: map[string]string{"org_id": "'; DROP TABLE organizations; --"},
	})
	if orgID != "" {
		t.Fatalf("expected empty for SQL injection attempt, got %q", orgID)
	}
}

func TestIsValidEmail_Valid(t *testing.T) {
	t.Parallel()
	cases := []string{
		"user@example.com",
		"test+tag@domain.co",
		"name@sub.domain.org",
	}
	for _, c := range cases {
		if !isValidEmail(c) {
			t.Errorf("expected valid for %q", c)
		}
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
		if isValidEmail(c) {
			t.Errorf("expected invalid for %q", c)
		}
	}
}

func FuzzResolveOrgID(f *testing.F) {
	f.Add("550e8400-e29b-41d4-a716-446655440000")
	f.Add("")
	f.Add("not-a-uuid")
	f.Add("'; DROP TABLE orgs; --")

	store := &mockBillingStore{}
	mapping := NewPolarMapping("s", "", "p", "")
	h := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil)

	f.Fuzz(func(t *testing.T, orgID string) {
		result := h.resolveOrgID(PolarSubscriptionData{
			Metadata: map[string]string{"org_id": orgID},
		})
		if result != "" && !isValidUUID(result) {
			t.Errorf("resolveOrgID returned non-UUID: %q", result)
		}
	})
}
