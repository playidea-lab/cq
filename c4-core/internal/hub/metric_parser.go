package hub

import (
	"regexp"
	"strconv"
)

// metricPattern matches @key=value where value is a number (int, float, scientific notation).
var metricPattern = regexp.MustCompile(`@(\w+)=([0-9]+\.?[0-9]*(?:[eE][+-]?[0-9]+)?)`)

// ParseMetrics extracts @key=value pairs from a single line.
// Returns nil if no metrics are found.
func ParseMetrics(line string) map[string]float64 {
	matches := metricPattern.FindAllStringSubmatch(line, -1)
	if len(matches) == 0 {
		return nil
	}
	result := make(map[string]float64, len(matches))
	for _, m := range matches {
		if v, err := strconv.ParseFloat(m[2], 64); err == nil {
			result[m[1]] = v
		}
	}
	return result
}
