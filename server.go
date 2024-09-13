package main

import (
	"net/http"	
	"github.com/labstack/echo/v4"
	"fmt"
    "io/ioutil"
    "encoding/json"
	"strings"
	"bytes"
)
type SimpleTask struct {
			Task_id string `json:"task_id"`
			Name string `json:"name"`
			Assigned_by string `json:"assigned_by"`
		}
	
type Tasks struct {
	TaskIds []string `json:"task_ids"`
	StartDate string `json:"startDate"`
	EndDate string `json:"endDate"`
}

func main() {
	e := echo.New()
	e.GET("/", func(c echo.Context) error {
		timecamp_api_key := "cb27cd4c3598f624e85309c40c"
		server_url := "https://app.timecamp.com/third_party/api" //mock server url
		tasks_url := server_url + "/tasks/168140512"

		// Create a Bearer string by appending string access token
		bearer := "Bearer " + timecamp_api_key

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
		fmt.Println(x)


		for key, value := range x {
			
			fmt.Println("Task ID:", key)
			fmt.Println("Name:", value.Name)

			if !strings.Contains(value.Name, "[") || !strings.Contains(value.Name, "]") {
				tasks := Tasks{
					TaskIds: []string{key},
					StartDate: "2024-09-01",
					EndDate: "2024-09-15",
				}
				marshalled, err := json.Marshal(tasks)

				fmt.Println(string(marshalled))

				if err != nil {
					fmt.Println("error marshalling")
				}

				request_url := "https://app.timecamp.com/third_party/api/v3/time-entries"
				// Create a new request using http
				req, err := http.NewRequest("POST", request_url, bytes.NewReader(marshalled))
				// add authorization header to the req
				req.Header.Add("Authorization", bearer)
				req.Header.Add("Accept", "application/json")
				req.Header.Add("Content-Type", "application/json")
				// Add request body
				//req.Body = ioutil.NopCloser(strings.NewReader(request_body))
				// Send req using http Client
				client := &http.Client{}
				resp, err := client.Do(req)
				if err != nil {
					fmt.Printf("error making time entry request")
				}
				defer resp.Body.Close()
				body, err := ioutil.ReadAll(resp.Body)
				if err != nil {
					fmt.Printf("error reading time entry request")
				}
				fmt.Println(string(body))
			}

		}

		return c.String(http.StatusOK, string(body))
	})
	e.Logger.Fatal(e.Start(":1324"))
}
