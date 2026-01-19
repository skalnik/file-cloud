package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"mime/multipart"
	"net/textproto"
	"os"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	lru "github.com/hashicorp/golang-lru/v2"
)

// Mock S3 client for testing
type mockS3Client struct {
	putObjectFunc     func(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	listObjectsV2Func func(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
	headObjectFunc    func(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error)
}

func (m *mockS3Client) PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	if m.putObjectFunc != nil {
		return m.putObjectFunc(ctx, params, optFns...)
	}
	return &s3.PutObjectOutput{}, nil
}

func (m *mockS3Client) ListObjectsV2(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	if m.listObjectsV2Func != nil {
		return m.listObjectsV2Func(ctx, params, optFns...)
	}
	return &s3.ListObjectsV2Output{KeyCount: aws.Int32(0)}, nil
}

func (m *mockS3Client) HeadObject(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
	if m.headObjectFunc != nil {
		return m.headObjectFunc(ctx, params, optFns...)
	}
	return &s3.HeadObjectOutput{}, nil
}

// Mock presign client for testing
type mockPresignClient struct {
	presignGetObjectFunc func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.PresignOptions)) (*v4.PresignedHTTPRequest, error)
}

func (m *mockPresignClient) PresignGetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.PresignOptions)) (*v4.PresignedHTTPRequest, error) {
	if m.presignGetObjectFunc != nil {
		return m.presignGetObjectFunc(ctx, params, optFns...)
	}
	return &v4.PresignedHTTPRequest{URL: "https://presigned.example.com/file"}, nil
}

// Helper to create a mock multipart.FileHeader
func createMockFileHeader(filename string, content []byte, contentType string) (*multipart.FileHeader, *bytes.Buffer) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", `form-data; name="file"; filename="`+filename+`"`)
	h.Set("Content-Type", contentType)

	part, _ := writer.CreatePart(h)
	part.Write(content)
	writer.Close()

	reader := multipart.NewReader(body, writer.Boundary())
	form, _ := reader.ReadForm(int64(len(content) + 1024))
	fileHeader := form.File["file"][0]

	return fileHeader, body
}

func TestFilenameText(t *testing.T) {
	sampleFile, _ := os.Open("testdata/egg.txt")
	expectedName := "6ziwppLVo8-9ZA4RddxHhHWIXCznXwcVJmMDLSQhg7Y/egg.txt"
	newName, err := Filename("egg.txt", bufio.NewReader(sampleFile))

	if expectedName != newName || err != nil {
		t.Fatalf(
			`Filename("test.txt", io.Reader)) = %s, %v, want %s`,
			newName, err, expectedName,
		)
	}
}

func TestFilenameBinary(t *testing.T) {
	sampleFile, _ := os.Open("testdata/smol.gif")
	expectedName := "IoFqAN_p_NwwBj0icXq5y6s66yqOmETp13TSVtxIt8g/smol.gif"
	newName, err := Filename("smol.gif", bufio.NewReader(sampleFile))

	if expectedName != newName || err != nil {
		t.Fatalf(
			`Filename("smol.gif", io.Reader)) = %s, %v, want %s`,
			newName, err, expectedName,
		)
	}
}

func TestFilenameEmpty(t *testing.T) {
	// Empty file should still produce a valid hash
	reader := strings.NewReader("")
	newName, err := Filename("empty.txt", reader)

	if err != nil {
		t.Fatalf(`Filename with empty reader returned error: %v`, err)
	}

	// Should have format "hash/empty.txt"
	if !strings.HasSuffix(newName, "/empty.txt") {
		t.Fatalf(`Expected filename to end with /empty.txt, got %s`, newName)
	}

	// Hash should be the SHA-256 of empty string
	expectedHash := "47DEQpj8HBSa-_TImW-5JCeuQeRkm5NMpJWZG3hSuFU"
	if !strings.HasPrefix(newName, expectedHash) {
		t.Fatalf(`Expected hash prefix %s, got %s`, expectedHash, newName)
	}
}

func TestFilenameSameContentDifferentName(t *testing.T) {
	// Same content with different filenames should have same hash but different full name
	content := "hello world"
	reader1 := strings.NewReader(content)
	reader2 := strings.NewReader(content)

	name1, _ := Filename("file1.txt", reader1)
	name2, _ := Filename("file2.txt", reader2)

	// Extract hash portion (before the /)
	hash1 := strings.Split(name1, "/")[0]
	hash2 := strings.Split(name2, "/")[0]

	if hash1 != hash2 {
		t.Errorf(`Same content should produce same hash, got %s and %s`, hash1, hash2)
	}

	if name1 == name2 {
		t.Errorf(`Different filenames should produce different full names`)
	}
}

func TestFilenameSpecialCharacters(t *testing.T) {
	// Filename with special characters should be preserved
	reader := strings.NewReader("test")
	specialName := "file with spaces & symbols!.txt"
	newName, err := Filename(specialName, reader)

	if err != nil {
		t.Fatalf(`Filename with special characters returned error: %v`, err)
	}

	if !strings.HasSuffix(newName, "/"+specialName) {
		t.Fatalf(`Expected filename to preserve special characters, got %s`, newName)
	}
}

func TestCacheGetNilCache(t *testing.T) {
	// AWSClient with nil cache should return false for cache get
	client := &AWSClient{
		cache: nil,
	}

	value, found := client.cacheGet("anykey")

	if found {
		t.Error("Expected found to be false when cache is nil")
	}
	if value != nil {
		t.Error("Expected value to be nil when cache is nil")
	}
}

func TestCacheSetNilCache(t *testing.T) {
	// AWSClient with nil cache should return error for cache set
	client := &AWSClient{
		cache: nil,
	}

	err := client.cacheSet("anykey", &StoredFile{})

	if err == nil {
		t.Error("Expected error when setting cache with nil cache")
	}
}

// LookupFile tests

func TestLookupFileWithCDN(t *testing.T) {
	cache, _ := lru.New[string, *StoredFile](128)

	mockS3 := &mockS3Client{
		listObjectsV2Func: func(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			return &s3.ListObjectsV2Output{
				KeyCount: aws.Int32(1),
				Contents: []types.Object{
					{Key: aws.String("abc123/testfile.txt")},
				},
			}, nil
		},
		headObjectFunc: func(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
			return &s3.HeadObjectOutput{
				ContentType: aws.String("text/plain"),
			}, nil
		},
	}

	client := &AWSClient{
		Bucket:   "test-bucket",
		CDN:      "https://cdn.example.com",
		s3Client: mockS3,
		cache:    cache,
	}

	file, err := client.LookupFile("abc12")

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if file.OriginalName != "testfile.txt" {
		t.Errorf("Expected OriginalName 'testfile.txt', got '%s'", file.OriginalName)
	}

	expectedURL := "https://cdn.example.com/abc123%2Ftestfile.txt"
	if file.Url != expectedURL {
		t.Errorf("Expected URL '%s', got '%s'", expectedURL, file.Url)
	}

	if file.Image != false {
		t.Error("Expected Image to be false for text/plain")
	}
}

func TestLookupFileWithPresignedURL(t *testing.T) {
	mockS3 := &mockS3Client{
		listObjectsV2Func: func(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			return &s3.ListObjectsV2Output{
				KeyCount: aws.Int32(1),
				Contents: []types.Object{
					{Key: aws.String("abc123/testfile.txt")},
				},
			}, nil
		},
		headObjectFunc: func(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
			return &s3.HeadObjectOutput{
				ContentType: aws.String("text/plain"),
			}, nil
		},
	}

	mockPresign := &mockPresignClient{
		presignGetObjectFunc: func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.PresignOptions)) (*v4.PresignedHTTPRequest, error) {
			return &v4.PresignedHTTPRequest{URL: "https://s3.amazonaws.com/presigned-url"}, nil
		},
	}

	client := &AWSClient{
		Bucket:        "test-bucket",
		CDN:           "", // No CDN, should use presigned URL
		s3Client:      mockS3,
		presignClient: mockPresign,
		cache:         nil,
	}

	file, err := client.LookupFile("abc12")

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if file.Url != "https://s3.amazonaws.com/presigned-url" {
		t.Errorf("Expected presigned URL, got '%s'", file.Url)
	}
}

func TestLookupFileNotFound(t *testing.T) {
	mockS3 := &mockS3Client{
		listObjectsV2Func: func(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			return &s3.ListObjectsV2Output{
				KeyCount: aws.Int32(0),
				Contents: []types.Object{},
			}, nil
		},
	}

	client := &AWSClient{
		Bucket:   "test-bucket",
		CDN:      "https://cdn.example.com",
		s3Client: mockS3,
		cache:    nil,
	}

	_, err := client.LookupFile("nonexistent")

	if !errors.Is(err, ErrorObjectMissing) {
		t.Errorf("Expected ErrorObjectMissing, got %v", err)
	}
}

func TestLookupFileCacheHit(t *testing.T) {
	cache, _ := lru.New[string, *StoredFile](128)
	cachedFile := &StoredFile{
		OriginalName: "cached.txt",
		Url:          "https://cdn.example.com/cached",
		Image:        false,
	}
	cache.Add("abc12", cachedFile)

	// S3 client should not be called if cache hit
	mockS3 := &mockS3Client{
		listObjectsV2Func: func(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			t.Error("S3 ListObjectsV2 should not be called on cache hit")
			return nil, nil
		},
	}

	client := &AWSClient{
		Bucket:   "test-bucket",
		CDN:      "https://cdn.example.com",
		s3Client: mockS3,
		cache:    cache,
	}

	file, err := client.LookupFile("abc12")

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if file.OriginalName != "cached.txt" {
		t.Errorf("Expected cached file, got '%s'", file.OriginalName)
	}
}

func TestLookupFileInvalidKey(t *testing.T) {
	mockS3 := &mockS3Client{
		listObjectsV2Func: func(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			return &s3.ListObjectsV2Output{
				KeyCount: aws.Int32(1),
				Contents: []types.Object{
					{Key: aws.String("invalidkey")}, // No slash, invalid format
				},
			}, nil
		},
		headObjectFunc: func(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
			return &s3.HeadObjectOutput{
				ContentType: aws.String("text/plain"),
			}, nil
		},
	}

	client := &AWSClient{
		Bucket:   "test-bucket",
		CDN:      "https://cdn.example.com",
		s3Client: mockS3,
		cache:    nil,
	}

	_, err := client.LookupFile("invalid")

	if !errors.Is(err, ErrorInvalidKey) {
		t.Errorf("Expected ErrorInvalidKey, got %v", err)
	}
}

func TestLookupFileImageContentType(t *testing.T) {
	cache, _ := lru.New[string, *StoredFile](128)

	mockS3 := &mockS3Client{
		listObjectsV2Func: func(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			return &s3.ListObjectsV2Output{
				KeyCount: aws.Int32(1),
				Contents: []types.Object{
					{Key: aws.String("abc123/image.png")},
				},
			}, nil
		},
		headObjectFunc: func(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
			return &s3.HeadObjectOutput{
				ContentType: aws.String("image/png"),
			}, nil
		},
	}

	client := &AWSClient{
		Bucket:   "test-bucket",
		CDN:      "https://cdn.example.com",
		s3Client: mockS3,
		cache:    cache,
	}

	file, err := client.LookupFile("abc12")

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if !file.Image {
		t.Error("Expected Image to be true for image/png content type")
	}
}

func TestLookupFileListError(t *testing.T) {
	mockS3 := &mockS3Client{
		listObjectsV2Func: func(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			return nil, errors.New("S3 list error")
		},
	}

	client := &AWSClient{
		Bucket:   "test-bucket",
		CDN:      "https://cdn.example.com",
		s3Client: mockS3,
		cache:    nil,
	}

	_, err := client.LookupFile("abc12")

	if err == nil {
		t.Error("Expected error, got nil")
	}
}

func TestLookupFileGetObjectError(t *testing.T) {
	mockS3 := &mockS3Client{
		listObjectsV2Func: func(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			return &s3.ListObjectsV2Output{
				KeyCount: aws.Int32(1),
				Contents: []types.Object{
					{Key: aws.String("abc123/testfile.txt")},
				},
			}, nil
		},
		headObjectFunc: func(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
			return nil, errors.New("S3 get error")
		},
	}

	client := &AWSClient{
		Bucket:   "test-bucket",
		CDN:      "https://cdn.example.com",
		s3Client: mockS3,
		cache:    nil,
	}

	_, err := client.LookupFile("abc12")

	if !errors.Is(err, ErrorObjectMissing) {
		t.Errorf("Expected ErrorObjectMissing, got %v", err)
	}
}

func TestLookupFilePresignError(t *testing.T) {
	mockS3 := &mockS3Client{
		listObjectsV2Func: func(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			return &s3.ListObjectsV2Output{
				KeyCount: aws.Int32(1),
				Contents: []types.Object{
					{Key: aws.String("abc123/testfile.txt")},
				},
			}, nil
		},
		headObjectFunc: func(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
			return &s3.HeadObjectOutput{
				ContentType: aws.String("text/plain"),
			}, nil
		},
	}

	mockPresign := &mockPresignClient{
		presignGetObjectFunc: func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.PresignOptions)) (*v4.PresignedHTTPRequest, error) {
			return nil, errors.New("presign error")
		},
	}

	client := &AWSClient{
		Bucket:        "test-bucket",
		CDN:           "", // No CDN, will try presigned URL
		s3Client:      mockS3,
		presignClient: mockPresign,
		cache:         nil,
	}

	_, err := client.LookupFile("abc12")

	if err == nil {
		t.Error("Expected error, got nil")
	}
}

// UploadFile tests

func TestUploadFileSuccess(t *testing.T) {
	mockS3 := &mockS3Client{
		listObjectsV2Func: func(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			// File doesn't exist yet
			return &s3.ListObjectsV2Output{
				KeyCount: aws.Int32(0),
				Contents: []types.Object{},
			}, nil
		},
		putObjectFunc: func(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
			return &s3.PutObjectOutput{}, nil
		},
	}

	client := &AWSClient{
		Bucket:   "test-bucket",
		CDN:      "https://cdn.example.com",
		s3Client: mockS3,
		cache:    nil,
	}

	fileHeader, _ := createMockFileHeader("test.txt", []byte("test content"), "text/plain")
	file, _ := fileHeader.Open()
	defer file.Close()

	url, err := client.UploadFile(file, *fileHeader)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// URL should be /keyLength characters (5)
	if len(url) != 6 { // "/" + 5 chars
		t.Errorf("Expected URL length 6, got %d: %s", len(url), url)
	}

	if url[0] != '/' {
		t.Errorf("Expected URL to start with '/', got '%s'", url)
	}
}

func TestUploadFileAlreadyExists(t *testing.T) {
	cache, _ := lru.New[string, *StoredFile](128)

	mockS3 := &mockS3Client{
		listObjectsV2Func: func(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			// File already exists
			return &s3.ListObjectsV2Output{
				KeyCount: aws.Int32(1),
				Contents: []types.Object{
					{Key: aws.String(*params.Prefix + "/existing.txt")},
				},
			}, nil
		},
		headObjectFunc: func(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
			return &s3.HeadObjectOutput{
				ContentType: aws.String("text/plain"),
			}, nil
		},
		putObjectFunc: func(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
			t.Error("PutObject should not be called for existing file")
			return nil, nil
		},
	}

	client := &AWSClient{
		Bucket:   "test-bucket",
		CDN:      "https://cdn.example.com",
		s3Client: mockS3,
		cache:    cache,
	}

	fileHeader, _ := createMockFileHeader("test.txt", []byte("test content"), "text/plain")
	file, _ := fileHeader.Open()
	defer file.Close()

	url, err := client.UploadFile(file, *fileHeader)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if url == "" {
		t.Error("Expected URL, got empty string")
	}
}

func TestUploadFilePutObjectError(t *testing.T) {
	mockS3 := &mockS3Client{
		listObjectsV2Func: func(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			return &s3.ListObjectsV2Output{
				KeyCount: aws.Int32(0),
				Contents: []types.Object{},
			}, nil
		},
		putObjectFunc: func(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
			return nil, errors.New("S3 put error")
		},
	}

	client := &AWSClient{
		Bucket:   "test-bucket",
		CDN:      "https://cdn.example.com",
		s3Client: mockS3,
		cache:    nil,
	}

	fileHeader, _ := createMockFileHeader("test.txt", []byte("test content"), "text/plain")
	file, _ := fileHeader.Open()
	defer file.Close()

	_, err := client.UploadFile(file, *fileHeader)

	if err == nil {
		t.Error("Expected error, got nil")
	}
}

func TestUploadFileVerifiesContentType(t *testing.T) {
	var capturedContentType string

	mockS3 := &mockS3Client{
		listObjectsV2Func: func(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			return &s3.ListObjectsV2Output{
				KeyCount: aws.Int32(0),
				Contents: []types.Object{},
			}, nil
		},
		putObjectFunc: func(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
			capturedContentType = *params.ContentType
			return &s3.PutObjectOutput{}, nil
		},
	}

	client := &AWSClient{
		Bucket:   "test-bucket",
		CDN:      "https://cdn.example.com",
		s3Client: mockS3,
		cache:    nil,
	}

	fileHeader, _ := createMockFileHeader("image.png", []byte("fake png"), "image/png")
	file, _ := fileHeader.Open()
	defer file.Close()

	_, err := client.UploadFile(file, *fileHeader)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if capturedContentType != "image/png" {
		t.Errorf("Expected content type 'image/png', got '%s'", capturedContentType)
	}
}

// Test cache operations with actual cache

func TestCacheGetAndSet(t *testing.T) {
	cache, _ := lru.New[string, *StoredFile](128)

	client := &AWSClient{
		cache: cache,
	}

	testFile := &StoredFile{
		OriginalName: "test.txt",
		Url:          "https://example.com/test.txt",
		Image:        false,
	}

	// Test set
	err := client.cacheSet("testkey", testFile)
	if err != nil {
		t.Fatalf("Expected no error from cacheSet, got %v", err)
	}

	// Test get
	retrieved, found := client.cacheGet("testkey")
	if !found {
		t.Fatal("Expected to find cached item")
	}

	if retrieved.OriginalName != testFile.OriginalName {
		t.Errorf("Expected OriginalName '%s', got '%s'", testFile.OriginalName, retrieved.OriginalName)
	}
}

func TestCacheMiss(t *testing.T) {
	cache, _ := lru.New[string, *StoredFile](128)

	client := &AWSClient{
		cache: cache,
	}

	_, found := client.cacheGet("nonexistent")
	if found {
		t.Error("Expected cache miss for nonexistent key")
	}
}
