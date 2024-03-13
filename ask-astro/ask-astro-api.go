package askastro

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const ASTRO_API_URL = "https://ask-astro-prod-sxjq32dlaa-uc.a.run.app"

var (
	spinnerMessages = []string{
		"Checking Idempotency",
		"Checking Atomic Tasks",
		"Checking for Unique Code",
		"Checking for No Top Level Code",
		"Checking Task Dependency Consistency",
		"Checking for Static Start Date",
		"Checking for Task Retries",
	}
)

type PostRequestBody struct {
	Prompt          string `json:"prompt"`
	FromRequestUUID string `json:"from_request_uuid,omitempty"`
}

func postRequest(req PostRequestBody) (map[string]interface{}, error) {
	postURL := urljoin(ASTRO_API_URL, "requests")
	reqJSON, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	resp, err := http.Post(postURL, "application/json", strings.NewReader(string(reqJSON)))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	err = json.Unmarshal(body, &result)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func monitorRequest(requestUUID string, timeout int, pollInterval int) (map[string]interface{}, error) {
	getURL := urljoin(ASTRO_API_URL, fmt.Sprintf("requests/%s", requestUUID))
	for i := 0; i < timeout; i += pollInterval {
		resp, err := http.Get(getURL)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}

		var result map[string]interface{}
		err = json.Unmarshal(body, &result)
		if err != nil {
			return nil, err
		}

		if result["status"] == "in_progress" {
			time.Sleep(time.Duration(pollInterval) * time.Second)
		} else {
			fmt.Println("done")
			return result, nil
		}
	}

	return nil, fmt.Errorf("Timeout exceeded")
}

func urljoin(base, path string) string {
	baseURL, err := url.Parse(base)
	if err != nil {
		panic(err)
	}

	pathURL, err := url.Parse(path)
	if err != nil {
		panic(err)
	}

	return baseURL.ResolveReference(pathURL).String()
}

func askAstroRequest(prompt string) (string, error) {
	// Example usage
	req := PostRequestBody{
		Prompt: prompt,
	}

	result, err := postRequest(req)
	if err != nil {
		return "", err
	}

	requestUUIDRaw, ok := result["request_uuid"]
	if !ok || requestUUIDRaw == nil {
		fmt.Println(result)
		fmt.Println("request_uuid is nil or not found in the result map")
	}

	requestUUID := result["request_uuid"].(string)

	result, err = monitorRequest(requestUUID, 600, 5)
	if err != nil {
		return "", err
	}

	responseRaw, ok := result["response"]
	if !ok || responseRaw == nil {
		fmt.Println(result)
		fmt.Println("request_uuid is nil or not found in the result map")
	}

	response := result["response"].(string)

	return response, nil
}
