//go:build !research

package main

import "github.com/changmin/c4-core/internal/serve"

// registerLoopOrchestratorComponent is a no-op stub for non-research builds.
func registerLoopOrchestratorComponent(_ *serve.Manager, _ *initContext) {}
