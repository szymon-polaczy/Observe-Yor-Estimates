package main

import (
	"log"
	"os"
)

// Logger provides structured logging for the application
type Logger struct {
	InfoLogger  *log.Logger
	WarnLogger  *log.Logger
	ErrorLogger *log.Logger
	DebugLogger *log.Logger
}

// NewLogger creates a new logger instance with different log levels
func NewLogger() *Logger {
	return &Logger{
		InfoLogger:  log.New(os.Stdout, "INFO: ", log.Ldate|log.Ltime|log.Lshortfile),
		WarnLogger:  log.New(os.Stdout, "WARN: ", log.Ldate|log.Ltime|log.Lshortfile),
		ErrorLogger: log.New(os.Stderr, "ERROR: ", log.Ldate|log.Ltime|log.Lshortfile),
		DebugLogger: log.New(os.Stdout, "DEBUG: ", log.Ldate|log.Ltime|log.Lshortfile),
	}
}

// Global logger instance
var appLogger = NewLogger()

// GetGlobalLogger returns the current global logger instance
// This ensures that JSON output mode is respected throughout the application
func GetGlobalLogger() *Logger {
	return appLogger
}

// Info logs informational messages
func (l *Logger) Info(v ...interface{}) {
	l.InfoLogger.Println(v...)
}

// Infof logs informational messages with formatting
func (l *Logger) Infof(format string, v ...interface{}) {
	l.InfoLogger.Printf(format, v...)
}

// Warn logs warning messages
func (l *Logger) Warn(v ...interface{}) {
	l.WarnLogger.Println(v...)
}

// Warnf logs warning messages with formatting
func (l *Logger) Warnf(format string, v ...interface{}) {
	l.WarnLogger.Printf(format, v...)
}

// Error logs error messages
func (l *Logger) Error(v ...interface{}) {
	l.ErrorLogger.Println(v...)
}

// Errorf logs error messages with formatting
func (l *Logger) Errorf(format string, v ...interface{}) {
	l.ErrorLogger.Printf(format, v...)
}

// Debug logs debug messages
func (l *Logger) Debug(v ...interface{}) {
	l.DebugLogger.Println(v...)
}

// Debugf logs debug messages with formatting
func (l *Logger) Debugf(format string, v ...interface{}) {
	l.DebugLogger.Printf(format, v...)
}

// Fatalf logs error with formatting and exits the application
func (l *Logger) Fatalf(format string, v ...interface{}) {
	l.ErrorLogger.Fatalf(format, v...)
}
