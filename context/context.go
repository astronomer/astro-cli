package context

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/domainutil"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/printutil"
	"github.com/spf13/cobra"
)

var (
	// CloudDomainRegex is used to differentiate cloud domain from software domain
	CloudDomainRegex     = regexp.MustCompile(`(?:https:\/\/|^)(?:(pr\d{4,6})\.|)(?:cloud\.|)astronomer(?:-(dev|stage|perf))?\.io(?:\/|)$`)
	contextDeleteWarnMsg = "Are you sure you want to delete currently used context: %s"
	cancelCtxDeleteMsg   = "Canceling context delete..."
	failCtxDeleteMsg     = "Error deleting context %s: "
	successCtxDeleteMsg  = "Successfully deleted context: %s"
)

var tab = printutil.Table{
	Padding:      []int{44},
	Header:       []string{"NAME"},
	ColorRowCode: [2]string{"\033[1;32m", "\033[0m"},
}

// newTableOut construct new printutil.Table
func newTableOut() *printutil.Table {
	return &printutil.Table{
		Padding: []int{36, 36},
		Header:  []string{"CONTEXT DOMAIN", "WORKSPACE"},
	}
}

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

func Delete(domain string, noPrompt bool) error {
	currentCtx, _ := GetCurrentContext()
	if currentCtx.Domain != "" && currentCtx.Domain == domain && !noPrompt {
		i, _ := input.Confirm(fmt.Sprintf(contextDeleteWarnMsg, domain))
		if !i {
			fmt.Println(cancelCtxDeleteMsg)
			return nil
		}
	}

	c := config.Context{Domain: domain}
	err := c.DeleteContext()
	if err != nil {
		fmt.Printf(failCtxDeleteMsg, domain)
		return err
	}

	if currentCtx.Domain == domain {
		if err := config.ResetCurrentContext(); err != nil {
			return err
		}
	}

	fmt.Println(fmt.Sprintf(successCtxDeleteMsg, domain))
	return nil
}

func SwitchContext(cmd *cobra.Command, args []string) error {
	cmd.SilenceUsage = true

	domain := ""
	if len(args) == 1 {
		domain = args[0]
	}

	err := Switch(domain)
	if err != nil {
		return err
	}

	c := config.Context{Domain: domain}
	ctx, err := c.GetContext()
	if err != nil {
		return err
	}

	tab := newTableOut()
	tab.AddRow([]string{ctx.Domain, ctx.Workspace}, false)
	tab.SuccessMsg = "\n Switched context"
	tab.Print(os.Stdout)

	return nil
}

func ListContext(cmd *cobra.Command, args []string, out io.Writer) error {
	cmd.SilenceUsage = true

	var domain string
	contexts, err := config.GetContexts()
	if err != nil {
		return err
	}

	currentCtx, err := config.GetCurrentContext()
	if err != nil {
		return err
	}
	//nolint:gocritic
	for ctxKey, ctx := range contexts.Contexts {
		if ctx.Domain != "" {
			domain = ctx.Domain
		} else {
			domain = strings.Replace(ctxKey, "_", ".", -1)
		}

		if domain == currentCtx.Domain {
			tab.AddRow([]string{domain}, true)
		} else {
			tab.AddRow([]string{domain}, false)
		}
	}

	tab.Print(out)
	return nil
}

func DeleteContext(cmd *cobra.Command, args []string, noPrompt bool) error {
	cmd.SilenceUsage = true
	domain := args[0]
	return Delete(domain, noPrompt)
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
