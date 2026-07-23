package store

import (
	"sync"
	"time"
)

type Logger struct {
	mu   sync.Mutex
	logs []string
	max  int
}

func NewLogger(max int) *Logger {
	if max <= 0 {
		max = 500
	}
	return &Logger{max: max}
}

func (l *Logger) Add(msg string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	line := time.Now().Format("2006-01-02 15:04:05") + " " + msg
	l.logs = append(l.logs, line)
	if len(l.logs) > l.max {
		l.logs = l.logs[len(l.logs)-l.max:]
	}
}

func (l *Logger) List() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]string, len(l.logs))
	copy(out, l.logs)
	return out
}

func (l *Logger) Clear() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.logs = nil
}
