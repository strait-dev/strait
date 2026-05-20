package notification

import "regexp"

var urlLikePattern = regexp.MustCompile(`https?://[^\s"']+`)

func sanitizeDeliveryError(err error) string {
	if err == nil {
		return ""
	}
	return redactURLSubstrings(sanitizeWebhookError(err))
}

func redactURLSubstrings(message string) string {
	return urlLikePattern.ReplaceAllStringFunc(message, func(string) string {
		return "[redacted-url]"
	})
}
