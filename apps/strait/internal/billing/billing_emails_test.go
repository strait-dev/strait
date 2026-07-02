package billing

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"

	"strait/internal/transactional"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockBillingTransactionalClient struct {
	sendFn func(context.Context, transactional.Request) error
	calls  []transactional.Request
}

func (m *mockBillingTransactionalClient) Send(ctx context.Context, req transactional.Request) error {
	m.calls = append(m.calls, req)
	if m.sendFn != nil {
		return m.sendFn(ctx, req)
	}
	return nil
}

func transactionalPropsMap(t *testing.T, props any) map[string]any {
	t.Helper()
	payload, err := json.Marshal(props)
	require.NoError(t, err)
	var out map[string]any
	require.NoError(t, json.Unmarshal(payload, &out))
	return out
}

func TestNewBillingEmailSender_NilClient_ReturnsNil(t *testing.T) {
	t.Parallel()
	s := NewBillingEmailSender(nil, "", nil)
	require.Nil(t, s)
}

func TestNewBillingEmailSender_DefaultFromEmail(t *testing.T) {
	t.Parallel()
	s := NewBillingEmailSender(&mockBillingTransactionalClient{}, "", nil)
	require.NotNil(t, s)
	assert.Equal(t, "billing@strait.dev", s.fromEmail)
}

func TestNewBillingEmailSender_CustomFromEmail(t *testing.T) {
	t.Parallel()
	s := NewBillingEmailSender(&mockBillingTransactionalClient{}, "custom@example.com", nil)
	require.NotNil(t, s)
	assert.Equal(t, "custom@example.com", s.fromEmail)
}

func TestBillingEmailSender_NilSafety(t *testing.T) {
	t.Parallel()
	var s *BillingEmailSender
	s.SendSpendingLimitWarning(context.Background(), nil, "", "", "", "")
	s.SendOverageAlert(context.Background(), nil, "", "", "")
	s.SendPaymentFailed(context.Background(), nil, "", time.Now())
	s.SendPlanChanged(context.Background(), nil, "", "")
	s.SendEnterpriseContractReminder(context.Background(), nil, "", true, 30)
	s.SendDowngradeHTTPJobsWarning(context.Background(), nil, "", 0)
	s.SendContractExpired(context.Background(), nil, "")
	s.SendTrialEndingSoon(context.Background(), nil, "", 0)
	s.SendDisputeAlert(context.Background(), nil, "")
	s.SendInvoiceUpcoming(context.Background(), nil, "", "")
	s.SendDunningStep(context.Background(), nil, "", 0)
}

func TestBillingEmailSender_EmptyRecipients(t *testing.T) {
	t.Parallel()
	client := &mockBillingTransactionalClient{}
	s := NewBillingEmailSender(client, "", slog.New(slog.DiscardHandler))

	s.SendSpendingLimitWarning(context.Background(), []string{}, "Pro", "$50", "$100", "80%")
	s.SendOverageAlert(context.Background(), []string{}, "Pro", "$10", "$50")
	s.SendPaymentFailed(context.Background(), []string{}, "Pro", time.Now())
	s.SendPlanChanged(context.Background(), []string{}, "Pro", "Scale")
	s.SendEnterpriseContractReminder(context.Background(), []string{}, "2026-12-31", false, 30)
	s.SendDowngradeHTTPJobsWarning(context.Background(), []string{}, "2026-05-01", 3)
	s.SendContractExpired(context.Background(), []string{}, "2026-12-31")
	s.SendTrialEndingSoon(context.Background(), []string{}, "2026-12-31", 7)
	s.SendDisputeAlert(context.Background(), []string{}, "$25.00")
	s.SendInvoiceUpcoming(context.Background(), []string{}, "$100.00", "2026-05-01")
	s.SendDunningStep(context.Background(), []string{}, "Pro", 1)

	assert.Empty(t, client.calls)
}

func TestBillingEmailSender_SendsExpectedTemplateIntents(t *testing.T) {
	t.Parallel()

	graceEnd := time.Date(2026, time.April, 15, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name         string
		send         func(*BillingEmailSender)
		wantTemplate string
		wantProps    map[string]any
	}{
		{
			name: "spending limit warning",
			send: func(sender *BillingEmailSender) {
				sender.SendSpendingLimitWarning(context.Background(), []string{"admin@example.com"}, "Pro", "$80.00", "$100.00", "80%")
			},
			wantTemplate: "billing.spending_limit_warning",
			wantProps: map[string]any{
				"planName":      "Pro",
				"currentSpend":  "$80.00",
				"spendingLimit": "$100.00",
				"percentUsed":   "80%",
			},
		},
		{
			name: "overage alert",
			send: func(sender *BillingEmailSender) {
				sender.SendOverageAlert(context.Background(), []string{"admin@example.com"}, "Scale", "$25.00", "100000")
			},
			wantTemplate: "billing.overage_alert",
			wantProps: map[string]any{
				"planName":          "Scale",
				"overageAmount":     "$25.00",
				"includedAllowance": "100000",
			},
		},
		{
			name: "payment failed",
			send: func(sender *BillingEmailSender) {
				sender.SendPaymentFailed(context.Background(), []string{"admin@example.com"}, "Starter", graceEnd)
			},
			wantTemplate: "billing.payment_failed",
			wantProps: map[string]any{
				"planName":       "Starter",
				"gracePeriodEnd": "April 15, 2026",
			},
		},
		{
			name: "contract reminder",
			send: func(sender *BillingEmailSender) {
				sender.SendEnterpriseContractReminder(context.Background(), []string{"admin@example.com"}, "2026-12-31", true, 30)
			},
			wantTemplate: "billing.enterprise_contract_reminder",
			wantProps: map[string]any{
				"contractEndDate": "2026-12-31",
				"autoRenew":       true,
				"daysRemaining":   30,
			},
		},
		{
			name: "dunning step",
			send: func(sender *BillingEmailSender) {
				sender.SendDunningStep(context.Background(), []string{"admin@example.com"}, "Business", 4)
			},
			wantTemplate: "billing.dunning_step",
			wantProps: map[string]any{
				"planName": "Business",
				"step":     4,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			client := &mockBillingTransactionalClient{}
			sender := NewBillingEmailSender(client, "billing@example.com", slog.New(slog.DiscardHandler))

			tc.send(sender)

			require.Len(t, client.calls, 1)
			got := client.calls[0]
			assert.Equal(t, "billing@example.com", got.From)
			assert.Equal(t, []string{"admin@example.com"}, got.To)
			assert.Equal(t, tc.wantTemplate, string(got.Template))
			assert.NotEmpty(t, got.IdempotencyKey)
			props := transactionalPropsMap(t, got.Props)
			for key, want := range tc.wantProps {
				assert.EqualValues(t, want, props[key])
			}
		})
	}
}

func TestBillingEmailSender_SendHandlesTransportEdges(t *testing.T) {
	t.Parallel()

	noTransport := &BillingEmailSender{
		fromEmail: "billing@example.com",
		logger:    slog.New(slog.DiscardHandler),
	}
	assert.NotPanics(t, func() {
		noTransport.SendDisputeAlert(context.Background(), []string{"admin@example.com"}, "$25.00")
	})

	failing := NewBillingEmailSender(&mockBillingTransactionalClient{
		sendFn: func(context.Context, transactional.Request) error {
			return errors.New("app unavailable")
		},
	}, "billing@example.com", nil)
	failing.logger = nil
	assert.NotPanics(t, func() {
		failing.SendInvoiceUpcoming(context.Background(), []string{"admin@example.com"}, "$125.00", "2026-07-01")
	})
}
