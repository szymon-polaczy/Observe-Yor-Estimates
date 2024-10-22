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
	"time"

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
	Add_date    string `json:"add_date"`
	Modify_time string `json:"modify_time"`
}

type SimpleTimeEntry struct {
	ID         int `json:"id"`
	Duration   string `json:"duration"`
	Last_modify string `json:"last_modify"`
	Task_id 	string `json:"task_id"`
}

func getApiKey(db *sql.DB) string{
	api_rows, err := db.Query("SELECT value FROM `settings` WHERE `key` = 'api_key'")
	if err != nil {
		log.Fatal(err)
	}

	defer api_rows.Close()

	var api_key string
	for api_rows.Next() {
		err = api_rows.Scan(&api_key)
		if err != nil {
			log.Fatal(err)
		}
	}

	return api_key
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

	http.HandleFunc("/sync_tasks", func(w http.ResponseWriter, r *http.Request) {
		db, err := sql.Open("mysql", "root:password@tcp(localhost:3306)/oye")
		if err != nil {
			log.Fatal(err)
		}

		defer db.Close()

		err = db.Ping()
		if err != nil {
			log.Fatal(err)
		}

		api_key := getApiKey(db)

		server_url := "https://app.timecamp.com/third_party/api"
		tasks_url := server_url + "/tasks"

		bearer := "Bearer " + api_key

		req, err := http.NewRequest("GET", tasks_url, nil)

		req.Header.Add("Authorization", bearer)
		req.Header.Add("Accept", "application/json")

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
			insert, err := db.Query("INSERT INTO tasks (Task_id, Name, Assigned_By, Add_date, Modify_time) VALUES (?, ?, ?, ?, ?)", key, value.Name, value.Assigned_by, value.Add_date, value.Modify_time)

			if err != nil {
				log.Fatal(err)
			}

			defer insert.Close()
		}

		fmt.Fprintf(w, "Sync finished")
	})

	http.HandleFunc("/sync_time_entries", func(w http.ResponseWriter, r *http.Request) {
		db, err := sql.Open("mysql", "root:password@tcp(localhost:3306)/oye")
		if err != nil {
			log.Fatal(err)
		}

		defer db.Close()

		err = db.Ping()
		if err != nil {
			log.Fatal(err)
		}

		api_key := getApiKey(db)

		rows, err := db.Query("SELECT Add_date FROM `tasks` ORDER BY Add_date ASC LIMIT 1")
		if err != nil {
			log.Fatal(err)
		}

		defer rows.Close()

		var oldest_task string
		for rows.Next() {
			err = rows.Scan(&oldest_task)
			if err != nil {
				log.Fatal(err)
			}
		}

		oldest_task = oldest_task[:10]//TODO: fix this
		tomorrow := time.Now().AddDate(0, 0, 1).Format("2006-01-02")

		server_url := "https://app.timecamp.com/third_party/api/entries"
		server_url = server_url + "?from=" + oldest_task + "&to=" + tomorrow

		bearer := "Bearer " + api_key

		req, err := http.NewRequest("GET", server_url, nil)

		req.Header.Add("Authorization", bearer)
		req.Header.Add("Accept", "application/json")

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

		var entries []SimpleTimeEntry
		err = json.Unmarshal([]byte(body), &entries)
		if err := json.Unmarshal([]byte(body), &entries); err != nil {
			fmt.Println("error")
		}

		for _, value := range entries {
			insert, err := db.Query("INSERT INTO time_entries (ID, Duration, Task_id, Last_modify) VALUES (?, ?, ?, ?)", value.ID, value.Duration, value.Task_id, value.Last_modify)

			if err != nil {
				log.Fatal(err)
			}

			defer insert.Close()
		}

		fmt.Fprintf(w, "Sync finished")
	})

	log.Fatal(http.ListenAndServe(":1234", nil))
}
