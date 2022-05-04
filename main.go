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
	flag.StringVar(&fileCloudConfig.bucket, "bucket", lookupEnvDefault("BUCKET", "file-cloud"), "AWS S3 Bucket name to store files in")
	flag.StringVar(&fileCloudConfig.key, "key", lookupEnvDefault("KEY", "ABC123"), "AWS Key to use")
	flag.StringVar(&fileCloudConfig.secret, "secret", lookupEnvDefault("SECRET", "ABC/123"), "AWS Secret to use")
	flag.StringVar(&fileCloudConfig.port, "port", lookupEnvDefault("PORT", "8080"), "Port to listen on") // https://twitter.com/keith_duncan/status/638582305917833217
	flag.StringVar(&fileCloudConfig.user, "user", lookupEnvDefault("USERNAME", ""), "A username for basic auth. Leave blank (along with pass) to disable")
	flag.StringVar(&fileCloudConfig.pass, "pass", lookupEnvDefault("PASSWORD", ""), "A password for basic auth. Leave blank (along with user) to disable")
	flag.Parse()

	creds := credentials.NewStaticCredentialsProvider(fileCloudConfig.key, fileCloudConfig.secret, "")
	cfg, err := config.LoadDefaultConfig(context.TODO(), config.WithCredentialsProvider(creds), config.WithRegion("us-west-1"))
	if err != nil {
		log.Fatal("Couldn't load S3 Credentials")
		os.Exit(1)
	}

	s3Client = s3.NewFromConfig(cfg)

	router := mux.NewRouter()
	router.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir("static/"))))
	router.HandleFunc("/", IndexHandler).Methods("GET")
	router.HandleFunc("/", UploadHandler).Methods("POST")
	router.HandleFunc("/{id:[a-zA-Z0-9-_=]+}", LookupHandler)

	if fileCloudConfig.user == "" && fileCloudConfig.pass == "" {
		log.Println("Setting up without auth...")
	} else {
		log.Println("Setting up with basic auth...")
		router.Use(BasicAuthWrapper)
	}

	log.Printf("Listening on port %s", fileCloudConfig.port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%s", fileCloudConfig.port), router))
}

func BasicAuthWrapper(next http.Handler) http.Handler {
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

		writer.Header().Set("WWW-Authenticate", `Basic realm="restricted", charset="UTF-8"`)
		http.Error(writer, "Unauthorized", http.StatusUnauthorized)
	})
}

func IndexHandler(writer http.ResponseWriter, request *http.Request) {
	log.Println("Serving index")
	t, err := template.ParseFiles("index.tmpl.html")
	if err != nil {
		log.Fatal(err)
		http.Error(writer, err.Error(), http.StatusInternalServerError)
	}
	if err := t.Execute(writer, ""); err != nil {
		log.Println(err)
		http.Error(writer, err.Error(), http.StatusInternalServerError)
	}
}

func UploadHandler(writer http.ResponseWriter, request *http.Request) {
	log.Println("Processing upload...")
	file, handler, err := request.FormFile("file")
	if err != nil {
		log.Fatal(err)
		http.Error(writer, err.Error(), http.StatusInternalServerError)
	}
	defer file.Close()

	err = UploadFile(handler.Filename, file)

	if err != nil {
		log.Fatal(err)
		http.Error(writer, err.Error(), http.StatusInternalServerError)
	} else {
		log.Println("File uploaded")
		writer.Write([]byte(fmt.Sprintf("Got file: %s", handler.Filename)))
	}
}

func LookupHandler(writer http.ResponseWriter, request *http.Request) {
	log.Printf("Got request with URL: %s", request.URL)
}

func UploadFile(name string, file multipart.File) error {
	buffer := &bytes.Buffer{}
	tee := io.TeeReader(file, buffer)
	key, err := Filename(tee)

	if err != nil {
		return err
	}

	log.Printf("Uploading file as %s", key)

	_, err = s3Client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket: aws.String(fileCloudConfig.bucket),
		Key:    aws.String(key),
		Body:   buffer,
	})

	if err != nil {
		return err
	}

	return nil
}

func Filename(file io.Reader) (string, error) {
	hasher := sha256.New()

	if _, err := io.Copy(hasher, file); err != nil {
		log.Fatal(err)
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
