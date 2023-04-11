package registry

import (
	"encoding/json"
	log "github.com/sirupsen/logrus"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

const (
	registryHost        = "api.astronomer.io"
	registryApi         = "registryV2/v1alpha1/organizations/public"
	dagRoute            = "dags/%s/versions/%s"
	providerRoute       = "providers/%s/versions/%s"
	providerSearchRoute = "providers"
	providerSearchQuery = "limit=1&query=%s"
)

func logFatal(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func logKeyNotExists(exists bool, key string, jsonValue map[string]interface{}) {
	if !exists {
		jsonString, _ := json.Marshal(jsonValue)
		log.Fatalf("Couldn't find key %s in Response! %s", key, jsonString)
	}
}

func requestAndGetJsonBody(route string) map[string]interface{} {
	res, err := http.Get(route)
	logFatal(err)
	body, err := io.ReadAll(res.Body)
	logFatal(err)
	if res.StatusCode > 299 {
		log.Fatalf("Response failed with status code: %d and\nbody: %s\n", res.StatusCode, body)
	}
	defer res.Body.Close()
	logFatal(err)
	var bodyJson map[string]interface{}
	err = json.Unmarshal(body, &bodyJson)
	logFatal(err)
	log.Debugf("%s - GET %s %s", res.Status, route, string(body))
	return bodyJson
}

func create(p string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(p), 0770); err != nil {
		return nil, err
	}
	return os.Create(p)
}
