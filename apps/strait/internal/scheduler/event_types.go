package scheduler

// containsEventType reports whether a subscription's event-type slice matches
// the target. A "*" entry is treated as a wildcard and matches any event.
func containsEventType(types []string, target string) bool {
	for _, t := range types {
		if eventTypeMatches(t, target) {
			return true
		}
	}
	return false
}

func eventTypeMatches(candidate, target string) bool {
	return candidate == target || candidate == "*"
}
