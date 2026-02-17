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
