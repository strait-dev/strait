package api

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"strait/internal/domain"
)

func memberRoleFixture() *domain.ProjectMemberRole {
	return &domain.ProjectMemberRole{
		ProjectID: "proj-1",
		UserID:    "user-1",
		RoleID:    "role-1",
	}
}

func TestAssignMemberRoleWithBillingLimit_CloudNilEnforcerFailsClosed(t *testing.T) {
	t.Parallel()

	assignCalls := atomic.Int64{}
	srv := &Server{
		edition: domain.EditionCloud,
		store: &APIStoreMock{
			AssignMemberRoleFunc: func(context.Context, *domain.ProjectMemberRole) error {
				assignCalls.Add(1)
				return nil
			},
		},
	}

	err := srv.assignMemberRoleWithBillingLimit(context.Background(), memberRoleFixture())
	if err == nil || !strings.Contains(err.Error(), "billing enforcement unavailable") {
		t.Fatalf("expected billing enforcement unavailable, got %v", err)
	}
	if got := assignCalls.Load(); got != 0 {
		t.Fatalf("AssignMemberRole calls = %d, want 0", got)
	}
}

func TestAssignMemberRoleWithBillingLimit_CommunityNilEnforcerAllows(t *testing.T) {
	t.Parallel()

	assignCalls := atomic.Int64{}
	srv := &Server{
		edition: domain.EditionCommunity,
		store: &APIStoreMock{
			AssignMemberRoleFunc: func(context.Context, *domain.ProjectMemberRole) error {
				assignCalls.Add(1)
				return nil
			},
		},
	}

	if err := srv.assignMemberRoleWithBillingLimit(context.Background(), memberRoleFixture()); err != nil {
		t.Fatalf("expected community nil enforcer to allow assignment, got %v", err)
	}
	if got := assignCalls.Load(); got != 1 {
		t.Fatalf("AssignMemberRole calls = %d, want 1", got)
	}
}

func TestAssignMemberRoleWithBillingLimit_OrgLookupErrorFailsClosed(t *testing.T) {
	t.Parallel()

	assignCalls := atomic.Int64{}
	srv := &Server{
		edition:         domain.EditionCloud,
		billingEnforcer: &tunableLimitsEnforcer{orgErr: errors.New("billing store down")},
		store: &APIStoreMock{
			AssignMemberRoleFunc: func(context.Context, *domain.ProjectMemberRole) error {
				assignCalls.Add(1)
				return nil
			},
		},
	}

	err := srv.assignMemberRoleWithBillingLimit(context.Background(), memberRoleFixture())
	if err == nil || !strings.Contains(err.Error(), "billing enforcement unavailable") {
		t.Fatalf("expected billing enforcement unavailable, got %v", err)
	}
	if got := assignCalls.Load(); got != 0 {
		t.Fatalf("AssignMemberRole calls = %d, want 0", got)
	}
}

type emptyOrgMemberLimitEnforcer struct {
	mockBillingEnforcer
}

func (e *emptyOrgMemberLimitEnforcer) GetActiveProjectOrgID(context.Context, string) (string, error) {
	return "", nil
}

func TestAssignMemberRoleWithBillingLimit_EmptyOrgFailsClosed(t *testing.T) {
	t.Parallel()

	assignCalls := atomic.Int64{}
	srv := &Server{
		edition:         domain.EditionCloud,
		billingEnforcer: &emptyOrgMemberLimitEnforcer{},
		store: &APIStoreMock{
			AssignMemberRoleFunc: func(context.Context, *domain.ProjectMemberRole) error {
				assignCalls.Add(1)
				return nil
			},
		},
	}

	err := srv.assignMemberRoleWithBillingLimit(context.Background(), memberRoleFixture())
	if err == nil || !strings.Contains(err.Error(), "billing enforcement unavailable") {
		t.Fatalf("expected billing enforcement unavailable, got %v", err)
	}
	if got := assignCalls.Load(); got != 0 {
		t.Fatalf("AssignMemberRole calls = %d, want 0", got)
	}
}

func TestAssignMemberRoleWithBillingLimit_PlanLookupErrorFailsClosed(t *testing.T) {
	t.Parallel()

	assignCalls := atomic.Int64{}
	srv := &Server{
		edition: domain.EditionCloud,
		billingEnforcer: &tunableLimitsEnforcer{
			limitsErr: errors.New("plan lookup failed"),
		},
		store: &APIStoreMock{
			AssignMemberRoleFunc: func(context.Context, *domain.ProjectMemberRole) error {
				assignCalls.Add(1)
				return nil
			},
		},
	}

	err := srv.assignMemberRoleWithBillingLimit(context.Background(), memberRoleFixture())
	if err == nil || !strings.Contains(err.Error(), "billing enforcement unavailable") {
		t.Fatalf("expected billing enforcement unavailable, got %v", err)
	}
	if got := assignCalls.Load(); got != 0 {
		t.Fatalf("AssignMemberRole calls = %d, want 0", got)
	}
}
