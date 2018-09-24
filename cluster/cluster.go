package cluster

import (
	"strings"

	"github.com/astronomerio/astro-cli/config"
	"github.com/astronomerio/astro-cli/pkg/printutil"
)

// List all available clusters a user has previously authenticated to
// Returns error
func List() error {
	tab := printutil.Table{
		Padding:      []int{44},
		Header:       []string{"NAME"},
		ColorRowCode: [2]string{"\033[33;m", "\033[0m"},
	}

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
			tab.AddRow([]string{domain}, true)
		} else {
			tab.AddRow([]string{domain}, false)
		}
	}

	tab.Print()

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
