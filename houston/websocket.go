package houston

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/gorilla/websocket"
)

type AuthPayload struct {
	Authorization string `json:"authorization"`
}

type InitSubscription struct {
	Type    string      `json:"type"`
	Payload AuthPayload `json:"payload"`
}

type StartSubscription struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
}

type WSResponse struct {
	ID      string
	Type    string `json:"type"`
	Payload struct {
		Data struct {
			Log struct {
				ID        string `json:"id"`
				CreatedAt string `json:"createdAt"`
				Log       string `json:"log"`
			} `json:"log"`
		} `json:"data"`
	} `json:"payload"`
}

func BuildDeploymentLogsSubscribeRequest(deploymentID, component, search string, timestamp time.Time) (string, error) {
	payload := Request{
		Query: DeploymentLogsSubscribeRequest,
		Variables: map[string]interface{}{
			"component": component, "deploymentId": deploymentID, "search": search, "timestamp": timestamp,
		},
	}
	s := StartSubscription{Type: "start", Payload: payload}
	b, _ := json.Marshal(s)
	return string(b), nil
}

func Subscribe(jwtToken, url, queryMessage string) error {
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)
	h := http.Header{"Sec-WebSocket-Protocol": []string{"graphql-ws"}}
	ws, _, err := websocket.DefaultDialer.Dial(url, h)
	if err != nil {
		log.Fatal("dial:", err)
	}
	defer ws.Close()

	initSubscription := InitSubscription{Type: "connection_init", Payload: AuthPayload{Authorization: jwtToken}}
	js, _ := json.Marshal(&initSubscription)

	err = ws.WriteMessage(websocket.TextMessage, []byte(js))
	if err != nil {
		fmt.Printf("Could not init connection: %s", err.Error())
	}

	err = ws.WriteMessage(websocket.TextMessage, []byte(queryMessage))
	if err != nil {
		fmt.Printf("Could not subscribe to logs: %s", err.Error())
	}

	fmt.Println("Waiting for logs...")
	done := make(chan struct{})

	go func() {
		defer close(done)
		for {
			_, message, err := ws.ReadMessage()
			if err != nil {
				switch err.(type) {
				case *websocket.CloseError:
					fmt.Println("Your token has expired. Please log in again.")
					return
				default:
					log.Fatal(err)
				}
			}
			var resp WSResponse
			json.Unmarshal(message, &resp)
			fmt.Print(resp.Payload.Data.Log.Log)
		}
	}()

	for {
		select {
		case <-done:
			return nil
		case <-interrupt:
			log.Println("Bye bye ...")
			// Cleanly close the connection by sending a close message and then
			// waiting (with timeout) for the server to close the connection.
			err := ws.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, `{"id":"1","type":"stop"}`))
			if err != nil {
				fmt.Println("Close connection...")
				return err
			}
			select {
			case <-done:
			case <-time.After(time.Second):
			}
			return nil
		}
	}
}
