package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

var (
	ErrCtxConfigErr = errors.New("context config invalid, no domain specified")

	ErrGetHomeString = errors.New("no context set, have you authenticated to Astro or Astronomer Software? Run astro login and try again")
	errNotConnected  = errors.New("not connected, have you authenticated to Astro? Run astro login and try again")
)

const (
	contextsKey = "contexts"
)

// Contexts holds all available Context structs in a map
type Contexts struct {
	Contexts map[string]Context `mapstructure:"contexts"`
}

// Context represents a single context
type Context struct {
	Domain              string `mapstructure:"domain"`
	Organization        string `mapstructure:"organization"`
	OrganizationProduct string `mapstructure:"organization_product"`
	Workspace           string `mapstructure:"workspace"`
	LastUsedWorkspace   string `mapstructure:"last_used_workspace"`
	Token               string `mapstructure:"token"`
	RefreshToken        string `mapstructure:"refreshtoken"`
	UserEmail           string `mapstructure:"user_email"`
}

// GetCurrentContext looks up current context and gets corresponding Context struct
func GetCurrentContext() (Context, error) {
	c := Context{}
	var err error
	c.Domain, err = GetCurrentDomain()
	if err != nil {
		return Context{}, err
	}
	return c.GetContext()
}

// Get CurrentDonain returns the currently configured astro domain, or an error if one is not set
func GetCurrentDomain() (string, error) {
	var domain string
	if domain = os.Getenv("ASTRO_DOMAIN"); domain == "" {
		if domain = CFG.Context.GetHomeString(); domain == "" {
			return "", ErrGetHomeString
		}
	}
	return domain, nil
}

// ResetCurrentContext reset the current context and is used when someone logs out
func ResetCurrentContext() error {
	return CFG.Context.SetHomeString("")
}

// GetContextKey allows a context domain to be used without interfering
// with viper's dot (.) notation for fetching configs by replacing with underscores (_)
func (c *Context) GetContextKey() (string, error) {
	if c.Domain == "" {
		return "", ErrCtxConfigErr
	}

	return strings.Replace(c.Domain, ".", "_", -1), nil
}

// ContextExists checks if a context struct exists in config
// based on Context.Domain
// Returns a boolean indicating whether or not context exists
func (c *Context) ContextExists() bool {
	key, err := c.GetContextKey()
	if err != nil {
		return false
	}

	return viperHome.IsSet(contextsKey + "." + key)
}

// GetContext gets the full context from the specified Context receiver struct
// Returns based on Domain prop
func (c *Context) GetContext() (Context, error) {
	key, err := c.GetContextKey()
	if err != nil {
		return *c, err
	}

	if !c.ContextExists() {
		return *c, errNotConnected
	}
	err = viperHome.UnmarshalKey(contextsKey+"."+key, &c)
	if err != nil {
		return *c, err
	}
	return *c, nil
}

// GetContexts gets all contexts currently configured in the global config
// Returns a Contexts struct containing a map of all Context structs
func GetContexts() (Contexts, error) {
	var c Contexts
	err := viperHome.Unmarshal(&c)
	if err != nil {
		return c, err
	}

	return c, nil
}

// SetContext saves Context to the config
func (c *Context) SetContext() error {
	key, err := c.GetContextKey()
	if err != nil {
		return err
	}

	context := map[string]string{
		"token":                c.Token,
		"domain":               c.Domain,
		"organization":         c.Organization,
		"organization_product": c.OrganizationProduct,
		"workspace":            c.Workspace,
		"last_used_workspace":  c.Workspace,
		"refreshtoken":         c.RefreshToken,
		"user_email":           c.UserEmail,
	}

	viperHome.Set(contextsKey+"."+key, context)
	err = saveConfig(viperHome, HomeConfigFile)
	if err != nil {
		return err
	}

	return nil
}

// SetContextKey saves a single context key value pair
func (c *Context) SetContextKey(key, value string) error {
	cKey, err := c.GetContextKey()
	if err != nil {
		return err
	}

	cfgPath := fmt.Sprintf("%s.%s.%s", contextsKey, cKey, key)
	viperHome.Set(cfgPath, value)
	err = saveConfig(viperHome, HomeConfigFile)
	if err != nil {
		return err
	}

	return nil
}

// set organization id and short name in context config
func (c *Context) SetOrganizationContext(orgID, orgProduct string) error {
	err := c.SetContextKey("organization", orgID) // c.Organization
	if err != nil {
		return err
	}

	err = c.SetContextKey("organization_product", orgProduct)
	if err != nil {
		return err
	}
	return nil
}

// SwitchContext sets the current config context to the one matching the provided Context struct
func (c *Context) SwitchContext() error {
	var err error
	ctx, err := c.GetContext()
	if err != nil {
		return err
	}

	viperHome.Set("context", ctx.Domain)
	err = saveConfig(viperHome, HomeConfigFile)
	if err != nil {
		return err
	}

	return nil
}

func (c *Context) DeleteContext() error {
	cKey, err := c.GetContextKey()
	if err != nil {
		return err
	}
	// Since viper does not have a way to unset or delete a key,
	// hence getting all contexts and delete the required context
	contexts := viperHome.Get(contextsKey).(map[string]interface{})
	delete(contexts, cKey)
	viperHome.Set(contextsKey, contexts)
	err = saveConfig(viperHome, HomeConfigFile)
	if err != nil {
		return err
	}
	return nil
}

func (c *Context) SetExpiresIn(value int64) error {
	cKey, err := c.GetContextKey()
	if err != nil {
		return err
	}

	expiretime := time.Now().Add(time.Duration(value) * time.Second)

	cfgPath := fmt.Sprintf("%s.%s.%s", contextsKey, cKey, "ExpiresIn")
	viperHome.Set(cfgPath, expiretime)
	err = saveConfig(viperHome, HomeConfigFile)
	if err != nil {
		return err
	}

	return nil
}

func (c *Context) GetExpiresIn() (time.Time, error) {
	cKey, err := c.GetContextKey()
	if err != nil {
		return time.Time{}, err
	}

	cfgPath := fmt.Sprintf("%s.%s.%s", contextsKey, cKey, "ExpiresIn")
	return viperHome.GetTime(cfgPath), nil
}
