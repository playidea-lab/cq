//go:build !c3_eventbus

package main

// EventBus is disabled — no init hook registered.

// startEventSink is a no-op when c3_eventbus is disabled.
func startEventSink(_ *initContext) {}
