package middleware

import (
	"fmt"
	"log"
	"net/http"
	"runtime"

	"github.com/m1z23r/drift/pkg/drift"
)

// RecoveryConfig defines the config for Recovery middleware
type RecoveryConfig struct {
	// StackSize is the size of the stack trace buffer
	StackSize int

	// DisableStackAll disables stack trace for all goroutines
	DisableStackAll bool

	// PrintStack enables printing stack trace to stderr
	PrintStack bool

	// Handler is called when a panic is recovered
	// If nil, a default JSON error response is sent
	Handler func(*drift.Context, any)
}

// DefaultRecoveryConfig returns a default recovery configuration
func DefaultRecoveryConfig() RecoveryConfig {
	return RecoveryConfig{
		StackSize:       4 << 10, // 4 KB
		DisableStackAll: false,
		PrintStack:      true,
		Handler:         nil,
	}
}

// Recovery returns a recovery middleware with default config
func Recovery() drift.HandlerFunc {
	return RecoveryWithConfig(DefaultRecoveryConfig())
}

// RecoveryWithConfig returns a recovery middleware with custom config
func RecoveryWithConfig(config RecoveryConfig) drift.HandlerFunc {
	// Set defaults
	if config.StackSize == 0 {
		config.StackSize = 4 << 10
	}

	return func(c *drift.Context) {
		defer func() {
			if err := recover(); err != nil {
				// Log the panic
				if config.PrintStack {
					stack := make([]byte, config.StackSize)
					length := runtime.Stack(stack, !config.DisableStackAll)
					log.Printf("[RECOVERY] panic recovered:\n%v\n%s\n", err, stack[:length])
				} else {
					log.Printf("[RECOVERY] panic recovered: %v", err)
				}

				// Call custom handler if provided
				if config.Handler != nil {
					config.Handler(c, err)
					return
				}

				// Default error response
				c.AbortWithStatusJSON(http.StatusInternalServerError, map[string]any{
					"error":   "Internal Server Error",
					"message": fmt.Sprintf("%v", err),
				})
			}
		}()

		c.Next()
	}
}

// RecoveryWithHandler returns a recovery middleware with a custom handler
func RecoveryWithHandler(handler func(*drift.Context, any)) drift.HandlerFunc {
	return RecoveryWithConfig(RecoveryConfig{
		StackSize:       4 << 10,
		DisableStackAll: false,
		PrintStack:      true,
		Handler:         handler,
	})
}
