package api

import (
	"fmt"
	"strconv"
	"strings"
)

// parseSemver parses a semver string like "v0.62.0" or "0.62.0"
// into [major, minor, patch] int slice.
// Returns an error if the string is not a valid semver.
func parseSemver(s string) ([3]int, error) {
	s = strings.TrimPrefix(s, "v")
	parts := strings.SplitN(s, ".", 3)
	if len(parts) != 3 {
		return [3]int{}, fmt.Errorf("invalid semver %q: expected major.minor.patch", s)
	}
	var v [3]int
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return [3]int{}, fmt.Errorf("invalid semver %q: part %q is not an integer", s, p)
		}
		v[i] = n
	}
	return v, nil
}

// semverLess returns true if a < b using integer comparison per component.
// This avoids lexicographic bugs like "v0.9" > "v0.10".
func semverLess(a, b [3]int) bool {
	if a[0] != b[0] {
		return a[0] < b[0]
	}
	if a[1] != b[1] {
		return a[1] < b[1]
	}
	return a[2] < b[2]
}
