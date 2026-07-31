package helpers

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Structured Severity "enum"
type Severity struct {
	PANIC SeverityType
	ERROR SeverityType
	WARN  SeverityType
	INFO  SeverityType
	DEBUG SeverityType
}

// SeverityType defines the type for severity levels
type SeverityType string

// Instantiate Severity constants
var SeverityLevels = &Severity{
	PANIC: "PANIC",
	ERROR: "ERROR",
	WARN:  "WARN",
	INFO:  "INFO",
	DEBUG: "DEBUG",
}

type Reporter struct {
	lock     sync.Mutex
	filePath string
	file     *os.File
	closed   bool
}

func NewReporter(filePath string) (*Reporter, error) {
	// Create directory structure if it doesn't exist
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}

	return &Reporter{
		filePath: filePath,
		file:     file,
	}, nil
}

// Report writes a log entry to the file
func (r *Reporter) Report(level SeverityType, message string) error {
	// PRE-COMPUTE BEFORE LOCK
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	entry := fmt.Sprintf("%s [%s] %s\n", timestamp, level, message)

	r.lock.Lock()
	defer r.lock.Unlock()

	if r.closed {
		return fmt.Errorf("reporter is closed")
	}
	_, err := r.file.WriteString(entry)
	return err
}

// Close the report file
func (r *Reporter) Close() error {
	r.lock.Lock()
	defer r.lock.Unlock()

	if r.closed {
		return nil
	}
	r.closed = true
	return r.file.Close()
}

func (r *Reporter) Cleanup(frequency time.Duration) {
	// Remove old report files in the report directory
	files, err := os.ReadDir(r.filePath)
	if err != nil {
		slog.Error("Failed to read report directory", slog.Any("error", err))
		return
	}
	for _, file := range files {
		if file.IsDir() {
			continue
		}

		fileInfo, err := file.Info()
		if err != nil {
			slog.Error("Failed to get file info", slog.Any("error", err))
			continue
		}
		if time.Since(fileInfo.ModTime()) > frequency {
			err := os.Remove(filepath.Join(r.filePath, file.Name()))
			if err != nil {
				slog.Error("Failed to remove file", slog.Any("error", err))
			}
		}
	}
}
