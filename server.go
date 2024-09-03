package main

import (
	"net/http"	
	"github.com/labstack/echo/v4"
	"fmt"
    "io/ioutil"
    "encoding/json"
)

type Task struct {
	task_id int `json:"task_id"`
	name string `json:"name"`
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

		fmt.Printf(tasks_url)

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
		var result map[string]map[string]Task

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
		}

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

		return c.String(http.StatusOK, string(body))
	})
	e.Logger.Fatal(e.Start(":1324"))
}
