package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/astronomer/astro-cli/pkg/printutil"
)

var (
	ErrCtxConfigErr = errors.New("context config invalid, no domain specified")

	errGetHomeString = errors.New("no context set, have you authenticated to Astro or Astronomer Software? Run astro login and try again")
	errNotConnected  = errors.New("not connected, have you authenticated to Astro? Run astro login and try again")
)

var splitNum = 2

const (
	contextsKey = "contexts"
)

// newTableOut construct new printutil.Table
func newTableOut() *printutil.Table {
	return &printutil.Table{
		Padding: []int{36, 36},
		Header:  []string{"CONTEXT DOMAIN", "WORKSPACE"},
	}
}

// Contexts holds all available Context structs in a map
type Contexts struct {
	Contexts map[string]Context `mapstructure:"contexts"`
}

// Context represents a single context
type Context struct {
	Domain            string `mapstructure:"domain"`
	Workspace         string `mapstructure:"workspace"`
	LastUsedWorkspace string `mapstructure:"last_used_workspace"`
	Token             string `mapstructure:"token"`
	RefreshToken      string `mapstructure:"refreshtoken"`
	UserEmail         string `mapstructure:"user_email"`
}

// GetCurrentContext looks up current context and gets corresponding Context struct
func GetCurrentContext() (Context, error) {
	c := Context{}

	domain := CFG.Context.GetHomeString()
	if domain == "" {
		return Context{}, errGetHomeString
	}

	c.Domain = domain

	return c.GetContext()
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

	return viperHome.IsSet("contexts" + "." + key)
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
	err = viperHome.UnmarshalKey("contexts"+"."+key, &c)
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

	context := map[string]interface{}{
		"token":               c.Token,
		"domain":              c.Domain,
		"workspace":           c.Workspace,
		"last_used_workspace": c.Workspace,
		"refreshtoken":        c.RefreshToken,
		"user_email":          c.UserEmail,
	}

	viperHome.Set("contexts"+"."+key, context)
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

	cfgPath := fmt.Sprintf("contexts.%s.%s", cKey, key)
	viperHome.Set(cfgPath, value)
	err = saveConfig(viperHome, HomeConfigFile)
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

	tab := newTableOut()
	tab.AddRow([]string{ctx.Domain, ctx.Workspace}, false)
	tab.SuccessMsg = "\n Switched context"
	tab.Print(os.Stdout)

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

func (c *Context) SetSystemAdmin(value bool) error {
	cKey, err := c.GetContextKey()
	if err != nil {
		return err
	}

	cfgPath := fmt.Sprintf("contexts.%s.%s", cKey, "isSystemAdmin")
	viperHome.Set(cfgPath, value)
	err = saveConfig(viperHome, HomeConfigFile)
	if err != nil {
		return err
	}

	return nil
}

func (c *Context) GetSystemAdmin() (bool, error) {
	cKey, err := c.GetContextKey()
	if err != nil {
		return false, err
	}

	cfgPath := fmt.Sprintf("contexts.%s.%s", cKey, "isSystemAdmin")
	return viperHome.GetBool(cfgPath), nil
}

func (c *Context) SetExpiresIn(value int64) error {
	cKey, err := c.GetContextKey()
	if err != nil {
		return err
	}

	expiretime := time.Now().Add(time.Duration(value) * time.Second)

	cfgPath := fmt.Sprintf("contexts.%s.%s", cKey, "ExpiresIn")
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

	cfgPath := fmt.Sprintf("contexts.%s.%s", cKey, "ExpiresIn")
	return viperHome.GetTime(cfgPath), nil
}
