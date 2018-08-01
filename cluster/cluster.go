package cluster

import (
	"fmt"
	"strings"

	"github.com/astronomerio/astro-cli/config"
)

func ListClusters() error {
	c, err := config.GetClusters()
	if err != nil {
		return err
	}

	for k, v := range c.Clusters {
		var name string
		if v.Domain != "" {
			name = v.Domain
		} else {
			name = strings.Replace(k, "_", ".", -1)
		}
		fmt.Println(name)
		fmt.Printf("\t Workspace: %s", v.Workspace)
		fmt.Println("\n")
	}

	return nil
}
