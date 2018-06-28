package docker

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/astronomerio/astro-cli/config"
	"github.com/astronomerio/astro-cli/messages"
	"github.com/pkg/errors"
)

// ListRepositoryTagsResponse is a response for listing tags
type ListRepositoryTagsResponse struct {
	Name string   `json:"name"`
	Tags []string `json:"tags"`
}

// ListRepositoryTags lists the tags for a given repository
func ListRepositoryTags(repository string) ([]string, error) {
	registry := config.CFG.RegistryAuthority.GetString()
	user, password, err := config.GetDecodedAuth()
	if err != nil {
		return []string{}, errors.Wrap(err, messages.REGISTRY_DECODE_AUTH_ERROR)
	}

	// Get an HTTP Client
	client := &http.Client{}

	// Create our request
	url := fmt.Sprintf("https://%s/v2/%s/tags/list", registry, repository)
	req, createErr := http.NewRequest("GET", url, nil)
	if createErr != nil {
		return []string{}, errors.Wrap(createErr, messages.REGISTRY_TAGS_REQUEST_ERROR)
	}
	req.SetBasicAuth(user, password)

	// Make the request
	resp, reqErr := client.Do(req)

	if reqErr != nil {
		return []string{}, errors.Wrap(reqErr, messages.REGISTRY_TAGS_REQUEST_ERROR)
	}

	// TODO Remove config suggestion and 401 check once houston is handling auth
	if resp.StatusCode == 401 {

		fmt.Println(messages.REGISTRY_AUTH_FAIL)
		return []string{}, errors.New("")
	}

	// Close body reader
	defer resp.Body.Close()

	// Decode and return result
	tags := ListRepositoryTagsResponse{}
	json.NewDecoder(resp.Body).Decode(&tags)

	return tags.Tags, nil
}
