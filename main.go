package main

import (
	"context"
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

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	fs := http.FileServer(http.Dir("static/"))
	http.Handle("/static/", http.StripPrefix("/static/", fs))
	http.HandleFunc("/", index)

	http.ListenAndServe(fmt.Sprintf(":%s", port), nil)
}

func index(writer http.ResponseWriter, request *http.Request) {
	if request.Method == "POST" {
		file, handler, err := request.FormFile("file")
		if err != nil {
			http.Error(writer, err.Error(), http.StatusInternalServerError)
		}
		defer file.Close()

		status, err := upload(file)

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

func upload(file multipart.File) (bool, error) {
	// https://481495170185.signin.aws.amazon.com/console
	key := os.Getenv("KEY")
	secret := os.Getenv("SECRET")
	creds := credentials.NewStaticCredentialsProvider(key, secret, "")

	cfg, err := config.LoadDefaultConfig(context.TODO(), config.WithCredentialsProvider(creds), config.WithRegion("us-west-1"))
	if err != nil {
		return false, err
	}

	awsS3Client := s3.NewFromConfig(cfg)
	_, err = awsS3Client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket: aws.String("file-cloud-test"),
		Key:    aws.String("lmaolol"),
		Body:   file,
	})

	return true, nil
}
