package main

import (
	"html/template"
	"log"
	"net/http"
	"os"
	"database/sql"
	"fmt"
	"io/ioutil"
	"encoding/json"

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

type SimpleTask struct {
	Task_id     string `json:"task_id"`
	Name        string `json:"name"`
	Assigned_by string `json:"assigned_by"`
}
//add_date string `json:"add_date"`
//modify_time string `json:"modify_time"`

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

		//GET api_key from the settings table
		rows, err := db.Query("SELECT value FROM `settings` WHERE `key` = 'api_key'")
		if err != nil {
			log.Fatal(err)
		}

		defer rows.Close()

		var api_key string
		for rows.Next() {
			err = rows.Scan(&api_key)
			if err != nil {
				log.Fatal(err)
			}
		}

		//GET data from the api
		server_url := "https://app.timecamp.com/third_party/api" //mock server url
		tasks_url := server_url + "/tasks"

		// Create a Bearer string by appending string access token
		bearer := "Bearer " + api_key

		// Create a new request using http
		req, err := http.NewRequest("GET", tasks_url, nil)

		// add authorization header to the req
		req.Header.Add("Authorization", bearer)
		req.Header.Add("Accept", "application/json")

		// Send req using http Client
		client := &http.Client{}
		resp, err := client.Do(req)

		if err != nil {
			fmt.Printf("error making tasks request")
		}
		defer resp.Body.Close()

		body, err := ioutil.ReadAll(resp.Body)

		if err != nil {
			fmt.Printf("error reading tasks request")
		}

		x := make(map[string]SimpleTask)
		err = json.Unmarshal([]byte(body), &x)
		if err := json.Unmarshal([]byte(body), &x); err != nil {
			fmt.Println("error")
		}

		for key, value := range x {

			//INSERT data into the database
			insert, err := db.Query("INSERT INTO tasks (Task_id, Name, Assigned_By) VALUES (?, ?, ?)", key, value.Name, value.Assigned_by)

			if err != nil {
				log.Fatal(err)
			}

			defer insert.Close()
		}

		//send to the request text that we finished
		fmt.Fprintf(w, "Sync finished")

		/*rows, err := db.Query("SELECT * FROM tasks")
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
		}*/
	})

	log.Fatal(http.ListenAndServe(":1234", nil))
}
