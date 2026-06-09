package api

import (
	"context"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

type PerformanceAnalyticsInput struct {
	PeriodHours int `query:"period_hours"`
}

type PerformanceAnalyticsOutput struct {
	Body any
}

func (s *Server) handleGetPerformanceAnalytics(ctx context.Context, input *PerformanceAnalyticsInput) (*PerformanceAnalyticsOutput, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "api.GetPerformanceAnalytics")
	defer span.End()

	projectID := projectIDFromContext(ctx)

	periodHours := input.PeriodHours
	if periodHours == 0 {
		if r := requestFromContext(ctx); r != nil && r.URL.Query().Has("period_hours") {
			return nil, huma.Error400BadRequest("period_hours must be between 1 and 720")
		}
		periodHours = 24
	}
	if periodHours < 1 || periodHours > 720 {
		return nil, huma.Error400BadRequest("period_hours must be between 1 and 720")
	}

	span.SetAttributes(attribute.Int("period_hours", periodHours))

	as, asErr := s.requireAnalytics()
	if asErr != nil {
		return nil, asErr
	}
	analytics, err := as.GetPerformanceAnalytics(ctx, projectID, periodHours)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get analytics")
	}

	return &PerformanceAnalyticsOutput{Body: analytics}, nil
}

func parseCostTimeRangeTyped(fromStr, toStr string) (time.Time, time.Time, error) {
	if fromStr == "" || toStr == "" {
		return time.Time{}, time.Time{}, huma.Error400BadRequest("from and to query parameters are required (RFC3339 format)")
	}
	from, err := time.Parse(time.RFC3339, fromStr)
	if err != nil {
		return time.Time{}, time.Time{}, huma.Error400BadRequest("from must be in RFC3339 format")
	}
	to, err := time.Parse(time.RFC3339, toStr)
	if err != nil {
		return time.Time{}, time.Time{}, huma.Error400BadRequest("to must be in RFC3339 format")
	}
	if !to.After(from) {
		return time.Time{}, time.Time{}, huma.Error400BadRequest("to must be after from")
	}
	if to.Sub(from) > maxCostWindow {
		return time.Time{}, time.Time{}, huma.Error400BadRequest("time range must not exceed 90 days")
	}
	return from, to, nil
}

func normalizeAnalyticsBucket(bucket string) (string, error) {
	if bucket == "" {
		return "day", nil
	}

	switch bucket {
	case "hour", "day":
		return bucket, nil
	default:
		return "", huma.Error400BadRequest("bucket must be 'hour' or 'day'")
	}
}
