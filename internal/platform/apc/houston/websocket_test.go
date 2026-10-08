package houston

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{}

// scriptedServer is a log subscription server: it reads the client's init
// and subscribe messages, writes frames, then either closes the connection
// abruptly (hold false) or keeps writing keep-alives without reading again,
// so the client's close is never answered (hold true).
func scriptedServer(frames []string, hold bool) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		for range 2 {
			if _, _, err := c.ReadMessage(); err != nil {
				return
			}
		}
		for _, f := range frames {
			if err := c.WriteMessage(websocket.TextMessage, []byte(f)); err != nil {
				return
			}
		}
		for hold {
			if err := c.WriteMessage(websocket.TextMessage, []byte(`{"type":"data","payload":{"data":{"log":{"log":"more"}}}}`)); err != nil {
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
	}))
}

func wsURL(srv *httptest.Server) string { return "ws" + strings.TrimPrefix(srv.URL, "http") }

func data(log string) string {
	return `{"type":"data","id":"1","payload":{"data":{"log":{"id":"x","createdAt":"t","log":"` + log + `"}}}}`
}

// collect is an onLog that records each record's text.
type collect struct {
	sync.Mutex
	got []string
}

func (c *collect) onLog(l DeploymentLog) error {
	c.Lock()
	defer c.Unlock()
	c.got = append(c.got, l.Log)
	return nil
}

func (c *collect) records() []string {
	c.Lock()
	defer c.Unlock()
	return append([]string(nil), c.got...)
}

func (s *Suite) TestBuildDeploymentLogsSubscribeRequest() {
	resp, err := BuildDeploymentLogsSubscribeRequest("test-id", "test-component", "test", time.Time{})
	s.NoError(err)
	s.Contains(resp, "test-id")
	s.Contains(resp, "test-component")
	s.Contains(resp, "test")
}

func (s *Suite) TestSubscribe() {
	// Only "data" frames are records. connection_ack and keep-alives carry
	// none; "complete" ends the stream without an error.
	s.Run("records are the data frames; complete ends it", func() {
		srv := scriptedServer([]string{`{"type":"connection_ack"}`, `{"type":"ka"}`, data("a"), `{"type":"ka"}`, data("b"), `{"type":"complete","id":"1"}`}, false)
		defer srv.Close()

		c := new(collect)
		notes := new(bytes.Buffer)
		err := Subscribe("test-token", wsURL(srv), `{}`, notes, c.onLog)
		s.NoError(err)
		s.Equal([]string{"a", "b"}, c.records())
		s.Equal("Waiting for logs...\n", notes.String())
	})

	// A GraphQL execution error inside a subscription arrives in a data
	// frame, with payload.errors, as subscriptions-transport-ws (which Houston
	// serves subscriptions with) sends it. It is a failure, not an empty
	// record.
	s.Run("a data frame with errors fails the stream", func() {
		srv := scriptedServer([]string{`{"type":"data","id":"1","payload":{"errors":[{"message":"Insufficient permissions."}]}}`, `{"type":"complete","id":"1"}`}, false)
		defer srv.Close()

		c := new(collect)
		err := Subscribe("test-token", wsURL(srv), `{}`, io.Discard, c.onLog)
		s.EqualError(err, "the log subscription failed: Insufficient permissions.")
		s.Empty(c.records())
	})

	s.Run("a data frame with no record is skipped", func() {
		srv := scriptedServer([]string{`{"type":"data","id":"1","payload":{"data":{"log":null}}}`, data("a"), `{"type":"complete","id":"1"}`}, false)
		defer srv.Close()

		c := new(collect)
		s.NoError(Subscribe("test-token", wsURL(srv), `{}`, io.Discard, c.onLog))
		s.Equal([]string{"a"}, c.records())
	})

	// A token Houston refuses when the connection opens is a
	// connection_error frame, then a close; its message is the failure.
	s.Run("a connection_error frame fails the stream with its message", func() {
		srv := scriptedServer([]string{`{"type":"connection_error","payload":{"message":"Prohibited connection!"}}`}, false)
		defer srv.Close()

		err := Subscribe("test-token", wsURL(srv), `{}`, io.Discard, func(DeploymentLog) error { return nil })
		s.EqualError(err, "the log stream refused your login (Prohibited connection!); it may have expired: log in again with `astro login`")
	})

	s.Run("the start names its operation", func() {
		msg, err := BuildDeploymentLogsSubscribeRequest("d", "scheduler", "", time.Time{})
		s.NoError(err)
		s.Contains(msg, `"id":"1"`)
		s.Contains(msg, `"type":"start"`)
	})

	s.Run("an error frame fails the stream", func() {
		srv := scriptedServer([]string{data("a"), `{"type":"error","id":"1","payload":{"message":"not authorized"}}`}, false)
		defer srv.Close()

		c := new(collect)
		err := Subscribe("test-token", wsURL(srv), `{}`, io.Discard, c.onLog)
		s.ErrorContains(err, "the log subscription failed")
		s.ErrorContains(err, "not authorized")
		s.Equal([]string{"a"}, c.records())
	})

	// It used to print "Your token has expired" and return nil, so a follow
	// that ended exited 0.
	s.Run("the server closing the connection is an error", func() {
		srv := scriptedServer([]string{data("a")}, false)
		defer srv.Close()

		c := new(collect)
		err := Subscribe("test-token", wsURL(srv), `{}`, io.Discard, c.onLog)
		s.ErrorIs(err, ErrLogStreamClosed)
		s.Equal([]string{"a"}, c.records())
	})

	s.Run("onLog's error ends the stream", func() {
		srv := scriptedServer([]string{data("a")}, true)
		defer srv.Close()

		errStop := errors.New("stop")
		err := Subscribe("test-token", wsURL(srv), `{}`, io.Discard, func(DeploymentLog) error { return errStop })
		s.ErrorIs(err, errStop)
	})

	s.Run("no server", func() {
		err := Subscribe("test-token", "ws://127.0.0.1:1", `{}`, io.Discard, func(DeploymentLog) error { return nil })
		s.ErrorContains(err, "could not connect to the log stream")
	})

	// An interrupt the server never answers ends the stream after the wait,
	// and nothing reaches onLog once Subscribe has returned.
	s.Run("interrupt: nothing is delivered after it returns", func() {
		srv := scriptedServer([]string{data("a")}, true)
		defer srv.Close()
		prev := notifyInterrupt
		defer func() { notifyInterrupt = prev }()
		var interrupt chan<- os.Signal
		notifyInterrupt = func(c chan<- os.Signal) { interrupt = c }

		c := new(collect)
		var once sync.Once
		first := true
		err := Subscribe("test-token", wsURL(srv), `{}`, io.Discard, func(l DeploymentLog) error {
			once.Do(func() { interrupt <- os.Interrupt })
			// A slow consumer: the reader is inside onLog when the wait for
			// the server runs out, so a Subscribe that returned without
			// waiting for it would see this record land afterwards.
			if !first {
				time.Sleep(200 * time.Millisecond)
			}
			first = false
			return c.onLog(l)
		})
		s.NoError(err)
		returned := len(c.records())
		s.Positive(returned)
		time.Sleep(50 * time.Millisecond)
		s.Len(c.records(), returned, "a record was delivered after Subscribe returned")
	})
}

// The window reaches Houston as top-level startTime and endTime, and is left
// out when there is none: Houston then reads timestamp as a day.
func (s *Suite) TestListDeploymentLogsRequestVariables() {
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	b, err := json.Marshal(ListDeploymentLogsRequest{DeploymentID: "d", Timestamp: &at})
	s.NoError(err)
	s.NotContains(string(b), "startTime")

	b, err = json.Marshal(ListDeploymentLogsRequest{DeploymentID: "d", Timestamp: &at, LogWindow: &LogWindow{StartTime: at, EndTime: at.Add(time.Minute)}})
	s.NoError(err)
	s.Contains(string(b), `"startTime":"2026-01-02T03:04:05Z","endTime":"2026-01-02T03:05:05Z"`)
}
