package billing

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"strait/internal/domain"
)

type launchPromiseStatus string

const (
	launchPromiseRuntime launchPromiseStatus = "runtime"
	launchPromiseMetered launchPromiseStatus = "metered"
	launchPromiseDisplay launchPromiseStatus = "display"
	launchPromiseRoadmap launchPromiseStatus = "roadmap"
)

type launchPromiseEvidence struct {
	promise     string
	status      launchPromiseStatus
	gate        string
	test        string
	feature     Feature
	roadmapGate Feature
}

var launchEnforcementMatrix = []launchPromiseEvidence{
	{promise: "HTTP execution mode", status: launchPromiseRuntime, gate: "checkHTTPModeAllowed", test: "TestCheckHTTPModeAllowed_FreePlanAllowed", feature: FeatureHTTPMode},
	{promise: "monthly run allowance", status: launchPromiseRuntime, gate: "Enforcer.CheckMonthlyRunLimit", test: "TestCheckMonthlyRunLimit_PaidOverageDisabledHardCaps"},
	{promise: "legacy daily run override remains inert", status: launchPromiseRuntime, gate: "Enforcer.GetOrgPlanLimits", test: "TestReaderSwitch_LegacyDailyOverrideIgnoredForLaunch"},
	{promise: "Free overage requires payment method", status: launchPromiseRuntime, gate: "UsageService.SetOverageEnabled", test: "TestUsageService_SetOverageEnabled_FreeRequiresPaymentMethod"},
	{promise: "paid overage can be disabled", status: launchPromiseRuntime, gate: "Enforcer.CheckMonthlyRunLimit", test: "TestCheckMonthlyRunLimit_PaidOverageDisabledHardCaps"},
	{promise: "spending cap blocks and pauses schedules", status: launchPromiseRuntime, gate: "Enforcer.CheckSpendingLimit", test: "TestCheckSpendingLimit_DispatchesCapReachedAndOverageDisabled"},
	{promise: "spending cap raise resumes quota-paused jobs", status: launchPromiseRuntime, gate: "UsageService.SetSpendingLimit", test: "TestUsageService_SetSpendingLimit_RaisedAboveCurrentSpendResumesQuotaPausedJobs"},
	{promise: "overage re-enable resumes quota-paused jobs", status: launchPromiseRuntime, gate: "UsageService.SetOverageEnabled", test: "TestUsageService_SetOverageEnabled_EnableResumesQuotaPausedJobs"},
	{promise: "billing period rollover resumes quota-paused jobs", status: launchPromiseRuntime, gate: "scheduler.QuotaResumeEnforcer", test: "TestQuotaResumeEnforcer_UsesBillingBoundaryForUnpause"},
	{promise: "concurrent run cap", status: launchPromiseRuntime, gate: "Enforcer.CheckConcurrentRunLimit", test: "TestEnforcer_CheckConcurrentRunLimit"},
	{promise: "worker connection cap", status: launchPromiseRuntime, gate: "gRPC worker registration reservation gate", test: "TestCheckPlanConnectionLimit_UsesDistributedReservation"},
	{promise: "workflow step cap", status: launchPromiseRuntime, gate: "registry workflow registration gate", test: "TestCheckWorkflowStepLimit_TierBoundaries"},
	{promise: "cron schedule count cap", status: launchPromiseRuntime, gate: "scheduler admission gate", test: "TestEnforceCronScheduleLimit_SerializesJobsAndWorkflows"},
	{promise: "cron minimum interval cap", status: launchPromiseRuntime, gate: "scheduler cron validator", test: "TestCheckCronMinInterval_FreeRejectsEveryMinute"},
	{promise: "cron overlap policies Starter+", status: launchPromiseRuntime, gate: "checkCronOverlapPolicy", test: "TestPlanGate_DispatchesWorkflowRegistrationRejected_CronOverlapPolicy", feature: FeatureAllCronOverlap},
	{promise: "project cap", status: launchPromiseRuntime, gate: "project creation billing admission", test: "TestHandleCreateProject_ProjectLimitExceeded_Adversarial"},
	{promise: "member cap", status: launchPromiseRuntime, gate: "org-limited member assignment", test: "TestAssignMemberRoleWithOrgLimit_SerializesConcurrentNewMembers"},
	{promise: "webhook endpoint cap", status: launchPromiseRuntime, gate: "webhook endpoint admission", test: "TestCreateWebhookSubscriptionWithOrgLimit_ConcurrentCreatesCannotExceedLimit"},
	{promise: "environment cap", status: launchPromiseRuntime, gate: "environment admission", test: "TestCreateEnvironmentWithOrgLimit_SerializesConcurrentCreates"},
	{promise: "history retention cap", status: launchPromiseRuntime, gate: "per-org retention reaper", test: "TestReaper_OrgRetention_PrunesRunsByOrg"},
	{promise: "API rate limit", status: launchPromiseRuntime, gate: "ratelimit middleware", test: "TestResolveRateLimit_UsesPlanLimitBeforeGlobalDefault"},
	{promise: "concurrency add-on +100", status: launchPromiseRuntime, gate: "EffectiveLimits AddonConcurrency100", test: "TestEffectiveLimits_Concurrency100Pack"},
	{promise: "extended history add-on +30d with 365-day cap", status: launchPromiseRuntime, gate: "EffectiveLimits AddonHistory30d", test: "TestEffectiveLimits_History30dClampedToCatalogMaxTotal"},
	{promise: "additional environments add-on +5", status: launchPromiseRuntime, gate: "EffectiveLimits AddonEnvironments5", test: "TestEffectiveLimits_Environments5Pack"},
	{promise: "RBAC level", status: launchPromiseRuntime, gate: "RBACLevel plan limit", test: "TestHandleCreateRole_StarterBasicRBACRejectsCustomRole", feature: FeatureRBAC},
	{promise: "workflow policies Advanced RBAC", status: launchPromiseRuntime, gate: "RBACLevel plan limit", test: "TestHandleGetWorkflowPolicy_ProFullRBACRejectsAdvancedPolicy"},
	{promise: "audit logs Scale+", status: launchPromiseRuntime, gate: "FeatureAuditLogs", test: "TestAuditLogs_FreeTierRejected", feature: FeatureAuditLogs},
	{promise: "canary deployments Scale+", status: launchPromiseRuntime, gate: "FeatureCanaryDeployments", test: "TestCanaryDeploymentUpdate_FreeTierRejected", feature: FeatureCanaryDeployments},
	{promise: "canary status and rollback Scale+", status: launchPromiseRuntime, gate: "FeatureCanaryDeployments", test: "TestCanaryDeploymentStatus_FreeTierRejected"},
	{promise: "approval gates Pro+", status: launchPromiseRuntime, gate: "FeatureApprovalGates", test: "TestFeatureGating_ApprovalGates", feature: FeatureApprovalGates},
	{promise: "approval analytics Pro+", status: launchPromiseRuntime, gate: "FeatureApprovalGates", test: "TestHandleGetApprovalStats_FreeTierRejected"},
	{promise: "sub-workflows Pro+", status: launchPromiseRuntime, gate: "FeatureSubWorkflows", test: "TestFeatureGating_SubWorkflows", feature: FeatureSubWorkflows},
	{promise: "job chaining Pro+", status: launchPromiseRuntime, gate: "FeatureJobChaining", test: "TestFeatureGating_JobChaining", feature: FeatureJobChaining},
	{promise: "compensating transactions Pro+", status: launchPromiseRuntime, gate: "FeatureCompensatingTxns", test: "TestFeatureGating_CompensatingTxns", feature: FeatureCompensatingTxns},
	{promise: "compensation plan Pro+", status: launchPromiseRuntime, gate: "FeatureCompensatingTxns", test: "TestCompensationPlan_FreeTierRejected"},
	{promise: "log streaming Starter+", status: launchPromiseRuntime, gate: "FeatureLogStreaming", test: "TestRunLogStream_FreeTier_Rejected", feature: FeatureLogStreaming},
	{promise: "log drain count cap", status: launchPromiseRuntime, gate: "log drain admission gate", test: "TestCreateLogDrain_FreeTier_RejectsZeroCap"},
	{promise: "notification channel count cap", status: launchPromiseRuntime, gate: "notification channel admission gate", test: "TestCreateNotificationChannel_FreeTier_RejectsZeroCap"},
	{promise: "anomaly alert notification delivery", status: launchPromiseRuntime, gate: "scheduler.AnomalyMonitor", test: "TestAnomalyMonitor_NotificationDeliveryCreated"},
	{promise: "Redis required runtime dependency", status: launchPromiseRuntime, gate: "critical Redis health checker", test: "TestNewRedisChecker"},
	{promise: "Redis strong API cache runtime wiring", status: launchPromiseRuntime, gate: "cache registry namespaces", test: "TestAPIStrongCacheConstructorsRegisterRuntimeNamespaces"},
	{promise: "Redis strong worker cache runtime wiring", status: launchPromiseRuntime, gate: "worker job cache registry namespace", test: "TestWorkerStrongCacheConstructorRegistersRuntimeNamespace"},
	{promise: "Redis strong billing cache runtime wiring", status: launchPromiseRuntime, gate: "billing org limit cache registry namespace", test: "TestNewEnforcer_RegistersStrongCacheNamespace"},
	{promise: "Sequin required runtime dependency", status: launchPromiseRuntime, gate: "critical Sequin health checker", test: "TestNewSequinChecker"},
	{promise: "Sequin CDC consumer table coverage", status: launchPromiseRuntime, gate: "cdc.RequiredConsumerTables", test: "TestSequinConfigCoversRequiredConsumerTables"},
	{promise: "Postgres CDC replica identity coverage", status: launchPromiseRuntime, gate: "postgres-init CDC replica identity", test: "TestPostgresCDCInitSetsReplicaIdentityForRequiredConsumerTables"},
	{promise: "SLA target flag", status: launchPromiseDisplay, gate: "FeatureSLA", test: "TestNonEnterpriseTiers_NoSLA", feature: FeatureSLA},
	{promise: "overage metering to Stripe", status: launchPromiseMetered, gate: "worker recordTerminalRunBilling", test: "TestBillingEnforcement_TerminalFailureRecordsBillableRunCost"},
	{promise: "roadmap add-ons are not sellable", status: launchPromiseRuntime, gate: "CatalogResolver launch-active add-on filter", test: "TestCatalogResolver_RoadmapAddonLookupKeysUnmapped"},
	{promise: "roadmap add-on webhooks cannot activate entitlements", status: launchPromiseRuntime, gate: "WebhookHandler.handleAddonSubscriptionCreated", test: "TestWebhookHandler_LegacyRoadmapAddonPriceDoesNotCreateEntitlement"},
	{promise: "retired model/key names absent from final schema", status: launchPromiseRuntime, gate: "migration final-schema policy", test: "TestFinalSchemaDoesNotRetainRetiredModelOrKeyNames"},
	{promise: "SSO roadmap", status: launchPromiseRoadmap, roadmapGate: FeatureSSO},
	{promise: "SCIM roadmap", status: launchPromiseRoadmap, roadmapGate: FeatureSCIM},
	{promise: "IP allowlisting roadmap", status: launchPromiseRoadmap, roadmapGate: FeatureIPAllowlisting},
	{promise: "static IPs roadmap", status: launchPromiseRoadmap, roadmapGate: FeatureStaticIPs},
	{promise: "VPC peering roadmap", status: launchPromiseRoadmap, roadmapGate: FeatureVPCPeering},
	{promise: "data residency roadmap", status: launchPromiseRoadmap, roadmapGate: FeatureDataResidency},
	{promise: "custom RBAC roadmap", status: launchPromiseRoadmap, roadmapGate: FeatureCustomRBAC},
	{promise: "dedicated compute roadmap", status: launchPromiseRoadmap, roadmapGate: FeatureDedicatedCompute},
	{promise: "priority queue roadmap", status: launchPromiseRoadmap, roadmapGate: FeaturePriorityQueue},
	{promise: "session management roadmap", status: launchPromiseRoadmap, roadmapGate: FeatureSessionManagement},
	{promise: "secret rotation roadmap", status: launchPromiseRoadmap, roadmapGate: FeatureSecretRotation},
	{promise: "SIEM export roadmap", status: launchPromiseRoadmap, roadmapGate: FeatureSIEMExport},
}

func TestLaunchEnforcementMatrixHasEvidenceForActivePromises(t *testing.T) {
	t.Parallel()

	seen := map[string]bool{}
	for _, row := range launchEnforcementMatrix {
		if row.promise == "" {
			t.Fatal("launch enforcement matrix contains an unnamed promise")
		}
		if seen[row.promise] {
			t.Fatalf("duplicate launch enforcement promise %q", row.promise)
		}
		seen[row.promise] = true

		switch row.status {
		case launchPromiseRuntime, launchPromiseMetered, launchPromiseDisplay:
			if strings.TrimSpace(row.gate) == "" {
				t.Fatalf("%q is %s but has no gate evidence", row.promise, row.status)
			}
			if !strings.HasPrefix(row.test, "Test") {
				t.Fatalf("%q is %s but has no test evidence: %q", row.promise, row.status, row.test)
			}
		case launchPromiseRoadmap:
			if row.roadmapGate == "" {
				t.Fatalf("%q is roadmap but has no roadmap feature gate", row.promise)
			}
			if !IsRoadmapFeature(row.roadmapGate) {
				t.Fatalf("%q roadmap gate %q is not registered as roadmap", row.promise, row.roadmapGate)
			}
		default:
			t.Fatalf("%q has unknown launch status %q", row.promise, row.status)
		}
	}
}

func TestLaunchEnforcementMatrixRoadmapFeaturesStayInactive(t *testing.T) {
	t.Parallel()

	registry := NewStaticRegistry()
	for _, row := range launchEnforcementMatrix {
		if row.status != launchPromiseRoadmap {
			continue
		}
		for _, tier := range domain.AllPlanTiers() {
			if registry.AllowsFeature(tier, row.roadmapGate) {
				t.Fatalf("%s enables roadmap feature %q for %s", row.promise, row.roadmapGate, tier)
			}
		}
	}
}

func TestLaunchEnforcementMatrixCoversEveryRoadmapFeature(t *testing.T) {
	t.Parallel()

	covered := map[Feature]bool{}
	for _, row := range launchEnforcementMatrix {
		if row.status == launchPromiseRoadmap {
			covered[row.roadmapGate] = true
		}
	}
	for _, feature := range allRegistryFeatures() {
		if IsRoadmapFeature(feature) && !covered[feature] {
			t.Fatalf("roadmap feature %q is missing launch matrix coverage", feature)
		}
	}
}

func TestLaunchEnforcementMatrixCoversEveryLaunchActiveFeature(t *testing.T) {
	t.Parallel()

	registry := NewStaticRegistry()
	covered := map[Feature]bool{}
	for _, row := range launchEnforcementMatrix {
		if row.feature != "" {
			covered[row.feature] = true
		}
	}

	for _, feature := range allRegistryFeatures() {
		if IsRoadmapFeature(feature) {
			continue
		}
		active := false
		for _, tier := range domain.AllPlanTiers() {
			if registry.AllowsFeature(tier, feature) {
				active = true
				break
			}
		}
		if active && !covered[feature] {
			t.Fatalf("launch-active feature %q is missing launch matrix evidence", feature)
		}
	}
}

func TestLaunchEnforcementMatrixEvidenceTestsExist(t *testing.T) {
	t.Parallel()

	testNames := collectRepoTestNames(t)
	for _, row := range launchEnforcementMatrix {
		if row.status == launchPromiseRoadmap {
			continue
		}
		if !testNames[row.test] {
			t.Fatalf("%q cites missing evidence test %q", row.promise, row.test)
		}
	}
}

func allRegistryFeatures() []Feature {
	return []Feature{
		FeatureHTTPMode,
		FeatureApprovalGates,
		FeatureSubWorkflows,
		FeatureJobChaining,
		FeatureCompensatingTxns,
		FeatureCanaryDeployments,
		FeatureAuditLogs,
		FeatureSSO,
		FeatureSLA,
		FeatureRBAC,
		FeatureAllCronOverlap,
		FeatureDedicatedCompute,
		FeatureStaticIPs,
		FeatureVPCPeering,
		FeatureSCIM,
		FeatureDataResidency,
		FeatureCustomRBAC,
		FeaturePriorityQueue,
		FeatureIPAllowlisting,
		FeatureSessionManagement,
		FeatureSecretRotation,
		FeatureSIEMExport,
		FeatureLogStreaming,
	}
}

func TestLaunchPricingDoesNotWireLegacyDailyRunQuota(t *testing.T) {
	t.Parallel()

	forbidden := []string{
		"CheckDailyRunLimit(",
		"DecrDailyRunCount(",
		"GetDailyRunCount(",
		"ReconcileDailyRunCounts(",
		"WithDailyRunCounter(",
		"DailyRunCounter",
	}
	scanRoots := []string{
		"../api",
		"../queue",
		"../scheduler",
		"../worker",
		"../../cmd",
	}

	for _, root := range scanRoots {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}

			body, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			for _, token := range forbidden {
				if strings.Contains(string(body), token) {
					t.Fatalf("%s wires legacy daily run quota token %q; launch billing must use monthly run allowance", path, token)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("scan %s for legacy daily run quota wiring: %v", root, err)
		}
	}
}

func TestLaunchDocsDoNotAdvertisePlanGatedRBACAsUniversal(t *testing.T) {
	t.Parallel()

	for _, path := range []string{
		"../../../docs/guides/security.mdx",
		"../../../docs/guides/authentication.mdx",
	} {
		bodyBytes, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		body := string(bodyBytes)
		for _, phrase := range []string{
			"you can create custom roles per-project",
			"You can also create custom roles with any combination of scopes",
			"RBAC also supports role inheritance and policy-based grants",
		} {
			if strings.Contains(body, phrase) {
				t.Fatalf("%s advertises plan-gated RBAC as universal with phrase %q", path, phrase)
			}
		}
	}
}

func TestLaunchDocsDoNotAdvertiseRegionRoutingAsLaunchActive(t *testing.T) {
	t.Parallel()

	for _, path := range []string{
		"../../../docs/faq.mdx",
		"../../../docs/glossary.mdx",
		"../../../docs/billing/faq.mdx",
		"../../../docs/billing/pricing.mdx",
		"../../../docs/sdks/ruby.mdx",
	} {
		bodyBytes, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		body := string(bodyBytes)
		for _, phrase := range []string{
			"multi-region",
			"hosted multi-region orchestration",
			"data residency is included",
			"client.regions",
		} {
			if strings.Contains(body, phrase) {
				t.Fatalf("%s advertises region routing/residency as launch-active with phrase %q", path, phrase)
			}
		}
	}
}

func TestLaunchCopyDoesNotAdvertiseHTTPModeAsPaidUpgrade(t *testing.T) {
	t.Parallel()

	for _, root := range []string{
		"../api",
		"../worker",
		"../../../docs",
		"../../../app/src",
	} {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !isPublicCopyFile(path) || strings.Contains(path, "__tests__") {
				return nil
			}

			bodyBytes, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			body := string(bodyBytes)
			for _, phrase := range []string{
				"HTTP execution mode requires the Pro plan",
				"HTTP mode requires the Pro plan",
				"HTTP mode requires Pro",
			} {
				if strings.Contains(body, phrase) {
					t.Fatalf("%s advertises HTTP mode as a paid upgrade with phrase %q", path, phrase)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("scan %s for HTTP mode paid-upgrade copy: %v", root, err)
		}
	}
}

func TestLaunchPublicCopyDoesNotAdvertiseRoadmapSecurityAsIncluded(t *testing.T) {
	t.Parallel()

	for _, root := range []string{
		"../../../docs",
		"../../../app/src/components",
		"../../../app/src/hooks",
		"../../../app/src/lib",
		"../../../app/src/routes",
	} {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !isPublicCopyFile(path) || strings.Contains(path, "__tests__") {
				return nil
			}

			bodyBytes, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			body := string(bodyBytes)
			for _, phrase := range []string{
				"SSO coming soon",
				"includes SSO",
				"SSO included",
				"SAML included",
				"SCIM included",
				"IP allowlisting included",
				"static IPs included",
				"VPC peering included",
				"data residency included",
				"single-tenant included",
				"BYO-cloud included",
				"dedicated worker pool included",
				"dedicated compute included",
				"priority queue included",
			} {
				if strings.Contains(body, phrase) {
					t.Fatalf("%s advertises launch-roadmap security/enterprise feature as included with phrase %q", path, phrase)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("scan %s for roadmap feature copy: %v", root, err)
		}
	}
}

func TestLaunchPublicCopyDoesNotAdvertiseRoadmapSecurityAsActive(t *testing.T) {
	t.Parallel()

	for _, root := range []string{
		"../../../docs",
		"../../../app/src/components",
		"../../../app/src/hooks",
		"../../../app/src/lib",
		"../../../app/src/routes",
	} {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !isPublicCopyFile(path) || strings.Contains(path, "__tests__") {
				return nil
			}

			bodyBytes, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			body := string(bodyBytes)
			for _, phrase := range []string{
				"SSO is available",
				"SSO available",
				"SSO enabled",
				"SSO supported",
				"SAML available",
				"SAML enabled",
				"SAML supported",
				"SCIM available",
				"SCIM enabled",
				"SCIM supported",
				"IP allowlisting available",
				"IP allowlisting enabled",
				"IP allowlisting supported",
				"static IPs available",
				"static IPs enabled",
				"static IPs supported",
				"VPC peering available",
				"VPC peering enabled",
				"VPC peering supported",
				"data residency available",
				"data residency enabled",
				"data residency supported",
				"single-tenant orchestration available",
				"single-tenant orchestration enabled",
				"single-tenant orchestration supported",
				"BYO-cloud available",
				"BYO-cloud enabled",
				"BYO-cloud supported",
				"dedicated worker pool available",
				"dedicated worker pool enabled",
				"dedicated worker pool supported",
				"dedicated compute available",
				"dedicated compute enabled",
				"dedicated compute supported",
				"priority queue available",
				"priority queue enabled",
				"priority queue supported",
			} {
				if strings.Contains(body, phrase) {
					t.Fatalf("%s advertises launch-roadmap security/enterprise feature as active with phrase %q", path, phrase)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("scan %s for active roadmap feature copy: %v", root, err)
		}
	}
}

func TestLaunchPublicCopyDoesNotAdvertiseRetiredModelOrKeyFeatures(t *testing.T) {
	t.Parallel()

	for _, root := range []string{
		"../../../docs",
		"../../../app/src/components",
		"../../../app/src/hooks",
		"../../../app/src/lib",
		"../../../app/src/routes",
	} {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !isPublicCopyFile(path) || strings.Contains(path, "__tests__") {
				return nil
			}

			bodyBytes, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			body := string(bodyBytes)
			for _, phrase := range []string{
				strings.Join([]string{"BY", "OK"}, ""),
				strings.Join([]string{"bring", " your", " own", " key"}, ""),
				"OpenAI",
				"Anthropic",
				"LLM provider",
				"model provider",
				"model usage",
				"token usage",
				"prompt tokens",
				"completion tokens",
				strings.Join([]string{"A", "I", " usage"}, ""),
				strings.Join([]string{"A", "I", " cost"}, ""),
			} {
				if strings.Contains(body, phrase) {
					t.Fatalf("%s advertises retired model/key launch feature with phrase %q", path, phrase)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("scan %s for retired model/key copy: %v", root, err)
		}
	}
}

func TestLaunchBillingEmailsDoNotAdvertiseSelfServeTrials(t *testing.T) {
	t.Parallel()

	bodyBytes, err := os.ReadFile("billing_emails.go")
	if err != nil {
		t.Fatalf("read billing email copy: %v", err)
	}
	body := string(bodyBytes)
	for _, phrase := range []string{
		"Your trial",
		"Strait trial",
		"after your trial",
		"Trial ending soon",
	} {
		if strings.Contains(body, phrase) {
			t.Fatalf("billing email copy advertises self-serve trials with phrase %q", phrase)
		}
	}
}

func isPublicCopyFile(path string) bool {
	for _, ext := range []string{".go", ".mdx", ".ts", ".tsx"} {
		if strings.HasSuffix(path, ext) {
			return true
		}
	}
	return false
}

func TestLaunchPricingDoesNotRequireRetiredModelTelemetryInCoreInterfaces(t *testing.T) {
	t.Parallel()

	forbidden := []string{
		"CreateRunUsage(",
		"ListRunUsage(",
		"CreateRunToolCall(",
		"ListRunToolCalls(",
		"SumRunTotalTokens(",
		"CountRunToolCalls(",
	}
	for _, path := range []string{"../api/server.go", "../store/store.go"} {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for _, token := range forbidden {
			if strings.Contains(string(body), token) {
				t.Fatalf("%s requires retired model telemetry token %q; launch API/store contracts must stay orchestration-only", path, token)
			}
		}
	}
}

func TestLaunchPricingDoesNotExportRetiredModelUsageToClickHouse(t *testing.T) {
	t.Parallel()

	forbidden := []string{
		"ClickHouseSubscriberDeps",
		"ListRunUsage(",
		"RunUsageEventRecord",
		"run_usage_events",
		"PromptTokens",
		"CompletionTokens",
	}
	for _, path := range []string{"../../cmd/strait/services.go", "../worker/subscriber_clickhouse.go"} {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for _, token := range forbidden {
			if strings.Contains(string(body), token) {
				t.Fatalf("%s wires retired model usage export token %q; launch ClickHouse subscriber must stay orchestration-only", path, token)
			}
		}
	}
}

func TestLaunchPricingDoesNotReadRetiredModelUsageForBillingUsage(t *testing.T) {
	t.Parallel()

	body, err := os.ReadFile("../billing/pg_store.go")
	if err != nil {
		t.Fatalf("read billing pg store: %v", err)
	}
	for _, token := range []string{
		"FROM run_usage",
		"JOIN run_usage",
		"ru.total_tokens",
		"ru.cost_microusd",
	} {
		if strings.Contains(string(body), token) {
			t.Fatalf("billing usage reads retired model usage token %q; launch billing usage must use orchestration-run records only", token)
		}
	}
}

func TestLaunchPricingDoesNotReadRetiredModelUsageForPostgresCostAnalytics(t *testing.T) {
	t.Parallel()

	body, err := os.ReadFile("../store/cost_analytics.go")
	if err != nil {
		t.Fatalf("read store cost analytics: %v", err)
	}
	for _, token := range []string{
		"run_usage",
		"u.cost_microusd",
		"u.total_tokens",
		"u.model",
		"usage_cost_microusd), 0)",
		"SUM(total_tokens)",
		"CostByModel",
		"ByModel",
		"TotalTokens",
	} {
		if strings.Contains(string(body), token) {
			t.Fatalf("Postgres cost analytics reads retired model usage token %q; launch analytics must use orchestration-run records only", token)
		}
	}
}

func TestLaunchPricingDoesNotReadRetiredModelUsageForPostgresPerformanceAnalytics(t *testing.T) {
	t.Parallel()

	body, err := os.ReadFile("../store/analytics.go")
	if err != nil {
		t.Fatalf("read store analytics: %v", err)
	}
	for _, token := range []string{
		"run_usage",
		"u.cost_microusd",
		"ru.cost_microusd",
		"SUM(ru.cost_microusd)",
		"SUM(u.cost_microusd)",
	} {
		if strings.Contains(string(body), token) {
			t.Fatalf("Postgres performance analytics reads retired model usage token %q; launch analytics must use orchestration-run records only", token)
		}
	}
}

func TestLaunchPricingCostBudgetSumsDoNotReadRetiredModelUsage(t *testing.T) {
	t.Parallel()

	bodyBytes, err := os.ReadFile("../store/runs.go")
	if err != nil {
		t.Fatalf("read store runs: %v", err)
	}
	body := string(bodyBytes)
	for _, fn := range []string{"SumRunCostMicrousd", "SumProjectDailyCostMicrousd"} {
		start := strings.Index(body, "func (q *Queries) "+fn)
		if start < 0 {
			t.Fatalf("store runs missing %s", fn)
		}
		next := strings.Index(body[start+1:], "\nfunc ")
		if next < 0 {
			t.Fatalf("store runs missing function boundary after %s", fn)
		}
		fnBody := body[start : start+1+next]
		for _, token := range []string{"run_usage", "cost_microusd) FROM run_usage", "u.cost_microusd"} {
			if strings.Contains(fnBody, token) {
				t.Fatalf("%s reads retired model usage token %q; launch cost budgets must use billing cost events", fn, token)
			}
		}
		if !strings.Contains(fnBody, "billing_cost_events") {
			t.Fatalf("%s must read billing_cost_events for launch cost budgets", fn)
		}
	}
}

func TestLaunchPricingDoesNotDefineLegacyRunTelemetryCode(t *testing.T) {
	t.Parallel()

	for _, path := range []string{"../store/runs.go", "../domain/types.go"} {
		bodyBytes, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		body := string(bodyBytes)
		for _, token := range []string{
			"RunUsage",
			"RunToolCall",
			"CreateRunUsage",
			"CreateRunUsageForActiveRun",
			"ListRunUsage",
			"CreateRunToolCall",
			"CreateRunToolCallForActiveRun",
			"ListRunToolCalls",
			"SumRunTotalTokens",
			"CountRunToolCalls",
			"pricing_catalog",
			"prompt_tokens",
			"completion_tokens",
			"total_tokens",
		} {
			if strings.Contains(body, token) {
				t.Fatalf("%s defines retired model telemetry token %q; launch code must not expose retired model telemetry", path, token)
			}
		}
	}
}

func TestLaunchPricingDoesNotReadRetiredModelUsageForClickHouseAnalytics(t *testing.T) {
	t.Parallel()

	body, err := os.ReadFile("../clickhouse/analytics.go")
	if err != nil {
		t.Fatalf("read ClickHouse analytics: %v", err)
	}
	for _, token := range []string{
		"run_usage_events",
		"prompt_tokens",
		"completion_tokens",
		"total_tokens",
		"ra.cost_microusd",
		"sum(ru.cost_microusd)",
		"sum(cost_microusd)",
		"cost_microusd + compute_cost_microusd",
		"sum(cost_microusd) AS daily_cost",
		"CostByModel",
		"ByModel",
		"TotalTokens",
	} {
		if strings.Contains(string(body), token) {
			t.Fatalf("ClickHouse analytics reads retired model usage token %q; launch analytics must use orchestration-run records only", token)
		}
	}
}

func TestLaunchPricingDoesNotDefineRetiredModelUsageClickHouseExport(t *testing.T) {
	t.Parallel()

	for _, path := range []string{"../clickhouse/exporter.go", "../clickhouse/schema.go"} {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for _, token := range []string{
			"RunUsageEventRecord",
			"run_usage_events",
			"prompt_tokens",
			"completion_tokens",
			"total_tokens",
			"insertRunUsageEvents",
		} {
			if strings.Contains(string(body), token) {
				t.Fatalf("%s defines retired model usage export token %q; launch ClickHouse export must stay orchestration-only", path, token)
			}
		}
	}
}

func TestLaunchSourceDoesNotExposeRetiredModelOrKeyMarketingTerms(t *testing.T) {
	t.Parallel()

	pattern := strings.Join([]string{
		`\b`, "A", "I", `\b`,
		`|`, "a", "i", `_`,
		`|`, "BY", "OK",
		`|`, strings.Join([]string{"bring", " your", " own", " key"}, ""),
	}, "")
	forbidden := regexp.MustCompile(pattern)
	scanRoots := []string{
		"..",
		"../../cmd",
		"../../../app/src",
		"../../../docs",
		"../../../../packages/billing",
	}
	allowedExt := map[string]bool{
		".go":   true,
		".ts":   true,
		".tsx":  true,
		".mdx":  true,
		".json": true,
		".mjs":  true,
	}

	for _, root := range scanRoots {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				switch d.Name() {
				case "node_modules", ".turbo", "dist", "build":
					return filepath.SkipDir
				default:
					return nil
				}
			}
			if strings.HasSuffix(path, "routeTree.gen.ts") || !allowedExt[filepath.Ext(path)] {
				return nil
			}
			body, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			if match := forbidden.Find(body); len(match) > 0 {
				t.Fatalf("%s exposes retired model/key marketing token %q", path, string(match))
			}
			return nil
		})
		if err != nil {
			t.Fatalf("scan %s for retired model/key launch surfaces: %v", root, err)
		}
	}
}

func collectRepoTestNames(t *testing.T) map[string]bool {
	t.Helper()

	names := map[string]bool{}
	testDecl := regexp.MustCompile(`func\s+(Test[A-Za-z0-9_]+)\s*\(`)
	err := filepath.WalkDir("..", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for _, match := range testDecl.FindAllSubmatch(body, -1) {
			names[string(match[1])] = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("collect repo test names: %v", err)
	}
	return names
}
