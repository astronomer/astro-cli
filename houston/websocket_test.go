package houston

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
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

func TestBuildDeploymentLogsSubscribeRequest(t *testing.T) {
	resp, err := BuildDeploymentLogsSubscribeRequest("test-id", "test-component", "test", time.Time{})
	assert.NoError(t, err)
	assert.Contains(t, resp, "test-id")
	assert.Contains(t, resp, "test-component")
	assert.Contains(t, resp, "test")
}

func TestSubscribe(t *testing.T) {
	// Create test server with the echo handler.
	s := httptest.NewServer(http.HandlerFunc(websocketHandler))
	defer s.Close()

	t.Run("success", func(t *testing.T) {
		// Convert http://127.0.0.1 to ws://127.0.0.
		url := "ws" + strings.TrimPrefix(s.URL, "http")

		err := Subscribe("test-token", url, `{"type": "test", "payload": {"data": {"log": {"log": "test"}}}}`)
		assert.NoError(t, err)
	})
}
