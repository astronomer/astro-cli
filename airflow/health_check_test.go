package airflow

import (
	"context"
	"net/http"
	"net/http/httptest"
	"time"
)

func (s *Suite) TestWebserverHealthCheck() {
	s.Run("Success", func() {
		// Create a mock server that returns a 200 status
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		err := checkWebserverHealth(server.URL, 5*time.Second)
		s.NoError(err)
	})
	s.Run("Failure", func() {
		// Create a mock server that responds slowly.
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(2 * time.Second) // Simulate a long response
		}))
		defer server.Close()

		err := checkWebserverHealth(server.URL, 1*time.Second)
		s.Error(err)
	})
}

func (s *Suite) TestHealthCheck() {
	s.Run("Scenarios", func() {
		// Create a mock server that returns a 200 status
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := &http.Client{Timeout: 5 * time.Second}

		tests := []struct {
			name          string
			url           string
			expectedCode  int
			expectedError bool
		}{
			{"Webserver not running yet", "https://server-not-running", 0, true},
			{"Webserver is running and healthy", server.URL, http.StatusOK, false},
		}

		for _, tt := range tests {
			s.Run(tt.name, func() {
				ctx := context.Background()
				code, err := healthCheck(ctx, client, tt.url)
				// Check if the response code matches the expected code
				if code != tt.expectedCode {
					s.Equal(tt.expectedCode, code)
				}
				// If we expect an error, ensure we got one.
				if tt.expectedError {
					s.Error(err)
				} else {
					s.NoError(err)
				}
			})
		}
	})

	s.Run("Timeout", func() {
		// Create a server that never responds
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(2 * time.Second) // Simulate a long response
		}))
		defer server.Close()

		// Set a shorter timeout
		client := &http.Client{Timeout: 1 * time.Second}
		ctx := context.Background()
		code, err := healthCheck(ctx, client, server.URL)
		s.Equal(0, code)
		s.Error(err)
	})

	s.Run("Error", func() {
		client := &http.Client{Timeout: 5 * time.Second}
		ctx := context.Background()
		code, err := healthCheck(ctx, client, "http://invalid.url")
		s.Equal(0, code)
		s.Error(err)
	})
}
