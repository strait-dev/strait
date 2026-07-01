package billing

import (
	"context"
	"errors"
	"testing"

	"strait/internal/domain"
	"strait/internal/transactional"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunAllowanceDisplay_Enterprise(t *testing.T) {
	t.Parallel()
	got := runAllowanceDisplay("enterprise")
	assert.Equal(t, "Custom (per contract)", got)
}

func TestRunAllowanceDisplay_AllTiers(t *testing.T) {
	t.Parallel()
	tests := []struct {
		tier string
		want string
	}{
		{"free", "5000"},
		{"starter", "50000"},
		{"pro", "1000000"},
		{"scale", "5000000"},
		{"business", "25000000"},
		{"enterprise", "Custom (per contract)"},
		{"unknown", "5000"},
	}
	for _, tt := range tests {
		t.Run(tt.tier, func(t *testing.T) {
			t.Parallel()
			got := runAllowanceDisplay(domain.PlanTier(tt.tier))
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestPlanDisplayName_AllTiers(t *testing.T) {
	t.Parallel()
	tests := []struct {
		tier string
		want string
	}{
		{"free", "Free"},
		{"starter", "Starter"},
		{"pro", "Pro"},
		{"scale", "Scale"},
		{"enterprise", "Enterprise"},
		{"unknown", "Free"},
	}
	for _, tt := range tests {
		t.Run(tt.tier, func(t *testing.T) {
			t.Parallel()
			got := planDisplayName(domain.PlanTier(tt.tier))
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNewTransactionalWelcomeEmailFunc_NilClient(t *testing.T) {
	t.Parallel()
	require.Nil(t, NewTransactionalWelcomeEmailFunc(nil, ""))
}

func TestNewTransactionalWelcomeEmailFunc_InvalidEmail(t *testing.T) {
	t.Parallel()
	fn := NewTransactionalWelcomeEmailFunc(&mockBillingTransactionalClient{}, "")
	err := fn(context.Background(), "org-1", domain.PlanStarter, "not-an-email")
	require.Error(t, err)
}

func TestNewTransactionalWelcomeEmailFunc_SendsExpectedTemplateIntents(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		fromEmail    string
		tier         domain.PlanTier
		wantFrom     string
		wantTemplate string
		wantProps    map[string]any
	}{
		{
			name:         "starter uses default sender",
			tier:         domain.PlanStarter,
			wantFrom:     "noreply@strait.dev",
			wantTemplate: "billing.paid_plan_welcome",
			wantProps: map[string]any{
				"planName":            "Starter",
				"monthlyRunAllowance": "50000",
			},
		},
		{
			name:         "enterprise uses custom sender",
			fromEmail:    "welcome@example.com",
			tier:         domain.PlanEnterprise,
			wantFrom:     "welcome@example.com",
			wantTemplate: "billing.enterprise_welcome",
			wantProps:    map[string]any{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			client := &mockBillingTransactionalClient{}
			fn := NewTransactionalWelcomeEmailFunc(client, tc.fromEmail)

			err := fn(context.Background(), "org-1", tc.tier, "customer@example.com")
			require.NoError(t, err)
			require.Len(t, client.calls, 1)
			got := client.calls[0]
			assert.Equal(t, tc.wantFrom, got.From)
			assert.Equal(t, []string{"customer@example.com"}, got.To)
			assert.Equal(t, tc.wantTemplate, got.Template)
			assert.Contains(t, got.IdempotencyKey, "billing:welcome:org-1:")
			for key, want := range tc.wantProps {
				assert.Equal(t, want, got.Props[key])
			}
		})
	}
}

func TestNewTransactionalWelcomeEmailFunc_ReturnsSendError(t *testing.T) {
	t.Parallel()

	sendErr := errors.New("app unavailable")
	fn := NewTransactionalWelcomeEmailFunc(&mockBillingTransactionalClient{
		sendFn: func(context.Context, transactional.Request) error {
			return sendErr
		},
	}, "")

	err := fn(context.Background(), "org-1", domain.PlanPro, "customer@example.com")
	require.ErrorIs(t, err, sendErr)
	require.ErrorContains(t, err, "send welcome email through transactional endpoint")
}
