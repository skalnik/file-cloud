package main

import (
	"embed"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"

	"github.com/gorilla/mux"
)

//go:embed templates/*
var templates embed.FS

//go:embed static/*
var static embed.FS

type WebServer struct {
	User      string
	Pass      string
	Port      string
	Plausible string // Plausible domain
	Router    *mux.Router
	storage   StorageClient
}

func NewWebServer(user string, pass string, port string, plausible string, storage StorageClient) *WebServer {
	webServer := new(WebServer)
	webServer.User = user
	webServer.Pass = pass
	webServer.Port = port
	webServer.Plausible = plausible
	webServer.storage = storage

	router := mux.NewRouter()
	router.Use(webServer.LoggingMiddleware)
	router.PathPrefix("/static/").Handler(http.FileServer(http.FS(static)))
	router.HandleFunc("/healthz", webServer.HealthHandler)
	router.HandleFunc(fmt.Sprintf("/{key:[a-zA-Z0-9-_=]{%d,}}", KEY_LENGTH), webServer.LookupHandler)

	if webServer.User == "" && webServer.Pass == "" {
		log.Println("Setting up without auth...")
		router.HandleFunc("/", webServer.IndexHandler).Methods("GET")
		router.HandleFunc("/", webServer.UploadHandler).Methods("POST")
	} else {
		log.Println("Setting up with basic auth...")
		router.HandleFunc("/", webServer.BasicAuthWrapper(webServer.IndexHandler)).Methods("GET")
		router.HandleFunc("/", webServer.BasicAuthWrapper(webServer.UploadHandler)).Methods("POST")
	}

	webServer.Router = router

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

func (webServer *WebServer) LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		log.Printf("%s %s", request.Method, request.URL)
		next.ServeHTTP(writer, request)
	})
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
	defer file.Close()

	url, err := webServer.storage.UploadFile(file, *header)

	if err != nil {
		webServer.ServeError(writer, err)
	} else {
		writer.Header().Set("Content-Type", "application/json")
		writer.Write([]byte(fmt.Sprintf("{\"url\":\"%s\"}", url)))
	}
}

func (webServer *WebServer) LookupHandler(writer http.ResponseWriter, request *http.Request) {
	vars := mux.Vars(request)
	key := vars["key"]

	file, err := webServer.storage.LookupFile(key)

	if err != nil {
		webServer.ServeError(writer, err)
	} else {
		webServer.ServeTemplate(writer, "file", file)
	}
}

func (webServer *WebServer) HealthHandler(writer http.ResponseWriter, request *http.Request) {
	writer.WriteHeader(http.StatusNoContent)
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
