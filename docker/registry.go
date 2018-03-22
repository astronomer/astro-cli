package docker

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/astronomerio/astro-cli/config"
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
	user := config.CFG.RegistryUser.GetString()
	password := config.CFG.RegistryPassword.GetString()

	// Get an HTTP Client
	client := &http.Client{}

	// Create our request
	url := fmt.Sprintf("https://%s/v2/%s/tags/list", registry, repository)
	req, createErr := http.NewRequest("GET", url, nil)
	if createErr != nil {
		return []string{}, errors.Wrap(createErr, "Error requesting repositories")
	}
	req.SetBasicAuth(user, password)

	// Make the request
	resp, reqErr := client.Do(req)

	if reqErr != nil {
		return []string{}, errors.Wrap(reqErr, "Error requesting repositories")
	}

	// TODO Remove config suggestion and 401 check once houston is handling auth
	if resp.StatusCode == 401 {

		fmt.Println(`Failed to authenticate to registry
	
	You can set your registry auth using
		astro config set docker.registry.user [registry_user] -g
		astro config set docker.registry.password [registry_password] -g
		`)
		return []string{}, errors.New("")
	}

	// Close body reader
	defer resp.Body.Close()

	// Decode and return result
	tags := ListRepositoryTagsResponse{}
	json.NewDecoder(resp.Body).Decode(&tags)

	return tags.Tags, nil
}
