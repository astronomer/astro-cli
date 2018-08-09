package config

import (
	"errors"
	"fmt"
	"strings"
)

// Contexts holds all available Context structs in a map
type Contexts struct {
	Contexts map[string]Context `mapstructure:"contexts"`
}

// Context represents a single cluster context
type Context struct {
	Domain    string `mapstructure:"domain"`
	Workspace string `mapstructure:"workspace"`
	Token     string `mapstructure:"token"`
}

// GetContextKey allows a cluster domain to be used without interfering
// with viper's dot (.) notation for fetching configs by replacing with underscores (_)
func (c Context) GetContextKey() (string, error) {
	if len(c.Domain) == 0 {
		return "", errors.New("Cluster config invalid, no domain specified")
	}

	return strings.Replace(c.Domain, ".", "_", -1), nil
}

// ContextExists checks if a cluster struct exists in config
// based on Cluster.Domain
// Returns a boolean indicating whether or not cluster exists
func (c Context) ContextExists() bool {
	key, err := c.GetContextKey()
	if err != nil {
		return false
	}

	return viperHome.IsSet("clusters" + "." + key)
}

// GetContext gets the full context from the specified Context receiver struct
// Returns based on Domain prop
func (c Context) GetContext() (Context, error) {
	key, err := c.GetContextKey()
	if err != nil {
		return c, err
	}

	if !c.ContextExists() {
		return c, errors.New("Cluster not set, have you authenticated to this cluster?")
	}

	err = viperHome.UnmarshalKey("clusters"+"."+key, &c)
	if err != nil {
		return c, err
	}
	return c, nil
}

// GetContexts gets all contexts currently configured in the global config
// Returns a Contexts struct containing a map of all Context structs
func (c Contexts) GetContexts() (Contexts, error) {
	err := viperHome.Unmarshal(&c)
	if err != nil {
		return c, err
	}

	return c, nil
}

// SetContext saves Context to the config
func (c Context) SetContext() error {
	key, err := c.GetContextKey()
	if err != nil {
		return err
	}

	context := map[string]string{
		"token":     c.Token,
		"domain":    c.Domain,
		"workspace": c.Workspace,
	}

	viperHome.Set("clusters"+"."+key, context)
	saveConfig(viperHome, HomeConfigFile)

	return nil
}

// SetContextKey saves a single context key value pair
func (c Context) SetContextKey(key, value string) error {
	cKey, err := c.GetContextKey()
	if err != nil {
		return err
	}

	cfgPath := fmt.Sprintf("clusters.%s.%s", cKey, key)
	viperHome.Set(cfgPath, value)
	saveConfig(viperHome, HomeConfigFile)

	return nil
}

// SwitchContext sets the current config context to the one matching the provided Context struct
func (c Context) SwitchContext() error {
	if c.ContextExists() {
		viperHome.Set("context", c.Domain)
		saveConfig(viperHome, HomeConfigFile)
	}

	return nil
}

// GetAPIURL returns full Houston API Url for the provided Context
func (c Context) GetAPIURL() string {
	return fmt.Sprintf(
		"%s://houston.%s:%s/v1",
		CFG.CloudAPIProtocol.GetString(),
		c.Domain,
		CFG.CloudAPIPort.GetString(),
	)
}
