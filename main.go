package main

import (
	"context"
	"flag"
	"fmt"
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

func main() {
	flag.StringVar(&fileCloudConfig.bucket, "bucket", lookupEnvDefault("BUCKET", "file-cloud"), "AWS S3 Bucket name to store files in")
	flag.StringVar(&fileCloudConfig.key, "key", lookupEnvDefault("KEY", "ABC123"), "AWS Key to use")
	flag.StringVar(&fileCloudConfig.secret, "secret", lookupEnvDefault("SECRET", "ABC/123"), "AWS Secret to use")
	flag.StringVar(&fileCloudConfig.port, "port", lookupEnvDefault("PORT", "8080"), "Port to listen on") // https://twitter.com/keith_duncan/status/638582305917833217
	flag.Parse()

	creds := credentials.NewStaticCredentialsProvider(fileCloudConfig.key, fileCloudConfig.secret, "")
	cfg, err := config.LoadDefaultConfig(context.TODO(), config.WithCredentialsProvider(creds), config.WithRegion("us-west-1"))
	if err != nil {
		fmt.Println("Shits fucked")
		os.Exit(1)
	}

	s3Client = s3.NewFromConfig(cfg)

	fs := http.FileServer(http.Dir("static/"))
	http.Handle("/static/", http.StripPrefix("/static/", fs))
	http.HandleFunc("/", index)

	http.ListenAndServe(fmt.Sprintf(":%s", fileCloudConfig.port), nil)
}

func index(writer http.ResponseWriter, request *http.Request) {
	if request.Method == "POST" {
		file, handler, err := request.FormFile("file")
		if err != nil {
			http.Error(writer, err.Error(), http.StatusInternalServerError)
		}
		defer file.Close()

		status, err := upload(handler.Filename, file)

		if err != nil {
			http.Error(writer, err.Error(), http.StatusInternalServerError)
		}

		if status {
			writer.Write([]byte(fmt.Sprintf("Got file: %s", handler.Filename)))
		} else {
			writer.Write([]byte(fmt.Sprintf("Something is fucked")))
		}
	} else {
		t, err := template.ParseFiles("index.tmpl.html")
		if err != nil {
			http.Error(writer, err.Error(), http.StatusInternalServerError)
		}
		if err := t.Execute(writer, ""); err != nil {
			http.Error(writer, err.Error(), http.StatusInternalServerError)
		}
	}
}

func upload(name string, file multipart.File) (bool, error) {
	_, err := s3Client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket: aws.String(fileCloudConfig.bucket),
		Key:    aws.String("lmaolol"),
		Body:   file,
	})

	if err != nil {
		return false, err
	}

	return true, nil
}

func lookupEnvDefault(envKey, defaultValue string) string {
	value, exists := os.LookupEnv(envKey)

	if exists {
		return value
	} else {
		return defaultValue
	}
}
