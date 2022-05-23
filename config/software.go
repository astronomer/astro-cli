package config

import (
	"errors"
	"fmt"
	"io"

	"github.com/astronomer/astro-cli/pkg/printutil"
)

// PrintSoftwareContext prints current context to stdOut
func (c *Context) PrintSoftwareContext(out io.Writer) error {
	context, err := c.GetContext()
	if err != nil && !errors.Is(err, errNotConnected) {
		return err
	}

	ctx := context.Domain
	if ctx == "" {
		ctx = "N/A"
	}

	workspace := context.Workspace
	if workspace == "" {
		workspace = "N/A"
	}
	tab := printutil.Table{
		Padding: []int{36, 36},
		Header:  []string{"CLUSTER", "WORKSPACE"},
	}

	tab.AddRow([]string{ctx, workspace}, false)
	tab.Print(out)

	return nil
}

// PrintCurrentSoftwareContext prints the current config context
func PrintCurrentSoftwareContext(out io.Writer) error {
	c, err := GetCurrentContext()
	if err != nil {
		return err
	}

	err = c.PrintSoftwareContext(out)
	if err != nil {
		return err
	}

	return nil
}

// GetSoftwareAPIURL returns full Software API Url for the provided Context
func (c *Context) GetSoftwareAPIURL() string {
	if c.Domain == localhostDomain || c.Domain == houstonDomain {
		return CFG.LocalHouston.GetString()
	}

	return fmt.Sprintf(
		"%s://houston.%s:%s/v1",
		CFG.CloudAPIProtocol.GetString(),
		c.Domain,
		CFG.CloudAPIPort.GetString(),
	)
}

// GetSoftwareAppURL returns full Houston API Url for the provided Context
func (c *Context) GetSoftwareAppURL() string {
	if c.Domain == localhostDomain || c.Domain == houstonDomain {
		return CFG.LocalHouston.GetString()
	}

	return fmt.Sprintf(
		"%s://app.%s",
		CFG.CloudAPIProtocol.GetString(),
		c.Domain,
	)
}

// GetSoftwareWebsocketURL returns full Houston websocket Url for the provided Context
func (c *Context) GetSoftwareWebsocketURL() string {
	if c.Domain == localhostDomain || c.Domain == houstonDomain {
		return CFG.LocalHouston.GetString()
	}

	return fmt.Sprintf(
		"%s://houston.%s:%s/ws",
		CFG.CloudWSProtocol.GetString(),
		c.Domain,
		CFG.CloudAPIPort.GetString(),
	)
}
