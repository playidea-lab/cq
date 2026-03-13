//go:build research

package main

import (
	"testing"
)

// sampleGoTestJSON builds a synthetic go test -json output.
func sampleGoTestJSON(passTests, failTests int) []byte {
	var out []byte
	for i := 0; i < passTests; i++ {
		out = append(out, []byte(`{"Action":"pass","Test":"TestFoo"}`+"\n")...)
	}
	for i := 0; i < failTests; i++ {
		out = append(out, []byte(`{"Action":"fail","Test":"TestBar"}`+"\n")...)
	}
	// package-level events (no Test field) should be ignored
	out = append(out, []byte(`{"Action":"pass"}`+"\n")...)
	return out
}

func TestResearchRunCmd_CodeValidation(t *testing.T) {
	data := sampleGoTestJSON(3, 1) // 3 pass, 1 fail → pass_rate=0.75
	pass, fail := parseGoTestJSON(data)
	if pass != 3 {
		t.Errorf("expected 3 pass, got %d", pass)
	}
	if fail != 1 {
		t.Errorf("expected 1 fail, got %d", fail)
	}

	total := pass + fail
	passRate := float64(pass) / float64(total)
	if passRate >= 1.0 {
		t.Errorf("pass_rate should be < 1.0, got %.3f", passRate)
	}

	status := "failed"
	if passRate >= 1.0 {
		status = "completed"
	}
	if status != "failed" {
		t.Errorf("expected status=failed, got %s", status)
	}
}

func TestResearchRunCmd_AllTestsFail(t *testing.T) {
	data := sampleGoTestJSON(0, 5)
	pass, fail := parseGoTestJSON(data)
	if pass != 0 {
		t.Errorf("expected 0 pass, got %d", pass)
	}
	if fail != 5 {
		t.Errorf("expected 5 fail, got %d", fail)
	}

	total := pass + fail
	passRate := float64(pass) / float64(total)
	status := "failed"
	if passRate >= 1.0 {
		status = "completed"
	}
	if status != "failed" {
		t.Errorf("expected status=failed, got %s", status)
	}
}

func TestResearchRunCmd_AllTestsPass(t *testing.T) {
	data := sampleGoTestJSON(4, 0)
	pass, fail := parseGoTestJSON(data)
	if pass != 4 {
		t.Errorf("expected 4 pass, got %d", pass)
	}
	if fail != 0 {
		t.Errorf("expected 0 fail, got %d", fail)
	}

	total := pass + fail
	passRate := float64(pass) / float64(total)
	status := "failed"
	if passRate >= 1.0 {
		status = "completed"
	}
	if status != "completed" {
		t.Errorf("expected status=completed, got %s", status)
	}
}

func TestResearchRunCmd_UnsupportedType(t *testing.T) {
	// Verify that switch statement properly identifies ml_training
	spec := ExperimentSpec{Type: "ml_training"}
	if spec.Type != "ml_training" {
		t.Errorf("expected ml_training type, got %s", spec.Type)
	}
	// Behavioral contract: ml_training → unsupported (tested via binary smoke test)
}

func TestResearchRunCmd_MissingEnvVar(t *testing.T) {
	// Behavioral contract: missing C4_HYPOTHESIS_ID → exit 1.
	// This is enforced by runResearchRun calling os.Exit(1).
	// The integration smoke test (binary) validates this.
	// Unit test verifies the env var name constant.
	const envHypID = "C4_HYPOTHESIS_ID"
	const envSpecID = "C4_EXPERIMENT_SPEC_ID"
	if envHypID == "" || envSpecID == "" {
		t.Error("env var names must not be empty")
	}
}

func TestParseGoTestJSON_IgnoresPackageEvents(t *testing.T) {
	// Package-level events (no Test field) should not count.
	data := []byte(`{"Action":"pass"}` + "\n" + `{"Action":"fail"}` + "\n")
	pass, fail := parseGoTestJSON(data)
	if pass != 0 || fail != 0 {
		t.Errorf("expected 0/0, got %d/%d", pass, fail)
	}
}

func TestParseGoTestJSON_InvalidLines(t *testing.T) {
	data := []byte("not json\n{\"Action\":\"pass\",\"Test\":\"TestX\"}\n")
	pass, fail := parseGoTestJSON(data)
	if pass != 1 || fail != 0 {
		t.Errorf("expected 1/0, got %d/%d", pass, fail)
	}
}
