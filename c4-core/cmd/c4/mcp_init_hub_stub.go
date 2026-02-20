//go:build !c5_hub

package main

import "github.com/changmin/c4-core/internal/eventbus"

// Hub is disabled — no init hook registered.

// hubJobSubmitter returns nil when c5_hub is disabled.
func hubJobSubmitter(_ *initContext) eventbus.JobSubmitter { return nil }

// startHubPoller is a no-op when c5_hub is disabled.
func startHubPoller(_ *initContext) {}
