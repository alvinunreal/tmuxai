package logger

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
)

var (
	instance *Logger
	once     sync.Once
)

// Logger represents a custom logger for TmuxAI
type Logger struct {
	logFile *os.File
	logger  *log.Logger
	mu      sync.Mutex
}

// Init initializes the logger
func Init() error {
	var err error
	once.Do(func() {
		instance, err = newLogger()
	})
	return err
}

// newLogger creates a new logger instance
func newLogger() (*Logger, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get user home directory: %w", err)
	}

	logDir := filepath.Join(homeDir, ".config", "tmuxai")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	logPath := filepath.Join(logDir, "tmuxai.log")
	logFile, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}

	logger := log.New(logFile, "", log.LstdFlags)

	return &Logger{
		logFile: logFile,
		logger:  logger,
		mu:      sync.Mutex{},
	}, nil
}

// GetInstance returns the singleton logger instance
func GetInstance() (*Logger, error) {
	if instance == nil {
		return nil, fmt.Errorf("logger not initialized")
	}
	return instance, nil
}

// Close closes the logger
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.logFile.Close()
}

// Info logs an info message
func (l *Logger) Info(format string, v ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.logger.Printf("[INFO] "+format, v...)
}

// Error logs an error message
func (l *Logger) Error(format string, v ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.logger.Printf("[ERROR] "+format, v...)
}

// Debug logs a debug message
func (l *Logger) Debug(format string, v ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.logger.Printf("[DEBUG] "+format, v...)
}

// Info logs an info message using the singleton instance
func Info(format string, v ...interface{}) {
	if instance != nil {
		instance.Info(format, v...)
	}
}

// Error logs an error message using the singleton instance
func Error(format string, v ...interface{}) {
	if instance != nil {
		instance.Error(format, v...)
	}
}

// Debug logs a debug message using the singleton instance
func Debug(format string, v ...interface{}) {
	if instance != nil {
		instance.Debug(format, v...)
	}
}
