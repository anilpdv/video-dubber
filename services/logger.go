package services

import (
	"fmt"
	"time"
)

// LogInfo logs an informational message with timestamp
func LogInfo(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Printf("[%s] INFO: %s\n", time.Now().Format("15:04:05"), msg)
}

// LogDebug logs a debug message with timestamp
func LogDebug(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Printf("[%s] DEBUG: %s\n", time.Now().Format("15:04:05"), msg)
}

// LogError logs an error message with timestamp
func LogError(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Printf("[%s] ERROR: %s\n", time.Now().Format("15:04:05"), msg)
}
