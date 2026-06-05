package billing

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"strait/internal/domain"
)

type sourcePricingCatalog struct {
	Version        string        `json:"version"`
	MeteredUnit    string        `json:"meteredUnit"`
	Plans          []sourcePlan  `json:"plans"`
	Addons         []sourceAddon `json:"addons"`
	RoadmapFeature []string      `json:"roadmapFeatures"`
}

type sourcePlan struct {
	Tier               string           `json:"tier"`
	DisplayName        string           `json:"displayName"`
	Prices             sourcePrices     `json:"prices"`
	LookupKeys         sourceLookupKeys `json:"lookupKeys"`
	Overage            sourceOverage    `json:"overage"`
	Limits             sourceLimits     `json:"limits"`
	Features           sourceFeatures   `json:"features"`
	SupportLevel       string           `json:"supportLevel"`
	CreditCardRequired bool             `json:"creditCardRequired"`
	RoadmapFeatures    []string         `json:"roadmapFeatures"`
}

type sourcePrices struct {
	MonthlyCents int `json:"monthlyCents"`
	AnnualCents  int `json:"annualCents"`
}

type sourceLookupKeys struct {
	Monthly string `json:"monthly"`
	Annual  string `json:"annual"`
	Overage string `json:"overage"`
}

type sourceOverage struct {
	MicrousdPer1K              int64 `json:"microusdPer1K"`
	DefaultEnabled             bool  `json:"defaultEnabled"`
	DefaultSpendingCapMicrousd int64 `json:"defaultSpendingCapMicrousd"`
}

type sourceLimits struct {
	Orgs                 int      `json:"orgs"`
	Projects             int      `json:"projects"`
	Members              int      `json:"members"`
	RunsPerMonth         int      `json:"runsPerMonth"`
	ConcurrentRuns       int      `json:"concurrentRuns"`
	RetentionDays        int      `json:"retentionDays"`
	WorkflowSteps        int      `json:"workflowSteps"`
	ScheduledJobs        int      `json:"scheduledJobs"`
	CronMinIntervalSec   int      `json:"cronMinIntervalSec"`
	Environments         int      `json:"environments"`
	WebhookSubscriptions int      `json:"webhookSubscriptions"`
	WebhookEndpoints     int      `json:"webhookEndpoints"`
	LogDrains            int      `json:"logDrains"`
	NotificationChannels int      `json:"notificationChannels"`
	APIRateLimit         int      `json:"apiRateLimit"`
	WorkerConnections    int      `json:"workerConnections"`
	AlertRules           int      `json:"alertRules"`
	AllowedRegions       []string `json:"allowedRegions"`
	LogDrainGB           int      `json:"logDrainGB"`
	DispatchPriority     int      `json:"dispatchPriority"`
}

type sourceFeatures struct {
	RBAC                     bool   `json:"rbac"`
	RBACLevel                string `json:"rbacLevel"`
	AuditLogs                bool   `json:"auditLogs"`
	SLATarget                bool   `json:"slaTarget"`
	LogStreaming             bool   `json:"logStreaming"`
	ApprovalGates            bool   `json:"approvalGates"`
	SubWorkflows             bool   `json:"subWorkflows"`
	JobChaining              bool   `json:"jobChaining"`
	CompensatingTransactions bool   `json:"compensatingTransactions"`
	CanaryDeployments        bool   `json:"canaryDeployments"`
}

type sourceAddon struct {
	Type        string   `json:"type"`
	DisplayName string   `json:"displayName"`
	LookupKey   string   `json:"lookupKey"`
	PackSize    int      `json:"packSize"`
	PriceCents  int      `json:"priceCents"`
	MaxTotal    int      `json:"maxTotal"`
	Status      string   `json:"status"`
	AvailableOn []string `json:"availableOn"`
}

func TestGeneratedCatalogHashMatchesSource(t *testing.T) {
	sourcePath := pricingCatalogSourcePath()
	source, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read pricing catalog source: %v", err)
	}
	sum := sha256.Sum256(source)
	if got := hex.EncodeToString(sum[:]); got != PricingCatalogHash {
		t.Fatalf("pricing catalog hash = %s, want %s; run bun run --cwd packages/billing generate", PricingCatalogHash, got)
	}
}

func TestGeneratedPlanLimitsMatchCatalogSource(t *testing.T) {
	t.Parallel()

	source := loadSourcePricingCatalog(t)
	assertEqual(t, "PricingCatalogVersion", PricingCatalogVersion, source.Version)
	assertEqual(t, "MeteredUnit", MeteredUnit, source.MeteredUnit)
	assertEqual(t, "Plans length", len(Plans), len(source.Plans))
	assertEqual(t, "PlanCatalogs length", len(PlanCatalogs), len(source.Plans))

	for _, sourcePlan := range source.Plans {
		tier := sourcePlanTier(t, sourcePlan.Tier)
		t.Run(sourcePlan.Tier, func(t *testing.T) {
			t.Parallel()

			limits := GetPlanLimits(tier)
			assertEqual(t, "PlanTier", limits.PlanTier, tier)
			assertEqual(t, "DisplayName", limits.DisplayName, sourcePlan.DisplayName)
			assertEqual(t, "PriceMonthlyUsd", limits.PriceMonthlyUsd, generatedPlanPriceCents(sourcePlan.Prices.MonthlyCents))
			assertEqual(t, "PriceAnnualUsd", limits.PriceAnnualUsd, generatedPlanPriceCents(sourcePlan.Prices.AnnualCents))
			assertEqual(t, "MaxOrgsPerUser", limits.MaxOrgsPerUser, sourcePlan.Limits.Orgs)
			assertEqual(t, "MaxProjectsPerOrg", limits.MaxProjectsPerOrg, sourcePlan.Limits.Projects)
			assertEqual(t, "MaxMembersPerOrg", limits.MaxMembersPerOrg, sourcePlan.Limits.Members)
			assertEqual(t, "MaxRunsPerDay", limits.MaxRunsPerDay, int64(-1))
			assertEqual(t, "MaxRunsPerMonth", limits.MaxRunsPerMonth, sourcePlan.Limits.RunsPerMonth)
			assertEqual(t, "OveragePerKMicrousd", limits.OveragePerKMicrousd, sourcePlan.Overage.MicrousdPer1K)
			assertEqual(t, "MaxConcurrentRuns", limits.MaxConcurrentRuns, sourcePlan.Limits.ConcurrentRuns)
			assertEqual(t, "RetentionDays", limits.RetentionDays, sourcePlan.Limits.RetentionDays)
			assertDeepEqual(t, "AllowedRegions", limits.AllowedRegions, sourcePlan.Limits.AllowedRegions)
			assertEqual(t, "MaxAlertRulesPerProj", limits.MaxAlertRulesPerProj, sourcePlan.Limits.AlertRules)
			assertEqual(t, "MaxWebhookSubsPerProj", limits.MaxWebhookSubsPerProj, sourcePlan.Limits.WebhookSubscriptions)
			assertEqual(t, "MaxLogDrainsPerOrg", limits.MaxLogDrainsPerOrg, sourcePlan.Limits.LogDrains)
			assertEqual(t, "MaxNotificationChannels", limits.MaxNotificationChannels, sourcePlan.Limits.NotificationChannels)
			assertEqual(t, "HasRBAC", limits.HasRBAC, sourcePlan.Features.RBAC)
			assertEqual(t, "RBACLevel", limits.RBACLevel, generatedRBACLevel(sourcePlan.Features.RBACLevel))
			assertEqual(t, "HasAuditLogs", limits.HasAuditLogs, sourcePlan.Features.AuditLogs)
			assertEqual(t, "HasSLA", limits.HasSLA, sourcePlan.Features.SLATarget)
			assertEqual(t, "RequiresCreditCard", limits.RequiresCreditCard, sourcePlan.CreditCardRequired)
			assertEqual(t, "AllowsHTTPMode", limits.AllowsHTTPMode, true)
			assertEqual(t, "LogStreamingEnabled", limits.LogStreamingEnabled, sourcePlan.Features.LogStreaming)
			assertEqual(t, "MaxDispatchPriority", limits.MaxDispatchPriority, sourcePlan.Limits.DispatchPriority)
			assertEqual(t, "WorkerConnections", limits.WorkerConnections, sourcePlan.Limits.WorkerConnections)
			assertEqual(t, "SupportLevel", limits.SupportLevel, sourcePlan.SupportLevel)
			assertEqual(t, "MaxWorkflowDAGSteps", limits.MaxWorkflowDAGSteps, sourcePlan.Limits.WorkflowSteps)
			assertEqual(t, "HasApprovalGates", limits.HasApprovalGates, sourcePlan.Features.ApprovalGates)
			assertEqual(t, "HasSubWorkflows", limits.HasSubWorkflows, sourcePlan.Features.SubWorkflows)
			assertEqual(t, "HasJobChaining", limits.HasJobChaining, sourcePlan.Features.JobChaining)
			assertEqual(t, "HasCompensatingTxns", limits.HasCompensatingTxns, sourcePlan.Features.CompensatingTransactions)
			assertEqual(t, "HasCanaryDeployments", limits.HasCanaryDeployments, sourcePlan.Features.CanaryDeployments)
			assertEqual(t, "MaxScheduledJobs", limits.MaxScheduledJobs, sourcePlan.Limits.ScheduledJobs)
			assertEqual(t, "CronMinIntervalSec", limits.CronMinIntervalSec, sourcePlan.Limits.CronMinIntervalSec)
			assertEqual(t, "AllCronOverlapPolicies", limits.AllCronOverlapPolicies, tier != domain.PlanFree)
			assertEqual(t, "MaxEnvironments", limits.MaxEnvironments, sourcePlan.Limits.Environments)
			assertEqual(t, "MaxWebhookEndpoints", limits.MaxWebhookEndpoints, sourcePlan.Limits.WebhookEndpoints)
			assertEqual(t, "APIRateLimit", limits.APIRateLimit, sourcePlan.Limits.APIRateLimit)

			catalog, ok := PlanCatalogs[tier]
			if !ok {
				t.Fatalf("PlanCatalogs missing %s", tier)
			}
			assertEqual(t, "PlanCatalog.Tier", catalog.Tier, tier)
			assertEqual(t, "PlanCatalog.DisplayName", catalog.DisplayName, sourcePlan.DisplayName)
			assertEqual(t, "PlanCatalog.PriceMonthlyCents", catalog.PriceMonthlyCents, generatedPlanPriceCents(sourcePlan.Prices.MonthlyCents))
			assertEqual(t, "PlanCatalog.PriceAnnualCents", catalog.PriceAnnualCents, generatedPlanPriceCents(sourcePlan.Prices.AnnualCents))
			assertEqual(t, "PlanCatalog.LookupKeyMonthly", catalog.LookupKeyMonthly, sourcePlan.LookupKeys.Monthly)
			assertEqual(t, "PlanCatalog.LookupKeyAnnual", catalog.LookupKeyAnnual, sourcePlan.LookupKeys.Annual)
			assertEqual(t, "PlanCatalog.LookupKeyOverage", catalog.LookupKeyOverage, sourcePlan.LookupKeys.Overage)
			assertEqual(t, "PlanCatalog.OverageMicrousdPer1K", catalog.OverageMicrousdPer1K, sourcePlan.Overage.MicrousdPer1K)
			assertEqual(t, "PlanCatalog.OverageDefaultEnabled", catalog.OverageDefaultEnabled, sourcePlan.Overage.DefaultEnabled)
			assertEqual(t, "PlanCatalog.DefaultSpendingCapMicrousd", catalog.DefaultSpendingCapMicrousd, sourcePlan.Overage.DefaultSpendingCapMicrousd)
			assertEqual(t, "PlanCatalog.IncludedRunsPerMonth", catalog.IncludedRunsPerMonth, sourcePlan.Limits.RunsPerMonth)
			assertEqual(t, "PlanCatalog.RetentionDays", catalog.RetentionDays, sourcePlan.Limits.RetentionDays)
			assertEqual(t, "PlanCatalog.Concurrency", catalog.Concurrency, sourcePlan.Limits.ConcurrentRuns)
			assertEqual(t, "PlanCatalog.Environments", catalog.Environments, sourcePlan.Limits.Environments)
			assertEqual(t, "PlanCatalog.LogDrainGB", catalog.LogDrainGB, sourcePlan.Limits.LogDrainGB)
			assertDeepEqual(t, "PlanCatalog.RoadmapFeatures", catalog.RoadmapFeatures, sourcePlan.RoadmapFeatures)
		})
	}
}

func TestGeneratedAddonCatalogMatchesCatalogSource(t *testing.T) {
	t.Parallel()

	source := loadSourcePricingCatalog(t)
	assertEqual(t, "AddonCatalogs length", len(AddonCatalogs), len(source.Addons))
	assertEqual(t, "AddonCatalogOrder length", len(AddonCatalogOrder), len(source.Addons))

	for i, sourceAddon := range source.Addons {
		addonType := AddonType(sourceAddon.Type)
		assertEqual(t, "AddonCatalogOrder", AddonCatalogOrder[i], addonType)
		t.Run(sourceAddon.Type, func(t *testing.T) {
			t.Parallel()

			catalog, ok := AddonCatalogs[addonType]
			if !ok {
				t.Fatalf("AddonCatalogs missing %s", addonType)
			}
			assertEqual(t, "Type", catalog.Type, addonType)
			assertEqual(t, "DisplayName", catalog.DisplayName, sourceAddon.DisplayName)
			assertEqual(t, "LookupKey", catalog.LookupKey, sourceAddon.LookupKey)
			assertEqual(t, "PackSize", catalog.PackSize, sourceAddon.PackSize)
			assertEqual(t, "PriceCents", catalog.PriceCents, sourceAddon.PriceCents)
			assertEqual(t, "MaxTotal", catalog.MaxTotal, sourceAddon.MaxTotal)
			assertEqual(t, "Status", catalog.Status, sourceAddon.Status)
			assertDeepEqual(t, "AvailableOn", catalog.AvailableOn, sourcePlanTiers(t, sourceAddon.AvailableOn))

			switch sourceAddon.Status {
			case "active":
				if sourceAddon.LookupKey == "" {
					t.Fatalf("%s is active but has no Stripe lookup key", addonType)
				}
				if sourceAddon.PriceCents <= 0 {
					t.Fatalf("%s is active but has non-positive price %d", addonType, sourceAddon.PriceCents)
				}
				if len(sourceAddon.AvailableOn) == 0 {
					t.Fatalf("%s is active but has no available tiers", addonType)
				}
				if !IsLaunchActiveAddonType(addonType) {
					t.Fatalf("%s should be launch-active", addonType)
				}
			case "roadmap":
				if sourceAddon.LookupKey != "" {
					t.Fatalf("%s is roadmap but has Stripe lookup key %q", addonType, sourceAddon.LookupKey)
				}
				if sourceAddon.PriceCents != 0 {
					t.Fatalf("%s is roadmap but has price %d", addonType, sourceAddon.PriceCents)
				}
				if len(sourceAddon.AvailableOn) != 0 {
					t.Fatalf("%s is roadmap but is available on %v", addonType, sourceAddon.AvailableOn)
				}
				if IsLaunchActiveAddonType(addonType) {
					t.Fatalf("%s should not be launch-active", addonType)
				}
			default:
				t.Fatalf("%s has unknown add-on status %q", addonType, sourceAddon.Status)
			}
		})
	}
}

func TestLaunchCatalogKeepsRoadmapFeaturesInactive(t *testing.T) {
	t.Parallel()

	source := loadSourcePricingCatalog(t)
	if len(source.RoadmapFeature) == 0 {
		t.Fatal("source catalog has no roadmap feature list")
	}
	for _, sourcePlan := range source.Plans {
		tier := sourcePlanTier(t, sourcePlan.Tier)
		if len(sourcePlan.RoadmapFeatures) == 0 && tier != domain.PlanEnterprise {
			continue
		}
		t.Run(sourcePlan.Tier, func(t *testing.T) {
			t.Parallel()

			limits := GetPlanLimits(tier)
			if limits.HasSSO ||
				limits.HasSCIM ||
				limits.HasIPAllowlisting ||
				limits.HasStaticIPs ||
				limits.HasVPCPeering ||
				limits.HasDataResidency ||
				limits.HasCustomRBAC ||
				limits.HasDedicatedCompute ||
				limits.HasPriorityQueue ||
				limits.HasSessionManagement ||
				limits.HasSecretRotation ||
				limits.HasSIEMExport {
				t.Fatalf("%s exposes a roadmap feature as an active entitlement: %+v", tier, limits)
			}
		})
	}
}

func TestLaunchCatalogKeepsRegionsAtLaunchDefault(t *testing.T) {
	t.Parallel()

	source := loadSourcePricingCatalog(t)
	for _, sourcePlan := range source.Plans {
		tier := sourcePlanTier(t, sourcePlan.Tier)
		t.Run(sourcePlan.Tier, func(t *testing.T) {
			t.Parallel()

			wantRegions := []string{"iad"}
			assertDeepEqual(t, "sourcePlan.Limits.AllowedRegions", sourcePlan.Limits.AllowedRegions, wantRegions)
			assertDeepEqual(t, "GetPlanLimits.AllowedRegions", GetPlanLimits(tier).AllowedRegions, wantRegions)
		})
	}
}

func pricingCatalogSourcePath() string {
	return filepath.Join("..", "..", "..", "..", "packages", "billing", "catalog", "strait-pricing.json")
}

func loadSourcePricingCatalog(t *testing.T) sourcePricingCatalog {
	t.Helper()

	source, err := os.ReadFile(pricingCatalogSourcePath())
	if err != nil {
		t.Fatalf("read pricing catalog source: %v", err)
	}
	var catalog sourcePricingCatalog
	if err := json.Unmarshal(source, &catalog); err != nil {
		t.Fatalf("parse pricing catalog source: %v", err)
	}
	return catalog
}

func sourcePlanTier(t *testing.T, tier string) domain.PlanTier {
	t.Helper()

	planTier := domain.PlanTier(tier)
	if !planTier.IsValid() {
		t.Fatalf("unknown source plan tier %q", tier)
	}
	return planTier
}

func sourcePlanTiers(t *testing.T, tiers []string) []domain.PlanTier {
	t.Helper()

	result := make([]domain.PlanTier, 0, len(tiers))
	for _, tier := range tiers {
		result = append(result, sourcePlanTier(t, tier))
	}
	return result
}

func generatedPlanPriceCents(sourcePrice int) int {
	if sourcePrice < 0 {
		return 0
	}
	return sourcePrice
}

func generatedRBACLevel(sourceLevel string) string {
	if sourceLevel == "none" {
		return ""
	}
	return sourceLevel
}

func assertEqual[T comparable](t *testing.T, field string, got, want T) {
	t.Helper()

	if got != want {
		t.Fatalf("%s = %v, want %v", field, got, want)
	}
}

func assertDeepEqual(t *testing.T, field string, got, want any) {
	t.Helper()

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s = %#v, want %#v", field, got, want)
	}
}
