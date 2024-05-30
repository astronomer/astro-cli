package houston

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

var (
	upgrader = websocket.Upgrader{}

	websocketServerRequestCount = 1
)

func websocketHandler(w http.ResponseWriter, r *http.Request) {
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer c.Close()
	currCount := 0
	for {
		mt, message, err := c.ReadMessage()
		if err != nil {
			break
		}
		// simply write back what was read from the connection
		err = c.WriteMessage(mt, message)
		if err != nil {
			break
		}
		if currCount >= websocketServerRequestCount {
			break
		}
		currCount++
	}
}

func (s *Suite) TestBuildDeploymentLogsSubscribeRequest() {
	resp, err := BuildDeploymentLogsSubscribeRequest("test-id", "test-component", "test", time.Time{})
	s.NoError(err)
	s.Contains(resp, "test-id")
	s.Contains(resp, "test-component")
	s.Contains(resp, "test")
}

func (s *Suite) TestSubscribe() {
	// Create test server with the echo handler.
	srv := httptest.NewServer(http.HandlerFunc(websocketHandler))
	defer srv.Close()

	s.Run("success", func() {
		// Convert http://127.0.0.1 to ws://127.0.0.
		url := "ws" + strings.TrimPrefix(srv.URL, "http")

		err := Subscribe("test-token", url, `{"type": "test", "payload": {"data": {"log": {"log": "test"}}}}`)
		s.NoError(err)
	})
}
