package cluster

import (
	"fmt"
	"strings"

	"github.com/astronomerio/astro-cli/config"
)

// List all available clusters a user has previously
// authenticated to
// Returns error
func List() error {
	r := "  %-44s"
	colorFmt := "\033[33;m"
	colorTrm := "\033[0m"
	var domain string

	c, err := GetClusters()
	if err != nil {
		return err
	}

	ctx, err := GetCurrentCluster()
	if err != nil {
		return err
	}

	h := fmt.Sprintf(r, "NAME")
	fmt.Println(h)
	for k, v := range c.Contexts {
		if v.Domain != "" {
			domain = v.Domain
		} else {
			domain = strings.Replace(k, "_", ".", -1)
		}

		fullStr := fmt.Sprintf(r, domain)
		if domain == ctx.Domain {
			fullStr = colorFmt + fullStr + colorTrm
		}

		fmt.Println(fullStr)

	}

	return nil
}

// ClusterExists checks to see if cluster exist in config
func Exists(domain string) bool {
	c := config.Context{Domain: domain}

	return c.ContextExists()
}

// GetCurrentCluster gets the  current cluster set in the config
// Is a convenience wrapp around config.GetCurrentContext()
// Returns full Context struct
func GetCurrentCluster() (config.Context, error) {
	return config.GetCurrentContext()
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
	config.PrintCurrentContext()
	return nil
}
