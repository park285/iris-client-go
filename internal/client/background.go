package client

import (
	"fmt"
	"log/slog"
	"runtime/debug"
)

func safeGo(logger *slog.Logger, event string, fn func()) {
	go runProtected(logger, event, fn)
}

func runProtected(logger *slog.Logger, event string, fn func()) {
	defer func() {
		if recovered := recover(); recovered != nil {
			if logger == nil {
				logger = slog.Default()
			}
			logger.Error(
				event,
				slog.String("panic_type", fmt.Sprintf("%T", recovered)),
				slog.String("stack", string(debug.Stack())),
			)
		}
	}()

	fn()
}
