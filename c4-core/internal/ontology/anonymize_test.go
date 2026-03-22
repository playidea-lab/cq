package ontology

import (
	"testing"
)

func TestAnonymize_FiltersByEvidenceThreshold(t *testing.T) {
	proj := &ProjectOntology{
		Version: defaultVersion,
		Schema: CoreSchema{
			Nodes: map[string]Node{
				"api":    {Label: "API Gateway", Frequency: 5, NodeConfidence: ConfidenceHigh},
				"logger": {Label: "Logger", Frequency: 3, NodeConfidence: ConfidenceMedium},
				"cache":  {Label: "Cache", Frequency: 10, NodeConfidence: ConfidenceHigh},
				"draft":  {Label: "Draft", Frequency: 1, NodeConfidence: ConfidenceLow},
			},
		},
	}

	patterns := Anonymize(proj, "go-backend")

	// Only api (5) and cache (10) meet threshold.
	if len(patterns) != 2 {
		t.Errorf("expected 2 patterns above threshold, got %d", len(patterns))
	}
	for _, p := range patterns {
		if p.Frequency < AnonymizeThreshold {
			t.Errorf("pattern %q has frequency %d below threshold %d", p.Path, p.Frequency, AnonymizeThreshold)
		}
	}
}

func TestAnonymize_DomainTagged(t *testing.T) {
	proj := &ProjectOntology{
		Version: defaultVersion,
		Schema: CoreSchema{
			Nodes: map[string]Node{
				"svc": {Label: "Service", Frequency: 6, NodeConfidence: ConfidenceHigh},
			},
		},
	}

	patterns := Anonymize(proj, "ml-pipeline")

	if len(patterns) != 1 {
		t.Fatalf("expected 1 pattern, got %d", len(patterns))
	}
	if patterns[0].Domain != "ml-pipeline" {
		t.Errorf("expected domain=ml-pipeline, got %q", patterns[0].Domain)
	}
}

func TestAnonymize_FilepathStrippedToBasename(t *testing.T) {
	proj := &ProjectOntology{
		Version: defaultVersion,
		Schema: CoreSchema{
			Nodes: map[string]Node{
				"src/alice/project/handler.go": {
					Label:     "HTTP Handler",
					Frequency: 7,
					NodeConfidence: ConfidenceHigh,
				},
			},
		},
	}

	patterns := Anonymize(proj, "go-backend")

	if len(patterns) != 1 {
		t.Fatalf("expected 1 pattern, got %d", len(patterns))
	}
	p := patterns[0]
	// Path should be reduced to base name without extension.
	if p.Path != "handler" {
		t.Errorf("expected path=handler, got %q", p.Path)
	}
}

func TestAnonymize_AbsolutePathInValueSanitized(t *testing.T) {
	proj := &ProjectOntology{
		Version: defaultVersion,
		Schema: CoreSchema{
			Nodes: map[string]Node{
				"db": {
					Label:     "DB at /home/alice/project/data",
					Frequency: 8,
					NodeConfidence: ConfidenceHigh,
				},
			},
		},
	}

	patterns := Anonymize(proj, "go-backend")

	if len(patterns) != 1 {
		t.Fatalf("expected 1 pattern, got %d", len(patterns))
	}
	value := patterns[0].Value
	if containsAbsPath(value) {
		t.Errorf("expected absolute path to be removed from value, got %q", value)
	}
}

func TestAnonymize_EmptyLabelSkipped(t *testing.T) {
	proj := &ProjectOntology{
		Version: defaultVersion,
		Schema: CoreSchema{
			Nodes: map[string]Node{
				"ghost": {Label: "", Frequency: 10, NodeConfidence: ConfidenceHigh},
			},
		},
	}

	patterns := Anonymize(proj, "go-backend")
	if len(patterns) != 0 {
		t.Errorf("expected empty label node to be skipped, got %d patterns", len(patterns))
	}
}

func TestAnonymize_NilOntologyReturnsNil(t *testing.T) {
	patterns := Anonymize(nil, "go-backend")
	if patterns != nil {
		t.Errorf("expected nil for nil ontology, got %v", patterns)
	}
}

func TestAnonymize_TagsPreserved(t *testing.T) {
	proj := &ProjectOntology{
		Version: defaultVersion,
		Schema: CoreSchema{
			Nodes: map[string]Node{
				"queue": {
					Label:          "Message Queue",
					Frequency:      5,
					NodeConfidence: ConfidenceHigh,
					Tags:           []string{"async", "infra"},
				},
			},
		},
	}

	patterns := Anonymize(proj, "go-backend")

	if len(patterns) != 1 {
		t.Fatalf("expected 1 pattern, got %d", len(patterns))
	}
	tags := patterns[0].Tags
	if len(tags) != 2 || tags[0] != "async" || tags[1] != "infra" {
		t.Errorf("expected tags=[async infra], got %v", tags)
	}
}

// containsAbsPath checks whether s contains an absolute-looking path.
func containsAbsPath(s string) bool {
	for i, ch := range s {
		if ch == '/' && i > 0 {
			return true
		}
	}
	return false
}
