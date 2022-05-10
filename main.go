package main

import (
	"errors"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"

	"github.com/gorilla/mux"
)

type Config struct {
	// Basic Auth
	User string
	Pass string
	// General
	Port      string // https://twitter.com/keith_duncan/status/638582305917833217
	Plausible string // Plausible domain
}

var awsClient AWSClient
var fileCloudConfig Config

const KEY_LENGTH = 5

func main() {
	log.Println("File Cloud starting up...")
	flag.StringVar(&awsClient.Bucket, "bucket", LookupEnvDefault("BUCKET", "file-cloud"), "AWS S3 Bucket name to store files in")
	flag.StringVar(&awsClient.Secret, "secret", LookupEnvDefault("SECRET", "ABC/123"), "AWS Secret to use")
	flag.StringVar(&awsClient.Key, "key", LookupEnvDefault("KEY", "ABC123"), "AWS Key to use")
	flag.StringVar(&fileCloudConfig.Port, "port", LookupEnvDefault("PORT", "8080"), "Port to listen on") // https://twitter.com/keith_duncan/status/638582305917833217
	flag.StringVar(&fileCloudConfig.User, "user", LookupEnvDefault("USERNAME", ""), "A username for basic auth. Leave blank (along with pass) to disable")
	flag.StringVar(&fileCloudConfig.Pass, "pass", LookupEnvDefault("PASSWORD", ""), "A password for basic auth. Leave blank (along with user) to disable")
	flag.StringVar(&fileCloudConfig.Plausible, "plausible", LookupEnvDefault("PLAUSIBLE", ""), "The domain setup for Plausible. Leave blank to disable")
	flag.Parse()

	awsClient.init()

	router := mux.NewRouter()
	router.Use(LoggingMiddleware)
	router.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir("static/"))))
	router.HandleFunc("/healthz", HealthHandler)
	router.HandleFunc(fmt.Sprintf("/{key:[a-zA-Z0-9-_=]{%d,}}", KEY_LENGTH), LookupHandler)

	if fileCloudConfig.User == "" && fileCloudConfig.Pass == "" {
		log.Println("Setting up without auth...")
		router.HandleFunc("/", IndexHandler).Methods("GET")
		router.HandleFunc("/", UploadHandler).Methods("POST")
	} else {
		log.Println("Setting up with basic auth...")
		router.HandleFunc("/", BasicAuthWrapper(IndexHandler)).Methods("GET")
		router.HandleFunc("/", BasicAuthWrapper(UploadHandler)).Methods("POST")
	}

	log.Printf("Listening on port %s", fileCloudConfig.Port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%s", fileCloudConfig.Port), router))
}

func BasicAuthWrapper(next http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		user, pass, ok := request.BasicAuth()

		if !ok {
			log.Println("Couldn't parse basic auth")
		} else {
			if user == fileCloudConfig.User && pass == fileCloudConfig.Pass {
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

func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		log.Printf("%s %s", request.Method, request.URL)
		next.ServeHTTP(writer, request)
	})
}

func IndexHandler(writer http.ResponseWriter, request *http.Request) {
	ServeTemplate(writer, "index", StoredFile{})
}

func UploadHandler(writer http.ResponseWriter, request *http.Request) {
	file, header, err := request.FormFile("file")
	if err != nil {
		ServeError(writer, err)
		return
	}
	defer file.Close()

	url, err := awsClient.UploadFile(file, *header)

	if err != nil {
		ServeError(writer, err)
	} else {
		writer.Header().Set("Content-Type", "application/json")
		writer.Write([]byte(fmt.Sprintf("{\"url\":\"%s\"}", url)))
	}
}

func LookupHandler(writer http.ResponseWriter, request *http.Request) {
	vars := mux.Vars(request)
	key := vars["key"]

	file, err := awsClient.LookupFile(key)

	if err != nil {
		ServeError(writer, err)
	} else {
		ServeTemplate(writer, "file", file)
	}
}

func HealthHandler(writer http.ResponseWriter, request *http.Request) {
	writer.WriteHeader(http.StatusNoContent)
}

func ServeError(writer http.ResponseWriter, err error) {
	log.Printf("\033[31m%s\033[0m", err.Error())

	if errors.Is(err, ErrorObjectMissing) {
		writer.WriteHeader(http.StatusNotFound)
		ServeTemplate(writer, "404", StoredFile{})
	} else {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
	}
}

func ServeTemplate(writer http.ResponseWriter, name string, data StoredFile) {
	t, err := template.ParseFiles("templates/layout.tmpl.html", fmt.Sprintf("templates/%s.tmpl.html", name))
	if err != nil {
		ServeError(writer, err)
		return
	}

	templateData := struct {
		Plausible string
		StoredFile
	}{
		Plausible:  fileCloudConfig.Plausible,
		StoredFile: data,
	}

	err = t.ExecuteTemplate(writer, "layout", templateData)
	if err != nil {
		ServeError(writer, err)
	}
}

func LookupEnvDefault(envKey, defaultValue string) string {
	value, exists := os.LookupEnv(envKey)

	if exists {
		return value
	} else {
		return defaultValue
	}
}
