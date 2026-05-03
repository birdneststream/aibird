// Package helpers provides shared utilities including mode mapping, string helpers, and markdown conversion.
package helpers

import (
	"runtime/debug"

	"aibird/logger"
)

// GoSafe launches a goroutine with panic recovery. If the goroutine panics,
// the panic is caught and logged with a stack trace instead of crashing the process.
// This should be used for fire-and-forget goroutines in command handlers and
// queue processing where an unhandled panic would take down the entire bot.
func GoSafe(fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("goroutine panic recovered",
					"error", r,
					"stack", string(debug.Stack()),
				)
			}
		}()
		fn()
	}()
}
