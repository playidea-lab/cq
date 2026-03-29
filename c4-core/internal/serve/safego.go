package serve

import (
	"fmt"
	"os"
	"runtime/debug"
	"time"
)

// safeGo runs fn in a new goroutine with panic recovery.
// On panic: logs the stack to stderr, waits 1 second, then restarts fn.
func safeGo(name string, fn func()) {
	go func() {
		for {
			func() {
				defer func() {
					if r := recover(); r != nil {
						fmt.Fprintf(os.Stderr, "[safeGo] %s panicked: %v\n%s\n", name, r, debug.Stack())
					}
				}()
				fn()
			}()
			// fn returned (either normally or after panic recovery) — restart after delay
			time.Sleep(time.Second)
		}
	}()
}
