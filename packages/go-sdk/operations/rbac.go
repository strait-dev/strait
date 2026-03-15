package operations

import "context"

// RBACService provides role-based access control operations.
type RBACService struct{ r Requester }

func NewRBACService(r Requester) *RBACService { return &RBACService{r: r} }

func (s *RBACService) ListAuditEvents(ctx context.Context, query map[string]string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "GET", "/v1/audit-events", query, nil, nil, &result)
	return result, err
}

func (s *RBACService) ListMembers(ctx context.Context, query map[string]string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "GET", "/v1/members", query, nil, nil, &result)
	return result, err
}

func (s *RBACService) CreateMember(ctx context.Context, body any) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", "/v1/members", nil, nil, body, &result)
	return result, err
}

func (s *RBACService) DeleteMember(ctx context.Context, userID string) error {
	return s.r.DoRequest(ctx, "DELETE", PathParams("/v1/members/{userID}", map[string]string{"userID": userID}), nil, nil, nil, nil)
}

func (s *RBACService) BulkMember(ctx context.Context, body any) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", "/v1/members/bulk", nil, nil, body, &result)
	return result, err
}

func (s *RBACService) ListRoles(ctx context.Context, query map[string]string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "GET", "/v1/roles", query, nil, nil, &result)
	return result, err
}

func (s *RBACService) CreateRole(ctx context.Context, body any) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", "/v1/roles", nil, nil, body, &result)
	return result, err
}

func (s *RBACService) GetRole(ctx context.Context, roleID string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "GET", PathParams("/v1/roles/{roleID}", map[string]string{"roleID": roleID}), nil, nil, nil, &result)
	return result, err
}

func (s *RBACService) UpdateRole(ctx context.Context, roleID string, body any) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "PATCH", PathParams("/v1/roles/{roleID}", map[string]string{"roleID": roleID}), nil, nil, body, &result)
	return result, err
}

func (s *RBACService) DeleteRole(ctx context.Context, roleID string) error {
	return s.r.DoRequest(ctx, "DELETE", PathParams("/v1/roles/{roleID}", map[string]string{"roleID": roleID}), nil, nil, nil, nil)
}

func (s *RBACService) ListResourcePolicies(ctx context.Context, query map[string]string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "GET", "/v1/resource-policies", query, nil, nil, &result)
	return result, err
}

func (s *RBACService) CreateResourcePolicy(ctx context.Context, body any) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", "/v1/resource-policies", nil, nil, body, &result)
	return result, err
}

func (s *RBACService) DeleteResourcePolicy(ctx context.Context, policyID string) error {
	return s.r.DoRequest(ctx, "DELETE", PathParams("/v1/resource-policies/{policyID}", map[string]string{"policyID": policyID}), nil, nil, nil, nil)
}

func (s *RBACService) ListTagPolicies(ctx context.Context, query map[string]string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "GET", "/v1/tag-policies", query, nil, nil, &result)
	return result, err
}

func (s *RBACService) CreateTagPolicy(ctx context.Context, body any) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", "/v1/tag-policies", nil, nil, body, &result)
	return result, err
}

func (s *RBACService) DeleteTagPolicy(ctx context.Context, policyID string) error {
	return s.r.DoRequest(ctx, "DELETE", PathParams("/v1/tag-policies/{policyID}", map[string]string{"policyID": policyID}), nil, nil, nil, nil)
}

func (s *RBACService) SeedRoles(ctx context.Context) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", "/v1/seed-roles", nil, nil, nil, &result)
	return result, err
}
