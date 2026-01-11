package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	lru "github.com/hashicorp/golang-lru/v2"
)

type StorageClient interface {
	UploadFile(file multipart.File, fileHeader multipart.FileHeader) (string, error)
	LookupFile(prefix string) (*StoredFile, error)
}

// S3API defines the S3 operations used by AWSClient
type S3API interface {
	PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	ListObjectsV2(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
	GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
}

// S3PresignAPI defines the presigning operations used by AWSClient
type S3PresignAPI interface {
	PresignGetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.PresignOptions)) (*v4.PresignedHTTPRequest, error)
}

type AWSClient struct {
	Bucket        string
	CDN           string
	s3Client      S3API
	presignClient S3PresignAPI
	cache         *lru.Cache[string, *StoredFile]
}

type StoredFile struct {
	OriginalName string
	Url          string
	Image        bool
}

var ErrorObjectMissing = errors.New("could not find object on S3")
var ErrorInvalidKey = errors.New("encountered S3 object with unexpected key")

const s3Timeout = 30 * time.Second

func formatKey(key string) string {
	return fmt.Sprintf("/%s", key[0:KEY_LENGTH])
}

func NewAWSClient(bucket string, secret string, key string, cdn string) (*AWSClient, error) {
	client := new(AWSClient)
	client.Bucket = bucket
	client.CDN = cdn

	creds := credentials.NewStaticCredentialsProvider(key, secret, "")
	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithCredentialsProvider(creds),
		config.WithRegion("us-west-1"))
	if err != nil {
		return nil, errors.New("couldn't load S3 Credentials")
	}

	s3Client := s3.NewFromConfig(cfg)
	client.s3Client = s3Client
	client.presignClient = s3.NewPresignClient(s3Client)

	// We don't want to cache presigned URLs
	if cdn != "" {
		cache, err := lru.New[string, *StoredFile](128)
		if err != nil {
			return nil, errors.New("couldn't initialize cache")
		}
		client.cache = cache
	} else {
		slog.Info("Not setting up cache due to lack of CDN")
	}

	return client, nil
}

func (awsClient *AWSClient) UploadFile(file multipart.File, fileHeader multipart.FileHeader) (string, error) {
	key, err := Filename(fileHeader.Filename, file)
	if err != nil {
		return "", err
	}

	_, err = file.Seek(0, 0)
	if err != nil {
		return "", err
	}

	awsFile, err := awsClient.LookupFile(key)
	if awsFile != nil {
		slog.Debug("File already uploaded", "key", key)
		return formatKey(key), nil
	}

	// Object missing is to be expected here, since we're uploading a new file
	if err != nil && !errors.Is(err, ErrorObjectMissing) {
		return "", err
	}

	contentType := fileHeader.Header.Get("Content-Type")

	slog.Debug("Uploading file", "contentType", contentType, "key", key)

	_, err = awsClient.s3Client.PutObject(context.Background(), &s3.PutObjectInput{
		Bucket:      aws.String(awsClient.Bucket),
		Key:         aws.String(key),
		ContentType: aws.String(contentType),
		Body:        file,
	})

	if err != nil {
		return "", err
	}

	return formatKey(key), nil
}

func (awsClient *AWSClient) LookupFile(prefix string) (*StoredFile, error) {
	value, found := awsClient.cacheGet(prefix)

	if found {
		return value, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), s3Timeout)
	defer cancel()

	listInput := &s3.ListObjectsV2Input{
		Bucket:  aws.String(awsClient.Bucket),
		Prefix:  aws.String(prefix),
		MaxKeys: aws.Int32(1),
	}

	objectList, err := awsClient.s3Client.ListObjectsV2(ctx, listInput)
	if err != nil {
		return nil, err
	}

	if *objectList.KeyCount < 1 {
		return nil, ErrorObjectMissing
	}

	objectKey := *objectList.Contents[0].Key

	input := &s3.GetObjectInput{
		Bucket: aws.String(awsClient.Bucket),
		Key:    aws.String(objectKey),
	}

	object, err := awsClient.s3Client.GetObject(ctx, input)
	if err != nil {
		return nil, ErrorObjectMissing
	}

	parts := strings.Split(objectKey, "/")
	if len(parts) < 2 {
		return nil, ErrorInvalidKey
	}

	var fileURL string
	if awsClient.CDN == "" {
		presign, err := awsClient.presignClient.PresignGetObject(ctx, input)
		if err != nil {
			return nil, err
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

	err = awsClient.cacheSet(prefix, &file)
	if err != nil {
		slog.Warn("Error setting cache", "error", err)
	}

	return &file, nil
}

func (awsClient *AWSClient) cacheGet(key string) (*StoredFile, bool) {
	if awsClient.cache == nil {
		return nil, false
	}

	value, found := awsClient.cache.Get(key)

	if !found {
		slog.Debug("Cache miss", "key", key)
		return nil, false
	}

	slog.Debug("Cache hit", "key", key)
	return value, true
}

func (awsClient *AWSClient) cacheSet(key string, file *StoredFile) error {
	if awsClient.cache == nil {
		return errors.New("no cache initailized")
	}

	awsClient.cache.Add(key, file)

	return nil
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
