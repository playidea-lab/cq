package task

import "testing"

func TestValidateTaskID(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{name: "impl with version", id: "T-UA016-0", wantErr: false},
		{name: "review with version", id: "R-UA016-0", wantErr: false},
		{name: "repair with version", id: "RPR-001-2", wantErr: false},
		{name: "refine with version", id: "RF-001-0", wantErr: false},
		{name: "refine legacy no version", id: "RF-ROUND1", wantErr: false},
		{name: "checkpoint", id: "CP-001", wantErr: false},
		{name: "hyphen base", id: "T-LH-my-cool-tool-0", wantErr: false},
		{name: "legacy impl no version", id: "T-LEGACY", wantErr: false},
		{name: "invalid prefix", id: "INVALID", wantErr: true},
		{name: "missing base", id: "T--0", wantErr: true},
		{name: "invalid char", id: "T-INV!ALID-0", wantErr: true},
		{name: "empty checkpoint base", id: "CP-", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTaskID(tt.id)
			if tt.wantErr && err == nil {
				t.Fatalf("ValidateTaskID(%q) expected error", tt.id)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("ValidateTaskID(%q) unexpected error: %v", tt.id, err)
			}
		})
	}
}

func TestParseTaskIDDeterministic(t *testing.T) {
	tests := []struct {
		name       string
		id         string
		normalized string
		baseID     string
		version    int
		taskType   Type
	}{
		{
			name:       "impl with padded version",
			id:         "T-NEW-001",
			normalized: "T-NEW-1",
			baseID:     "NEW",
			version:    1,
			taskType:   TypeImplementation,
		},
		{
			name:       "hyphenated base",
			id:         "T-LH-my-cool-tool-0",
			normalized: "T-LH-my-cool-tool-0",
			baseID:     "LH-my-cool-tool",
			version:    0,
			taskType:   TypeImplementation,
		},
		{
			name:       "review",
			id:         "R-ALPHA-BETA-12",
			normalized: "R-ALPHA-BETA-12",
			baseID:     "ALPHA-BETA",
			version:    12,
			taskType:   TypeReview,
		},
		{
			name:       "repair",
			id:         "RPR-fix_bug-3",
			normalized: "RPR-fix_bug-3",
			baseID:     "fix_bug",
			version:    3,
			taskType:   TypeImplementation,
		},
		{
			name:       "legacy impl no version",
			id:         "T-LEGACY",
			normalized: "T-LEGACY-0",
			baseID:     "LEGACY",
			version:    0,
			taskType:   TypeImplementation,
		},
		{
			name:       "refine with version",
			id:         "RF-round1-0",
			normalized: "RF-round1-0",
			baseID:     "round1",
			version:    0,
			taskType:   TypeRefine,
		},
		{
			name:       "refine hyphenated base",
			id:         "RF-hub-review-2",
			normalized: "RF-hub-review-2",
			baseID:     "hub-review",
			version:    2,
			taskType:   TypeRefine,
		},
		{
			name:       "checkpoint",
			id:         "CP-001",
			normalized: "CP-001",
			baseID:     "001",
			version:    0,
			taskType:   TypeCheckpoint,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			normalized, baseID, version, taskType := ParseTaskID(tt.id)
			if normalized != tt.normalized {
				t.Fatalf("normalized = %q, want %q", normalized, tt.normalized)
			}
			if baseID != tt.baseID {
				t.Fatalf("baseID = %q, want %q", baseID, tt.baseID)
			}
			if version != tt.version {
				t.Fatalf("version = %d, want %d", version, tt.version)
			}
			if taskType != tt.taskType {
				t.Fatalf("taskType = %q, want %q", taskType, tt.taskType)
			}
		})
	}
}

// TestParseTaskID verifies CR-027 requirements: last-hyphen split for version,
// correct baseID extraction for single and multi-hyphen IDs.
func TestParseTaskID(t *testing.T) {
	tests := []struct {
		id         string
		wantBase   string
		wantVer    int
		reviewID   string
	}{
		// CR-027: T-F01-0 → ReviewID R-F01-0
		{id: "T-F01-0", wantBase: "F01", wantVer: 0, reviewID: "R-F01-0"},
		// CR-027: T-001-0 → ReviewID R-001-0
		{id: "T-001-0", wantBase: "001", wantVer: 0, reviewID: "R-001-0"},
		// CR-027: multi-hyphen IDs must not collide — T-MULTI-HYPHEN-0 stays distinct
		{id: "T-MULTI-HYPHEN-0", wantBase: "MULTI-HYPHEN", wantVer: 0, reviewID: "R-MULTI-HYPHEN-0"},
		// Ensure T-A-0 and T-A-B-0 produce different review IDs
		{id: "T-A-0", wantBase: "A", wantVer: 0, reviewID: "R-A-0"},
		{id: "T-A-B-0", wantBase: "A-B", wantVer: 0, reviewID: "R-A-B-0"},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			_, baseID, version, _ := ParseTaskID(tt.id)
			if baseID != tt.wantBase {
				t.Fatalf("ParseTaskID(%q) baseID = %q, want %q", tt.id, baseID, tt.wantBase)
			}
			if version != tt.wantVer {
				t.Fatalf("ParseTaskID(%q) version = %d, want %d", tt.id, version, tt.wantVer)
			}
			got := ReviewID(baseID, version)
			if got != tt.reviewID {
				t.Fatalf("ReviewID for %q = %q, want %q", tt.id, got, tt.reviewID)
			}
		})
	}

	// Verify no collision: T-A-0 and T-A-B-0 must produce different review IDs.
	_, base1, ver1, _ := ParseTaskID("T-A-0")
	_, base2, ver2, _ := ParseTaskID("T-A-B-0")
	rid1 := ReviewID(base1, ver1)
	rid2 := ReviewID(base2, ver2)
	if rid1 == rid2 {
		t.Fatalf("ReviewID collision: T-A-0 and T-A-B-0 both produce %q", rid1)
	}
}

// TestValidateTaskIDGrammarViolations verifies CR-027: grammar violations return errors.
func TestValidateTaskIDGrammarViolations(t *testing.T) {
	violations := []string{
		"T--0",       // empty base segment
		"INVALID",    // no recognised prefix
		"T-",         // missing base entirely (body = "")
		"T-INV!-0",   // illegal character
		"CP-",        // empty checkpoint base
		"R--0",       // empty review base
	}
	for _, id := range violations {
		if err := ValidateTaskID(id); err == nil {
			t.Errorf("ValidateTaskID(%q) expected error, got nil", id)
		}
	}

	// Sanity: these must still be valid after CR-027.
	valid := []string{
		"T-F01-0",
		"T-001-0",
		"T-MULTI-HYPHEN-0",
		"R-F01-0",
		"R-001-0",
		"CP-F01",
	}
	for _, id := range valid {
		if err := ValidateTaskID(id); err != nil {
			t.Errorf("ValidateTaskID(%q) unexpected error: %v", id, err)
		}
	}

}
