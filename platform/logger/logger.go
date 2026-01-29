// Package logger provides structured logging infrastructure for the application.
// This is part of the platform layer and contains no business logic.
package logger

import (
	"context"
	"log/slog"
	"os"
	"strings"
)

// Logger wraps slog.Logger for structured logging
type Logger struct {
	*slog.Logger
}

// New creates a new logger based on environment
func New(env string) *Logger {
	var handler slog.Handler

	opts := &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}

	if strings.EqualFold(env, "development") {
		opts.Level = slog.LevelDebug
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}

	return &Logger{
		Logger: slog.New(handler),
	}
}

// WithContext returns a logger with context values
func (l *Logger) WithContext(ctx context.Context) *Logger {
	return l
}

// WithRequestID returns a logger with request ID
func (l *Logger) WithRequestID(requestID string) *Logger {
	return &Logger{
		Logger: l.Logger.With(slog.String("request_id", requestID)),
	}
}

// WithUserID returns a logger with user ID
func (l *Logger) WithUserID(userID string) *Logger {
	return &Logger{
		Logger: l.Logger.With(slog.String("user_id", userID)),
	}
}

// HTTPRequest logs an HTTP request
func (l *Logger) HTTPRequest(method, path string, status int, latencyMs float64, clientIP string) {
	l.Info("http_request",
		slog.String("method", method),
		slog.String("path", path),
		slog.Int("status", status),
		slog.Float64("latency_ms", latencyMs),
		slog.String("client_ip", clientIP),
	)
}

// HTTPError logs an HTTP error
func (l *Logger) HTTPError(method, path string, status int, err error, clientIP string) {
	l.Error("http_error",
		slog.String("method", method),
		slog.String("path", path),
		slog.Int("status", status),
		slog.String("error", err.Error()),
		slog.String("client_ip", clientIP),
	)
}

// AuthEvent logs authentication events
func (l *Logger) AuthEvent(event, email string, success bool, reason string) {
	if success {
		l.Info("auth_event",
			slog.String("event", event),
			slog.String("email", email),
			slog.Bool("success", success),
		)
	} else {
		l.Warn("auth_event",
			slog.String("event", event),
			slog.String("email", email),
			slog.Bool("success", success),
			slog.String("reason", reason),
		)
	}
}

// DatabaseError logs database errors
func (l *Logger) DatabaseError(operation string, err error) {
	l.Error("database_error",
		slog.String("operation", operation),
		slog.String("error", err.Error()),
	)
}

// RateLimitExceeded logs rate limit events
func (l *Logger) RateLimitExceeded(clientIP, path string) {
	l.Warn("rate_limit_exceeded",
		slog.String("client_ip", clientIP),
		slog.String("path", path),
	)
}
