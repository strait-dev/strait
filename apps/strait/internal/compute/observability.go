package compute

// ClassifyFlyError classifies a Fly API HTTP status code into a retry strategy.
func ClassifyFlyError(statusCode int) (retryable bool, fatal bool, backoffSecs int) {
	switch {
	case statusCode == 200 || statusCode == 201:
		return false, false, 0
	case statusCode == 429:
		return true, false, 10
	case statusCode == 503:
		return true, false, 30
	case statusCode == 422:
		return false, true, 0
	case statusCode >= 500:
		return true, false, 5
	default:
		return true, false, 5
	}
}

// InfraRetryMetadata tracks infrastructure retry state for a managed run.
type InfraRetryMetadata struct {
	InfraRetryCount int `json:"infra_retry_count"`
	LastStatusCode  int `json:"last_status_code,omitempty"`
}
