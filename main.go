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
)

type Config struct {
	bucket string
	key    string
	secret string
	port   string // https://twitter.com/keith_duncan/status/638582305917833217
}

var s3Client *s3.Client
var fileCloudConfig Config

const BYTE_COUNT = 4

func main() {
	log.Println("File Cloud starting up...")
	flag.StringVar(&fileCloudConfig.bucket, "bucket", lookupEnvDefault("BUCKET", "file-cloud"), "AWS S3 Bucket name to store files in")
	flag.StringVar(&fileCloudConfig.key, "key", lookupEnvDefault("KEY", "ABC123"), "AWS Key to use")
	flag.StringVar(&fileCloudConfig.secret, "secret", lookupEnvDefault("SECRET", "ABC/123"), "AWS Secret to use")
	flag.StringVar(&fileCloudConfig.port, "port", lookupEnvDefault("PORT", "8080"), "Port to listen on") // https://twitter.com/keith_duncan/status/638582305917833217
	flag.Parse()

	creds := credentials.NewStaticCredentialsProvider(fileCloudConfig.key, fileCloudConfig.secret, "")
	cfg, err := config.LoadDefaultConfig(context.TODO(), config.WithCredentialsProvider(creds), config.WithRegion("us-west-1"))
	if err != nil {
		log.Fatal("Couldn't load S3 Credentials")
		os.Exit(1)
	}

	s3Client = s3.NewFromConfig(cfg)

	fs := http.FileServer(http.Dir("static/"))
	http.Handle("/static/", http.StripPrefix("/static/", fs))
	http.HandleFunc("/", index)

	log.Printf("Listening on port %s", fileCloudConfig.port)
	http.ListenAndServe(fmt.Sprintf(":%s", fileCloudConfig.port), nil)
}

func index(writer http.ResponseWriter, request *http.Request) {
	if request.Method == "POST" {
		log.Println("Processing upload...")
		file, handler, err := request.FormFile("file")
		if err != nil {
			log.Fatal(err)
			http.Error(writer, err.Error(), http.StatusInternalServerError)
		}
		defer file.Close()

		err = upload(handler.Filename, file)

		if err != nil {
			log.Fatal(err)
			http.Error(writer, err.Error(), http.StatusInternalServerError)
		} else {
			log.Println("File uploaded")
			writer.Write([]byte(fmt.Sprintf("Got file: %s", handler.Filename)))
		}
	} else {
		log.Println("Serving index")
		t, err := template.ParseFiles("index.tmpl.html")
		if err != nil {
			log.Fatal(err)
			http.Error(writer, err.Error(), http.StatusInternalServerError)
		}
		if err := t.Execute(writer, ""); err != nil {
			log.Fatal(err)
			http.Error(writer, err.Error(), http.StatusInternalServerError)
		}
	}
}

func upload(name string, file multipart.File) error {
	buffer := &bytes.Buffer{}
	tee := io.TeeReader(file, buffer)
	key, err := filename(tee)

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

func filename(file io.Reader) (string, error) {
	hasher := sha256.New()

	if _, err := io.Copy(hasher, file); err != nil {
		log.Fatal(err)
		return "", err
	}

	hash := hasher.Sum(nil)
	return base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(hash[0:BYTE_COUNT]), nil
}

func lookupEnvDefault(envKey, defaultValue string) string {
	value, exists := os.LookupEnv(envKey)

	if exists {
		return value
	} else {
		return defaultValue
	}
}
