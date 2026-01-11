package main

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

//go:embed templates/*
var templates embed.FS

//go:embed static/*
var static embed.FS

// General interface for ServeMux or middlewares
type Router interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}

type WebServer struct {
	User       string
	Pass       string
	Port       string
	Plausible  string // Plausible domain
	Router     Router
	storage    StorageClient
	httpClient *http.Client
}

const plausibleAPIURL = "https://plausible.io/api/event"

func NewWebServer(user string, pass string, port string, plausible string, storage StorageClient) *WebServer {
	webServer := &WebServer{
		User:      user,
		Pass:      pass,
		Port:      port,
		Plausible: plausible,
		storage:   storage,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /ping", webServer.Heartbeat)

	mux.Handle("GET /static/", http.FileServer(http.FS(static)))
	mux.HandleFunc("GET /{key}", webServer.LookupHandler)

	if webServer.User == "" && webServer.Pass == "" {
		slog.Info("Setting up without auth")
		mux.HandleFunc("GET /", webServer.IndexHandler)
		mux.HandleFunc("POST /", webServer.UploadHandler)
	} else {
		slog.Info("Setting up with basic auth")
		mux.HandleFunc("GET /", webServer.BasicAuthWrapper(webServer.IndexHandler))
		mux.HandleFunc("POST /", webServer.BasicAuthWrapper(webServer.UploadHandler))
	}

	webServer.Router = NewLogger(mux)

	return webServer
}

func (webServer *WebServer) Start() {
	server := &http.Server{
		Addr:    fmt.Sprintf(":%s", webServer.Port),
		Handler: webServer.Router,
	}

	// Channel to listen for shutdown signals
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	// Start server in a goroutine
	go func() {
		slog.Info("Listening", "port", webServer.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Server error", "error", err)
			os.Exit(1)
		}
	}()

	<-shutdown
	slog.Info("Shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		slog.Error("Shutdown error", "error", err)
		os.Exit(1)
	}

	slog.Info("Server stopped")
}

func (webServer *WebServer) BasicAuthWrapper(next http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		user, pass, ok := request.BasicAuth()

		if !ok {
			slog.Debug("Couldn't parse basic auth")
		} else {
			if user == webServer.User && pass == webServer.Pass {
				next.ServeHTTP(writer, request)
				return
			}
			slog.Warn("Incorrect authentication provided")
		}

		writer.Header().Set("WWW-Authenticate", `Basic realm="File Cloud", charset="UTF-8"`)
		http.Error(writer, "Unauthorized", http.StatusUnauthorized)
	})
}

func (webServer *WebServer) Heartbeat(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Content-Type", "text/plain")
	writer.WriteHeader(http.StatusOK)
	_, err := writer.Write([]byte("."))
	if err != nil {
		slog.Error("Error writing heartbeat response", "error", err)
	}
}

func (webServer *WebServer) IndexHandler(writer http.ResponseWriter, request *http.Request) {
	webServer.ServeTemplate(writer, "index", StoredFile{})
}

func (webServer *WebServer) UploadHandler(writer http.ResponseWriter, request *http.Request) {
	file, header, err := request.FormFile("file")
	if err != nil {
		webServer.ServeError(writer, err)
		return
	}
	defer func() {
		err := file.Close()
		if err != nil {
			slog.Error("Error closing uploaded file", "error", err)
		}
	}()

	url, err := webServer.storage.UploadFile(file, *header)

	if err != nil {
		webServer.ServeError(writer, err)
	} else {
		writer.Header().Set("Content-Type", "application/json")
		_, err := fmt.Fprintf(writer, "{\"url\":\"%s\"}", url)
		if err != nil {
			slog.Error("Error writing JSON response", "error", err)
		}
	}
}

func (webServer *WebServer) LookupHandler(writer http.ResponseWriter, request *http.Request) {
	key := request.PathValue("key")
	idx := strings.Index(key, ".")

	if len(key) > keyLength && idx >= keyLength {
		ext := strings.ToLower(key[idx+1:])
		key = key[:idx]

		webServer.DirectHandler(writer, request, key, ext)
		return
	}

	file, err := webServer.storage.LookupFile(key)
	if err != nil {
		webServer.ServeError(writer, err)
		return
	}

	webServer.ServeTemplate(writer, "file", *file)
}

func (webServer *WebServer) DirectHandler(writer http.ResponseWriter, request *http.Request, key string, ext string) {
	file, err := webServer.storage.LookupFile(key)
	if err != nil {
		webServer.ServeError(writer, err)
		return
	}

	fileExt := strings.ToLower(filepath.Ext(file.OriginalName))
	if fileExt != "."+ext {
		webServer.ServeError(writer, ErrorObjectMissing)
		return
	}

	if len(webServer.Plausible) > 0 {
		webServer.logPlausibleEvent(*request, plausibleAPIURL)
	}

	http.Redirect(writer, request, file.Url, http.StatusMovedPermanently)
}

func (webServer *WebServer) ServeError(writer http.ResponseWriter, err error) {
	slog.Error("Request error", "error", err)

	if errors.Is(err, ErrorObjectMissing) {
		writer.WriteHeader(http.StatusNotFound)
		webServer.ServeTemplate(writer, "404", StoredFile{})
	} else {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
	}
}

func (webServer *WebServer) ServeTemplate(writer http.ResponseWriter, name string, data StoredFile) {
	t, err := template.ParseFS(templates, "templates/layout.tmpl.html", fmt.Sprintf("templates/%s.tmpl.html", name))
	if err != nil {
		webServer.ServeError(writer, err)
		return
	}

	templateData := struct {
		Plausible string
		StoredFile
	}{
		Plausible:  webServer.Plausible,
		StoredFile: data,
	}

	err = t.ExecuteTemplate(writer, "layout", templateData)
	if err != nil {
		webServer.ServeError(writer, err)
	}
}

type plausibleEvent struct {
	Name   string `json:"name"`
	Domain string `json:"domain"`
	URL    string `json:"url"`
}

func (webServer *WebServer) logPlausibleEvent(request http.Request, apiURL string) {
	event := plausibleEvent{
		Name:   "pageview",
		Domain: webServer.Plausible,
		URL:    request.URL.String(),
	}

	var body bytes.Buffer
	err := json.NewEncoder(&body).Encode(event)
	if err != nil {
		slog.Error("Failed to encode Plausible event", "error", err)
		return
	}

	req, err := http.NewRequest(http.MethodPost, apiURL, &body)
	if err != nil {
		slog.Error("Failed to create Plausible request", "error", err)
		return
	}
	req.Header.Add("User-Agent", request.UserAgent())
	req.Header.Add("X-Forwarded-For", request.RemoteAddr)
	req.Header.Add("Content-Type", "application/json")

	resp, err := webServer.httpClient.Do(req)
	if err != nil {
		slog.Error("Failed to send Plausible event", "error", err)
		return
	}
	defer func() {
		err := resp.Body.Close()
		if err != nil {
			slog.Error("Error closing response body", "error", err)
		}
	}()
}
