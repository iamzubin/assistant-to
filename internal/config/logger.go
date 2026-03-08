package config

import (
	"fmt"
	"log"
	"strings"
	"sync"
)

// LogLevel represents the verbosity level
type LogLevel int

const (
	LogLevelError LogLevel = iota
	LogLevelWarn
	LogLevelInfo
	LogLevelDebug
)

var (
	currentLevel   LogLevel = LogLevelInfo
	redactSecrets  bool     = true
	loggerMu       sync.RWMutex
	secretPatterns = []string{
		"api_key",
		"apikey",
		"api-key",
		"secret",
		"token",
		"password",
		"auth",
	}
)

// InitLogging initializes the logging system from config
func InitLogging(cfg *Config) {
	loggerMu.Lock()
	defer loggerMu.Unlock()

	if cfg.Logging.Verbose {
		currentLevel = LogLevelDebug
	} else {
		currentLevel = LogLevelInfo
	}

	redactSecrets = cfg.Logging.RedactSecrets
}

// SetLogLevel sets the global log level
func SetLogLevel(level LogLevel) {
	loggerMu.Lock()
	defer loggerMu.Unlock()
	currentLevel = level
}

// SetRedactSecrets sets whether to redact secrets in logs
func SetRedactSecrets(redact bool) {
	loggerMu.Lock()
	defer loggerMu.Unlock()
	redactSecrets = redact
}

// redactSensitiveData redacts potentially sensitive information from log messages
func redactSensitiveData(msg string) string {
	if !redactSecrets {
		return msg
	}

	result := msg
	for _, pattern := range secretPatterns {
		// Simple redaction - replace value after pattern with [REDACTED]
		lowerMsg := strings.ToLower(result)
		idx := strings.Index(lowerMsg, pattern)
		if idx != -1 {
			// Find the end of the value (next space, newline, or comma)
			start := idx + len(pattern)
			if start < len(result) {
				// Skip any separators like =, :, or spaces
				for start < len(result) && (result[start] == '=' || result[start] == ':' || result[start] == ' ' || result[start] == '\t') {
					start++
				}

				// Find end of value
				end := start
				for end < len(result) && result[end] != ' ' && result[end] != '\n' && result[end] != ',' && result[end] != '\t' {
					end++
				}

				if end > start {
					result = result[:start] + "[REDACTED]" + result[end:]
				}
			}
		}
	}

	return result
}

// shouldLog checks if the message should be logged at current level
func shouldLog(level LogLevel) bool {
	loggerMu.RLock()
	defer loggerMu.RUnlock()
	return level <= currentLevel
}

// Error logs an error message
func Error(format string, v ...interface{}) {
	if shouldLog(LogLevelError) {
		msg := fmt.Sprintf(format, v...)
		msg = redactSensitiveData(msg)
		log.Printf("[ERROR] %s", msg)
	}
}

// Warn logs a warning message
func Warn(format string, v ...interface{}) {
	if shouldLog(LogLevelWarn) {
		msg := fmt.Sprintf(format, v...)
		msg = redactSensitiveData(msg)
		log.Printf("[WARN] %s", msg)
	}
}

// Info logs an info message
func Info(format string, v ...interface{}) {
	if shouldLog(LogLevelInfo) {
		msg := fmt.Sprintf(format, v...)
		msg = redactSensitiveData(msg)
		log.Printf("[INFO] %s", msg)
	}
}

// Debug logs a debug message (only if verbose)
func Debug(format string, v ...interface{}) {
	if shouldLog(LogLevelDebug) {
		msg := fmt.Sprintf(format, v...)
		msg = redactSensitiveData(msg)
		log.Printf("[DEBUG] %s", msg)
	}
}

// Fatal logs an error message and exits
func Fatal(format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	msg = redactSensitiveData(msg)
	log.Fatalf("[FATAL] %s", msg)
}

// Printf is a passthrough for standard logging (always logs)
func Printf(format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	msg = redactSensitiveData(msg)
	log.Print(msg)
}

// Println is a passthrough for standard logging (always logs)
func Println(v ...interface{}) {
	msg := fmt.Sprintln(v...)
	msg = redactSensitiveData(msg)
	log.Print(msg)
}
