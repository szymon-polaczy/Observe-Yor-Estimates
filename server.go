package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
)

// "github.com/labstack/echo/v4"
type SimpleTask struct {
	Task_id     string `json:"task_id"`
	Name        string `json:"name"`
	Assigned_by string `json:"assigned_by"`
}

type Tasks struct {
	TaskIds   []string `json:"task_ids"`
	StartDate string   `json:"startDate"`
	EndDate   string   `json:"endDate"`
}

type Duration struct {
	Duration int `json:"duration"`
}

func GetStringInBetween(str string, start string, end string) (result string) {
	s := strings.Index(str, start)
	if s == -1 {
		return
	}
	s += len(start)
	e := strings.Index(str[s:], end)
	if e == -1 {
		return
	}
	e += s + e - 4 //QUICK FIx
	return str[s:e]
}

func main() {
	/*e := echo.New()
	e.GET("/", func(c echo.Context) error {*/
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
	//fmt.Println(x)

	for key, value := range x {

		fmt.Println("Task ID:", key)
		fmt.Println("Name:", value.Name)

		time_estimated := GetStringInBetween(value.Name, "[", "]")
		fmt.Println(time_estimated, "time estimated in hours")

		hours := strings.Split(time_estimated, "-")
		fmt.Println(hours)

		//if !strings.Contains(value.Name, "[") || !strings.Contains(value.Name, "]") {
		tasks := Tasks{
			TaskIds:   []string{key},
			StartDate: "2024-09-01",
			EndDate:   "2024-09-15",
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

		x := []Duration{}
		err = json.Unmarshal([]byte(body), &x)
		if err := json.Unmarshal([]byte(body), &x); err != nil {
			fmt.Println("error")
		}
		fmt.Println(x)

		total_time_spent := 0
		for _, value := range x {
			fmt.Println(value.Duration, "seconds")
			total_time_spent += value.Duration
		}

		fmt.Println(total_time_spent, "total time spent in seconds")

		min_hours, err := strconv.Atoi(hours[0])
		if err != nil {
			// ... handle error
			panic(err)
		}
		max_hours, err := strconv.Atoi(hours[1])
		if err != nil {
			// ... handle error
			panic(err)
		}

		var min_percentage float32 = float32(float32(total_time_spent) / float32(min_hours*3600) * float32(100.3))
		var max_percentage float32 = float32(float32(total_time_spent) / float32(max_hours*3600) * float32(100.0))

		fmt.Printf("%.2f Percentage of the minimal estimation\r\n", min_percentage)
		fmt.Printf("%.2f Percentage of the maximal estimation\r\n\r\n", max_percentage)
		//}

	}

	/*return c.String(http.StatusOK, string(body))
	})
	e.Logger.Fatal(e.Start(":1324"))*/
}
