package main

import (
	"bytes"
	"context"
	"crypto/subtle"
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

// Router interface for ServeMux or middlewares
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

// URLs can get weirdly long, so lets cap it at 10
const maxAlbumFiles = 10

type AlbumFile struct {
	Key string
	StoredFile
}

type Album struct {
	Title   string
	OgImage string
	Files   []AlbumFile
}

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
	mux.HandleFunc("HEAD /ping", webServer.Heartbeat)

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

// validateBasicAuth performs constant-time comparison of credentials to prevent timing attacks
func (webServer *WebServer) validateBasicAuth(user, pass string) bool {
	userMatch := subtle.ConstantTimeCompare([]byte(user), []byte(webServer.User))
	passMatch := subtle.ConstantTimeCompare([]byte(pass), []byte(webServer.Pass))
	return userMatch == 1 && passMatch == 1
}

func (webServer *WebServer) BasicAuthWrapper(next http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		user, pass, ok := request.BasicAuth()

		if !ok {
			slog.Debug("Couldn't parse basic auth")
		} else {
			if webServer.validateBasicAuth(user, pass) {
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
	if request.Method != http.MethodHead {
		_, err := writer.Write([]byte("."))
		if err != nil {
			slog.Error("Error writing heartbeat response", "error", err)
		}
	}
}

func (webServer *WebServer) IndexHandler(writer http.ResponseWriter, request *http.Request) {
	webServer.ServeTemplate(writer, nil, "index", StoredFile{})
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

	if strings.Contains(key, "+") {
		webServer.StatelessAlbumHandler(writer, request, key)
		return
	}

	if len(key) < keyLength {
		webServer.ServeError(writer, ErrorObjectMissing)
		return
	}

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

	webServer.ServeTemplate(writer, request, "file", *file)
}

func (webServer *WebServer) StatelessAlbumHandler(writer http.ResponseWriter, request *http.Request, keys string) {
	splitKeys := strings.Split(keys, "+")

	if len(splitKeys) > maxAlbumFiles {
		http.Error(writer, fmt.Sprintf("Albums are limited to %d files", maxAlbumFiles), http.StatusBadRequest)
		return
	}

	files := make([]AlbumFile, 0, len(splitKeys))
	for _, key := range splitKeys {
		if len(key) < keyLength {
			webServer.ServeError(writer, ErrorObjectMissing)
			return
		}

		file, err := webServer.storage.LookupFile(key)
		if err != nil {
			webServer.ServeError(writer, err)
			return
		}

		files = append(files, AlbumFile{Key: key[:keyLength], StoredFile: *file})
	}

	album := Album{
		Title: fmt.Sprintf("%d files", len(files)),
		Files: files,
	}

	for _, file := range files {
		if file.Kind == KindImage {
			album.OgImage = file.Url
			break
		}
	}

	webServer.ServeAlbumTemplate(writer, request, album)
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
		webServer.ServeTemplate(writer, nil, "404", StoredFile{})
	} else {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
	}
}

func pageURL(request *http.Request) string {
	if request != nil && request.Host != "" {
		return fmt.Sprintf("https://%s%s", request.Host, request.URL.Path)
	}

	return ""
}

func (webServer *WebServer) ServeTemplate(writer http.ResponseWriter, request *http.Request, name string, data StoredFile) {
	t, err := template.ParseFS(templates, "templates/layout.tmpl.html", fmt.Sprintf("templates/%s.tmpl.html", name))
	if err != nil {
		webServer.ServeError(writer, err)
		return
	}

	templateData := struct {
		Plausible string
		PageURL   string
		StoredFile
	}{
		Plausible:  webServer.Plausible,
		PageURL:    pageURL(request),
		StoredFile: data,
	}

	err = t.ExecuteTemplate(writer, "layout", templateData)
	if err != nil {
		webServer.ServeError(writer, err)
	}
}

func (webServer *WebServer) ServeAlbumTemplate(writer http.ResponseWriter, request *http.Request, album Album) {
	t, err := template.ParseFS(templates, "templates/layout.tmpl.html", "templates/album.tmpl.html")
	if err != nil {
		webServer.ServeError(writer, err)
		return
	}

	templateData := struct {
		Plausible string
		PageURL   string
		Album
	}{
		Plausible: webServer.Plausible,
		PageURL:   pageURL(request),
		Album:     album,
	}

	err = t.ExecuteTemplate(writer, "layout", templateData)
	if err != nil {
		webServer.ServeError(writer, err)
	}
}

type plausibleEvent struct {
	Name     string `json:"name"`
	Domain   string `json:"domain"`
	URL      string `json:"url"`
	Referrer string `json:"referrer"`
}

func (webServer *WebServer) logPlausibleEvent(request http.Request, apiURL string) {
	event := plausibleEvent{
		Name:     "pageview",
		Domain:   webServer.Plausible,
		URL:      fmt.Sprintf("https://%s%s", request.Host, request.URL.String()),
		Referrer: request.Referer(),
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
