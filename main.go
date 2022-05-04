package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"text/template"

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
	flag.StringVar(&fileCloudConfig.key,    "key",    LookupEnvDefault("KEY", "ABC123"), "AWS Key to use")
	flag.StringVar(&fileCloudConfig.secret, "secret", LookupEnvDefault("SECRET", "ABC/123"), "AWS Secret to use")
	flag.StringVar(&fileCloudConfig.port,   "port",   LookupEnvDefault("PORT", "8080"), "Port to listen on") // https://twitter.com/keith_duncan/status/638582305917833217
	flag.StringVar(&fileCloudConfig.user,   "user",   LookupEnvDefault("USERNAME", ""), "A username for basic auth. Leave blank (along with pass) to disable")
	flag.StringVar(&fileCloudConfig.pass,   "pass",   LookupEnvDefault("PASSWORD", ""), "A password for basic auth. Leave blank (along with user) to disable")
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
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%s", fileCloudConfig.port), router))
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
	t, err := template.ParseFiles("index.tmpl.html")
	if err != nil {
		log.Println(err)
		http.Error(writer, err.Error(), http.StatusInternalServerError)
	}
	if err := t.Execute(writer, ""); err != nil {
		log.Println(err)
		http.Error(writer, err.Error(), http.StatusInternalServerError)
	}
}

func UploadHandler(writer http.ResponseWriter, request *http.Request) {
	file, handler, err := request.FormFile("file")
	if err != nil {
		log.Println(err)
		http.Error(writer, err.Error(), http.StatusInternalServerError)
	}
	defer file.Close()

	err = UploadFile(file, handler.Header)

	if err != nil {
		log.Println(err)
		http.Error(writer, err.Error(), http.StatusInternalServerError)
	} else {
		// TODO Redirect to lookup URL for it
		writer.Write([]byte(fmt.Sprintf("Got file: %s", handler.Filename)))
	}
}

func LookupHandler(writer http.ResponseWriter, request *http.Request) {
	vars := mux.Vars(request)
	key := vars["key"]
	log.Printf("Got request for key: %s", key)

	response, err := s3.NewPresignClient(s3Client).PresignGetObject(context.TODO(), &s3.GetObjectInput{
		Bucket: aws.String(fileCloudConfig.bucket),
		Key:    aws.String(key),
	})

	if err != nil {
		log.Println(err)
		http.Error(writer, err.Error(), http.StatusInternalServerError)
	}

	log.Printf("%s", response.URL)
}

func UploadFile(file multipart.File, header textproto.MIMEHeader) error {
	buffer := &bytes.Buffer{}
	tee := io.TeeReader(file, buffer)
	key, err := Filename(tee)

	if err != nil {
		return err
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
		return err
	}

	return nil
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

func LookupEnvDefault(envKey, defaultValue string) string {
	value, exists := os.LookupEnv(envKey)

	if exists {
		return value
	} else {
		return defaultValue
	}
}
