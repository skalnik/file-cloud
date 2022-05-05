package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"flag"
	"html/template"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gorilla/mux"
)

type Config struct {
	// AWS
	bucket string
	key    string
	secret string
	// Basic Auth
	user string
	pass string
	// General
	port string // https://twitter.com/keith_duncan/status/638582305917833217
}

var s3Client *s3.Client
var fileCloudConfig Config

const BYTE_COUNT = 5

func main() {
	log.Println("File Cloud starting up...")
	flag.StringVar(&fileCloudConfig.bucket, "bucket", LookupEnvDefault("BUCKET", "file-cloud"), "AWS S3 Bucket name to store files in")
	flag.StringVar(&fileCloudConfig.secret, "secret", LookupEnvDefault("SECRET", "ABC/123"), "AWS Secret to use")
	flag.StringVar(&fileCloudConfig.key, "key", LookupEnvDefault("KEY", "ABC123"), "AWS Key to use")
	flag.StringVar(&fileCloudConfig.port, "port", LookupEnvDefault("PORT", "8080"), "Port to listen on") // https://twitter.com/keith_duncan/status/638582305917833217
	flag.StringVar(&fileCloudConfig.user, "user", LookupEnvDefault("USERNAME", ""), "A username for basic auth. Leave blank (along with pass) to disable")
	flag.StringVar(&fileCloudConfig.pass, "pass", LookupEnvDefault("PASSWORD", ""), "A password for basic auth. Leave blank (along with user) to disable")
	flag.Parse()

	creds := credentials.NewStaticCredentialsProvider(fileCloudConfig.key, fileCloudConfig.secret, "")
	cfg, err := config.LoadDefaultConfig(context.TODO(), config.WithCredentialsProvider(creds), config.WithRegion("us-west-1"))
	if err != nil {
		log.Fatal("Couldn't load S3 Credentials")
		os.Exit(1)
	}

	s3Client = s3.NewFromConfig(cfg)

	router := mux.NewRouter()
	router.Use(LoggingMiddleware)
	router.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir("static/"))))
	router.HandleFunc("/", IndexHandler).Methods("GET")
	router.HandleFunc("/", UploadHandler).Methods("POST")
	router.HandleFunc("/{key:[a-zA-Z0-9-_=]+}", LookupHandler)

	if fileCloudConfig.user == "" && fileCloudConfig.pass == "" {
		log.Println("Setting up without auth...")
	} else {
		log.Println("Setting up with basic auth...")
		router.Use(BasicAuthMiddleware)
	}

	log.Printf("Listening on port %s", fileCloudConfig.port)
	log.Fatal(http.ListenAndServe(":"+fileCloudConfig.port, router))
}

func BasicAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		user, pass, ok := request.BasicAuth()

		if !ok {
			log.Println("Couldn't parse basic auth")
		} else {
			if user == fileCloudConfig.user && pass == fileCloudConfig.pass {
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
		log.Printf("Handling %s %s", request.Method, request.URL)
		next.ServeHTTP(writer, request)
	})
}

func IndexHandler(writer http.ResponseWriter, request *http.Request) {
	t, err := template.ParseFiles("templates/index.tmpl.html")
	if err != nil {
		ServeError(writer, err)
	}
	if err := t.Execute(writer, ""); err != nil {
		ServeError(writer, err)
	}
}

func UploadHandler(writer http.ResponseWriter, request *http.Request) {
	file, handler, err := request.FormFile("file")
	if err != nil {
		ServeError(writer, err)
		return
	}
	defer file.Close()

	url, err := UploadFile(file, handler.Header)

	if err != nil {
		ServeError(writer, err)
	} else {
		writer.Header().Set("Content-Type", "application/json")
		writer.Write([]byte("{\"url\":\"" + url + "\"}"))
	}
}

func LookupHandler(writer http.ResponseWriter, request *http.Request) {
	vars := mux.Vars(request)
	key := vars["key"]

	input := &s3.GetObjectInput{
		Bucket: aws.String(fileCloudConfig.bucket),
		Key:    aws.String(key),
	}

	presign, err := s3.NewPresignClient(s3Client).PresignGetObject(context.TODO(), input)
	if err != nil {
		ServeError(writer, err)
		return
	}
	object, err := s3Client.GetObject(context.TODO(), input)
	if err != nil {
		//TODO Serve 404 page
		ServeError(writer, err)
		return
	}

	templateData := struct {
		Key string
		Url string
	}{
		Key: key,
		Url: presign.URL,
	}

	var t *template.Template
	if strings.Split(*object.ContentType, "/")[0] == "image" {
		t, err = template.ParseFiles("templates/img.tmpl.html")
	} else {
		t, err = template.ParseFiles("templates/file.tmpl.html")
	}
	if err != nil {
		log.Println(err)
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}

	if err = t.Execute(writer, templateData); err != nil {
		log.Println(err)
		http.Error(writer, err.Error(), http.StatusInternalServerError)
	}
}

func UploadFile(file multipart.File, header textproto.MIMEHeader) (string, error) {
	buffer := &bytes.Buffer{}
	tee := io.TeeReader(file, buffer)
	key, err := Filename(tee)

	if err != nil {
		return "", err
	}
	contentType := header.Get("Content-Type")

	log.Printf("Uploading file as %s", key)

	_, err = s3Client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket:      aws.String(fileCloudConfig.bucket),
		Key:         aws.String(key),
		ContentType: aws.String(contentType),
		Body:        buffer,
	})

	if err != nil {
		return "", err
	}

	return "/" + key, nil
}

func Filename(file io.Reader) (string, error) {
	hasher := sha256.New()

	if _, err := io.Copy(hasher, file); err != nil {
		log.Println(err)
		return "", err
	}

	hash := hasher.Sum(nil)
	return base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(hash[0:BYTE_COUNT]), nil
}

func ServeError(writer http.ResponseWriter, err error) {
	log.Println("\033[31m" + err.Error() + "\033[0m")
	http.Error(writer, err.Error(), http.StatusInternalServerError)
}

func LookupEnvDefault(envKey, defaultValue string) string {
	value, exists := os.LookupEnv(envKey)

	if exists {
		return value
	} else {
		return defaultValue
	}
}
