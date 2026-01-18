package main

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
)

type mockStorage struct {
	StorageClient
}

func (c *mockStorage) LookupFile(prefix string) (*StoredFile, error) {
	return &StoredFile{
		OriginalName: "file.txt",
		Url:          "http://cdn.example.com/file.txt",
		Image:        false,
	}, nil
}

func (c *mockStorage) UploadFile(file multipart.File, fileHeader multipart.FileHeader) (string, error) {
	return "/ABCDE", nil
}

type mockImageStorage struct {
	StorageClient
}

func (c *mockImageStorage) LookupFile(prefix string) (*StoredFile, error) {
	return &StoredFile{
		OriginalName: "image.png",
		Url:          "http://cdn.example.com/image.png",
		Image:        true,
	}, nil
}

func (c *mockImageStorage) UploadFile(file multipart.File, fileHeader multipart.FileHeader) (string, error) {
	return "/ABCDE", nil
}

type mockEmptyStorage struct {
	StorageClient
}

func (c *mockEmptyStorage) LookupFile(prefix string) (*StoredFile, error) {
	return nil, ErrorObjectMissing
}

func TestBasicAuth(t *testing.T) {
	username := "skalnik"
	password := "hunter2"
	mockClient := &mockStorage{}
	server := NewWebServer(username, password, "", "", mockClient)

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	responseRecorder := httptest.NewRecorder()
	server.Router.ServeHTTP(responseRecorder, request)
	response := responseRecorder.Result()
	if response.StatusCode != http.StatusUnauthorized {
		t.Errorf(`Expected unauthorized, but instead got %s`, response.Status)
	}

	request.SetBasicAuth(username, password)

	responseRecorder = httptest.NewRecorder()
	server.Router.ServeHTTP(responseRecorder, request)
	response = responseRecorder.Result()
	if response.StatusCode != http.StatusOK {
		t.Errorf(`Expected 200 OK, but instead got %s`, response.Status)
	}
}

func TestExtensionRedirect(t *testing.T) {
	mockClient := &mockStorage{}
	server := NewWebServer("", "", "", "", mockClient)

	request := httptest.NewRequest(http.MethodGet, "/ACAB1.txt", nil)
	responseRecorder := httptest.NewRecorder()
	server.Router.ServeHTTP(responseRecorder, request)
	response := responseRecorder.Result()

	if response.StatusCode != http.StatusMovedPermanently {
		t.Errorf(
			`Expected redirect, but instead got %s`,
			response.Status,
		)
	}

	request = httptest.NewRequest(http.MethodGet, "/ACAB1.TXT", nil)
	responseRecorder = httptest.NewRecorder()
	server.Router.ServeHTTP(responseRecorder, request)
	response = responseRecorder.Result()

	if response.StatusCode != http.StatusMovedPermanently {
		t.Errorf(
			`Expected redirect, but instead got %s`,
			response.Status,
		)
	}

	request = httptest.NewRequest(http.MethodGet, "/ACAB1.gif", nil)
	responseRecorder = httptest.NewRecorder()
	server.Router.ServeHTTP(responseRecorder, request)
	response = responseRecorder.Result()

	if response.StatusCode == http.StatusMovedPermanently {
		t.Error(
			`Did not expect redirect, got it anyway`,
		)
	}
}

func TestPlausibleEvent(t *testing.T) {
	userAgent := "golang test"
	requestIP := "127.0.0.1"
	domain := "example.com"
	url := "/acab1.txt"

	plausible := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Expected Content-Type: application/json header, got: %s", request.Header.Get("Accept"))
		}
		if request.Header.Get("User-Agent") != userAgent {
			t.Errorf("Expected User-Agent: %s header, got: %s", userAgent, request.Header.Get("Accept"))
		}
		if request.Header.Get("X-Forwarded-For") != requestIP {
			t.Errorf("Expected X-Forwarded-For: %s header, got: %s", requestIP, request.Header.Get("X-Forwarded-For"))
		}

		content, err := io.ReadAll(request.Body)
		if err != nil {
			t.Error("Got malformed body")
		}

		event := &plausibleEvent{}
		err = json.Unmarshal(content, event)
		if err != nil {
			t.Error("Got malformed JSON")
		}

		if event.Name != "pageview" {
			t.Errorf(`Expected {name: "pageview"} but got {name: %s}`, event.Name)
		}
		if event.Domain != domain {
			t.Errorf(`Expected {domain: %s} but got {domain: %s}`, domain, event.Domain)
		}
		if event.URL != url {
			t.Errorf(`Expected {url: %s} but got {url: %s}`, url, event.URL)
		}

		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write([]byte(`ok`))
	}))
	defer plausible.Close()

	mockClient := &mockStorage{}
	server := NewWebServer("", "", "", domain, mockClient)

	var body bytes.Buffer
	request, _ := http.NewRequest(http.MethodGet, url, &body)
	request.Header.Add("User-Agent", userAgent)
	request.RemoteAddr = requestIP

	server.logPlausibleEvent(*request, plausible.URL)

	responseRecorder := httptest.NewRecorder()
	server.Router.ServeHTTP(responseRecorder, request)
	response := responseRecorder.Result()
	if response.StatusCode != http.StatusMovedPermanently {
		t.Fatalf(
			`Expected redirect, but instead got %s`,
			response.Status,
		)
	}
}

func Test404(t *testing.T) {
	mockClient := &mockEmptyStorage{}
	server := NewWebServer("", "", "", "", mockClient)

	request := httptest.NewRequest(http.MethodGet, "/ACAB1", nil)
	responseRecorder := httptest.NewRecorder()
	server.Router.ServeHTTP(responseRecorder, request)
	response := responseRecorder.Result()

	if response.StatusCode != 404 {
		t.Errorf(
			`Expected 404, but instead got %s`,
			response.Status,
		)
	}

	var title = regexp.MustCompile(`<title>\s+No file found!\s+</title>`)
	if title.FindString(responseRecorder.Body.String()) == "" {
		t.Errorf(
			`Could not find expected title in body: %s`,
			responseRecorder.Body.String(),
		)
	}
}

func TestHeartbeat(t *testing.T) {
	mockClient := &mockStorage{}
	server := NewWebServer("", "", "", "", mockClient)

	request := httptest.NewRequest(http.MethodGet, "/ping", nil)
	responseRecorder := httptest.NewRecorder()
	server.Router.ServeHTTP(responseRecorder, request)
	response := responseRecorder.Result()

	if response.StatusCode != http.StatusOK {
		t.Errorf(`Expected 200 OK, but instead got %s`, response.Status)
	}

	body, _ := io.ReadAll(response.Body)
	if string(body) != "." {
		t.Errorf(`Expected ".", but got %s`, string(body))
	}
}

func TestIndexHandler(t *testing.T) {
	mockClient := &mockStorage{}
	server := NewWebServer("", "", "", "", mockClient)

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	responseRecorder := httptest.NewRecorder()
	server.Router.ServeHTTP(responseRecorder, request)
	response := responseRecorder.Result()

	if response.StatusCode != http.StatusOK {
		t.Errorf(`Expected 200 OK, but instead got %s`, response.Status)
	}

	var title = regexp.MustCompile(`<title>\s+File Cloud\s+</title>`)
	if title.FindString(responseRecorder.Body.String()) == "" {
		t.Errorf(
			`Could not find expected title in body: %s`,
			responseRecorder.Body.String(),
		)
	}
}

func TestIndexHandlerWithPlausible(t *testing.T) {
	mockClient := &mockStorage{}
	server := NewWebServer("", "", "", "example.com", mockClient)

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	responseRecorder := httptest.NewRecorder()
	server.Router.ServeHTTP(responseRecorder, request)
	response := responseRecorder.Result()

	if response.StatusCode != http.StatusOK {
		t.Errorf(`Expected 200 OK, but instead got %s`, response.Status)
	}

	var plausibleScript = regexp.MustCompile(`data-domain="example.com"`)
	if plausibleScript.FindString(responseRecorder.Body.String()) == "" {
		t.Errorf(
			`Could not find Plausible script in body: %s`,
			responseRecorder.Body.String(),
		)
	}
}

func TestUploadHandler(t *testing.T) {
	mockClient := &mockStorage{}
	server := NewWebServer("", "", "", "", mockClient)

	// Create a multipart form with a file
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "test.txt")
	if err != nil {
		t.Fatalf("Failed to create form file: %v", err)
	}
	part.Write([]byte("test content"))
	writer.Close()

	request := httptest.NewRequest(http.MethodPost, "/", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	responseRecorder := httptest.NewRecorder()
	server.Router.ServeHTTP(responseRecorder, request)
	response := responseRecorder.Result()

	if response.StatusCode != http.StatusOK {
		t.Errorf(`Expected 200 OK, but instead got %s`, response.Status)
	}

	responseBody, _ := io.ReadAll(response.Body)
	if string(responseBody) != `{"url":"/ABCDE"}` {
		t.Errorf(`Expected {"url":"/ABCDE"}, but got %s`, string(responseBody))
	}
}

func TestUploadHandlerNoFile(t *testing.T) {
	mockClient := &mockStorage{}
	server := NewWebServer("", "", "", "", mockClient)

	request := httptest.NewRequest(http.MethodPost, "/", nil)
	responseRecorder := httptest.NewRecorder()
	server.Router.ServeHTTP(responseRecorder, request)
	response := responseRecorder.Result()

	if response.StatusCode != http.StatusInternalServerError {
		t.Errorf(`Expected 500, but instead got %s`, response.Status)
	}
}

func TestLookupHandler(t *testing.T) {
	mockClient := &mockStorage{}
	server := NewWebServer("", "", "", "", mockClient)

	request := httptest.NewRequest(http.MethodGet, "/ABCDE", nil)
	responseRecorder := httptest.NewRecorder()
	server.Router.ServeHTTP(responseRecorder, request)
	response := responseRecorder.Result()

	if response.StatusCode != http.StatusOK {
		t.Errorf(`Expected 200 OK, but instead got %s`, response.Status)
	}

	// Check that we got the file page with the correct URL
	var downloadLink = regexp.MustCompile(`href="http://cdn.example.com/file.txt"`)
	if downloadLink.FindString(responseRecorder.Body.String()) == "" {
		t.Errorf(
			`Could not find download link in body: %s`,
			responseRecorder.Body.String(),
		)
	}
}

func TestLookupHandlerImage(t *testing.T) {
	mockClient := &mockImageStorage{}
	server := NewWebServer("", "", "", "", mockClient)

	request := httptest.NewRequest(http.MethodGet, "/ABCDE", nil)
	responseRecorder := httptest.NewRecorder()
	server.Router.ServeHTTP(responseRecorder, request)
	response := responseRecorder.Result()

	if response.StatusCode != http.StatusOK {
		t.Errorf(`Expected 200 OK, but instead got %s`, response.Status)
	}

	// Check that we got an img tag for the image
	var imgTag = regexp.MustCompile(`<img src="http://cdn.example.com/image.png"`)
	if imgTag.FindString(responseRecorder.Body.String()) == "" {
		t.Errorf(
			`Could not find img tag in body: %s`,
			responseRecorder.Body.String(),
		)
	}
}

func TestLookupInvalidKeyLength(t *testing.T) {
	mockClient := &mockStorage{}
	server := NewWebServer("", "", "", "", mockClient)

	invalidKeys := []string{"/a", "/AB", "/ABC", "/ABCD"}

	for _, path := range invalidKeys {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		responseRecorder := httptest.NewRecorder()
		server.Router.ServeHTTP(responseRecorder, request)
		response := responseRecorder.Result()

		if response.StatusCode != http.StatusNotFound {
			t.Errorf(`Expected 404 for key "%s", but got %s`, path, response.Status)
		}
	}
}

func TestBasicAuthWrongCredentials(t *testing.T) {
	username := "skalnik"
	password := "hunter2"
	mockClient := &mockStorage{}
	server := NewWebServer(username, password, "", "", mockClient)

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.SetBasicAuth("wronguser", "wrongpass")

	responseRecorder := httptest.NewRecorder()
	server.Router.ServeHTTP(responseRecorder, request)
	response := responseRecorder.Result()

	if response.StatusCode != http.StatusUnauthorized {
		t.Errorf(`Expected unauthorized, but instead got %s`, response.Status)
	}
}

func TestBasicAuthWrongPassword(t *testing.T) {
	username := "skalnik"
	password := "hunter2"
	mockClient := &mockStorage{}
	server := NewWebServer(username, password, "", "", mockClient)

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.SetBasicAuth(username, "wrongpass")

	responseRecorder := httptest.NewRecorder()
	server.Router.ServeHTTP(responseRecorder, request)
	response := responseRecorder.Result()

	if response.StatusCode != http.StatusUnauthorized {
		t.Errorf(`Expected unauthorized, but instead got %s`, response.Status)
	}
}

func TestDirectHandlerWithoutExtension(t *testing.T) {
	mockClient := &mockStorage{}
	server := NewWebServer("", "", "", "", mockClient)

	// Key without extension should show file page, not redirect
	request := httptest.NewRequest(http.MethodGet, "/ABCDE", nil)
	responseRecorder := httptest.NewRecorder()
	server.Router.ServeHTTP(responseRecorder, request)
	response := responseRecorder.Result()

	if response.StatusCode != http.StatusOK {
		t.Errorf(`Expected 200 OK for file page, but instead got %s`, response.Status)
	}
}
