package airflow

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCheckWebserverHealthSuccess(t *testing.T) {
	// Create a mock server that returns a 200 status
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	err := checkWebserverHealth(server.URL, 5*time.Second)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
}

func TestCheckWebserverHealthFailure(t *testing.T) {
	// Create a mock server that responds slowly.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second) // Simulate a long response
	}))
	defer server.Close()

	err := checkWebserverHealth(server.URL, 1*time.Second)
	if err == nil {
		t.Errorf("Expected an error, got nil")
	}
}

func TestHealthCheck(t *testing.T) {
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
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			code, err := healthCheck(ctx, client, tt.url)
			// Check if the response code matches the expected code
			if code != tt.expectedCode {
				t.Errorf("Expected status code %d, got %d", tt.expectedCode, code)
			}
			// If we expect an error, ensure we got one.
			if tt.expectedError {
				if err == nil {
					t.Errorf("Expected an error, got status code %d", code)
				}
				return
			} else if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestHealthCheckTimeout(t *testing.T) {
	// Create a server that never responds
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second) // Simulate a long response
	}))
	defer server.Close()

	// Set a shorter timeout
	client := &http.Client{Timeout: 1 * time.Second}

	ctx := context.Background()
	code, err := healthCheck(ctx, client, server.URL)
	if code != 0 {
		t.Errorf("Expected status code 0, got %d", code)
	}
	if err == nil {
		t.Errorf("Expected an error due to timeout, got status code %d", code)
	}
}

func TestHealthCheckError(t *testing.T) {
	client := &http.Client{Timeout: 5 * time.Second}

	ctx := context.Background()
	code, err := healthCheck(ctx, client, "http://invalid.url")
	if code != 0 {
		t.Errorf("Expected status code 0, got %d", code)
	}
	if err == nil {
		t.Errorf("Expected an error for invalid URL, got status code %d", code)
	}
}
