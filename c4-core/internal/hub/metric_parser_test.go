package hub

import (
	"testing"
)

func TestParseMetrics_SingleMetric(t *testing.T) {
	result := ParseMetrics("Epoch 10 @loss=0.17")
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 metric, got %d", len(result))
	}
	if result["loss"] != 0.17 {
		t.Errorf("expected loss=0.17, got %v", result["loss"])
	}
}

func TestParseMetrics_MultipleMetrics(t *testing.T) {
	result := ParseMetrics("@loss=0.5 @acc=0.91 @lr=1e-4")
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result) != 3 {
		t.Fatalf("expected 3 metrics, got %d", len(result))
	}
	if result["loss"] != 0.5 {
		t.Errorf("expected loss=0.5, got %v", result["loss"])
	}
	if result["acc"] != 0.91 {
		t.Errorf("expected acc=0.91, got %v", result["acc"])
	}
	if result["lr"] != 1e-4 {
		t.Errorf("expected lr=1e-4, got %v", result["lr"])
	}
}

func TestParseMetrics_NoMatch(t *testing.T) {
	result := ParseMetrics("Hello world")
	if result != nil {
		t.Errorf("expected nil result, got %v", result)
	}
}

func TestParseMetrics_ScientificNotation(t *testing.T) {
	result := ParseMetrics("@lr=3.5e-05")
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result["lr"] != 3.5e-05 {
		t.Errorf("expected lr=3.5e-05, got %v", result["lr"])
	}
}

func TestParseMetrics_Integer(t *testing.T) {
	result := ParseMetrics("@epoch=10")
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result["epoch"] != 10.0 {
		t.Errorf("expected epoch=10.0, got %v", result["epoch"])
	}
}

func TestParseMetrics_EmptyString(t *testing.T) {
	result := ParseMetrics("")
	if result != nil {
		t.Errorf("expected nil result, got %v", result)
	}
}
