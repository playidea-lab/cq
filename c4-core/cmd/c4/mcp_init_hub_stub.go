//go:build !hub

package main

import "github.com/changmin/c4-core/internal/eventbus"

// Hub is disabled — no init hook registered.

// hubJobSubmitter returns nil when hub is disabled.
func hubJobSubmitter(_ *initContext) eventbus.JobSubmitter { return nil }

// startHubPoller is a no-op when hub is disabled.
func startHubPoller(_ *initContext) {}
