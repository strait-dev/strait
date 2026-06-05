package api

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/robfig/cron/v3"
)

const defaultJobQueueName = "default"

// validateCreateJobCronFields validates the cron and execution_window_cron expressions.
func validateCreateJobCronFields(req *CreateJobRequest) error {
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	if req.Cron != "" {
		if err := validateCronFieldCount(req.Cron); err != nil {
			return huma.Error400BadRequest(err.Error())
		}
		if _, err := parser.Parse(req.Cron); err != nil {
			return huma.Error400BadRequest("invalid cron expression")
		}
	}
	if req.ExecutionWindowCron != "" {
		if err := validateCronFieldCount(req.ExecutionWindowCron); err != nil {
			return huma.Error400BadRequest(err.Error())
		}
		if _, err := parser.Parse(req.ExecutionWindowCron); err != nil {
			return huma.Error400BadRequest("invalid execution_window_cron expression")
		}
	}
	return nil
}

func validateOptionalCron(expr *string, invalidMessage string) error {
	if expr == nil || *expr == "" {
		return nil
	}
	if err := validateCronFieldCount(*expr); err != nil {
		return huma.Error400BadRequest(err.Error())
	}
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	if _, err := parser.Parse(*expr); err != nil {
		return huma.Error400BadRequest(invalidMessage)
	}
	return nil
}

// queueNameRe is the allowed pattern for queue names: alphanumerics, dashes,
// underscores, 1-63 chars.
var queueNameRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,63}$`)

// validateQueueName returns an error if the queue name is non-empty and does not match
// the required pattern ^[A-Za-z0-9_-]{1,63}$.
func validateQueueName(name string) error {
	if name == "" {
		return nil
	}
	if !queueNameRe.MatchString(name) {
		return fmt.Errorf("queue_name must match ^[A-Za-z0-9_-]{1,63}$")
	}
	return nil
}

func normalizeJobQueueName(name string) string {
	if name == "" {
		return defaultJobQueueName
	}
	return name
}

func validateTags(tags map[string]string) error {
	if len(tags) > 20 {
		return fmt.Errorf("too many tags (max 20)")
	}
	for key, value := range tags {
		if key == "" {
			return fmt.Errorf("tag keys must be non-empty")
		}
		if len(key) > 64 {
			return fmt.Errorf("tag key too long (max 64 characters)")
		}
		if len(value) > 256 {
			return fmt.Errorf("tag value too long (max 256 characters)")
		}
	}
	return nil
}

// validateRetryConfig validates retry_strategy and retry_delays_secs values.
func validateRetryConfig(strategy string, delays []int) error {
	if strategy != "" {
		switch strategy {
		case "exponential", "linear", "fixed", "custom":
			// valid
		default:
			return fmt.Errorf("invalid retry_strategy: must be exponential, linear, fixed, or custom")
		}
	}
	for _, d := range delays {
		if d <= 0 {
			return fmt.Errorf("retry_delays_secs values must be positive")
		}
	}
	return nil
}

func (s *Server) validateWindowsAgainstRetention(rateLimitWindowSecs, dedupWindowSecs int) error {
	if s.config == nil {
		return nil
	}
	maxSecs := int(s.config.RunRetentionShort.Seconds())
	if maxSecs <= 0 {
		return nil
	}
	if rateLimitWindowSecs > maxSecs {
		return fmt.Errorf("rate_limit_window_secs (%d) exceeds hot retention (%d seconds)", rateLimitWindowSecs, maxSecs)
	}
	if dedupWindowSecs > maxSecs {
		return fmt.Errorf("dedup_window_secs (%d) exceeds hot retention (%d seconds)", dedupWindowSecs, maxSecs)
	}
	return nil
}

// validateCronFieldCount checks that a cron expression has exactly 5 fields
// (minute, hour, day-of-month, month, day-of-week). The cron parser is
// configured without seconds support, so 6-field expressions are rejected.
func validateCronFieldCount(expr string) error {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return fmt.Errorf("cron expression must have exactly 5 fields (minute hour day-of-month month day-of-week), got %d", len(fields))
	}
	return nil
}
