package main

import (
	"testing"
)

// TestLoginCmd_DeviceFlag_Registered verifies that authLoginCmd has a "device" flag.
func TestLoginCmd_DeviceFlag_Registered(t *testing.T) {
	f := authLoginCmd.Flags().Lookup("device")
	if f == nil {
		t.Fatal("expected --device flag to be registered on authLoginCmd")
	}
	if f.Value.Type() != "bool" {
		t.Errorf("expected --device flag type bool, got %s", f.Value.Type())
	}
}

// TestLoginCmd_LinkFlag_Registered verifies that authLoginCmd has a "link" flag.
func TestLoginCmd_LinkFlag_Registered(t *testing.T) {
	f := authLoginCmd.Flags().Lookup("link")
	if f == nil {
		t.Fatal("expected --link flag to be registered on authLoginCmd")
	}
	if f.Value.Type() != "bool" {
		t.Errorf("expected --link flag type bool, got %s", f.Value.Type())
	}
}

// TestLoginCmd_DeviceAndLink_MutuallyExclusive verifies that --device and --link
// cannot be used together.
func TestLoginCmd_DeviceAndLink_MutuallyExclusive(t *testing.T) {
	// Reset flags to their default values before testing.
	if err := authLoginCmd.Flags().Set("device", "false"); err != nil {
		t.Fatalf("resetting --device: %v", err)
	}
	if err := authLoginCmd.Flags().Set("link", "false"); err != nil {
		t.Fatalf("resetting --link: %v", err)
	}

	// Set both flags.
	if err := authLoginCmd.Flags().Set("device", "true"); err != nil {
		t.Fatalf("setting --device: %v", err)
	}
	if err := authLoginCmd.Flags().Set("link", "true"); err != nil {
		t.Fatalf("setting --link: %v", err)
	}

	// ValidateFlagGroups checks mutually exclusive groups.
	err := authLoginCmd.ValidateFlagGroups()
	if err == nil {
		t.Error("expected error when --device and --link are both set, got nil")
	}

	// Reset for other tests.
	_ = authLoginCmd.Flags().Set("device", "false")
	_ = authLoginCmd.Flags().Set("link", "false")
}

// TestLoginCmd_NoHubURL_Error verifies that --device without hub.url returns an error.
func TestLoginCmd_NoHubURL_Error(t *testing.T) {
	// Save and clear projectDir so resolveHubURL returns "".
	orig := projectDir
	projectDir = t.TempDir() // no .c4/config.yaml inside
	defer func() { projectDir = orig }()

	// Reset flags.
	_ = authLoginCmd.Flags().Set("device", "false")
	_ = authLoginCmd.Flags().Set("link", "false")

	err := runAuthLoginHeadless(nil, true /* isDevice */)
	if err == nil {
		t.Fatal("expected error when hub.url is not configured, got nil")
	}
}
