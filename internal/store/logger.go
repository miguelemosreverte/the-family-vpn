package store

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"time"
)

// Logger is a structured logger that writes to both stdout and the store.
type Logger struct {
	store     *Store
	component string
	stdout    io.Writer
	mu        sync.Mutex
	fields    map[string]interface{}
}

// NewLogger creates a new logger for a component.
func NewLogger(store *Store, component string) *Logger {
	return &Logger{
		store:     store,
		component: component,
		stdout:    os.Stdout,
		fields:    make(map[string]interface{}),
	}
}

// WithField returns a new logger with an additional field.
func (l *Logger) WithField(key string, value interface{}) *Logger {
	newLogger := &Logger{
		store:     l.store,
		component: l.component,
		stdout:    l.stdout,
		fields:    make(map[string]interface{}),
	}
	for k, v := range l.fields {
		newLogger.fields[k] = v
	}
	newLogger.fields[key] = value
	return newLogger
}

// WithFields returns a new logger with additional fields.
func (l *Logger) WithFields(fields map[string]interface{}) *Logger {
	newLogger := &Logger{
		store:     l.store,
		component: l.component,
		stdout:    l.stdout,
		fields:    make(map[string]interface{}),
	}
	for k, v := range l.fields {
		newLogger.fields[k] = v
	}
	for k, v := range fields {
		newLogger.fields[k] = v
	}
	return newLogger
}

// Debug logs a debug message.
func (l *Logger) Debug(msg string, args ...interface{}) {
	l.log("DEBUG", msg, args...)
}

// Info logs an info message.
func (l *Logger) Info(msg string, args ...interface{}) {
	l.log("INFO", msg, args...)
}

// Warn logs a warning message.
func (l *Logger) Warn(msg string, args ...interface{}) {
	l.log("WARN", msg, args...)
}

// Error logs an error message.
func (l *Logger) Error(msg string, args ...interface{}) {
	l.log("ERROR", msg, args...)
}

func (l *Logger) log(level, msg string, args ...interface{}) {
	if len(args) > 0 {
		msg = fmt.Sprintf(msg, args...)
	}

	// Format fields as JSON
	var fieldsJSON string
	if len(l.fields) > 0 {
		if data, err := json.Marshal(l.fields); err == nil {
			fieldsJSON = string(data)
		}
	}

	// Write to stdout
	timestamp := time.Now().Format("2006/01/02 15:04:05")
	l.mu.Lock()
	fmt.Fprintf(l.stdout, "%s [%s] [%s] %s", timestamp, l.component, level, msg)
	if fieldsJSON != "" {
		fmt.Fprintf(l.stdout, " %s", fieldsJSON)
	}
	fmt.Fprintln(l.stdout)
	l.mu.Unlock()

	// Write to store
	if l.store != nil {
		l.store.WriteLog(level, l.component, msg, fieldsJSON)
	}
}

// StdLogWriter returns an io.Writer for use with the standard log package.
func (l *Logger) StdLogWriter(level string) io.Writer {
	return &stdLogWriter{logger: l, level: level}
}

type stdLogWriter struct {
	logger *Logger
	level  string
}

func (w *stdLogWriter) Write(p []byte) (n int, err error) {
	msg := strings.TrimSpace(string(p))
	// Remove standard log prefix if present
	if idx := strings.Index(msg, "] "); idx >= 0 {
		msg = msg[idx+2:]
	}
	w.logger.log(w.level, msg)
	return len(p), nil
}

// RedirectStdLog redirects the standard log package to this logger.
func (l *Logger) RedirectStdLog(level string) {
	log.SetOutput(l.StdLogWriter(level))
	log.SetFlags(0) // Remove default timestamp since we add our own
}

// LogWriter wraps Store to provide an io.Writer interface for existing log.* calls.
// This intercepts standard log output and stores it.
type LogWriter struct {
	store     *Store
	component string
	level     string
}

// NewLogWriter creates a writer that captures log output.
func NewLogWriter(store *Store, component, level string) *LogWriter {
	return &LogWriter{
		store:     store,
		component: component,
		level:     level,
	}
}

func (w *LogWriter) Write(p []byte) (n int, err error) {
	msg := strings.TrimSpace(string(p))
	if msg == "" {
		return len(p), nil
	}

	// Extract component from [component] prefix
	component := w.component
	level := w.level

	// Parse log format: "2024/01/15 14:30:00 [component] message"
	if idx := strings.Index(msg, "["); idx >= 0 {
		if endIdx := strings.Index(msg[idx:], "]"); endIdx > 0 {
			component = msg[idx+1 : idx+endIdx]
			msg = strings.TrimSpace(msg[idx+endIdx+1:])
		}
	}

	// Detect level from message content
	msgLower := strings.ToLower(msg)
	if strings.Contains(msgLower, "error") || strings.Contains(msgLower, "failed") {
		level = "ERROR"
	} else if strings.Contains(msgLower, "warn") {
		level = "WARN"
	} else if strings.Contains(msgLower, "debug") {
		level = "DEBUG"
	}

	// Write to store
	if w.store != nil {
		w.store.WriteLog(level, component, msg, "")
	}

	// Also write to original stdout
	os.Stdout.Write(p)
	return len(p), nil
}

// MultiWriter writes to multiple writers.
func MultiWriter(writers ...io.Writer) io.Writer {
	return io.MultiWriter(writers...)
}
