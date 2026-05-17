package platform

import (
	"fmt"
	"os"
	"sync"
	"time"
)

// Logger manages the launcher diagnostics logging session, supporting both in-memory buffering and file writing
type Logger struct {
	mu        sync.Mutex
	lines     []string
	logToFile bool
	logPath   string
}

// NewLogger instantiates a new Logger instance
func NewLogger(logToFile bool, logPath string) *Logger {
	return &Logger{logToFile: logToFile, logPath: logPath}
}

func appendToLogFile(path, msg string) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(fmt.Sprintf("[%s] %s\n", time.Now().Format(time.RFC3339), msg))
}

// Append logs a new message to the memory buffer and optionally persists it to the file path
func (l *Logger) Append(s string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, s)
	if len(l.lines) > 2000 {
		l.lines = l.lines[len(l.lines)-2000:]
	}
	if l.logToFile {
		appendToLogFile(l.logPath, s)
	}
}

// GetAll returns a copy of all log messages collected in the current session
func (l *Logger) GetAll() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]string, len(l.lines))
	copy(out, l.lines)
	return out
}

// AppendToLogFileRaw exposes the raw log file writing capability for bootstrapping and panic recovery logging
func AppendToLogFileRaw(path, msg string) {
	appendToLogFile(path, msg)
}

