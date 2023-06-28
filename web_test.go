package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

type mockStorage struct {
	StorageClient
}

func (c *mockStorage) LookupFile(prefix string) (StoredFile, error) {
	return StoredFile{
		OriginalName: "file.txt",
		Url:          "http://cdn.example.com/file.txt",
		Image:        false,
	}, nil
}

func TestExtensionRedirect(t *testing.T) {
	mockClient := &mockStorage{}
	server := NewWebServer("", "", "", "", mockClient)

	request := httptest.NewRequest(http.MethodGet, "/ACAB1.txt", nil)
	responseRecorder := httptest.NewRecorder()
	server.Router.ServeHTTP(responseRecorder, request)
	response := responseRecorder.Result()

	if response.StatusCode != http.StatusMovedPermanently {
		t.Fatalf(
			`Expected redirect, but instead got %s`,
			response.Status,
		)
	}

	request = httptest.NewRequest(http.MethodGet, "/ACAB1.gif", nil)
	responseRecorder = httptest.NewRecorder()
	server.Router.ServeHTTP(responseRecorder, request)
	response = responseRecorder.Result()

	if response.StatusCode == http.StatusMovedPermanently {
		t.Fatalf(
			`Did not expect redirect, got it anyway`,
		)
	}
}
