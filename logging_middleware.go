package main

import (
	"log"
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

	log.Printf("%s %s %d %dB/%dB %v %s",
		r.Method,
		r.URL.Path,
		wrapped.statusCode,
		requestSize,
		wrapped.responseSize,
		duration,
		clientIP,
	)

	if duration > time.Second {
		log.Printf("\033[31m[SLOW REQUEST] %s %s took %v\033[0m", r.Method, r.URL.Path, duration)
	}

	if wrapped.statusCode >= 400 {
		log.Printf("\033[31m[ERROR RESPONSE] %s %s returned %d\033[0m", r.Method, r.URL.Path, wrapped.statusCode)
	}

}

func NewLogger(handlerToWrap http.Handler) *LoggingMiddleware {
	return &LoggingMiddleware{handlerToWrap}
}
