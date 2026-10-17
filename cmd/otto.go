package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/internal/otto"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	"github.com/astronomer/astro-cli/pkg/httputil"
)

// hasHelpFlag reports whether args ask for help with a word of their own
// before the "--" that ends flags: -h or --help, bare or set true
// (`--help=true`, `-h=true`), as pflag reads them.
func hasHelpFlag(args []string) bool {
	for _, a := range args {
		if a == "--" {
			return false
		}
		name, value, hasValue := strings.Cut(a, "=")
		if name != "--help" && name != "-h" {
			continue
		}
		if on, err := strconv.ParseBool(value); !hasValue || (err == nil && on) {
			return true
		}
	}
	return false
}

func printAstroSubcommands(w io.Writer) {
	// Match Otto's help format: 2-space indent, description column at col 24.
	fmt.Fprintln(w, "Subcommands:")
	fmt.Fprintln(w, "  astro otto update     Download and install the latest Otto binary")
	fmt.Fprintln(w, "  astro otto version    Print the installed Otto version")
	fmt.Fprintln(w)
}

func newOttoCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                "otto [ARGS]...",
		Short:              "Start the Otto AI agent",
		Long:               "Start the Otto AI agent for AI-assisted Airflow development and operations. All flags and arguments are forwarded directly to Otto.",
		SilenceUsage:       true,
		DisableFlagParsing: true,
		RunE:               ottoRun,
		Example: `  # Start Otto
  astro otto`,
	}

	return cmd
}

func ottoRun(cmd *cobra.Command, args []string) error {
	// With DisableFlagParsing, cobra won't route to subcommands,
	// so we dispatch "update" and "version" ourselves. "upgrade" is
	// aliased to "update" — otherwise it falls through to Otto and gets
	// interpreted as a prompt asking it to upgrade the Astro project.
	if len(args) > 0 {
		switch args[0] {
		case "update", "upgrade":
			return otto.Update()
		case "version":
			installed, err := otto.InstalledVersion()
			if err != nil {
				return err
			}
			if installed == "" {
				fmt.Println("Otto is not installed. Run astro otto to install.")
				return nil
			}
			fmt.Printf("Otto %s\n", installed)

			available, latest, err := otto.IsUpdateAvailable()
			if err == nil && available {
				fmt.Printf("Update available: %s (run astro otto update)\n", latest)
			}
			return nil
		}
	}

	// `astro otto --help` forwards to Otto's own help, which doesn't know
	// about the astro CLI's `update` / `version` subcommands. Prepend a short
	// banner so they're discoverable.
	if hasHelpFlag(args) {
		printAstroSubcommands(os.Stdout)
	}

	// Everything else (flags, prompt, etc.) is forwarded directly to Otto.
	return launchOtto(args)
}

// launchOtto runs Otto with args and waits for it. It propagates Otto's exit
// code when it exits non-zero: Otto has already printed its own error, so
// cobra's "Error: exit status N" would be noise. Same treatment for
// ErrNotLoggedIn and ErrNotOrgMember: Start has already printed the guidance
// to stderr, so returning the error to cobra would just tack on a redundant
// "Error: ..." line.
func launchOtto(args []string) error {
	err := otto.Start(args, ottoMembership(astrov1.NewV1Client(httputil.NewHTTPClient())))
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		os.Exit(exitErr.ExitCode())
	}
	if errors.Is(err, otto.ErrNotLoggedIn) || errors.Is(err, otto.ErrNotOrgMember) {
		os.Exit(1)
	}
	return err
}

// ottoMembership is the check Start runs before spawning Otto, against the
// Astro API. A 403 or 404 for the organization means not a member only when
// the login is a user who can say who they are: an API token reaches an
// organization by its own grants, not membership, so it is never refused
// here. Every other answer lets the launch go ahead.
func ottoMembership(client astrov1.APIClient) otto.MembershipCheck {
	return func(ctx context.Context, org string) (string, bool) {
		resp, err := client.GetOrganizationWithResponse(ctx, org, &astrov1.GetOrganizationParams{})
		if err != nil || resp.HTTPResponse == nil {
			return "", false
		}
		if code := resp.StatusCode(); code != http.StatusForbidden && code != http.StatusNotFound {
			return "", false
		}
		createIfNotExist := false
		self, err := client.GetSelfUserWithResponse(ctx, &astrov1.GetSelfUserParams{CreateIfNotExist: &createIfNotExist})
		if err != nil || self.JSON200 == nil || self.JSON200.Username == "" {
			return "", false
		}
		return self.JSON200.Username, true
	}
}
