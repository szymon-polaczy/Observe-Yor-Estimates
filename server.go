package main

import (
	"html/template"
	"log"
	"net/http"
	"os"
)

type Page struct {
	Title string
	Body  []byte
}

func loadPage(title string, filename string) (*Page, error) {
	body, _ := os.ReadFile(filename)
	return &Page{Title: title, Body: body}, nil
}

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		title := "Homepage"
		p, err := loadPage(title, "homepage.html")

		if err != nil {
			p = &Page{Title: title}
		}

		t, _ := template.ParseFiles("homepage.html")

		t.Execute(w, p)
	})

	log.Fatal(http.ListenAndServe(":1234", nil))
}
