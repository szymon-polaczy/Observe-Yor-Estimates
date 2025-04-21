package main

import (
	"database/sql"
	"encoding/json"
	"net/http"

	_ "github.com/mattn/go-sqlite3"
)

type tasksHandler struct{}

type JsonTask struct {
	TaskID      int    `json:"task_id"`
	ParentID    int    `json:"parent_id"`
	AssignedBy  int    `json:"assigned_by"`
	Name        string `json:"name"`
	Level       int    `json:"level"`
	RootGroupID int    `json:"root_group_id"`
}

func main() {
	request_multiplexer := http.NewServeMux()

	request_multiplexer.Handle("/tasks", &tasksHandler{})

	http.ListenAndServe(":9238", request_multiplexer)
}

func (t *tasksHandler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	switch {
	case request.Method == http.MethodGet:
		t.GetTasks(response, request)
		return
	default:
		return
	}
}

func (t *tasksHandler) GetTasks(response http.ResponseWriter, request *http.Request) {
	db, err := sql.Open("sqlite3", "./oye.db")
	if err != nil {
		panic(err)
	}
	defer db.Close()

	select_all_tasks_sql := `SELECT * FROM tasks;`
	db_tasks, err := db.Query(select_all_tasks_sql)
	if err != nil {
		panic(err)
	}

	tasks_list := []JsonTask{}
	for db_tasks.Next() {
		task := JsonTask{}
		err = db_tasks.Scan(&task.TaskID, &task.ParentID, &task.AssignedBy, &task.Name, &task.Level, &task.RootGroupID)
		if err != nil {
			panic(err)
		}
		tasks_list = append(tasks_list, task)
	}

	json_tasks, err := json.Marshal(tasks_list)
	if err != nil {
		panic(err)
	}

	response.WriteHeader(http.StatusOK)
	response.Write(json_tasks)
}
