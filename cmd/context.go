package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/context"
	astroAuth "github.com/astronomer/astro-cli/internal/platform/astro/auth"
	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	"github.com/astronomer/astro-cli/pkg/domainutil"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/picker"
)

var (
	noPrompt bool

	cloudSwitch = astroAuth.Switch

	// contextPickerMayPrompt reports whether a bare `astro context switch` can
	// ask which context to use: someone at a terminal to answer, and a stdout
	// they will see the picker on, so a redirected run fails rather than blocks.
	contextPickerMayPrompt = func() bool {
		return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
	}
)

func newContextCmd(astroV1Client astrov1.APIClient, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "context",
		Aliases: []string{"c"},
		Short:   "Manage Astro & APC contexts",
		Long:    "Context represent a connection to Astro or APC in the form of a Domain URL. If your context is set to astronomer.io, for example, you are connected to Astro",
	}
	cmd.AddCommand(
		newContextListCmd(out),
		newContextSwitchCmd(astroV1Client, out),
		newContextDeleteCmd(),
	)
	return cmd
}

func newContextListCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all contexts",
		Long:    "List all Astro and APC contexts or domains that you've authenticated to on this machine",
		RunE: func(cmd *cobra.Command, args []string) error {
			return context.ListContext(cmd, args, out)
		},
	}
	return cmd
}

func newContextSwitchCmd(astroV1Client astrov1.APIClient, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "switch [domain]",
		Aliases: []string{"sw"},
		Short:   "Switch to a different context",
		Long:    "Switch to a different context. With no domain, pick one from the contexts saved on this machine. For Astro, the saved login for the domain is refreshed if it can be; the command never opens a browser.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return switchContext(cmd, args, astroV1Client, out)
		},
		Args: cobra.MaximumNArgs(1),
	}
	return cmd
}

func switchContext(cmd *cobra.Command, args []string, astroV1Client astrov1.APIClient, out io.Writer) error {
	if len(args) == 0 {
		cmd.SilenceUsage = true
		domain, err := pickContext(cmd.InOrStdin(), out)
		if err != nil {
			return err
		}
		args = []string{domain}
	}
	domain := domainutil.ExpandShortName(args[0])
	var err error
	if context.IsCloudDomain(domain) {
		cmd.SilenceUsage = true
		err = cloudSwitch(domain, astroV1Client, out)
	} else {
		err = context.SwitchContext(cmd, args)
	}
	if err == nil {
		noteDomainOverride(cmd.ErrOrStderr(), domain)
	}
	return err
}

// errInvalidContextSelection is the context picker's answer to a choice that is
// not a row number, worded as the Deployment and link pickers word theirs.
var errInvalidContextSelection = errors.New("invalid context selected")

// pickContext asks which saved context to switch to, with the table picker the
// Deployment and link pickers use. The context commands run on now is drawn
// bold green, so the reader sees what they are choosing away from. When
// ASTRO_DOMAIN is what makes it current, a last column on that row says so: a
// variable in this shell outranks the saved context, and it is what makes the
// CLI look as though it flips between hosts.
func pickContext(in io.Reader, out io.Writer) (string, error) {
	if !contextPickerMayPrompt() {
		return "", errors.New("name the context to switch to: `astro context switch <domain>`")
	}
	contexts, err := config.ListContexts()
	if err != nil {
		return "", err
	}
	domains := make([]string, 0, len(contexts.Contexts))
	emails := make(map[string]string, len(contexts.Contexts))
	for key := range contexts.Contexts {
		domain := contexts.Contexts[key].Domain
		if domain == "" {
			domain = strings.ReplaceAll(key, "_", ".")
		}
		domains = append(domains, domain)
		emails[domain] = contexts.Contexts[key].UserEmail
	}
	if len(domains) == 0 {
		return "", errors.New("no contexts are saved on this machine. Run `astro login <domain>` to add one")
	}
	slices.Sort(domains)

	current, _ := config.GetCurrentDomain() //nolint:errcheck // with no current context no row is highlighted
	list := picker.List{
		Title:   "Switch to which context?",
		Header:  []string{"DOMAIN", "USER"},
		Ask:     []input.Option{input.About("a context"), input.AnsweredBy("the domain as an argument")},
		Invalid: errInvalidContextSelection,
	}
	// The mark column exists only when a row carries one; otherwise every row
	// would end in blank padding.
	marked := os.Getenv("ASTRO_DOMAIN") != "" && slices.Contains(domains, current)
	if marked {
		list.Header = append(list.Header, "")
	}
	for _, domain := range domains {
		cells := []string{domain, emails[domain]}
		if marked {
			mark := ""
			if domain == current {
				mark = "ASTRO_DOMAIN"
			}
			cells = append(cells, mark)
		}
		list.AddRow(domain == current, cells...)
	}
	i, err := list.Pick(out, in)
	if err != nil {
		return "", err
	}
	return domains[i], nil
}

// noteDomainOverride says when ASTRO_DOMAIN outranks the context just switched
// to, since every command in this shell keeps going to that host regardless.
// Both are normalized first: the switch stores "cloud.astronomer.io/" as
// "astronomer.io", and a variable spelled either way names the same host.
func noteDomainOverride(errOut io.Writer, domain string) {
	if env := os.Getenv("ASTRO_DOMAIN"); env != "" && manifest.NormalizeDomain(env) != manifest.NormalizeDomain(domain) {
		fmt.Fprintf(errOut, "ASTRO_DOMAIN=%s is set in this shell and outranks the saved context; commands here keep using %s until it is unset\n", env, env)
	}
}

func newContextDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete [domain]",
		Aliases: []string{"de"},
		Short:   "Delete a context",
		Long:    "Delete a locally stored context to Astro or APC",
		RunE: func(cmd *cobra.Command, args []string) error {
			return context.DeleteContext(cmd, []string{domainutil.ExpandShortName(args[0])}, noPrompt)
		},
		Args: cobra.ExactArgs(1),
	}

	cmd.Flags().BoolVarP(&noPrompt, "yes", "y", false, "Don't ask for confirmation before deleting the current context")
	return cmd
}
