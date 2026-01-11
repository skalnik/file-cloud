package main

import (
	"log/slog"
	"net/http"
	"time"
)

// ResponseWriter wrapper to capture status code and response size
type responseWriter struct {
	http.ResponseWriter
	statusCode   int
	responseSize int64
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	size, err := rw.ResponseWriter.Write(b)
	rw.responseSize += int64(size)
	return size, err
}

type LoggingMiddleware struct {
	handler http.Handler
}

func (l *LoggingMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	wrapped := &responseWriter{
		ResponseWriter: w,
		statusCode:     200,
		responseSize:   0,
	}

	requestSize := max(r.ContentLength, 0)

	clientIP := r.Header.Get("X-Forwarded-For")
	if clientIP == "" {
		clientIP = r.Header.Get("X-Real-IP")
	}
	if clientIP == "" {
		clientIP = r.RemoteAddr
	}

	l.handler.ServeHTTP(wrapped, r)

	duration := time.Since(start)

	slog.Info("Request",
		"method", r.Method,
		"path", r.URL.Path,
		"status", wrapped.statusCode,
		"requestSize", requestSize,
		"responseSize", wrapped.responseSize,
		"duration", duration,
		"clientIP", clientIP,
	)

	if duration > time.Second {
		slog.Warn("Slow request",
			"method", r.Method,
			"path", r.URL.Path,
			"duration", duration,
		)
	}

	if wrapped.statusCode >= 400 {
		slog.Warn("Error response",
			"method", r.Method,
			"path", r.URL.Path,
			"status", wrapped.statusCode,
		)
	}

}

func NewLogger(handlerToWrap http.Handler) *LoggingMiddleware {
	return &LoggingMiddleware{handlerToWrap}
}
