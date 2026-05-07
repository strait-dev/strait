package telemetry

import (
	"strconv"
	"strings"

	"github.com/getsentry/sentry-go"
)

// SentryTag is a documented, low-cardinality Sentry tag key.
type SentryTag string

const (
	TagEdition        SentryTag = "edition"
	TagSubsystem      SentryTag = "subsystem"
	TagMode           SentryTag = "mode"
	TagRegion         SentryTag = "region"
	TagVersion        SentryTag = "version"
	TagProjectID      SentryTag = "project_id"
	TagOrgID          SentryTag = "org_id"
	TagEnvironmentID  SentryTag = "environment_id"
	TagAPIKeyID       SentryTag = "api_key_id"
	TagActorID        SentryTag = "actor_id"
	TagActorType      SentryTag = "actor_type"
	TagRequestID      SentryTag = "request_id"
	TagTraceID        SentryTag = "trace_id"
	TagSpanID         SentryTag = "span_id"
	TagRoute          SentryTag = "route"
	TagMethod         SentryTag = "method"
	TagStatusClass    SentryTag = "status_class"
	TagService        SentryTag = "service"
	TagRPC            SentryTag = "rpc"
	TagGRPCCode       SentryTag = "grpc_code"
	TagWorkerID       SentryTag = "worker_id"
	TagWorkerName     SentryTag = "worker_name"
	TagWorkerHost     SentryTag = "worker_host"
	TagSDKLanguage    SentryTag = "sdk_language"
	TagSDKVersion     SentryTag = "sdk_version"
	TagJobID          SentryTag = "job_id"
	TagRunID          SentryTag = "run_id"
	TagWorkflowID     SentryTag = "workflow_id"
	TagStepID         SentryTag = "step_id"
	TagDeliveryID     SentryTag = "delivery_id"
	TagTriggerID      SentryTag = "trigger_id"
	TagTable          SentryTag = "table"
	TagBatchKey       SentryTag = "batch_key"
	TagStatusCode     SentryTag = "status_code"
	TagOperation      SentryTag = "operation"
	TagConsumer       SentryTag = "consumer"
	TagApprovalID     SentryTag = "approval_id"
	TagKeyID          SentryTag = "key_id"
	TagSubscriptionID SentryTag = "subscription_id"
	TagSourceID       SentryTag = "source_id"
	TagDrainID        SentryTag = "drain_id"
	TagBatchID        SentryTag = "batch_id"
	TagAttempt        SentryTag = "attempt"
	TagErrorClass     SentryTag = "error_class"
)

const (
	SubsystemAPI       = "api"
	SubsystemGRPC      = "grpc"
	SubsystemWorker    = "worker"
	SubsystemQueue     = "queue"
	SubsystemWorkflow  = "workflow"
	SubsystemScheduler = "scheduler"
	SubsystemWebhook   = "webhook"
	SubsystemCDC       = "cdc"
	SubsystemLogDrain  = "logdrain"
	SubsystemUnknown   = "unknown"
)

const (
	ModeAPI     = "api"
	ModeWorker  = "worker"
	ModeAll     = "all"
	ModeUnknown = "unknown"
)

var knownSentryTags = map[string]SentryTag{
	string(TagEdition):        TagEdition,
	string(TagSubsystem):      TagSubsystem,
	string(TagMode):           TagMode,
	string(TagRegion):         TagRegion,
	string(TagVersion):        TagVersion,
	string(TagProjectID):      TagProjectID,
	string(TagOrgID):          TagOrgID,
	string(TagEnvironmentID):  TagEnvironmentID,
	string(TagAPIKeyID):       TagAPIKeyID,
	string(TagActorID):        TagActorID,
	string(TagActorType):      TagActorType,
	string(TagRequestID):      TagRequestID,
	string(TagTraceID):        TagTraceID,
	string(TagSpanID):         TagSpanID,
	string(TagRoute):          TagRoute,
	string(TagMethod):         TagMethod,
	string(TagStatusClass):    TagStatusClass,
	string(TagService):        TagService,
	string(TagRPC):            TagRPC,
	string(TagGRPCCode):       TagGRPCCode,
	string(TagWorkerID):       TagWorkerID,
	string(TagWorkerName):     TagWorkerName,
	string(TagWorkerHost):     TagWorkerHost,
	string(TagSDKLanguage):    TagSDKLanguage,
	string(TagSDKVersion):     TagSDKVersion,
	string(TagJobID):          TagJobID,
	string(TagRunID):          TagRunID,
	string(TagWorkflowID):     TagWorkflowID,
	string(TagStepID):         TagStepID,
	string(TagDeliveryID):     TagDeliveryID,
	string(TagTriggerID):      TagTriggerID,
	string(TagTable):          TagTable,
	string(TagBatchKey):       TagBatchKey,
	string(TagStatusCode):     TagStatusCode,
	string(TagOperation):      TagOperation,
	string(TagConsumer):       TagConsumer,
	string(TagApprovalID):     TagApprovalID,
	string(TagKeyID):          TagKeyID,
	string(TagSubscriptionID): TagSubscriptionID,
	string(TagSourceID):       TagSourceID,
	string(TagDrainID):        TagDrainID,
	string(TagBatchID):        TagBatchID,
	string(TagAttempt):        TagAttempt,
	string(TagErrorClass):     TagErrorClass,

	// Backward-compatible aliases already emitted by older capture paths.
	"workflow_run_id": TagWorkflowID,
	"step_run_id":     TagStepID,
}

// SentryTagFromString returns the typed tag key for a documented Sentry tag.
func SentryTagFromString(key string) (SentryTag, bool) {
	tag, ok := knownSentryTags[key]
	return tag, ok
}

// SetSentryTag sets a normalized tag value on scope.
func SetSentryTag(scope *sentry.Scope, tag SentryTag, value string) {
	if scope == nil || tag == "" {
		return
	}
	if value = normalizeSentryTagValueForKey(tag, value); value == "" {
		return
	}
	scope.SetTag(string(tag), value)
}

// SentryTagStrings converts typed tags to the string map expected by Sentry.
func SentryTagStrings(tags map[SentryTag]string) map[string]string {
	out := make(map[string]string, len(tags))
	for tag, value := range tags {
		if tag == "" {
			continue
		}
		if value = normalizeSentryTagValueForKey(tag, value); value != "" {
			out[string(tag)] = value
		}
	}
	return out
}

// RequiredSentryTags returns the baseline tags every Sentry event should carry.
func RequiredSentryTags(edition, subsystem, mode, region, version string) map[SentryTag]string {
	return map[SentryTag]string{
		TagEdition:   strings.ToLower(defaultTag(edition, "unknown")),
		TagSubsystem: NormalizeSubsystem(subsystem),
		TagMode:      NormalizeMode(mode),
		TagRegion:    strings.ToLower(defaultTag(region, "unknown")),
		TagVersion:   defaultTag(version, "unknown"),
	}
}

func NormalizeSentryTagValue(value string) string {
	return strings.TrimSpace(value)
}

func NormalizeMode(mode string) string {
	switch strings.ToLower(NormalizeSentryTagValue(mode)) {
	case ModeAPI:
		return ModeAPI
	case ModeWorker:
		return ModeWorker
	case ModeAll:
		return ModeAll
	default:
		return ModeUnknown
	}
}

func NormalizeSubsystem(subsystem string) string {
	switch strings.ToLower(NormalizeSentryTagValue(subsystem)) {
	case SubsystemAPI:
		return SubsystemAPI
	case SubsystemGRPC:
		return SubsystemGRPC
	case SubsystemWorker:
		return SubsystemWorker
	case SubsystemQueue:
		return SubsystemQueue
	case SubsystemWorkflow:
		return SubsystemWorkflow
	case SubsystemScheduler:
		return SubsystemScheduler
	case SubsystemWebhook:
		return SubsystemWebhook
	case SubsystemCDC:
		return SubsystemCDC
	case SubsystemLogDrain:
		return SubsystemLogDrain
	default:
		return SubsystemUnknown
	}
}

func NormalizeHTTPStatusClass(status int) string {
	if status < 100 || status > 599 {
		return ""
	}
	return strconv.Itoa(status/100) + "xx"
}

func defaultTag(value, fallback string) string {
	if value = NormalizeSentryTagValue(value); value != "" {
		return value
	}
	return fallback
}

func normalizeSentryTagValueForKey(tag SentryTag, value string) string {
	value = NormalizeSentryTagValue(value)
	switch tag {
	case TagMode:
		return NormalizeMode(value)
	case TagSubsystem:
		return NormalizeSubsystem(value)
	default:
		return value
	}
}
