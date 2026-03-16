# frozen_string_literal: true

module Strait
  module Operations
    # Service for role-based access control: members, roles, resource policies, tag policies, and audit events.
    class RBACService < BaseService
      # List audit events.
      # GET /v1/rbac/audit-events
      def list_audit_events(query: nil)
        _request(:get, "/v1/rbac/audit-events", query: query)
      end

      # List members.
      # GET /v1/rbac/members
      def list_members(query: nil)
        _request(:get, "/v1/rbac/members", query: query)
      end

      # Create a member.
      # POST /v1/rbac/members
      def create_member(body)
        _request(:post, "/v1/rbac/members", body: body)
      end

      # Delete a member.
      # DELETE /v1/rbac/members/{userID}
      def delete_member(user_id)
        _request(:delete, "/v1/rbac/members/{userID}", path_params: { "userID" => user_id })
        nil
      end

      # Bulk member operations.
      # POST /v1/rbac/members/bulk
      def bulk_member(body)
        _request(:post, "/v1/rbac/members/bulk", body: body)
      end

      # List roles.
      # GET /v1/rbac/roles
      def list_roles(query: nil)
        _request(:get, "/v1/rbac/roles", query: query)
      end

      # Create a role.
      # POST /v1/rbac/roles
      def create_role(body)
        _request(:post, "/v1/rbac/roles", body: body)
      end

      # Get a role by ID.
      # GET /v1/rbac/roles/{roleID}
      def get_role(role_id)
        _request(:get, "/v1/rbac/roles/{roleID}", path_params: { "roleID" => role_id })
      end

      # Update a role.
      # PATCH /v1/rbac/roles/{roleID}
      def update_role(role_id, body)
        _request(:patch, "/v1/rbac/roles/{roleID}", path_params: { "roleID" => role_id }, body: body)
      end

      # Delete a role.
      # DELETE /v1/rbac/roles/{roleID}
      def delete_role(role_id)
        _request(:delete, "/v1/rbac/roles/{roleID}", path_params: { "roleID" => role_id })
        nil
      end

      # List resource policies.
      # GET /v1/rbac/resource-policies
      def list_resource_policies(query: nil)
        _request(:get, "/v1/rbac/resource-policies", query: query)
      end

      # Create a resource policy.
      # POST /v1/rbac/resource-policies
      def create_resource_policy(body)
        _request(:post, "/v1/rbac/resource-policies", body: body)
      end

      # Delete a resource policy.
      # DELETE /v1/rbac/resource-policies/{policyID}
      def delete_resource_policy(policy_id)
        _request(:delete, "/v1/rbac/resource-policies/{policyID}", path_params: { "policyID" => policy_id })
        nil
      end

      # List tag policies.
      # GET /v1/rbac/tag-policies
      def list_tag_policies(query: nil)
        _request(:get, "/v1/rbac/tag-policies", query: query)
      end

      # Create a tag policy.
      # POST /v1/rbac/tag-policies
      def create_tag_policy(body)
        _request(:post, "/v1/rbac/tag-policies", body: body)
      end

      # Delete a tag policy.
      # DELETE /v1/rbac/tag-policies/{policyID}
      def delete_tag_policy(policy_id)
        _request(:delete, "/v1/rbac/tag-policies/{policyID}", path_params: { "policyID" => policy_id })
        nil
      end

      # Seed default roles.
      # POST /v1/rbac/seed
      def seed_roles
        _request(:post, "/v1/rbac/seed")
      end
    end
  end
end
