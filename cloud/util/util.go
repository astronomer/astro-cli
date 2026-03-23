package util

const ellipsisLen = 3

// TruncateString truncates a string to maxLen characters, appending "..." if truncated
func TruncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= ellipsisLen {
		return s[:maxLen]
	}
	return s[:maxLen-ellipsisLen] + "..."
}
