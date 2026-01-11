package main

import (
	"bufio"
	"os"
	"strings"
	"testing"
)

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
