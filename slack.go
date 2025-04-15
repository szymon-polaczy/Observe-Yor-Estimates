package main

import (
	//"github.com/gobwas/ws"
	//"github.com/gobwas/ws/wsutil"
	"encoding/json"
	"flag"
	"io"
	"log"
	"time"

	"net/http"
	//"net/url"
	"os"
	"os/signal"

	"github.com/gorilla/websocket"
	"github.com/joho/godotenv"
)

type SOCKET_URL_RESPONSE struct {
	Ok  bool   `json:"ok"`
	Url string `json:"url"`
}

func main() {
	flag.Parse()
	log.SetFlags(0)

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	err := godotenv.Load()
	if err != nil {
		panic("Error loading .env file")
	}

	new_socket_url := get_slack_socket_url()

	//var addr = flag.String("addr", new_socket_url, "http service address")

	//u := url.URL{Scheme: "ws", Host: new_socket_url, Path: "/"}
	//log.Printf("connecting to %s", u.String())

	//c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	c, _, err := websocket.DefaultDialer.Dial(new_socket_url, nil)
	if err != nil {
		log.Fatal("dial:", err)
	}
	defer c.Close()

	done := make(chan struct{})

	go func() {
		defer close(done)
		for {
			_, message, err := c.ReadMessage()
			if err != nil {
				log.Println("read:", err)
				return
			}
			log.Printf("recv: %s", message)

		}
	}()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			return
		case t := <-ticker.C:
			err := c.WriteMessage(websocket.TextMessage, []byte(t.String()))
			if err != nil {
				log.Println("write:", err)
				return
			}
		case <-interrupt:
			log.Println("interrupt")

			// Cleanly close the connection by sending a close message and then
			// waiting (with timeout) for the server to close the connection.
			err := c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			if err != nil {
				log.Println("write close:", err)
				return
			}
			select {
			case <-done:
			case <-time.After(time.Second):
			}
			return
		}
	}

}

func get_slack_socket_url() string {
	slack_url := "https://slack.com/api/apps.connections.open"

	request, err := http.NewRequest("POST", slack_url, nil)
	if err != nil {
		panic(err)
	}

	request.Header.Add("Authorization", "Bearer "+os.Getenv("SLACK_TOKEN"))
	request.Header.Add("Content-type", "application/x-www-form-urlencoded")
	request.Header.Add("User-Agent", "insomnia/11.0.0")

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		panic(err)
	}

	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		panic(err)
	}

	var socket_url_response SOCKET_URL_RESPONSE

	if err := json.Unmarshal(body, &socket_url_response); err != nil {
		panic(err)
	}

	if socket_url_response.Ok == false {
		log.Fatal("We somehow fucked up the first connection to slack")
	}

	return socket_url_response.Url
}
