package scheduler

// containsEventType reports whether a subscription's event-type slice matches
// the target. A "*" entry is treated as a wildcard and matches any event.
func containsEventType(types []string, target string) bool {
	for _, t := range types {
		if t == target || t == "*" {
			return true
		}
	}
	return false
}
