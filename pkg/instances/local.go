package instances

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/astronomer/astro-cli/pkg/airflowapi"
	"github.com/astronomer/astro-cli/pkg/airflowrt"
	"github.com/astronomer/astro-cli/pkg/manifest"
)

// What the local engines provision, which is not "nothing" and differs by
// Airflow generation:
//
//   - Airflow 3, both engines, runs the simple auth manager with
//     SIMPLE_AUTH_MANAGER_ALL_ADMINS on (pkg/airflowrt.BuildEnv for standalone,
//     the compose environment in pkg/localrt's docker engine). Everyone is an admin
//     and /auth/token mints for whoever asks, with no credentials to send —
//     which is what the minter's credential-less mint is for.
//   - Airflow 2, both engines, runs the basic_auth backend on (pkg/localrt's
//     standalone engine, the compose environment in its docker engine).
//     Flask-AppBuilder has no all-admins mode, so there is a real account:
//     docker mode creates admin/admin with the database migration, and so does
//     standalone's macOS launch shim; everywhere else `airflow standalone`
//     generates a password into standalone_admin_password.txt under its
//     AIRFLOW_HOME.
//
// Which of the two a project runs comes from the Airflow it pins — the same
// fact the standalone engine branches on when it launches.
const (
	// The account every local engine provisions, from the package that owns the
	// one writer nothing else can reach — standalone's macOS shim. NOT
	// docker-only: localAccount returns this username for standalone too, and
	// localPasswordFor falls back to this password whenever the generated file
	// is missing or blank. Changing it moves docker mode, standalone on macOS,
	// and every caller that authenticates, together.
	localUsername = airflowrt.Airflow2AdminUser
	localPassword = airflowrt.Airflow2AdminPassword
	// localPasswordFile is where Airflow 2's standalone writes the password it
	// generated, relative to the project's standalone AIRFLOW_HOME.
	localPasswordFile = "standalone_admin_password.txt"
)

// localCredentials mints against the local Airflow with whatever its engine
// provisioned. The token lives in memory for the run, and the refresh hook
// re-mints when a short-lived one expires mid-command.
func localCredentials(i Instance, baseURL string, d Deps) (airflowapi.CredentialSource, func(context.Context) error, error) {
	username, password := localAccount(i.AirflowMajor, i.Mode, i.Project)
	minter, err := airflowapi.NewTokenMinter(baseURL, username, password, d.httpOptions()...)
	if err != nil {
		return nil, nil, err
	}
	return minter.Credentials, minter.Refresh, nil
}

// airflow2 is the generation that needs an account, and modeDocker the engine
// that provisions one itself — both spelled as the record and the engines
// spell them.
const (
	airflow2   = "2"
	modeDocker = "docker"
)

// localAccount reports the credentials to mint a local Airflow's token with:
// none on Airflow 3, where all-admins mode mints for whoever asks, and the
// admin account on Airflow 2. Docker mode creates that account with a known
// password; standalone may have generated one instead, so only standalone reads
// the generated file — a project that ran standalone before it ran docker still
// has that file, and it names a different password.
//
// major comes from the runtime record — a fact about the process that is
// running, which the manifest is not: the pin can be edited, or the project
// deleted, while Airflow keeps serving. Only a record written before that field
// existed falls back to reading the manifest, and a manifest that cannot be
// read at all reads as Airflow 3, which is what the v2 scaffold writes.
func localAccount(major, mode, projectPath string) (username, password string) {
	if major == "" {
		major = pinnedAirflowMajor(projectPath)
	}
	if major != airflow2 {
		return "", ""
	}
	if mode == modeDocker {
		return localUsername, localPassword
	}
	return localUsername, localPasswordFor(projectPath)
}

// pinnedAirflowMajor reads the Airflow a project pins, for a runtime record too
// old to carry it.
func pinnedAirflowMajor(projectPath string) string {
	if projectPath == "" {
		return ""
	}
	m, err := manifest.Load(filepath.Join(projectPath, manifest.Marker))
	if err != nil {
		return ""
	}
	major, _, _ := strings.Cut(m.Astro.AirflowVersion, ".")
	return major
}

// localPasswordFor reads the password Airflow 2's standalone generated for this
// project, falling back to the one the macOS launch shim seeds.
func localPasswordFor(projectPath string) string {
	raw, err := os.ReadFile(filepath.Join(projectPath, airflowrt.StandaloneDir, localPasswordFile))
	if err != nil {
		return localPassword
	}
	if password := strings.TrimSpace(string(raw)); password != "" {
		return password
	}
	return localPassword
}
