package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/joho/godotenv"
)

type JsonTask struct {
	TaskID      int    `json:"task_id"`
	ParentID    int    `json:"parent_id"`
	AssignedBy  int    `json:"assigned_by"`
	Name        string `json:"name"`
	Level       int    `json:"level"`
	RootGroupID int    `json:"root_group_id"`
}

func SyncTasksToDatabase() {

	// contact timecamp and get the tasks
	// open a connection with the database - do we create it using an outside thing or do we add it's creation here?
	// how do we update everythign - if we would actually be getting a full database each time then we can basically override it and add data from the start
	// if we don't get everything then we would have to update things where most of the things don't change
	// so we already know that we have to at least get all of the json, run it through a hash and see what value we get - if we already synced that value then we don't have
	// anything more to sync and we can stop - with the option to override for testing

	err := godotenv.Load()
	if err != nil {
		panic("Error loading .env file")
	}

	timecamp_tasks := get_timecamp_tasks()

	db, err := GetDB()
	if err != nil {
		panic(err)
	}
	defer db.Close()

	// Use INSERT OR IGNORE to handle existing tasks
	insert_statement, err := db.Prepare("INSERT OR IGNORE INTO tasks values(?, ?, ?, ?, ?, ?)")
	if err != nil {
		panic(err)
	}

	index := 0
	for _, task := range timecamp_tasks {
		_, err := insert_statement.Exec(task.TaskID, task.ParentID, task.AssignedBy, task.Name, task.Level, task.RootGroupID)
		if err != nil {
			panic(err)
		}
		index++
	}
	fmt.Printf("We've imported %d tasks\n", index)
}

func get_timecamp_tasks() []JsonTask {
	timecamp_api_url := "https://app.timecamp.com/third_party/api"
	get_all_tasks_url := timecamp_api_url + "/tasks"

	auth_bearer := "Bearer " + os.Getenv("TIMECAMP_API_KEY")

	request, err := http.NewRequest("GET", get_all_tasks_url, nil)
	request.Header.Add("Authorization", auth_bearer)
	request.Header.Add("Accept", "application/json")

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		panic(err)
	}

	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		panic(err)
	}

	// Unmarshal into a map first
	taskMap := make(map[string]JsonTask)
	if err := json.Unmarshal(body, &taskMap); err != nil {
		panic(err)
	}

	// Convert the map to a slice
	tasks := make([]JsonTask, 0, len(taskMap))
	for _, task := range taskMap {
		tasks = append(tasks, task)
	}

	// Output the slice to check
	for _, t := range tasks {
		fmt.Printf("%+v\n", t)
	}

	return tasks
}
