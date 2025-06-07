package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"

	"net/http"

	"github.com/gorilla/websocket"
	"github.com/joho/godotenv"
	"github.com/robfig/cron/v3"
)

type SOCKET_URL_RESPONSE struct {
	Ok  bool   `json:"ok"`
	Url string `json:"url"`
}

type TEST_SLACK_PAYLOAD struct {
	EnvelopeID string `json:"envelope_id"`
}

type Payload struct {
	Blocks []Block `json:"blocks"`
}

type Block struct {
	Type     string    `json:"type"`
	Text     *Text     `json:"text,omitempty"`
	Fields   []Field   `json:"fields,omitempty"`
	Elements []Element `json:"elements,omitempty"`
}

type Text struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type TEST_SLACK_PAYLOAD_RESPONSE struct {
	EnvelopeID string  `json:"envelope_id"`
	Payload    Payload `json:"payload"`
}

func main() {
	err := godotenv.Load()
	if err != nil {
		panic("Error loading .env file")
	}

	// Check for command line arguments
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "daily-update":
			SendDailySlackUpdate()
			return
		}
	}

	// Run initial sync
	SyncTasksToDatabase()

	// Set up cron scheduler
	cronScheduler := cron.New()

	// Schedule SyncTasksToDatabase to run every 5 minutes
	// Using "*/5 * * * *" to run at :00, :05, :10, :15, :20, etc.
	_, err = cronScheduler.AddFunc("*/5 * * * *", SyncTasksToDatabase)
	if err != nil {
		log.Fatal("Failed to schedule cron job:", err)
	}

	// Schedule daily Slack update to run at 6 AM every day
	_, err = cronScheduler.AddFunc("0 6 * * *", SendDailySlackUpdate)
	if err != nil {
		log.Fatal("Failed to schedule daily Slack update:", err)
	}

	// Start the cron scheduler
	cronScheduler.Start()
	defer cronScheduler.Stop()

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	new_socket_url := get_slack_socket_url()

	conn, _, err := websocket.DefaultDialer.Dial(new_socket_url, nil)
	if err != nil {
		log.Fatal("dial:", err)
	}
	defer conn.Close()

	go func() {
		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				log.Println("read:", err)
				return
			}
			log.Printf("recv: %s", message)

			var test_payload TEST_SLACK_PAYLOAD

			if err := json.Unmarshal(message, &test_payload); err != nil {
				panic(err)
			}

			if len(test_payload.EnvelopeID) != 0 {
				response := TEST_SLACK_PAYLOAD_RESPONSE{
					EnvelopeID: test_payload.EnvelopeID,
					Payload: Payload{
						[]Block{
							Block{
								Type: "section",
								Text: &Text{
									Type: "mrkdwn",
									Text: "**Test text**",
								},
							},
						},
					},
				}

				json_response, err := json.Marshal(response)
				if err != nil {
					panic(err)
				}

				err = conn.WriteMessage(websocket.TextMessage, []byte(json_response))
				if err != nil {
					log.Println("write:", err)
					return
				}

				fmt.Printf("%s", json_response)
			}
		}
	}()

	for {
		select {
		case <-interrupt:
			log.Println("interrupt")

			// Cleanly close the connection by sending a close message and then
			// waiting (with timeout) for the server to close the connection.
			err := conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			if err != nil {
				log.Println("write close:", err)
				return
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
