package context

import (
	"errors"
	"fmt"
	"io"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/domainutil"
	"github.com/astronomer/astro-cli/pkg/input"
)

var (
	// CloudDomainRegex is used to differentiate cloud domain from software domain
	CloudDomainRegex     = regexp.MustCompile(`(?:https:\/\/|^)(?:(pr\d{4,6})\.|)(?:cloud\.|)astronomer(?:-(dev|stage|perf))?\.io(?:\/|)$`)
	contextDeleteWarnMsg = "Are you sure you want to delete currently used context: %s"
	cancelCtxDeleteMsg   = "Canceling context delete..."
)

// ContextExists checks to see if context exist in config
func Exists(domain string) bool {
	c := config.Context{Domain: domain}

	return c.ContextExists()
}

// GetCurrentContext gets the current contxt set in the config
// Is a convenience wrapp around config.GetCurrentContext()
// Returns full Context struct
func GetCurrentContext() (config.Context, error) {
	return config.GetCurrentContext()
}

// GetContext gets the specified context by domain name
// Returns the matching Context struct
func GetContext(domain string) (config.Context, error) {
	c := config.Context{Domain: domain}
	return c.GetContext()
}

// SetContext creates or updates a contexts domain name
// Returns an error
func SetContext(domain string) error {
	c := config.Context{Domain: domain}
	return c.SetContext()
}

// Switch switches to context of domain
func Switch(domain string) error {
	// Create context if it does not exist
	if !Exists(domain) {
		// Save new context since it did not exists
		err := SetContext(domain)
		if err != nil {
			return err
		}
	}
	c := config.Context{Domain: domain}
	return c.SwitchContext()
}

// Info is one saved context, as `astro context list` and `astro context
// switch` publish it. The login itself is never part of it.
type Info struct {
	Domain         string `json:"domain"`
	UserEmail      string `json:"user_email"`
	OrganizationID string `json:"organization_id"`
	WorkspaceID    string `json:"workspace_id"`
	// IsCurrent is whether commands run here use this context now, which
	// ASTRO_DOMAIN decides when it is set.
	IsCurrent bool `json:"is_current"`
}

// InfoList is `astro context list`: every saved context, sorted by domain.
type InfoList struct {
	Contexts []Info `json:"contexts"`
}

// Removal is what `astro context delete` did.
type Removal struct {
	Domain string `json:"domain"`
	Action string `json:"action"`
}

// List returns the contexts saved on this machine, in order. Like the
// table it always printed, it fails when no context is current.
func List() (InfoList, error) {
	// No current context is read: reading one reads its login, which can
	// mean waiting on the keyring. saved marks the current domain's.
	list, _, err := saved()
	if err != nil {
		return InfoList{}, err
	}
	if !slices.ContainsFunc(list.Contexts, func(i Info) bool { return i.IsCurrent }) {
		// None is current: no current domain at all, or one with no saved
		// context.
		if _, err := config.GetCurrentDomain(); err != nil {
			return InfoList{}, err
		}
		return InfoList{}, config.ErrNotConnected
	}
	return list, nil
}

// saved reads the contexts without their logins: nothing here needs one, and
// reading one can mean waiting on the keyring. Alongside them, the key the
// config files each under, which is how a domain finds its own.
func saved() (InfoList, []string, error) {
	contexts, err := config.ListContexts()
	if err != nil {
		return InfoList{}, nil, err
	}
	current, _ := config.GetCurrentDomain() //nolint:errcheck // with no current context none is marked
	keys := slices.Sorted(maps.Keys(contexts.Contexts))
	list := InfoList{Contexts: make([]Info, 0, len(keys))}
	for _, key := range keys {
		ctx := contexts.Contexts[key]
		domain := ctx.Domain
		if domain == "" {
			domain = strings.ReplaceAll(key, "_", ".")
		}
		list.Contexts = append(list.Contexts, Info{
			Domain:         domain,
			UserEmail:      ctx.UserEmail,
			OrganizationID: ctx.Organization,
			WorkspaceID:    ctx.Workspace,
			IsCurrent:      current != "" && key == contextKey(current),
		})
	}
	return list, keys, nil
}

// contextKey is the key the config files domain's context under. Viper
// lowercases every key it reads.
func contextKey(domain string) string {
	return strings.ToLower(strings.ReplaceAll(domain, ".", "_"))
}

// Saved returns the context saved as current, the one a switch just wrote,
// whether or not ASTRO_DOMAIN outranks it in this shell.
func Saved() (Info, error) {
	domain := config.CFG.Context.GetHomeString()
	list, keys, err := saved()
	if err != nil {
		return Info{}, err
	}
	if i := slices.Index(keys, contextKey(domain)); domain != "" && i >= 0 {
		return list.Contexts[i], nil
	}
	return Info{}, fmt.Errorf("%w: %s", config.ErrContextNotExist, domain)
}

// SwitchTo makes the APC context for domain current, creating it if needed,
// and returns it as it now is.
func SwitchTo(domain string) (Info, error) {
	if err := Switch(domain); err != nil {
		return Info{}, err
	}
	return Saved()
}

// Delete removes the saved context for domain. Deleting the current one asks
// first, unless noPrompt; a declined delete returns no removal. What it says
// along the way goes to out.
func Delete(domain string, noPrompt bool, out io.Writer) (*Removal, error) {
	currentCtx, _ := GetCurrentContext() //nolint:errcheck // error deliberately ignored in this shell code
	if currentCtx.Domain != "" && currentCtx.Domain == domain && !noPrompt {
		i, err := input.Confirm(fmt.Sprintf(contextDeleteWarnMsg, domain), input.AnsweredBy("--yes"))
		if err != nil {
			return nil, err
		}
		if !i {
			fmt.Fprintln(out, cancelCtxDeleteMsg)
			return nil, nil
		}
	}

	c := config.Context{Domain: domain}
	err := c.DeleteContext()
	if errors.Is(err, config.ErrContextNotExist) {
		// The error names the context already; the prefix below would print
		// it twice.
		return nil, err
	}
	if err != nil {
		return nil, fmt.Errorf("deleting context %s: %w", domain, err)
	}

	if currentCtx.Domain == domain {
		if err := config.ResetCurrentContext(); err != nil {
			return nil, err
		}
	}
	return &Removal{Domain: domain, Action: "deleted"}, nil
}

// IsCloudContext returns whether current context domain is related to cloud platform or not
func IsCloudContext() bool {
	currContext, err := GetCurrentContext()
	if err != nil { // Case when context is not set or something wrong when trying to pick up current context
		// TODO: Handle this error and possible add this to a debug log
		return true
	}

	return IsCloudDomain(currContext.Domain)
}

// IsCloudDomain returns whether the given domain is related to cloud platform or not
func IsCloudDomain(domain string) bool {
	if CloudDomainRegex.MatchString(domain) {
		return true
	}
	if domainutil.PRPreviewDomainRegex.MatchString(domain) {
		return true
	}

	// Case when user is connected to localhost && local.platform is set to cloud in astro config
	if strings.Contains(domain, "localhost") && config.CFG.LocalPlatform.GetString() == config.CloudPlatform {
		return true
	}

	return false
}
