package config

import (
	"io"

	"github.com/astronomer/astro-cli/pkg/domainutil"
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

// GetPublicRESTAPIURL returns full core API Url for the provided Context
func (c *Context) GetPublicRESTAPIURL(version string) string {
	if c.Domain == localhostDomain {
		return CFG.LocalCore.GetString() + "/" + version
	}

	return domainutil.GetURLToEndpoint(apiProtocol(), domainutil.FormatDomain(c.Domain), version)
}

// apiProtocol is the scheme the API URLs are built with: cloud.api.protocol
// when it is http or https, https otherwise. The value is spliced in front of
// "://" and the host, so anything else (a full URL, say) would move every
// request, token and all, to another host.
func apiProtocol() string {
	if p := CFG.CloudAPIProtocol.GetString(); p == "http" {
		return p
	}
	return "https"
}
