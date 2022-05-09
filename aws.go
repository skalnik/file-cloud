package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type AWSClient struct {
	Bucket   string
	Key      string
	Secret   string
	S3Client *s3.Client
}

type StoredFile struct {
	OriginalName string
	Url          string
	Image        bool
}

var ErrorObjectMissing = errors.New("Could not find object on S3")
var ErrorInvalidKey = errors.New("Encountered S3 object with unexpected key")

func (awsClient *AWSClient) init() {
	creds := credentials.NewStaticCredentialsProvider(awsClient.Key, awsClient.Secret, "")
	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithCredentialsProvider(creds),
		config.WithRegion("us-west-1"))
	if err != nil {
		log.Fatal("Couldn't load S3 Credentials")
	}

	awsClient.S3Client = s3.NewFromConfig(cfg)
}

func (awsClient *AWSClient) UploadFile(file multipart.File, fileHeader multipart.FileHeader) (string, error) {
	buffer := &bytes.Buffer{}
	tee := io.TeeReader(file, buffer)
	key, err := Filename(fileHeader.Filename, tee)

	if err != nil {
		return "", err
	}
	contentType := fileHeader.Header.Get("Content-Type")

	log.Printf("Uploading file as %s", key)

	_, err = awsClient.S3Client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket:      aws.String(awsClient.Bucket),
		Key:         aws.String(key),
		ContentType: aws.String(contentType),
		Body:        buffer,
	})

	if err != nil {
		return "", err
	}

	return fmt.Sprintf("/%s", key[0:KEY_LENGTH]), nil
}

func (awsClient *AWSClient) LookupFile(prefix string) (StoredFile, error) {
	listInput := &s3.ListObjectsV2Input{
		Bucket:  aws.String(awsClient.Bucket),
		Prefix:  aws.String(prefix),
		MaxKeys: 1,
	}

	objectList, err := awsClient.S3Client.ListObjectsV2(context.TODO(), listInput)
	if err != nil {
		return StoredFile{}, err
	}

	if objectList.KeyCount < 1 {
		return StoredFile{}, ErrorObjectMissing
	}

	objectKey := *objectList.Contents[0].Key

	input := &s3.GetObjectInput{
		Bucket: aws.String(awsClient.Bucket),
		Key:    aws.String(objectKey),
	}
	presign, err := s3.NewPresignClient(awsClient.S3Client).PresignGetObject(context.Background(), input)
	if err != nil {
		return StoredFile{}, err
	}

	object, err := awsClient.S3Client.GetObject(context.Background(), input)
	if err != nil {
		return StoredFile{}, ErrorObjectMissing
	}
	parts := strings.Split(objectKey, "/")
	if len(parts) < 2 {
		return StoredFile{}, ErrorInvalidKey
	}

	file := StoredFile{
		OriginalName: parts[1],
		Url:          presign.URL,
		Image:        strings.Split(*object.ContentType, "/")[0] == "image",
	}
	return file, nil
}

func Filename(originalName string, file io.Reader) (string, error) {
	hasher := sha256.New()

	if _, err := io.Copy(hasher, file); err != nil {
		log.Println(err)
		return "", err
	}

	hash := hasher.Sum(nil)
	encodedHash := base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(hash)
	filename := fmt.Sprintf("%s/%s", encodedHash, originalName)
	return filename, nil
}
