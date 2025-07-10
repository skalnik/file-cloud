package main

import (
	"log"
	"net/http"
	"time"
)

type LoggingMiddleware struct {
	handler http.Handler
}

func (l *LoggingMiddleware) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	start := time.Now()
	l.handler.ServeHTTP(response, request)
	log.Printf("%s %s %v", request.Method, request.URL.Path, time.Since(start))
}

func NewLogger(handlerToWrap http.Handler) *LoggingMiddleware {
	return &LoggingMiddleware{handlerToWrap}
}
