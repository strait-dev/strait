package telemetry

import (
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/getsentry/sentry-go"
	"github.com/jackc/pgx/v5/pgconn"
	"google.golang.org/grpc/status"

	"strait/internal/domain"
)

const defaultFingerprintValue = "unknown"

// BuildSentryRelease returns the default release string for Sentry events.
func BuildSentryRelease(version, commit string) string {
	version = strings.TrimSpace(version)
	commit = strings.TrimSpace(commit)
	if version == "" {
		version = "dev"
	}
	if commit == "" || commit == "none" || commit == "unknown" {
		return version
	}
	return version + "+" + shortCommit(commit)
}

// ApplySentryFingerprint sets a stable fingerprint for known logical errors.
func ApplySentryFingerprint(event *sentry.Event, hint *sentry.EventHint) {
	if event == nil || len(event.Fingerprint) > 0 {
		return
	}
	if fp := sentryFingerprint(event, hint); len(fp) > 0 {
		event.Fingerprint = fp
	}
}

func sentryFingerprint(event *sentry.Event, hint *sentry.EventHint) []string {
	err := eventError(event, hint)
	if fp := grpcFingerprint(event, err); len(fp) > 0 {
		return fp
	}
	if fp := dbFingerprint(err); len(fp) > 0 {
		return fp
	}
	if fp := workflowFingerprint(event); len(fp) > 0 {
		return fp
	}
	if fp := domainErrorFingerprint(err); len(fp) > 0 {
		return fp
	}
	if fp := httpStatusFingerprint(err); len(fp) > 0 {
		return fp
	}
	return nil
}

func grpcFingerprint(event *sentry.Event, err error) []string {
	service := tagValue(event, string(TagService))
	rpc := tagValue(event, string(TagRPC))
	code := tagValue(event, string(TagGRPCCode))
	if err != nil {
		if st, ok := status.FromError(err); ok {
			code = st.Code().String()
		}
	}
	if service == "" && rpc == "" && code == "" {
		return nil
	}
	return []string{"grpc", fallbackFingerprintValue(service), fallbackFingerprintValue(rpc), fallbackFingerprintValue(code)}
}

func dbFingerprint(err error) []string {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return nil
	}
	return []string{"db", fallbackFingerprintValue(pgErr.Code), fallbackFingerprintValue(pgErrorObject(pgErr))}
}

func workflowFingerprint(event *sentry.Event) []string {
	if event == nil || tagValue(event, string(TagSubsystem)) != SubsystemWorkflow {
		return nil
	}
	stepKind := firstNonEmpty(
		tagValue(event, "step_type"),
		latestBreadcrumbDataString(event, "workflow.step", "step_type"),
	)
	category := firstNonEmpty(
		tagValue(event, string(TagErrorClass)),
		tagValue(event, string(TagOperation)),
		latestBreadcrumbDataString(event, "workflow.step", "status"),
		event.Message,
	)
	return []string{"workflow", fallbackFingerprintValue(stepKind), fallbackFingerprintValue(category)}
}

func domainErrorFingerprint(err error) []string {
	if err == nil {
		return nil
	}
	var transitionErr *domain.TransitionError
	if errors.As(err, &transitionErr) {
		return []string{"domain.transition", string(transitionErr.From), string(transitionErr.To)}
	}
	var statusErr *domain.UnknownStatusError
	if errors.As(err, &statusErr) {
		return []string{"domain.unknown_status", string(statusErr.Status)}
	}
	var fieldErr *domain.FieldError
	if errors.As(err, &fieldErr) {
		return []string{"domain.field", fallbackFingerprintValue(fieldErr.Field)}
	}
	var configErr *domain.ConfigError
	if errors.As(err, &configErr) {
		return []string{"domain.config", fallbackFingerprintValue(configErr.Field)}
	}
	var endpointErr *domain.EndpointError
	if errors.As(err, &endpointErr) {
		return []string{"endpoint", NormalizeHTTPStatusClass(endpointErr.StatusCode)}
	}
	return nil
}

func httpStatusFingerprint(err error) []string {
	if err == nil {
		return nil
	}
	var se statusError
	if !errors.As(err, &se) {
		return nil
	}
	statusClass := NormalizeHTTPStatusClass(se.GetStatus())
	if statusClass == "" || se.GetStatus() < http.StatusInternalServerError {
		return nil
	}
	return []string{"http", statusClass}
}

func pgErrorObject(pgErr *pgconn.PgError) string {
	return firstNonEmpty(pgErr.TableName, pgErr.ConstraintName, pgErr.ColumnName, pgErr.SchemaName)
}

func tagValue(event *sentry.Event, key string) string {
	if event == nil || event.Tags == nil {
		return ""
	}
	return strings.TrimSpace(event.Tags[key])
}

func latestBreadcrumbDataString(event *sentry.Event, category, key string) string {
	if event == nil {
		return ""
	}
	for _, bc := range slices.Backward(event.Breadcrumbs) {
		if bc == nil || bc.Category != category || bc.Data == nil {
			continue
		}
		if value, ok := bc.Data[key].(string); ok {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func fallbackFingerprintValue(value string) string {
	if value = strings.TrimSpace(value); value != "" {
		return value
	}
	return defaultFingerprintValue
}

func shortCommit(commit string) string {
	if len(commit) <= 12 {
		return commit
	}
	return commit[:12]
}
