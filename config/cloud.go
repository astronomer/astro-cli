package config

import (
	"io"

	"github.com/astronomer/astro-cli/pkg/domainutil"
	"github.com/astronomer/astro-cli/pkg/printutil"
)

const (
	graphqlEndpoint = "graphql"
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

// GetPublicGraphQLAPIURL returns full Astrohub API Url for the provided Context
func (c *Context) GetPublicGraphQLAPIURL() string {
	if c.Domain == localhostDomain || c.Domain == astrohubDomain {
		return CFG.LocalPublicAstro.GetString()
	}
	domain := domainutil.FormatDomain(c.Domain)
	return domainutil.GetURLToEndpoint(CFG.CloudAPIProtocol.GetString(), domain, graphqlEndpoint)
}

// GetPublicRESTAPIURL returns full core API Url for the provided Context
func (c *Context) GetPublicRESTAPIURL(version string) string {
	if c.Domain == localhostDomain || c.Domain == astrohubDomain {
		return CFG.LocalCore.GetString() + "/" + version
	}

	domain := domainutil.FormatDomain(c.Domain)
	addr := domainutil.GetURLToEndpoint(CFG.CloudAPIProtocol.GetString(), domain, version)
	return domainutil.TransformToCoreAPIEndpoint(addr)
}
