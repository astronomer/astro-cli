package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	astroCmd "github.com/astronomer/astro-cli/cmd/astro"
	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/context"
	astroAuth "github.com/astronomer/astro-cli/internal/platform/astro/auth"
	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	"github.com/astronomer/astro-cli/pkg/domainutil"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/picker"
	"github.com/astronomer/astro-cli/pkg/printutil"
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
		// Saved contexts are this machine's own; switch never opens a browser.
		Annotations: map[string]string{astroCmd.NoLoginAnnotation: "true"},
		Long:        "Context represent a connection to Astro or APC in the form of a Domain URL. If your context is set to astronomer.io, for example, you are connected to Astro",
	}
	cmd.AddCommand(
		newContextListCmd(out),
		newContextSwitchCmd(astroV1Client, out),
		newContextDeleteCmd(),
	)
	return cmd
}

func newContextListCmd(out io.Writer) *cobra.Command {
	var output cliout.Format
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all contexts",
		Long:    "List all Astro and APC contexts or domains that you've authenticated to on this machine",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			list, err := context.List()
			if err != nil {
				return err
			}
			return emitContextList(cliout.Renderer{Format: output, Out: out}, &list)
		},
		Example: `  # List the contexts saved on this machine
  astro context list`,
	}
	cliout.AddOutputFlag(cmd, &output)
	return cmd
}

// emitContextList publishes the saved contexts; in text, the table it always
// printed, the current context's row in green.
func emitContextList(r cliout.Renderer, list *context.InfoList) error {
	return r.Emit(list, func(w io.Writer) error {
		table := printutil.Table{
			Padding:      []int{44},
			Header:       []string{"NAME"},
			ColorRowCode: [2]string{"\033[1;32m", "\033[0m"},
		}
		for _, c := range list.Contexts {
			table.AddRow([]string{c.Domain}, c.IsCurrent)
		}
		return table.Print(w)
	})
}

func newContextSwitchCmd(astroV1Client astrov1.APIClient, out io.Writer) *cobra.Command {
	var output cliout.Format
	cmd := &cobra.Command{
		Use:     "switch [DOMAIN]",
		Aliases: []string{"sw"},
		Short:   "Switch to a different context",
		Long:    "Switch to a different context. With no domain, pick one from the contexts saved on this machine. For Astro, the saved login for the domain is refreshed if it can be; the command never opens a browser.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return switchContext(cmd, args, astroV1Client, cliout.Renderer{Format: output, Out: out})
		},
		Args: cobra.MaximumNArgs(1),
		Example: `  # Pick a context from the ones saved on this machine
  astro context switch

  # Switch to Astro
  astro context switch astronomer.io`,
	}
	cliout.AddOutputFlag(cmd, &output)
	return cmd
}

// switchContext makes a context current and publishes it as it now is. In
// text, an Astro switch says what its login check found, as it always has,
// and an APC switch prints the context table.
func switchContext(cmd *cobra.Command, args []string, astroV1Client astrov1.APIClient, r cliout.Renderer) error {
	cmd.SilenceUsage = true
	if len(args) == 0 {
		domain, err := pickContext(cmd.InOrStdin(), r.Out)
		if err != nil {
			return err
		}
		args = []string{domain}
	}
	domain := domainutil.ExpandShortName(args[0])
	var (
		switched context.Info
		err      error
	)
	text := func(io.Writer) error { return nil }
	if context.IsCloudDomain(domain) {
		if switchErr := cloudSwitch(domain, astroV1Client, cliout.NotesTo(cmd, r.Format, r.Out)); switchErr != nil {
			return switchErr
		}
		switched, err = context.Saved()
	} else {
		switched, err = context.SwitchTo(args[0])
		text = func(w io.Writer) error { return printSwitchedContext(w, &switched) }
	}
	if err != nil {
		return err
	}
	if err := r.Emit(&switched, text); err != nil {
		return err
	}
	noteDomainOverride(cmd.ErrOrStderr(), domain)
	return nil
}

// printSwitchedContext is the table an APC switch has always printed.
func printSwitchedContext(w io.Writer, c *context.Info) error {
	table := printutil.Table{
		Padding:    []int{36, 36},
		Header:     []string{"CONTEXT DOMAIN", "WORKSPACE"},
		SuccessMsg: "\n Switched context",
	}
	table.AddRow([]string{c.Domain, c.WorkspaceID}, false)
	return table.Print(w)
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
		return "", input.Required(errors.New("name the context to switch to: `astro context switch <domain>`"))
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
	var output cliout.Format
	cmd := &cobra.Command{
		Use:     "delete <DOMAIN>",
		Aliases: []string{"de"},
		Short:   "Delete a context",
		Long:    "Delete a locally stored context to Astro or APC",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			out := cmd.OutOrStdout()
			removal, err := context.Delete(domainutil.ExpandShortName(args[0]), noPrompt, cliout.NotesTo(cmd, output, out))
			if err != nil || removal == nil {
				return err
			}
			return cliout.Renderer{Format: output, Out: out}.Emit(removal, cliout.Text(func(b *bufio.Writer) {
				fmt.Fprintf(b, "Successfully deleted context: %s\n", removal.Domain)
			}))
		},
		Args: cobra.ExactArgs(1),
		Example: `  # Delete a saved context
  astro context delete <DOMAIN>

  # Delete the current context without the confirmation prompt
  astro context delete <DOMAIN> --yes`,
	}

	cmd.Flags().BoolVarP(&noPrompt, "yes", "y", false, "Don't ask for confirmation before deleting the current context")
	cliout.AddOutputFlag(cmd, &output)
	return cmd
}
