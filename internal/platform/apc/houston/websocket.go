package houston

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
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
	// ID names the operation: the server tags its frames for it with this,
	// and a stop names it (subscriptions-transport-ws, which Houston serves
	// subscriptions with).
	ID      string      `json:"id,omitempty"`
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
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
	s := StartSubscription{ID: subscriptionID, Type: "start", Payload: payload}
	b, _ := json.Marshal(s) //nolint:errcheck // marshaling a plain struct that does not error in practice
	return string(b), nil
}

// notifyInterrupt delivers the person's interrupt to c; a test sends its own.
var notifyInterrupt = func(c chan<- os.Signal) { signal.Notify(c, os.Interrupt) }

// subscriptionID is the one operation a log subscription starts, and stops.
const subscriptionID = "1"

// ErrLogStreamClosed is the connection closing under a log subscription.
// Houston itself does not close it when a login expires: it checks the token
// once, when the connection opens, and a token it refuses then is a
// connection_error frame instead. A close is a proxy, a restart, or the
// network.
var ErrLogStreamClosed = errors.New("the log stream was closed")

// Subscribe follows a Deployment's logs over a websocket until the person
// interrupts it, handing each record to onLog. Notes ("Waiting for logs...")
// go to notes. It returns the failure that ended the stream: a connection
// that could not be made or set up, a record it could not read, the server
// closing it (ErrLogStreamClosed), or onLog's own error. An interrupt ends it
// without one.
func Subscribe(jwtToken, url, queryMessage string, notes io.Writer, onLog func(DeploymentLog) error) error {
	interrupt := make(chan os.Signal, 1)
	notifyInterrupt(interrupt)
	defer signal.Stop(interrupt)
	h := http.Header{"Sec-WebSocket-Protocol": []string{"graphql-ws"}}
	ws, resp, err := websocket.DefaultDialer.Dial(url, h)
	if err != nil {
		return fmt.Errorf("could not connect to the log stream: %w", err)
	}
	defer func() {
		ws.Close() //nolint:errcheck // best-effort close
		if resp != nil {
			resp.Body.Close()
		}
	}()

	initSubscription := InitSubscription{Type: "connection_init", Payload: AuthPayload{Authorization: jwtToken}}
	js, _ := json.Marshal(&initSubscription) //nolint:errcheck // marshaling a plain struct that does not error in practice

	if err := ws.WriteMessage(websocket.TextMessage, js); err != nil {
		return fmt.Errorf("could not init connection: %w", err)
	}
	if err := ws.WriteMessage(websocket.TextMessage, []byte(queryMessage)); err != nil {
		return fmt.Errorf("could not subscribe to logs: %w", err)
	}

	fmt.Fprintln(notes, "Waiting for logs...")
	done := make(chan error, 1)
	// stopped is set on an interrupt: what the reader has buffered by then
	// is not delivered.
	var stopped atomic.Bool

	go func() {
		for {
			_, message, err := ws.ReadMessage()
			if err != nil {
				var closed *websocket.CloseError
				if errors.As(err, &closed) {
					err = ErrLogStreamClosed
				}
				done <- err
				return
			}
			if stopped.Load() {
				continue
			}
			if err := handleFrame(message, onLog); err != nil {
				if errors.Is(err, errStreamComplete) {
					err = nil
				}
				done <- err
				return
			}
		}
	}()

	select {
	case err := <-done:
		return err
	case <-interrupt:
		stopped.Store(true)
		log.Println("Bye bye ...")
		return stopAndWait(ws, notes, done)
	}
}

// stopAndWait ends a subscription the person interrupted: it stops the
// operation, closes the connection cleanly, and waits (with a timeout) for
// the server to close it too. Either way it waits for the reader, so nothing
// reaches onLog once Subscribe has returned.
func stopAndWait(ws *websocket.Conn, notes io.Writer, done <-chan error) error {
	stop, _ := json.Marshal(StartSubscription{ID: subscriptionID, Type: "stop"}) //nolint:errcheck // marshaling a plain struct that does not error in practice
	err := ws.WriteMessage(websocket.TextMessage, stop)
	if err == nil {
		err = ws.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	}
	if err != nil {
		fmt.Fprintln(notes, "Close connection...")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		// Close it under the reader, and wait for the reader to see that.
		ws.Close() //nolint:errcheck // best-effort close; the read that follows ends either way
		<-done
	}
	return err
}

// errStreamComplete is the server ending a subscription it has no more
// records for (a graphql-ws "complete").
var errStreamComplete = errors.New("log stream complete")

// handleFrame hands a graphql-ws "data" frame's record to onLog. The protocol
// also sends "connection_ack", keep-alives ("ka") and others, which carry no
// record and are skipped; an "error" frame is the subscription failing, and
// "complete" its end (errStreamComplete).
func handleFrame(message []byte, onLog func(DeploymentLog) error) error {
	var frame struct {
		Type    string          `json:"type"`
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(message, &frame); err != nil {
		return fmt.Errorf("could not read a log record: %w", err)
	}
	switch frame.Type {
	case "data":
		// A GraphQL execution error comes in a data frame too, with
		// payload.errors: a permission denial, or a failure setting the
		// subscription up (subscriptions-transport-ws, which Houston serves the
		// subscription with). It is not a record.
		var payload struct {
			Data *struct {
				Log *struct {
					ID        string `json:"id"`
					CreatedAt string `json:"createdAt"`
					Log       string `json:"log"`
				} `json:"log"`
			} `json:"data"`
			Errors []struct {
				Message string `json:"message"`
			} `json:"errors"`
		}
		if err := json.Unmarshal(frame.Payload, &payload); err != nil {
			return fmt.Errorf("could not read a log record: %w", err)
		}
		if len(payload.Errors) > 0 {
			return fmt.Errorf("the log subscription failed: %s", payload.Errors[0].Message)
		}
		if payload.Data == nil || payload.Data.Log == nil {
			return nil
		}
		l := payload.Data.Log
		return onLog(DeploymentLog{ID: l.ID, CreatedAt: l.CreatedAt, Log: l.Log})
	case "connection_error":
		// Houston refused the token when the connection opened: its onConnect
		// found no user for it, an unknown or expired one, and returned false.
		return fmt.Errorf("the log stream refused your login (%s); it may have expired: log in again with `astro login`", frameMessage(frame.Payload))
	case "error":
		return fmt.Errorf("the log subscription failed: %s", frameMessage(frame.Payload))
	case "complete":
		return errStreamComplete
	default:
		// connection_ack, and keep-alives ("ka") should a server send them:
		// Houston configures none.
		return nil
	}
}

// frameMessage is an error frame's message: its payload's "message", or the
// payload itself when it has none.
func frameMessage(payload json.RawMessage) string {
	var p struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(payload, &p) == nil && p.Message != "" {
		return p.Message
	}
	return string(payload)
}
