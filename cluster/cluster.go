package cluster

import (
	"strconv"
	"strings"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/printutil"
	"github.com/pkg/errors"
)

var (
	tab = printutil.Table{
		Padding:      []int{44},
		Header:       []string{"NAME"},
		ColorRowCode: [2]string{"\033[1;32m", "\033[0m"},
	}
)

// List all available clusters a user has previously authenticated to
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

func getClusterSelection() (string, error) {
	var domain string
	tab.GetUserInput = true

	c, err := GetClusters()
	if err != nil {
		return "", err
	}

	ctx, err := GetCurrentCluster()
	if err != nil {
		return "", err
	}

	var contexts []string
	for k, v := range c.Contexts {
		if v.Domain != "" {
			domain = v.Domain
		} else {
			domain = strings.Replace(k, "_", ".", -1)
		}

		contexts = append(contexts, domain)

		colorRow := false
		if domain == ctx.Domain {
			colorRow = true
		}

		tab.AddRow([]string{domain}, colorRow)
	}

	tab.Print()

	in := input.InputText("\n> ")
	i, err := strconv.ParseInt(
		in,
		10,
		64,
	)

	if err != nil {
		return "", errors.Wrapf(err, "cannot parse %s to int", in)
	}

	return contexts[i-1], nil

}

// SwitchCluster is a thin wrapper around the switch cluster receiver
// Returns error
func Switch(domain string) error {
	if len(domain) == 0 {
		d, err := getClusterSelection()
		if err != nil {
			return err
		}
		domain = d
	}

	c := config.Context{Domain: domain}
	err := c.SwitchContext()
	if err != nil {
		return err
	}

	return nil
}
