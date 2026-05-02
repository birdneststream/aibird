package commands

// displayDefault returns the value if non-empty, otherwise the fallback.
// Used for formatting user-facing display strings where empty values
// should show a descriptive placeholder instead.
func displayDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
