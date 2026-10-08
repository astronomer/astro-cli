package astro

// terraformProviderURL is where Deployments as code live now that the CLI no
// longer reads deployment files.
const terraformProviderURL = "https://registry.terraform.io/providers/astronomer/astro/latest"

// Tombstones for the deployments-as-code flags v2 removed, registered
// with cliout.AddRemovedFlag. A CI job still passing one would otherwise fail
// with cobra's bare "unknown flag", which names neither replacement. Delete
// these in v3.
const (
	errDeploymentFileRemoved = "--deployment-file was removed in Astro CLI v2. " +
		"To manage Deployments as code, use the Astro Terraform provider: " + terraformProviderURL + ". " +
		"To copy a Deployment, use `astro deployment create --clone <deployment> --name <new name>`"
	errTemplateRemoved = "--template was removed in Astro CLI v2. " +
		"To copy a Deployment, use `astro deployment create --clone <deployment> --name <new name>`. " +
		"To manage Deployments as code, use the Astro Terraform provider: " + terraformProviderURL
)

// Tombstone for the flag `astro organization switch` had for printing a login
// link: a switch has not logged in again since 2023, so there is no login to
// link to. Delete it in v3.
const errLoginLinkRemoved = "--login-link was removed in Astro CLI v2: switching organizations no longer re-authenticates. " +
	"To log in on another device, use `astro login --login-link`"
