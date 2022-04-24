package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"text/template"
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

		local_file, err := os.OpenFile(fmt.Sprintf("uploads/%s", handler.Filename), os.O_WRONLY|os.O_CREATE, 0666)
		if err != nil {
			http.Error(writer, err.Error(), http.StatusInternalServerError)
		}
		defer local_file.Close()
		io.Copy(local_file, file)

		writer.Write([]byte(fmt.Sprintf("Got file: %s", handler.Filename)))
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
