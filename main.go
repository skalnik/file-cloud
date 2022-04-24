package main

import (
	"net/http"
	"text/template"
)

func main() {
	fs := http.FileServer(http.Dir("static/"))
	http.Handle("/static/", http.StripPrefix("/static/", fs))
	http.HandleFunc("/", index)
	http.ListenAndServe(":8080", nil)
}

func index(writer http.ResponseWriter, reqest *http.Request) {
	t, err := template.ParseFiles("index.tmpl.html")
	if err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := t.Execute(writer, ""); err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}
}
