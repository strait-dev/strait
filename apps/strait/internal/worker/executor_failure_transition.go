package worker

import (
	"fmt"
	"strconv"
	"time"

	"strait/internal/domain"
)

type failurePoisonPillDetection struct {
	hash      string
	count     int
	threshold int
}

type failureRunTransition struct {
	retry      bool
	retryAt    time.Time
	errMsg     string
	errClass   string
	fields     map[string]any
	poisonPill *failurePoisonPillDetection
}

func newFailureRunTransition(
	run *domain.JobRun,
	job *domain.Job,
	policy executionPolicy,
	err error,
	errMsg string,
	errClass string,
	finishedAt time.Time,
) failureRunTransition {
	shouldRetry := run.Attempt < policy.maxAttempts
	if shouldRetry && !shouldRetryForClass(errClass) {
		shouldRetry = false
	}

	var metadataModified bool
	var poisonPill *failurePoisonPillDetection
	if shouldRetry && job.PoisonPillThreshold != nil && *job.PoisonPillThreshold > 0 {
		hash := errorHashForError(err)
		prevHash := run.Metadata["_error_hash"]
		count := 1
		if prevHash == hash {
			if raw, ok := run.Metadata["_error_hash_count"]; ok {
				if n, parseErr := strconv.Atoi(raw); parseErr == nil {
					count = n + 1
				}
			}
		}
		if run.Metadata == nil {
			run.Metadata = make(map[string]string)
		}
		run.Metadata["_error_hash"] = hash
		run.Metadata["_error_hash_count"] = strconv.Itoa(count)
		metadataModified = true

		threshold := *job.PoisonPillThreshold
		if count >= threshold {
			shouldRetry = false
			errMsg = fmt.Sprintf("poison pill detected (same error %d times): %s", count, errMsg)
			poisonPill = &failurePoisonPillDetection{
				hash:      hash,
				count:     count,
				threshold: threshold,
			}
		}
	}

	if shouldRetry {
		fields := retryStatusFields(run, job, errMsg, errClass)
		if metadataModified {
			fields["metadata"] = run.Metadata
		}
		return failureRunTransition{
			retry:    true,
			retryAt:  NextRetryAtWithPolicy(run.Attempt, policy.retryBackoff, policy.retryInitialSecs, policy.retryMaxSecs),
			errMsg:   errMsg,
			errClass: errClass,
			fields:   fields,
		}
	}

	fields := terminalStatusFields(finishedAt, errMsg, errClass)
	if metadataModified {
		fields["metadata"] = run.Metadata
	}
	return failureRunTransition{
		errMsg:     errMsg,
		errClass:   errClass,
		fields:     fields,
		poisonPill: poisonPill,
	}
}
