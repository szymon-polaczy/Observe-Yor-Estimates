package main

import (
	"html/template"
	"log"
	"net/http"
	"os"
	"database/sql"

	_ "github.com/go-sql-driver/mysql"
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

	http.HandleFunc("/sync", func(w http.ResponseWriter, r *http.Request) {
		//connect with sql database, then connect with an api to get data and put it in the database
		//then return the new data to the user

		db, err := sql.Open("mysql", "root:password@tcp(localhost:3306)/oye")
		if err != nil {
			log.Fatal(err)
		}

		defer db.Close()

		err = db.Ping()
		if err != nil {
			log.Fatal(err)
		}

		rows, err := db.Query("SELECT * FROM tasks")
		if err != nil {
			log.Fatal(err)
		}

		defer rows.Close()

		for rows.Next() {
			var id int
			var name string
			err = rows.Scan(&id, &name)
			if err != nil {
				log.Fatal(err)
			}
			log.Println(id, name)
		}
	})

	log.Fatal(http.ListenAndServe(":1234", nil))
}
