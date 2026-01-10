// Package logger provides structured logging utilities for the video-translator application.
package logger

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// Level represents the severity of a log message.
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

// String returns the string representation of the log level.
func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// Logger is the main logging struct.
type Logger struct {
	level  Level
	output io.Writer
	mu     sync.Mutex
}

// Default logger instance
var defaultLogger = &Logger{
	level:  LevelInfo,
	output: os.Stdout,
}

// SetLevel sets the minimum log level for the default logger.
func SetLevel(level Level) {
	defaultLogger.mu.Lock()
	defer defaultLogger.mu.Unlock()
	defaultLogger.level = level
}

// SetOutput sets the output writer for the default logger.
func SetOutput(w io.Writer) {
	defaultLogger.mu.Lock()
	defer defaultLogger.mu.Unlock()
	defaultLogger.output = w
}

// New creates a new logger with the specified level and output.
func New(level Level, output io.Writer) *Logger {
	return &Logger{
		level:  level,
		output: output,
	}
}

// log writes a log message with the given level.
func (l *Logger) log(level Level, format string, args ...interface{}) {
	if level < l.level {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	msg := fmt.Sprintf(format, args...)
	timestamp := time.Now().Format("15:04:05")
	fmt.Fprintf(l.output, "[%s] %s: %s\n", timestamp, level.String(), msg)
}

// Debug logs a debug message.
func (l *Logger) Debug(format string, args ...interface{}) {
	l.log(LevelDebug, format, args...)
}

// Info logs an informational message.
func (l *Logger) Info(format string, args ...interface{}) {
	l.log(LevelInfo, format, args...)
}

// Warn logs a warning message.
func (l *Logger) Warn(format string, args ...interface{}) {
	l.log(LevelWarn, format, args...)
}

// Error logs an error message.
func (l *Logger) Error(format string, args ...interface{}) {
	l.log(LevelError, format, args...)
}

// Package-level functions that use the default logger

// Debug logs a debug message using the default logger.
func Debug(format string, args ...interface{}) {
	defaultLogger.Debug(format, args...)
}

// Info logs an informational message using the default logger.
func Info(format string, args ...interface{}) {
	defaultLogger.Info(format, args...)
}

// Warn logs a warning message using the default logger.
func Warn(format string, args ...interface{}) {
	defaultLogger.Warn(format, args...)
}

// Error logs an error message using the default logger.
func Error(format string, args ...interface{}) {
	defaultLogger.Error(format, args...)
}

// Legacy functions for backward compatibility with services package

// LogInfo logs an informational message (legacy compatibility).
func LogInfo(format string, args ...interface{}) {
	defaultLogger.Info(format, args...)
}

// LogDebug logs a debug message (legacy compatibility).
func LogDebug(format string, args ...interface{}) {
	defaultLogger.Debug(format, args...)
}

// LogError logs an error message (legacy compatibility).
func LogError(format string, args ...interface{}) {
	defaultLogger.Error(format, args...)
}
