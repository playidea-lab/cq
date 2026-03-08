package main

import (
	"fmt"
	"time"
)

// formatUptime formats uptime in seconds to a human-readable string.
// 0~59s → "45s", 60s~59m59s → "2m", 1h~23h59m → "2h 15m", 24h+ → "1d 3h"
func formatUptime(sec int64) string {
	if sec < 60 {
		return fmt.Sprintf("%ds", sec)
	}
	minutes := sec / 60
	if minutes < 60 {
		return fmt.Sprintf("%dm", minutes)
	}
	hours := minutes / 60
	mins := minutes % 60
	if hours < 24 {
		if mins == 0 {
			return fmt.Sprintf("%dh", hours)
		}
		return fmt.Sprintf("%dh %dm", hours, mins)
	}
	days := hours / 24
	hrs := hours % 24
	if hrs == 0 {
		return fmt.Sprintf("%dd", days)
	}
	return fmt.Sprintf("%dd %dh", days, hrs)
}

// formatLastJob formats an RFC3339 timestamp to a human-readable ago string.
// "" → "never", <60s → "just now", <60m → "5m ago", <24h → "2h ago", 24h+ → "1d ago"
func formatLastJob(rfc3339 string) string {
	if rfc3339 == "" {
		return "never"
	}
	t, err := time.Parse(time.RFC3339, rfc3339)
	if err != nil {
		return rfc3339
	}
	diff := time.Since(t)
	if diff < 60*time.Second {
		return "just now"
	}
	minutes := int(diff.Minutes())
	if minutes < 60 {
		return fmt.Sprintf("%dm ago", minutes)
	}
	hours := int(diff.Hours())
	if hours < 24 {
		return fmt.Sprintf("%dh ago", hours)
	}
	days := hours / 24
	return fmt.Sprintf("%dd ago", days)
}
