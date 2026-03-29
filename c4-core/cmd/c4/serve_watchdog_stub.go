//go:build !hub

package main

import (
	"context"
	"fmt"
)

func runWatchdog(ctx context.Context, hubClientAny any, extraArgs []string) error {
	_, _ = hubClientAny, extraArgs
	return fmt.Errorf("watchdog requires hub build tag")
}
