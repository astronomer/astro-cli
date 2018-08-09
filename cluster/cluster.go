package cluster

import (
	"errors"
	"fmt"
	"strings"

	"github.com/astronomerio/astro-cli/config"
)

// ListClusters lists all available clusters a user has previously
// authenticated to
// Returns error
func List() error {
	var domain string
	c, err := GetClusters()
	if err != nil {
		return err
	}

	ctx, err := GetCurrentCluster()
	if err != nil {
		return err
	}

	for k, v := range c.Contexts {
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

// ClusterExists checks to see if cluster exist in config
func Exists(domain string) bool {
	c := config.Context{Domain: domain}

	return c.ContextExists()
}

// GetCurrentCluster gets the  current cluster set in the config
// Returns full Context struct
func GetCurrentCluster() (config.Context, error) {
	c := config.Context{}

	domain := config.CFG.Context.GetHomeString()
	if len(domain) == 0 {
		return config.Context{}, errors.New("No context set, have you authenticated to a cluster?")
	}

	c.Domain = domain
	return c.GetContext()
}

// GetCluster gets the specified context by domain name
// Returns the matching Context struct
func GetCluster(domain string) (config.Context, error) {
	c := config.Context{Domain: domain}
	return c.GetContext()
}

// GetClusters gets all clusters currently in the config
// Returns a Contexts struct
func GetClusters() (config.Contexts, error) {
	var c config.Contexts
	return c.GetContexts()
}

// SetCluster creates or updates a clusters domain name
// Returns an error
func SetCluster(domain string) error {
	c := config.Context{Domain: domain}
	return c.SetContext()
}

// SwitchCluster is a thin wrapper around the switch cluster receiver
// Returns error
func Switch(domain string) error {
	c := config.Context{Domain: domain}
	err := c.SwitchContext()
	if err != nil {
		return err
	}
	fmt.Printf("Cluster: %s\n", c.Domain)
	return nil
}
