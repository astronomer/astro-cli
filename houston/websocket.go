package houston

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/astronomer/astro-cli/pkg/logger"
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

var DeploymentLogsSubscribeRequest = `
    subscription log(
		$deploymentId: Uuid!
		$component: String
		$timestamp: DateTime
		$search: String
    ){
      	log(
			deploymentUuid: $deploymentId
			component: $component
			timestamp: $timestamp
			search: $search
		){
        	id
        	createdAt: timestamp
        	log: message
    	}
    }`

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
	ws, resp, err := websocket.DefaultDialer.Dial(url, h)
	if err != nil {
		logger.Fatal("dial:", err)
	}
	defer func() {
		ws.Close()
		if resp != nil {
			resp.Body.Close()
		}
	}()

	initSubscription := InitSubscription{Type: "connection_init", Payload: AuthPayload{Authorization: jwtToken}}
	js, _ := json.Marshal(&initSubscription)

	err = ws.WriteMessage(websocket.TextMessage, js)
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
					logger.Fatal(err)
				}
			}
			var resp WSResponse
			if err = json.Unmarshal(message, &resp); err != nil {
				logger.Fatal(err)
			}
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
