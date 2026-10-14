package audit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Event struct {
	TimeUTC     string         `json:"time_utc"`
	RequestID   string         `json:"request_id,omitempty"`
	RemoteAddr  string         `json:"remote_addr,omitempty"`
	Method      string         `json:"method,omitempty"`
	Tool        string         `json:"tool,omitempty"`
	Target      string         `json:"target,omitempty"`
	Args        map[string]any `json:"args,omitempty"`
	Allowed     bool           `json:"allowed"`
	Success     bool           `json:"success"`
	DurationMS  int64          `json:"duration_ms,omitempty"`
	OutputBytes int            `json:"output_bytes,omitempty"`
	Error       string         `json:"error,omitempty"`
}

type FileLogger struct {
	mu   sync.Mutex
	file *os.File
}

func NewFileLogger(path string) (*FileLogger, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o640)
	if err != nil {
		return nil, err
	}
	return &FileLogger{file: file}, nil
}

func (l *FileLogger) Write(event Event) {
	if l == nil || l.file == nil {
		return
	}
	if event.TimeUTC == "" {
		event.TimeUTC = time.Now().UTC().Format(time.RFC3339Nano)
	}
	line, err := json.Marshal(event)
	if err != nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	_, _ = l.file.Write(append(line, '\n'))
}

func (l *FileLogger) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	return l.file.Close()
}
