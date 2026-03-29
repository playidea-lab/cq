package serve

import "strings"

// RingBuffer is a fixed-size circular buffer for log lines.
// When the buffer is full, the oldest line is overwritten.
type RingBuffer struct {
	lines    []string
	maxLines int
	head     int // index of next write position
	count    int // number of lines stored (≤ maxLines)
}

// NewRingBuffer creates a RingBuffer that holds at most maxLines lines.
func NewRingBuffer(maxLines int) *RingBuffer {
	if maxLines <= 0 {
		maxLines = 1
	}
	return &RingBuffer{
		lines:    make([]string, maxLines),
		maxLines: maxLines,
	}
}

// Write appends a line to the buffer. If the buffer is full, the oldest
// line is silently discarded.
func (r *RingBuffer) Write(line string) {
	r.lines[r.head] = line
	r.head = (r.head + 1) % r.maxLines
	if r.count < r.maxLines {
		r.count++
	}
}

// String returns all stored lines joined by newlines, in insertion order
// (oldest first).
func (r *RingBuffer) String() string {
	if r.count == 0 {
		return ""
	}
	out := make([]string, r.count)
	// When the buffer has wrapped, oldest line is at r.head.
	// When not yet full, oldest line is at index 0.
	start := 0
	if r.count == r.maxLines {
		start = r.head
	}
	for i := range r.count {
		out[i] = r.lines[(start+i)%r.maxLines]
	}
	return strings.Join(out, "\n")
}
