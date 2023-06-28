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
	"net/url"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type StorageClient interface {
	UploadFile(file multipart.File, fileHeader multipart.FileHeader) (string, error)
	LookupFile(prefix string) (StoredFile, error)
}

type AWSClient struct {
	Bucket   string
	CDN      string
	S3Client *s3.Client
}

type StoredFile struct {
	OriginalName string
	Url          string
	Image        bool
}

var ErrorObjectMissing = errors.New("Could not find object on S3")
var ErrorInvalidKey = errors.New("Encountered S3 object with unexpected key")

func NewAWSClient(bucket string, secret string, key string, cdn string) (*AWSClient, error) {
	client := new(AWSClient)
	client.Bucket = bucket
	client.CDN = cdn

	creds := credentials.NewStaticCredentialsProvider(key, secret, "")
	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithCredentialsProvider(creds),
		config.WithRegion("us-west-1"))
	if err != nil {
		return nil, errors.New("Couldn't load S3 Credentials")
	}

	client.S3Client = s3.NewFromConfig(cfg)

	return client, nil
}

func (awsClient *AWSClient) UploadFile(file multipart.File, fileHeader multipart.FileHeader) (string, error) {
	buffer := &bytes.Buffer{}
	tee := io.TeeReader(file, buffer)
	key, err := Filename(fileHeader.Filename, tee)

	if err != nil {
		return "", err
	}

	_, err = awsClient.LookupFile(key)
	if err == nil {
		log.Printf("File with key %s already uploaded!", key)
		return fmt.Sprintf("/%s", key[0:KEY_LENGTH]), nil
	}

	contentType := fileHeader.Header.Get("Content-Type")
	body := bytes.NewReader(buffer.Bytes())

	log.Printf("Uploading file as %s with key %s", contentType, key)

	_, err = awsClient.S3Client.PutObject(context.Background(), &s3.PutObjectInput{
		Bucket:      aws.String(awsClient.Bucket),
		Key:         aws.String(key),
		ContentType: aws.String(contentType),
		Body:        body,
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

	objectList, err := awsClient.S3Client.ListObjectsV2(context.Background(), listInput)
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

	object, err := awsClient.S3Client.GetObject(context.Background(), input)
	if err != nil {
		return StoredFile{}, ErrorObjectMissing
	}

	parts := strings.Split(objectKey, "/")
	if len(parts) < 2 {
		return StoredFile{}, ErrorInvalidKey
	}

	var fileURL string
	if awsClient.CDN == "" {
		presign, err := s3.NewPresignClient(awsClient.S3Client).PresignGetObject(context.Background(), input)
		if err != nil {
			return StoredFile{}, err
		}

		fileURL = presign.URL
	} else {
		// Files with URL-unsafe characters mean we need to URL encode our object key
		escapedKey := url.QueryEscape(objectKey)
		fileURL = fmt.Sprintf("%s/%s", awsClient.CDN, escapedKey)
	}

	file := StoredFile{
		OriginalName: parts[1],
		Url:          fileURL,
		Image:        strings.Split(*object.ContentType, "/")[0] == "image",
	}
	return file, nil
}

func Filename(originalName string, file io.Reader) (string, error) {
	hasher := sha256.New()

	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}

	hash := hasher.Sum(nil)
	encodedHash := base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(hash)
	filename := fmt.Sprintf("%s/%s", encodedHash, originalName)
	return filename, nil
}
