package serve

import (
	"strings"
	"testing"
)

func TestRingBuffer_Overflow(t *testing.T) {
	rb := NewRingBuffer(3)

	rb.Write("line1")
	rb.Write("line2")
	rb.Write("line3")
	// Buffer is now full; writing line4 should drop line1.
	rb.Write("line4")

	got := rb.String()
	if strings.Contains(got, "line1") {
		t.Errorf("expected line1 to be overwritten, got: %q", got)
	}
	for _, want := range []string{"line2", "line3", "line4"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in output, got: %q", want, got)
		}
	}
}

func TestRingBuffer_Order(t *testing.T) {
	rb := NewRingBuffer(5)
	rb.Write("a")
	rb.Write("b")
	rb.Write("c")

	got := rb.String()
	if got != "a\nb\nc" {
		t.Errorf("unexpected order: %q", got)
	}
}

func TestRingBuffer_Empty(t *testing.T) {
	rb := NewRingBuffer(4)
	if rb.String() != "" {
		t.Errorf("expected empty string, got %q", rb.String())
	}
}
