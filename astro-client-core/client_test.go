package astrocore

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeAPIError(t *testing.T) {
	testCases := []struct {
		name             string
		statusCode       int
		responseBody     string
		expectedError    string
		expectedContains []string
	}{
		{
			name:          "401 error with non-JSON body should return enhanced auth message",
			statusCode:    401,
			responseBody:  "Unauthorized",
			expectedError: "authentication required to access Astro API.\n\nTo resolve this issue:\n• Run 'astro login' to authenticate with your Astro account\n• Ensure you have valid credentials and access to the requested resource\n• If you recently logged in, your token may have expired - please login again\n\nYou must be authenticated to use commands that access Astro deployments, workspaces, or environment objects.",
		},
		{
			name:          "401 error with empty body should return enhanced auth message",
			statusCode:    401,
			responseBody:  "",
			expectedError: "authentication required to access Astro API.\n\nTo resolve this issue:\n• Run 'astro login' to authenticate with your Astro account\n• Ensure you have valid credentials and access to the requested resource\n• If you recently logged in, your token may have expired - please login again\n\nYou must be authenticated to use commands that access Astro deployments, workspaces, or environment objects.",
		},
		{
			name:          "401 error with HTML body should return enhanced auth message",
			statusCode:    401,
			responseBody:  "<html><body><h1>Unauthorized</h1></body></html>",
			expectedError: "authentication required to access Astro API.\n\nTo resolve this issue:\n• Run 'astro login' to authenticate with your Astro account\n• Ensure you have valid credentials and access to the requested resource\n• If you recently logged in, your token may have expired - please login again\n\nYou must be authenticated to use commands that access Astro deployments, workspaces, or environment objects.",
		},
		{
			name:          "401 error with valid JSON should return JSON message",
			statusCode:    401,
			responseBody:  `{"message": "Invalid token provided"}`,
			expectedError: "Invalid token provided",
		},
		{
			name:          "403 error with non-JSON body should return generic error",
			statusCode:    403,
			responseBody:  "Forbidden",
			expectedError: "failed to perform request, status 403",
		},
		{
			name:          "500 error with non-JSON body should return generic error",
			statusCode:    500,
			responseBody:  "Internal Server Error",
			expectedError: "failed to perform request, status 500",
		},
		{
			name:          "404 error with valid JSON should return JSON message",
			statusCode:    404,
			responseBody:  `{"message": "Resource not found"}`,
			expectedError: "Resource not found",
		},
		{
			name:          "200 success should return nil",
			statusCode:    200,
			responseBody:  `{"data": "success"}`,
			expectedError: "",
		},
		{
			name:          "204 success should return nil",
			statusCode:    204,
			responseBody:  "",
			expectedError: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create mock HTTP response
			resp := &http.Response{
				StatusCode: tc.statusCode,
			}
			body := []byte(tc.responseBody)

			err := NormalizeAPIError(resp, body)

			if tc.expectedError == "" {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Equal(t, tc.expectedError, err.Error())
			}

			// Additional checks for the enhanced 401 message
			if tc.statusCode == 401 && !isValidJSON(tc.responseBody) {
				assert.Contains(t, err.Error(), "astro login")
				assert.Contains(t, err.Error(), "authentication required")
				assert.Contains(t, err.Error(), "token may have expired")
			}
		})
	}
}

// Helper function to check if a string is valid JSON
func isValidJSON(s string) bool {
	var js Error
	return json.NewDecoder(bytes.NewReader([]byte(s))).Decode(&js) == nil
}

func TestAuthenticationRequiredErrorMessage(t *testing.T) {
	// Test that the authentication required error message contains all expected guidance
	expectedElements := []string{
		"authentication required",
		"astro login",
		"valid credentials",
		"token may have expired",
		"deployments, workspaces, or environment objects",
	}

	for _, element := range expectedElements {
		assert.Contains(t, authenticationRequiredErrMsg, element, fmt.Sprintf("Error message should contain: %s", element))
	}

	// Test that the message is multi-line and well-formatted
	assert.Contains(t, authenticationRequiredErrMsg, "To resolve this issue:")
	assert.Contains(t, authenticationRequiredErrMsg, "•")
}
