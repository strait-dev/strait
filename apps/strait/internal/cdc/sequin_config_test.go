package cdc

import (
	"os"
	"strings"
	"testing"
)

func TestSequinConfigCoversCacheRepairTables(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile("../../../../packages/configs/sequin.yml")
	if err != nil {
		t.Fatalf("read sequin config: %v", err)
	}
	config := string(raw)
	for _, table := range []string{
		"api_keys",
		"project_roles",
		"project_member_roles",
		"resource_policies",
		"tag_policies",
		"project_quotas",
		"organization_subscriptions",
		"jobs",
		"job_dependencies",
		"job_runs",
		"workflow_runs",
		"workflow_step_runs",
	} {
		if !strings.Contains(config, `table_name: "`+table+`"`) {
			t.Fatalf("sequin config missing table %s", table)
		}
	}
}

func TestPostgresCDCInitSetsReplicaIdentityForCacheRepairTables(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile("../../../../packages/configs/postgres-init.sql")
	if err != nil {
		t.Fatalf("read postgres init config: %v", err)
	}
	config := string(raw)
	for _, table := range []string{
		"api_keys",
		"project_roles",
		"project_member_roles",
		"resource_policies",
		"tag_policies",
		"project_quotas",
		"organization_subscriptions",
		"jobs",
		"job_dependencies",
		"job_runs",
		"workflow_runs",
		"workflow_step_runs",
	} {
		if !strings.Contains(config, "ALTER TABLE public."+table+" REPLICA IDENTITY FULL") {
			t.Fatalf("postgres init missing replica identity for %s", table)
		}
	}
}
