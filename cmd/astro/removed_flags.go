package astro

import (
	"errors"

	"github.com/spf13/cobra"
)

// terraformProviderURL is where Deployments as code live now that the CLI no
// longer reads deployment files.
const terraformProviderURL = "https://registry.terraform.io/providers/astronomer/astro/latest"

// Tombstones for the deployments-as-code flags v2 removed. A CI job
// still passing one would otherwise fail with cobra's bare "unknown flag",
// which names neither replacement. Delete these in v3.
const (
	errDeploymentFileRemoved = "--deployment-file was removed in Astro CLI v2. " +
		"To manage Deployments as code, use the Astro Terraform provider: " + terraformProviderURL + ". " +
		"To copy a Deployment, use `astro deployment create --clone <deployment> --name <new name>`"
	errTemplateRemoved = "--template was removed in Astro CLI v2. " +
		"To copy a Deployment, use `astro deployment create --clone <deployment> --name <new name>`. " +
		"To manage Deployments as code, use the Astro Terraform provider: " + terraformProviderURL
)

// addRemovedFlag registers name, hidden, so that a run passing it fails with
// msg instead of cobra's "unknown flag". isBool is the flag's old kind: a
// string flag still consumes its value, so `--deployment-file f.yaml` does not
// leave f.yaml behind as an argument.
//
// The check runs in Args, which cobra calls before any pre-run, so the run
// fails before it logs in or asks the API anything; cliout.Execute marks an
// Args error as a usage error (exit 2), and under --output json reports it as
// the error object.
func addRemovedFlag(cmd *cobra.Command, name, shorthand string, isBool bool, msg string) {
	if isBool {
		cmd.Flags().BoolP(name, shorthand, false, "")
	} else {
		cmd.Flags().StringP(name, shorthand, "", "")
	}
	_ = cmd.Flags().MarkHidden(name) //nolint:errcheck // the flag is defined just above; this only errors on an unknown flag name
	validate := cmd.Args
	cmd.Args = func(c *cobra.Command, args []string) error {
		if c.Flags().Changed(name) {
			return errors.New(msg)
		}
		if validate != nil {
			return validate(c, args)
		}
		return nil
	}
}
