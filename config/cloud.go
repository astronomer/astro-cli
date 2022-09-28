package config

import (
	"fmt"
	"io"
	"strings"

	"github.com/astronomer/astro-cli/pkg/printutil"
)

// PrintCloudContext prints current context to stdOut
func (c *Context) PrintCloudContext(out io.Writer) error {
	context, err := c.GetContext()
	if err != nil {
		return err
	}

	ctx := context.Domain
	if ctx == "" {
		ctx = noApply
	}

	workspace := context.Workspace
	if workspace == "" {
		workspace = noApply
	}
	tab := printutil.Table{
		Padding: []int{36, 36},
		Header:  []string{"CONTROLPLANE", "WORKSPACE"},
	}

	tab.AddRow([]string{ctx, workspace}, false)
	tab.Print(out)

	return nil
}

// PrintCurrentCloudContext prints the current config context
func PrintCurrentCloudContext(out io.Writer) error {
	c, err := GetCurrentContext()
	if err != nil {
		return err
	}

	err = c.PrintCloudContext(out)
	if err != nil {
		return err
	}

	return nil
}

// GetCloudAPIURL returns full Astronomer API Url for the provided Context
func (c *Context) GetCloudAPIURL() string {
	if c.Domain == localhostDomain || c.Domain == astrohubDomain {
		return CFG.LocalAstro.GetString()
	}

	domain := c.Domain
	if strings.Contains(domain, cloudDomain) {
		splitDomain := strings.SplitN(domain, ".", splitNum) // This splits out 'cloud' from the domain string
		domain = splitDomain[1]
	}

	return fmt.Sprintf(
		"%s://api.%s/hub/v1",
		CFG.CloudAPIProtocol.GetString(),
		domain,
	)
}

func (c *Context) GetAPIDomain() string {
	domain := c.Domain
	if strings.Contains(domain, cloudDomain) {
		splitDomain := strings.SplitN(domain, ".", splitNum) // This splits out 'cloud' from the domain string
		domain = splitDomain[1]
	}
	return domain
}

// GetPublicGraphQLAPIURL returns full Astrohub API Url for the provided Context
func (c *Context) GetPublicGraphQLAPIURL() string {
	if c.Domain == localhostDomain || c.Domain == astrohubDomain {
		return CFG.LocalPublicAstro.GetString()
	}

	return fmt.Sprintf(
		"%s://api.%s/hub/graphql",
		CFG.CloudAPIProtocol.GetString(),
		c.GetAPIDomain(),
	)
}

// GetPublicRESTAPIURL returns full core API Url for the provided Context
func (c *Context) GetPublicRESTAPIURL() string {
	if c.Domain == localhostDomain || c.Domain == astrohubDomain {
		return CFG.LocalCore.GetString()
	}

	return fmt.Sprintf(
		"%s://api.%s/v1alpha1",
		CFG.CloudAPIProtocol.GetString(),
		c.GetAPIDomain(),
	)
}
