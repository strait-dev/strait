package worker

import (
	"encoding/json"
	"maps"

	"strait/internal/domain"
)

func cloneJob(job *domain.Job) *domain.Job {
	if job == nil {
		return nil
	}
	cloned := *job
	if job.Tags != nil {
		cloned.Tags = maps.Clone(job.Tags)
	}
	if job.DefaultRunMetadata != nil {
		cloned.DefaultRunMetadata = maps.Clone(job.DefaultRunMetadata)
	}
	if job.RetryDelaysSecs != nil {
		cloned.RetryDelaysSecs = append([]int(nil), job.RetryDelaysSecs...)
	}
	if job.RateLimitKeys != nil {
		cloned.RateLimitKeys = append([]domain.RateLimitKey(nil), job.RateLimitKeys...)
	}
	if job.PreferredRegions != nil {
		cloned.PreferredRegions = append([]string(nil), job.PreferredRegions...)
	}
	if job.ResultSchema != nil {
		cloned.ResultSchema = append(json.RawMessage(nil), job.ResultSchema...)
	}
	if job.OnCompletePayloadMapping != nil {
		cloned.OnCompletePayloadMapping = append(json.RawMessage(nil), job.OnCompletePayloadMapping...)
	}
	if job.OnFailurePayloadMapping != nil {
		cloned.OnFailurePayloadMapping = append(json.RawMessage(nil), job.OnFailurePayloadMapping...)
	}
	return &cloned
}
