package main

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"path/filepath"
	"strings"
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
	User      string
	Pass      string
	Port      string
	Plausible string // Plausible domain
	Router    Router
	storage   StorageClient
}

const PLAUSIBLE_API_URL = "https://plausible.io/api/event"

func NewWebServer(user string, pass string, port string, plausible string, storage StorageClient) *WebServer {
	webServer := new(WebServer)
	webServer.User = user
	webServer.Pass = pass
	webServer.Port = port
	webServer.Plausible = plausible
	webServer.storage = storage

	mux := http.NewServeMux()
	mux.HandleFunc("GET /ping", webServer.Heartbeat)

	mux.Handle("GET /static/", http.FileServer(http.FS(static)))
	mux.HandleFunc("GET /{key}", webServer.LookupHandler)

	if webServer.User == "" && webServer.Pass == "" {
		log.Println("Setting up without auth...")
		mux.HandleFunc("GET /", webServer.IndexHandler)
		mux.HandleFunc("POST /", webServer.UploadHandler)
	} else {
		log.Println("Setting up with basic auth...")
		mux.HandleFunc("GET /", webServer.BasicAuthWrapper(webServer.IndexHandler))
		mux.HandleFunc("POST /", webServer.BasicAuthWrapper(webServer.UploadHandler))
	}

	webServer.Router = NewLogger(mux)

	return webServer
}

func (webServer *WebServer) Start() {
	log.Printf("Listening on port %s", webServer.Port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%s", webServer.Port), webServer.Router))
}

func (webServer *WebServer) BasicAuthWrapper(next http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		user, pass, ok := request.BasicAuth()

		if !ok {
			log.Println("Couldn't parse basic auth")
		} else {
			if user == webServer.User && pass == webServer.Pass {
				next.ServeHTTP(writer, request)
				return
			} else {
				log.Println("Incorrect authentication provided")
			}
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
		log.Printf("Error writing heartbeat response: %s", err)
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
			log.Printf("Error closing uploaded file: %s", err)
		}
	}()

	url, err := webServer.storage.UploadFile(file, *header)

	if err != nil {
		webServer.ServeError(writer, err)
	} else {
		writer.Header().Set("Content-Type", "application/json")
		_, err := fmt.Fprintf(writer, "{\"url\":\"%s\"}", url)
		if err != nil {
			log.Printf("Error writing JSON response: %s", err)
		}
	}
}

func (webServer *WebServer) LookupHandler(writer http.ResponseWriter, request *http.Request) {
	key := request.PathValue("key")
	idx := strings.Index(key, ".")

	if len(key) > KEY_LENGTH && idx >= KEY_LENGTH {
		ext := strings.ToLower(key[idx+1:])
		key = key[:idx]

		webServer.DirectHandler(writer, request, key, ext)
		return
	}

	file, err := webServer.storage.LookupFile(key)
	if err != nil {
		webServer.ServeError(writer, err)
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
		webServer.logPlausibleEvent(*request, PLAUSIBLE_API_URL)
	}

	http.Redirect(writer, request, file.Url, http.StatusMovedPermanently)
}

func (webServer *WebServer) ServeError(writer http.ResponseWriter, err error) {
	log.Printf("\033[31m%s\033[0m", err.Error())

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
		log.Printf("\033[31m%s\033[0m", err)
	}

	req, err := http.NewRequest(http.MethodPost, apiURL, &body)
	if err != nil {
		log.Printf("\033[31m%s\033[0m", err)
	}
	req.Header.Add("User-Agent", request.UserAgent())
	req.Header.Add("X-Forwarded-For", request.RemoteAddr)
	req.Header.Add("Content-Type", "application/json")

	client := &http.Client{}
	_, err = client.Do(req)
	if err != nil {
		log.Printf("\033[31m%s\033[0m", err)
	}
}
