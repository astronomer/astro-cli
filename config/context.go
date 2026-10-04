package config

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/astronomer/astro-cli/pkg/secrets"
)

var (
	ErrCtxConfigErr = errors.New("context config invalid, no domain specified")

	ErrGetHomeString = errors.New("no context set, have you authenticated to Astro or APC? Run astro login and try again")
	errNotConnected  = errors.New("not connected, have you authenticated to Astro? Run astro login and try again")
	errNotLoginField = errors.New("only a login field is shared across a tenant")
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
	AuthDomain          string `mapstructure:"auth_domain"`
	AuthClientID        string `mapstructure:"auth_client_id"`
}

var loginFields = []string{"token", "refreshtoken", "expiresin", "user_email"}

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
	resolveLogin(key, c)
	return *c, nil
}

// ListContexts returns every context in the global config, without their
// logins: Token and RefreshToken are empty. Reading a login can reach the OS
// keyring; GetContext returns one context with its login.
func ListContexts() (Contexts, error) {
	var c Contexts
	err := viperHome.Unmarshal(&c)
	if err != nil {
		return c, err
	}
	for key := range c.Contexts {
		ctx := c.Contexts[key]
		ctx.Token, ctx.RefreshToken = "", ""
		c.Contexts[key] = ctx
	}
	return c, nil
}

// SetContext saves Context to the config
func (c *Context) SetContext() error {
	key, err := c.GetContextKey()
	if err != nil {
		return err
	}

	login := saveContextLogin(key, secrets.Login{Token: c.Token, RefreshToken: c.RefreshToken})
	context := map[string]interface{}{
		"token":                login.Token,
		"domain":               c.Domain,
		"organization":         c.Organization,
		"organization_product": c.OrganizationProduct,
		"workspace":            c.Workspace,
		"last_used_workspace":  c.Workspace,
		"refreshtoken":         login.RefreshToken,
		"user_email":           c.UserEmail,
		"auth_domain":          c.AuthDomain,
		"auth_client_id":       c.AuthClientID,
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
	return setContextField(cKey, key, value)
}

// SetSharedContextKey saves a login field to this context and to every other
// context on its identity provider tenant, in the same write, so a login to one
// PR preview is a login to all of them. Only a login that any host on the tenant
// can use belongs here; a token for this host alone goes through SetContextKey.
func (c *Context) SetSharedContextKey(key, value string) error {
	cKey, err := c.GetContextKey()
	if err != nil {
		return err
	}
	return shareContextField(cKey, key, value)
}

// SetSharedExpiresIn is SetExpiresIn for a login shared across the tenant, as
// SetSharedContextKey is for SetContextKey.
func (c *Context) SetSharedExpiresIn(value int64) error {
	cKey, err := c.GetContextKey()
	if err != nil {
		return err
	}
	return shareContextField(cKey, "expiresin", time.Now().Add(time.Duration(value)*time.Second))
}

// setContextField updates one field in one context and persists the config.
func setContextField(cKey, field string, value interface{}) error {
	field = strings.ToLower(field)
	if s, ok := value.(string); ok && isLoginField(field) {
		putLoginField(cKey, field, s)
	} else {
		putContextField(cKey, field, value)
	}
	return saveConfig(viperHome, HomeConfigFile)
}

func shareContextField(cKey, field string, value interface{}) error {
	field = strings.ToLower(field)
	if !slices.Contains(loginFields, field) {
		return fmt.Errorf("%w: %s is not a login field", errNotLoginField, field)
	}
	for _, key := range append([]string{cKey}, contextKeysSharingLogin(cKey)...) {
		if s, ok := value.(string); ok && isLoginField(field) {
			putLoginField(key, field, s)
			continue
		}
		putContextField(key, field, value)
	}
	return saveConfig(viperHome, HomeConfigFile)
}

// putContextField updates one field in a context's map, in memory only. It
// intentionally reads the full context map, mutates the single field, and Sets
// the map back — rather than Set-ing the nested path directly — because viper's
// override layer doesn't merge with the file layer on nested Set. Subsequent
// UnmarshalKey calls on the parent key would otherwise return a partial struct
// with every unset field zeroed out.
// See https://github.com/spf13/viper/issues/1106.
//
// field must be lowercase: GetStringMap returns lowercase keys, and a second
// key that differs only in case collides with the first when viper folds them.
//
// Setting a token field marks the context's login as changed by this process,
// so the next write keeps it (see takeUnchangedLogins).
func putContextField(cKey, field string, value interface{}) {
	if isLoginField(field) {
		markLoginWrite(cKey)
	}
	storeContextField(cKey, field, value)
}

// storeContextField is putContextField without marking a login as changed.
func storeContextField(cKey, field string, value interface{}) {
	parentPath := fmt.Sprintf("%s.%s", contextsKey, cKey)
	ctxMap := viperHome.GetStringMap(parentPath)
	if ctxMap == nil {
		ctxMap = map[string]interface{}{}
	}
	ctxMap[field] = value
	viperHome.Set(parentPath, ctxMap)
}

// contextKeysSharingLogin returns the keys of the other contexts on cKey's
// identity provider tenant. A context that has not recorded its tenant shares
// with none.
func contextKeysSharingLogin(cKey string) []string {
	authDomain, authClientID := contextTenant(cKey)
	if authDomain == "" || authClientID == "" {
		return nil
	}
	var keys []string
	for key := range viperHome.GetStringMap(contextsKey) {
		if strings.EqualFold(key, cKey) {
			continue
		}
		if d, id := contextTenant(key); d == authDomain && id == authClientID {
			keys = append(keys, key)
		}
	}
	return keys
}

func contextTenant(cKey string) (authDomain, authClientID string) {
	ctxMap := viperHome.GetStringMap(contextsKey + "." + cKey)
	authDomain, _ = ctxMap["auth_domain"].(string)
	authClientID, _ = ctxMap["auth_client_id"].(string)
	return authDomain, authClientID
}

// SetAuthTenant records the identity provider tenant that issues this
// context's tokens: the domain and client ID from its host's auth config.
func (c *Context) SetAuthTenant(authDomain, authClientID string) error {
	cKey, err := c.GetContextKey()
	if err != nil {
		return err
	}
	if contextDomain, contextClientID := contextTenant(cKey); contextDomain == authDomain && contextClientID == authClientID {
		return nil
	}
	putContextField(cKey, "auth_domain", authDomain)
	putContextField(cKey, "auth_client_id", authClientID)
	return saveConfig(viperHome, HomeConfigFile)
}

// ContextsSharingLogin returns the contexts whose tokens come from the given
// identity provider tenant.
func ContextsSharingLogin(authDomain, authClientID string) ([]Context, error) {
	if authDomain == "" || authClientID == "" {
		return nil, nil
	}
	contexts, err := ListContexts()
	if err != nil {
		return nil, err
	}
	var sharing []Context
	for key := range contexts.Contexts {
		c := contexts.Contexts[key]
		if c.AuthDomain != authDomain || c.AuthClientID != authClientID {
			continue
		}
		// Configs written before SetContext stored a map[string]interface{}
		// often lack the domain field. The key still names it.
		if c.Domain == "" {
			c.Domain = strings.ReplaceAll(key, "_", ".")
		}
		ctxMap := viperHome.GetStringMap(contextsKey + "." + key)
		c.Token, c.RefreshToken = stringField(ctxMap, tokenField), stringField(ctxMap, refreshTokenField)
		resolveLogin(key, &c)
		sharing = append(sharing, c)
	}
	return sharing, nil
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
	logins().Save(HomeConfigFile, cKey, secrets.Login{})
	return nil
}

func (c *Context) SetExpiresIn(value int64) error {
	cKey, err := c.GetContextKey()
	if err != nil {
		return err
	}
	expiretime := time.Now().Add(time.Duration(value) * time.Second)
	return setContextField(cKey, "ExpiresIn", expiretime)
}

func (c *Context) GetExpiresIn() (time.Time, error) {
	cKey, err := c.GetContextKey()
	if err != nil {
		return time.Time{}, err
	}

	cfgPath := fmt.Sprintf("%s.%s.%s", contextsKey, cKey, "ExpiresIn")
	return viperHome.GetTime(cfgPath), nil
}
