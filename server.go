package main

import (
	"net/http"	
	"github.com/labstack/echo/v4"
	"fmt"
    "io/ioutil"
    "encoding/json"
	"strings"
)

	//"strconv"
type Task struct {
	task_id int `json:"task_id"`
	name string `json:"name"`
}

type SimpleTask struct {
			Task_id string `json:"task_id"`
			Name string `json:"name"`
			Assigned_by string `json:"assigned_by"`
		}

func TrimSuffix(s, suffix string) string {
    if strings.HasSuffix(s, suffix) {
        s = s[:len(s)-len(suffix)]
    }
    return s
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

		//fmt.Printf(tasks_url)

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

		//var result map[string]interface{}
		/*var result map[string]map[string]Task

		err = json.Unmarshal([]byte(body), &result)

		if err != nil {
			// print out if error is not nil
			fmt.Println(err)
		}

		fmt.Println(result)
		// printing details of map
		// iterate through the map
		for key, value := range result {
			fmt.Println(key, ":", value)
		}*/

		/*var tasks []Task
		err = json.Unmarshal([]byte(body), &tasks)

		if err != nil {
			fmt.Println("JSON decode error!")
		}

		fmt.Println(tasks)

		for key, value := range tasks {
			fmt.Println("Key:", key, "Value:", value)
		}*/

		//fmt.Println("Price of the second product:", objMap[1]["name"])


		/*str := string(body)

		str = TrimSuffix(str, "{")

		if last := len(str) - 1; last >= 0 && str[last] == '}' {
			str = str[:last]
		}

		str = "[" + str + "]"
		fmt.Println(str)*/

		x := make(map[string]SimpleTask)
		err = json.Unmarshal([]byte(body), &x)
		if err := json.Unmarshal([]byte(body), &x); err != nil {
			//panic(err)
			fmt.Println("error")
		}
		fmt.Println(x)

		

	//	taskList := make([]SimpleTask, 0)

		for key, value := range x {

			fmt.Println("Key:", key, "Value:", value)
			fmt.Println("Task ID:", value.Task_id)
			fmt.Println("Name:", value.Name)

			//task := SimpleTask{}


			if !strings.Contains(value.Name, "[") || !strings.Contains(value.Name, "]") {

				request_body := `{
					"task_ids": [` + value.Task_id + `],
				}`

				request_url := "https://app.timecamp.com/third_party/api/v3/time-entries"

				// Create a new request using http
				req, err := http.NewRequest("POST", request_url, nil)

				// add authorization header to the req
				req.Header.Add("Authorization", bearer)
				req.Header.Add("Accept", "application/json")
				req.Header.Add("Content-Type", "application/json")

				// Add request body
				req.Body = ioutil.NopCloser(strings.NewReader(request_body))

				// Send req using http Client
				client := &http.Client{}

				resp, err := client.Do(req)

				if err != nil {
					fmt.Printf("error making time entry request")
				}

				defer resp.Body.Close()

				/*body, err := ioutil.ReadAll(resp.Body)

				if err != nil {
					fmt.Printf("error reading time entry request")
				}

				fmt.Println(body)*/
			}

			/*for key2, value2 := range value.(map[string]interface{}) {
				if key2 == "name"  {
					fmt.Println("Name: ", value2)
					name, err := strconv.ParseFloat(value2.(string), 64)
					if err != nil {
						fmt.Println("error parsing name")
					}
					task.name = name.(string)
				}

				if key2 == "task_id"  {
					fmt.Println("Task ID: ", value2)
					task_id, err := strconv.ParseFloat(value2.(string), 64)
					if err != nil {
						fmt.Println("error parsing task_id")
					}
					task.task_id = task_id.(string)
				}
			}*/

			//taskList = append(taskList, task)
		}

		/*for _, value := range taskList {	
			if !strings.Contains(value.name, "[") || !strings.Contains(value.name, "]") {
				//TODO: actually validate that it's hours within brackets

				request_body := `{
					"task_ids": [` + value.task_id + `],
				}`

				request_url := "https://app.timecamp.com/third_party/api/v3/time-entries"

				// Create a new request using http
				req, err := http.NewRequest("POST", request_url, nil)

				// add authorization header to the req
				req.Header.Add("Authorization", bearer)
				req.Header.Add("Accept", "application/json")
				req.Header.Add("Content-Type", "application/json")

				// Add request body
				req.Body = ioutil.NopCloser(strings.NewReader(request_body))

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

				fmt.Println(body)
			}
		}*/

		return c.String(http.StatusOK, string(body))
	})
	e.Logger.Fatal(e.Start(":1324"))
}
