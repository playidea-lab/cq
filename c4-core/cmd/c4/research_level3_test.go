package main

import (
	"testing"
)

func TestResearchCLI_SpecCmdRegistered(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "research" || hasCmdPrefix(cmd.Use, "research") {
			for _, sub := range cmd.Commands() {
				if sub.Use == "spec <hyp-id>" || hasCmdPrefix(sub.Use, "spec") {
					found = true
				}
			}
		}
	}
	if !found {
		t.Error("'cq research spec' command not registered")
	}
}

func TestResearchCLI_CheckpointCmdRegistered(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if hasCmdPrefix(cmd.Use, "research") {
			for _, sub := range cmd.Commands() {
				if hasCmdPrefix(sub.Use, "checkpoint") {
					found = true
				}
			}
		}
	}
	if !found {
		t.Error("'cq research checkpoint' command not registered")
	}
}

func TestResearchCLI_DebateCmdRegistered(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if hasCmdPrefix(cmd.Use, "research") {
			for _, sub := range cmd.Commands() {
				if hasCmdPrefix(sub.Use, "debate") {
					found = true
				}
			}
		}
	}
	if !found {
		t.Error("'cq research debate' command not registered")
	}
}

// hasCmdPrefix reports whether s equals prefix or starts with prefix followed by a space.
func hasCmdPrefix(s, prefix string) bool {
	return s == prefix || (len(s) > len(prefix) && s[:len(prefix)] == prefix && s[len(prefix)] == ' ')
}
