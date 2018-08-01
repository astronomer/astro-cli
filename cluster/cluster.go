package cluster

import (
	"fmt"
	"strings"

	"github.com/astronomerio/astro-cli/config"
)

func ListClusters() error {
	var domain string
	c, err := config.GetClusters()
	if err != nil {
		return err
	}

	ctx, err := config.GetCurrentCluster()
	if err != nil {
		return err
	}

	for k, v := range c.Clusters {
		if v.Domain != "" {
			domain = v.Domain
		} else {
			domain = strings.Replace(k, "_", ".", -1)
		}

		if domain == ctx.Domain {
			domain = domain + " (current)"
		}

		fmt.Println(domain)

	}

	return nil
}
