package main

import (
	//"github.com/gobwas/ws"
	//"github.com/gobwas/ws/wsutil"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/davecgh/go-spew/spew"
	"github.com/joho/godotenv"
)

type SOCKET_URL_RESPONSE struct {
	ok  bool   `json:"ok"`
	url string `json:"url"`
}

func main() {
	err := godotenv.Load()
	if err != nil {
		panic("Error loading .env file")
	}

	//get_slack_socket_url()
	url := "https://slack.com/api/apps.connections.open"

	req, _ := http.NewRequest("POST", url, nil)

	req.Header.Add("Authorization", "Bearer "+os.Getenv("SLACK_TOKEN"))
	req.Header.Add("Content-type", "application/x-www-form-urlencoded")
	req.Header.Add("User-Agent", "insomnia/11.0.0")

	res, _ := http.DefaultClient.Do(req)

	defer res.Body.Close()
	body, _ := ioutil.ReadAll(res.Body)

	fmt.Println(res)
	fmt.Println(string(body))
}

func get_slack_socket_url() {
	slack_url := "https://slack.com/api/apps.connections.open"

	request, err := http.NewRequest("POST", slack_url, nil)
	if err != nil {
		panic(err)
	}

	request.Header.Add("Content-type", "application/x-www-form-urlencoded")
	request.Header.Add("Authorization", "Bearer "+os.Getenv("SLACK_TOKEN"))

	http_client := &http.Client{}
	response, err := http_client.Do(request)
	if err != nil {
		panic(err)
	}

	defer response.Body.Close()

	socket_url_response := &SOCKET_URL_RESPONSE{}
	json_decode_error := json.NewDecoder(response.Body).Decode(socket_url_response)
	if json_decode_error != nil {
		panic(json_decode_error)
	}

	bodyBytes, err := io.ReadAll(response.Body)
	if err != nil {
		panic(err)
	}
	spew.Dump(string(bodyBytes))

	fmt.Println("ok", socket_url_response.ok)
	fmt.Println("url", socket_url_response.url)
}
