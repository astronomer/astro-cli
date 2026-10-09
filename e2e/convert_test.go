//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// 1.x conversion: `astro init` in a project that states its shape in 1.x files.
//
// The most review-sensitive thing in the release, because it rewrites
// somebody's repository and deletes files out of it. Also the cheapest thing
// here to test properly: it is file in, file out, so the whole table is tier 0
// and runs on Windows too, where the path handling differs and the conversion
// still has to agree.
//
// Asserted through --output json rather than the rendered text. The prose is
// long and will be reworded; what a conversion DID is the contract, and the
// json names it — created, updated, skipped, deleted, adopted, notes,
// advisories.
//
// The notes/advisories split is the assertion worth having. Notes are work
// left for a person; advisories describe something already carried that now
// behaves differently. Result's own doc explains why they are separate fields,
// and every expected fragment here has to land in its own list AND be absent
// from the other: a consumer that mixes them up either tells somebody to do a
// thing that has happened, or hides a change they did not ask for.
//
// One shape is missing on purpose. A 1.x airflow_settings.yaml with a
// connection or a variable carries a VALUE, and values go to the vault, which
// opens the OS keyring — the one thing no environment variable relocates, per
// doc.go.
// Converting such a project does not fail, it BLOCKS on a keyring prompt, so
// it cannot live in a tier that runs unattended on every pull request. It needs
// a case gated on a real keyring, and the suite has none yet. Every fixture
// below is checked against having opened the vault, so that boundary is an
// assertion rather than a comment.

// initResult is the shape `astro init --output json` publishes.
type initResult struct {
	Dir      string   `json:"dir"`
	Name     string   `json:"name"`
	Airflow  string   `json:"airflow"`
	Created  []string `json:"created"`
	Skipped  []string `json:"skipped"`
	Updated  []string `json:"updated"`
	Deleted  []string `json:"deleted"`
	Adopted  bool     `json:"adopted"`
	Notes    []string `json:"notes"`
	Advisory []string `json:"advisories"`
}

// case1x is one 1.x project and what converting it has to produce.
type case1x struct {
	name  string
	files map[string]string

	airflow string
	adopted bool
	// projectName is the [project] name the run must choose. A 1.x project
	// states one in .astro/config.yaml, and taking the directory instead
	// renames somebody's project.
	projectName string
	// manifestLines must appear as whole lines, which is what pins the ORDER
	// of a carried list: "requirements.txt lines land in dependencies, in file
	// order" is a promise a set-wise check cannot keep. Whole lines also need
	// no TOML parser, which this module deliberately does not depend on.
	manifestLines []string
	// manifestHas and manifestLacks are substrings, for everything that is
	// about content rather than order. Substrings on purpose: a whole line
	// would also pin the writer's quote style and inline-table key order,
	// which are cosmetic and would break a fixture for no behavioral reason.
	manifestHas   []string
	manifestLacks []string
	// retired names files the run must report deleting AND that must be gone
	// from disk. kept is the other half: a 1.x file that must survive, must not
	// be reported as removed, and whose content must be intact — keptHas names
	// a fragment that has to still be in it.
	retired []string
	kept    []string
	keptHas map[string]string
	// reportedUpdated and reportedSkipped are fragments the run has to file
	// under those names. Which list a file lands in is the contract a preview
	// renders, and Changeset.report's doc records the time an adopted manifest
	// went into one list and not the other, blanking the most important line
	// of a conversion preview.
	reportedUpdated []string
	reportedSkipped []string
	// notes and advisories are substrings, each of which must match one entry
	// of its own list and nothing in the other. noNotes asserts the opposite:
	// that the run had nothing to say, which is itself a promise for a project
	// whose files all carried.
	notes      []string
	advisories []string
	noNotes    bool
	// platform, when set, gives the run a current context on that platform
	// ("software" for APC, "cloud" for Astro; see writeLogin). Empty is a
	// machine that never logged in, which the CLI treats as Astro.
	platform string
	// args are passed to init after its own.
	args []string
}

// stock1xSettings is the airflow_settings.yaml 1.x's `astro dev init` wrote,
// byte for byte.
const stock1xSettings = `# This file allows you to configure Airflow Connections, Pools, and Variables in a single place for local development only.
# NOTE: json dicts can be added to the conn_extra field as yaml key value pairs. See the example below.

# For more information, refer to our docs: https://www.astronomer.io/docs/astro/cli/develop-project#configure-airflow_settingsyaml-local-development-only
# For questions, reach out to: https://support.astronomer.io
# For issues create an issue ticket here: https://github.com/astronomer/astro-cli/issues

airflow:
  connections:
    - conn_id:
      conn_type:
      conn_host:
      conn_schema:
      conn_login:
      conn_password:
      conn_port:
      conn_extra:
        example_extra_field: example-value
  pools:
    - pool_name:
      pool_slot:
      pool_description:
  variables:
    - variable_name:
      variable_value:
`

func cases1x() []case1x {
	// A pre-3 runtime tag names a runtime and not an Airflow minor, so it
	// always earns a note — which, because a file a note names is never
	// retired, is also what spares the Dockerfile in every fixture using it.
	const runtime2 = "FROM quay.io/astronomer/astro-runtime:9.1.0\n"
	// A modern tag names the Airflow version outright. No note, and so
	// nothing to keep the Dockerfile for: this is the shape that deletes it.
	const runtime3 = "FROM quay.io/astronomer/astro-runtime:3.1-12\n"

	return []case1x{
		{
			// The ordinary Airflow 2 conversion. Both 1.x lists reach the
			// manifest and go; the Dockerfile stays, because the note about
			// its tag is also something still to be said about it.
			name: "the whole 1.x shape",
			files: map[string]string{
				"Dockerfile": runtime2,
				// Deliberately NOT in alphabetical order, so the assertion
				// below distinguishes "file order" from "sorted". With pandas
				// before requests it could not.
				"requirements.txt": "requests\npandas==2.1.0\n",
				"packages.txt":     "libpq-dev\ngit\n",
			},
			airflow: "2",
			manifestLines: []string{
				"dependencies = [\n    'apache-airflow==2.*',\n    'requests',\n    'pandas==2.1.0',\n]",
				"packages = [\n    'libpq-dev',\n    'git',\n]",
			},
			retired: []string{"requirements.txt", "packages.txt"},
			// The most destructive deletion available, so its absence is
			// asserted rather than assumed.
			kept:    []string{"Dockerfile"},
			keptHas: map[string]string{"Dockerfile": "astro-runtime:9.1.0"},
			notes:   []string{"does not name the Airflow minor"},
		},
		{
			// A modern runtime tag names the Airflow version, so there is
			// nothing left to say about the Dockerfile and it goes with the
			// rest. The case that covers a conversion deleting a Dockerfile at
			// all, which is the shape every Airflow 3 1.x project carries.
			name: "a runtime tag that names the Airflow version",
			files: map[string]string{
				"Dockerfile":       runtime3,
				"requirements.txt": "pandas==2.1.0\n",
			},
			airflow:       "3.1",
			manifestLines: []string{"dependencies = [\n    'apache-airflow==3.1.*',\n    'pandas==2.1.0',\n]"},
			retired:       []string{"Dockerfile", "requirements.txt"},
			// Everything carried, so the run has nothing to report.
			noNotes: true,
		},
		{
			// The same project, logged in to Astro: Astro's deploy builds a
			// manifest project without the Dockerfile, so it goes as it does
			// on a machine that never logged in.
			name: "a runtime tag that names the Airflow version, under an Astro context",
			files: map[string]string{
				"Dockerfile":       runtime3,
				"requirements.txt": "pandas==2.1.0\n",
			},
			platform: "cloud",
			airflow:  "3.1",
			retired:  []string{"Dockerfile", "requirements.txt"},
			noNotes:  true,
		},
		{
			// And logged in to Astro Private Cloud, whose `astro deploy` still
			// builds the 1.x layout: .astro/config.yaml, which a conversion
			// keeps, and the Dockerfile, whose runtime base installs
			// requirements.txt and packages.txt. Retiring them left a project
			// that passed APC's project check and failed its build.
			name: "a runtime tag that names the Airflow version, under an APC context",
			files: map[string]string{
				".astro/config.yaml": "project:\n  name: orders-pipeline\n",
				"Dockerfile":         runtime3,
				"requirements.txt":   "pandas==2.1.0\n",
				"packages.txt":       "libpq-dev\n",
			},
			platform:      "software",
			airflow:       "3.1",
			projectName:   "orders-pipeline",
			manifestLines: []string{"dependencies = [\n    'apache-airflow==3.1.*',\n    'pandas==2.1.0',\n]"},
			// Kept, not declared: Astro and `astro local` still build from the
			// manifest.
			manifestLacks: []string{"dockerfile ="},
			kept:          []string{"Dockerfile", "requirements.txt", "packages.txt", ".astro/config.yaml"},
			keptHas:       map[string]string{"Dockerfile": "astro-runtime:3.1-12", "requirements.txt": "pandas==2.1.0"},
			notes: []string{"Dockerfile, packages.txt and requirements.txt: kept for Astro Private Cloud, " +
				"whose `astro deploy` builds the project from its Dockerfile as it stands; " +
				"its runtime base image installs packages.txt and requirements.txt during that build. " +
				"pyproject.toml carries the same Airflow version, dependencies and OS packages for `astro local` and Astro, " +
				"so change both together while the project deploys to Astro Private Cloud, and delete them if it deploys " +
				"to Astro instead. This run converted the project for Astro Private Cloud because the current context is " +
				"Astro Private Cloud (localhost); to convert for Astro instead, pass --deploy-target astro"},
		},
		{
			// The flag outranks the context, in both directions.
			name: "a runtime tag that names the Airflow version, --deploy-target astro under an APC context",
			files: map[string]string{
				"Dockerfile":       runtime3,
				"requirements.txt": "pandas==2.1.0\n",
			},
			platform: "software",
			args:     []string{"--deploy-target", "astro"},
			airflow:  "3.1",
			retired:  []string{"Dockerfile", "requirements.txt"},
			noNotes:  true,
		},
		{
			// A saved deploy target in its cuid shape is what both platforms
			// save, so the context decides it, and the link follows the same
			// answer the retirement does: under APC the target stays a note
			// beside the kept build rather than becoming an Astro link.
			name: "a saved cuid deploy target, under an APC context",
			files: map[string]string{
				".astro/config.yaml": "project:\n  name: orders-pipeline\n" +
					"  deployment: cm1ordersdeployment000001\n  workspace: cm1ordersworkspace0000001\n",
				"Dockerfile": runtime3,
			},
			platform:      "software",
			airflow:       "3.1",
			projectName:   "orders-pipeline",
			manifestLacks: []string{"[tool.astro.deployments]", "dockerfile ="},
			kept:          []string{"Dockerfile", ".astro/config.yaml"},
			notes: []string{
				"Dockerfile: kept for Astro Private Cloud",
				"cm1ordersdeployment000001 in workspace cm1ordersworkspace0000001 is this project's saved deploy target, " +
					"which Astro Private Cloud's `astro deploy` reads from this file, so it stays here rather than " +
					"becoming a [tool.astro.deployments] link",
			},
		},
		{
			// --deploy-target apc converts for APC under an Astro context, and
			// a saved target that looks like a Software release name does not
			// decide either way: the flag did, and the notes say so.
			name: "a saved release name, --deploy-target apc under an Astro context",
			files: map[string]string{
				".astro/config.yaml": "project:\n  name: orders-pipeline\n  deployment: celestial-gravity-1234\n",
				"Dockerfile":         runtime3,
				"requirements.txt":   "pandas==2.1.0\n",
			},
			platform:      "cloud",
			args:          []string{"--deploy-target", "apc"},
			airflow:       "3.1",
			projectName:   "orders-pipeline",
			manifestLacks: []string{"[tool.astro.deployments]", "dockerfile ="},
			kept:          []string{"Dockerfile", "requirements.txt", ".astro/config.yaml"},
			notes: []string{
				"Dockerfile and requirements.txt: kept for Astro Private Cloud",
				"This run converted the project for Astro Private Cloud because of --deploy-target apc; " +
					"to convert for Astro instead, pass --deploy-target astro",
			},
		},
		{
			// Without the flag the same project converts for the Astro
			// context: the release-name shape is no signal, since an Astro
			// Deployment's namespace has it too.
			name: "a saved release name, under an Astro context",
			files: map[string]string{
				".astro/config.yaml": "project:\n  name: orders-pipeline\n  deployment: celestial-gravity-1234\n",
				"Dockerfile":         runtime3,
				"requirements.txt":   "pandas==2.1.0\n",
			},
			platform:    "cloud",
			airflow:     "3.1",
			projectName: "orders-pipeline",
			retired:     []string{"Dockerfile", "requirements.txt"},
			kept:        []string{".astro/config.yaml"},
			notes: []string{
				"celestial-gravity-1234 is this project's saved deploy target. If that is an Astro Deployment, " +
					"give it a name under [tool.astro.deployments]",
				"This run converted the project for Astro because the current context is Astro (localhost); " +
					"to convert for Astro Private Cloud instead, pass --deploy-target apc",
			},
		},
		{
			// A Dockerfile that does more than pin becomes the project's
			// declared build, which makes BOTH 1.x lists load-bearing again:
			// the base image's ONBUILD reads them during the build, so deleting
			// either would change what the image contains.
			//
			// Both, because that is the bug planRetirements describes fixing —
			// an ordinary conversion deleted the two files and left a build
			// that either failed on the missing COPY or produced an image with
			// none of the project's packages.
			name: "a Dockerfile that does more than pin",
			files: map[string]string{
				"Dockerfile":       runtime2 + "RUN apt-get update && apt-get install -y curl\n",
				"requirements.txt": "pandas==2.1.0\n",
				"packages.txt":     "libpq-dev\n",
			},
			airflow:       "2",
			manifestLines: []string{"dependencies = [\n    'apache-airflow==2.*',\n    'pandas==2.1.0',\n]"},
			manifestHas:   []string{"dockerfile = 'Dockerfile'", "packages = [\n    'libpq-dev',\n]"},
			kept:          []string{"Dockerfile", "requirements.txt", "packages.txt"},
			keptHas: map[string]string{
				"requirements.txt": "pandas==2.1.0",
				"packages.txt":     "libpq-dev",
			},
			notes: []string{
				"its RUN instructions were not read here",
				"kept, because your Dockerfile's base image reads it during the build",
			},
		},
		{
			// A declared build whose base names its Python. The manifest pins
			// that minor, so uv locks for, and standalone mode runs, the Python
			// the image runs rather than every one after the runtime's floor.
			name: "a declared Dockerfile whose base names its Python",
			files: map[string]string{
				"Dockerfile":       "FROM astrocrpublic.azurecr.io/runtime:3.3-2-python-3.13\nRUN pip install --no-cache-dir uv\n",
				"requirements.txt": "pandas==2.1.0\n",
			},
			airflow:     "3.3",
			manifestHas: []string{"requires-python = '==3.13.*'", "dockerfile = 'Dockerfile'"},
			kept:        []string{"Dockerfile", "requirements.txt"},
			notes:       []string{"its RUN instructions were not read here"},
		},
		{
			// A kept Dockerfile builds with the project as its context, so the
			// .dockerignore the 1.x CLI wrote keeps its lines and gains the per-machine
			// paths it lacks.
			name: "a kept Dockerfile beside a 1.x .dockerignore",
			files: map[string]string{
				"Dockerfile":    runtime3 + "RUN echo hi\n",
				".dockerignore": "astro\n.git\n.env\n.venv\n",
			},
			airflow:     "3.1",
			manifestHas: []string{"dockerfile = 'Dockerfile'"},
			kept:        []string{"Dockerfile", ".dockerignore"},
			keptHas: map[string]string{
				".dockerignore": "astro\n.git\n.env\n.venv\n\n# Per-machine files Astro tools write into the project. Keep them out of the image.\n.astro/standalone/\n",
			},
			reportedUpdated: []string{".dockerignore (added the per-machine rules)"},
			notes:           []string{"its RUN instructions were not read here"},
		},
		{
			// Four kinds of line [project.dependencies] cannot express. Each
			// gets a note naming the line and where it belongs, none is guessed
			// at, and the file stays with those lines still in it.
			name: "requirement lines a manifest cannot express",
			files: map[string]string{
				"Dockerfile": runtime2,
				"requirements.txt": "pandas==2.1.0\n" +
					"-e .\n" +
					"https://example.com/pkg.tar.gz\n" +
					"--index-url https://example.com/simple\n" +
					"./local-wheel.whl\n",
			},
			airflow:       "2",
			manifestLines: []string{"dependencies = [\n    'apache-airflow==2.*',\n    'pandas==2.1.0',\n]"},
			// None of the four reached the manifest in any form.
			manifestLacks: []string{"-e .", "example.com", "local-wheel"},
			kept:          []string{"requirements.txt"},
			keptHas:       map[string]string{"requirements.txt": "-e ."},
			notes: []string{
				"-e . is an editable install",
				"is a bare URL",
				"names a package index",
				"is a local path",
			},
		},
		{
			// An env schema left by an older desktop build is not a 1.x file:
			// the conversion reads nothing from it, says nothing about it, and
			// leaves it where it is.
			name: "an old desktop env schema is ignored",
			files: map[string]string{
				"Dockerfile":             runtime2,
				".astro/env.schema.yaml": "env_vars:\n  - key: API_URL\n    required: true\n",
			},
			airflow:       "2",
			manifestLacks: []string{"[tool.astro.env", "API_URL"},
			kept:          []string{".astro/env.schema.yaml"},
			keptHas:       map[string]string{".astro/env.schema.yaml": "API_URL"},
		},
		{
			// A manifest that already pins Airflow keeps its own pin: the
			// Dockerfile does not overrule what the project said about itself.
			name: "a manifest that already pins Airflow",
			files: map[string]string{
				"Dockerfile": runtime2,
				"pyproject.toml": "[project]\n" +
					"name = 'already'\n" +
					"version = '0.1.0'\n" +
					"requires-python = '>=3.10'\n" +
					"dependencies = ['apache-airflow==2.9.*']\n",
			},
			airflow: "2.9",
			adopted: true,
			manifestHas: []string{
				"name = 'already'",
				"dependencies = ['apache-airflow==2.9.*']",
			},
			kept: []string{"Dockerfile"},
			// The manifest was there and gained a section, so it is an update.
			// Filed as a creation it would blank the most important line of a
			// preview, which is a thing that has happened.
			reportedUpdated: []string{"pyproject.toml"},
			notes: []string{
				"admits a Python that Airflow 2.9 cannot run",
			},
		},
		{
			// An Airflow pin stated in requirements.txt itself, which is the
			// arm of pickAirflowVersion below the Dockerfile and above the
			// default. Three things at once: the pin is extracted, the
			// interpreter range narrows to what that Airflow can run, and the
			// carried list does not repeat the pin it produced.
			name: "an Airflow pin inside requirements.txt",
			files: map[string]string{
				"requirements.txt": "apache-airflow==2.8.1\npandas\n",
			},
			airflow:       "2.8.1",
			manifestLines: []string{"dependencies = [\n    'apache-airflow==2.8.1',\n    'pandas',\n]"},
			manifestHas: []string{
				"requires-python = '>=3.10,<3.12'",
			},
			retired: []string{"requirements.txt"},
		},
		{
			// Nothing names a version, so the pin is the default, and both
			// places that carry it agree. Deliberately quiet: a project that
			// never expressed an opinion is not owed a warning, which is what
			// project1x.statedVersion separates from "said something
			// unreadable".
			name: "nothing names an Airflow version",
			files: map[string]string{
				"requirements.txt": "pandas==2.1.0\n",
			},
			airflow:       "3.3",
			manifestLines: []string{"dependencies = [\n    'apache-airflow==3.3.*',\n    'pandas==2.1.0',\n]"},
			retired:       []string{"requirements.txt"},
			noNotes:       true,
		},
		{
			// A settings file with nothing left in it is retired like any
			// other carried 1.x file. The variable is empty so the
			// case stores nothing and stays off the keyring. An empty variable
			// is declared optional, since the 1.x CLI skipped it rather than create it,
			// and the manifest carries no value for it.
			name: "a settings file whose contents all carry",
			files: map[string]string{
				"Dockerfile": runtime2,
				"airflow_settings.yaml": "airflow:\n" +
					"  variables:\n" +
					"    - variable_name: region\n" +
					"      variable_value: \"\"\n",
			},
			airflow: "2",
			manifestHas: []string{
				"[tool.astro.env.airflow_variables]",
				"region = {optional = true, secret = true}",
			},
			manifestLacks: []string{"default"},
			retired:       []string{"airflow_settings.yaml"},
			advisories:    []string{"region: declared as an optional Airflow variable"},
		},
		{
			// The file 1.x's `astro dev init` wrote, untouched. Its entries
			// have blank ids, which the 1.x CLI skipped, so nothing carries and the
			// file goes without a note.
			name: "the stock 1.x settings file",
			files: map[string]string{
				"requirements.txt":      "pandas==2.1.0\n",
				"airflow_settings.yaml": stock1xSettings,
			},
			manifestLacks: []string{"tool.astro.env"},
			retired:       []string{"requirements.txt", "airflow_settings.yaml"},
			noNotes:       true,
		},
		{
			// Pools move into [tool.astro.pools], which `astro local start`
			// creates in Airflow, so a file holding only pools has nothing
			// left in it and goes.
			name: "a settings file with pools",
			files: map[string]string{
				"Dockerfile": runtime2,
				"airflow_settings.yaml": "airflow:\n" +
					"  pools:\n" +
					"    - pool_name: heavy\n" +
					"      pool_slot: 5\n",
			},
			airflow:     "2",
			manifestHas: []string{"[tool.astro.pools]", "heavy = {slots = 5}"},
			retired:     []string{"airflow_settings.yaml"},
		},
		{
			// What `astro dev init` actually left behind: a project config, a
			// DAG, a .gitignore. What is already there is kept and reported as
			// skipped, rather than written over.
			name: "a real 1.x repository",
			files: map[string]string{
				".astro/config.yaml": "project:\n  name: orders-pipeline\n",
				"Dockerfile":         runtime2,
				"dags/my_dag.py":     "# the project's own dag\n",
				".gitignore":         ".env\n",
			},
			airflow: "2",
			// The project keeps the name it calls itself, not the directory's.
			// A 1.x project named orders-pipeline in a directory called
			// something else is still orders-pipeline — it is the manifest's
			// identity, and what `astro package` names an artifact after.
			projectName: "orders-pipeline",
			manifestHas: []string{"name = 'orders-pipeline'"},
			kept:        []string{"Dockerfile", ".astro/config.yaml"},
			// Untouched, which is the point of reporting them separately from
			// what this run wrote.
			keptHas: map[string]string{"dags/my_dag.py": "the project's own dag"},
			reportedSkipped: []string{
				"dags/",
				".gitignore",
			},
			// No note about .astro/config.yaml. This is what `astro dev init`
			// writes, it saved no deploy target, and the name it does state was
			// carried. The case below is the one that earns the note.
		},
		{
			// A 1.x repository that also keeps a pyproject.toml, holding only
			// tool settings. Common in real 1.x repositories, and not a v2
			// project: the conversion adopts the file, carries the 1.x lists into
			// it, and leaves the tool tables as they were.
			name: "a 1.x repository whose pyproject.toml only configures tools",
			files: map[string]string{
				".astro/config.yaml": "project:\n  name: orders-pipeline\n",
				"Dockerfile":         runtime3,
				"requirements.txt":   "pandas==2.1.0\n",
				"pyproject.toml":     "[tool.ruff]\nline-length = 120\n\n[tool.pytest.ini_options]\ntestpaths = ['tests']\n",
			},
			airflow:         "3.1",
			adopted:         true,
			projectName:     "orders-pipeline",
			manifestHas:     []string{"'pandas==2.1.0'", "'apache-airflow==3.1.*'", "[tool.astro]", "[tool.ruff]", "line-length = 120", "[tool.pytest.ini_options]"},
			retired:         []string{"Dockerfile", "requirements.txt"},
			kept:            []string{".astro/config.yaml"},
			reportedUpdated: []string{"pyproject.toml"},
		},
		{
			// The same file after `astro deploy --save`. The saved target is
			// the one thing here a project wants, so the note states the
			// whole manifest entry for it, table name included, rather than
			// pointing at the section it belongs in.
			name: "a 1.x project with a saved deploy target",
			files: map[string]string{
				".astro/config.yaml": "project:\n  name: orders-pipeline\n" +
					"  deployment: cm1orders\n  workspace: cm1ws\n",
				"Dockerfile": runtime2,
			},
			airflow:     "2",
			projectName: "orders-pipeline",
			kept:        []string{"Dockerfile", ".astro/config.yaml"},
			notes: []string{
				"cm1orders in workspace cm1ws is this project's saved deploy target",
				"[tool.astro.deployments.prod]",
			},
		},
		{
			// Both ids in their Astro shape: the saved target is carried as a
			// link, the way `astro link add` writes one, so it is an advisory
			// and not a note.
			name: "a 1.x project whose saved deploy target can be linked",
			files: map[string]string{
				".astro/config.yaml": "project:\n  name: orders-pipeline\n" +
					"  deployment: cm1ordersdeployment000001\n  workspace: cm1ordersworkspace0000001\n",
				"Dockerfile": runtime2,
			},
			airflow:     "2",
			projectName: "orders-pipeline",
			kept:        []string{"Dockerfile", ".astro/config.yaml"},
			manifestHas: []string{
				"[tool.astro.deployments]\ndefault = {deployment = 'cm1ordersdeployment000001', workspace = 'cm1ordersworkspace0000001', default = true}",
			},
			advisories: []string{"its saved deploy target is now the default link"},
		},
		{
			// The `instances:` list an early v2 build wrote, which keeps each
			// deployment id under `auth:`. The link command names that id.
			name: "a config with instances",
			files: map[string]string{
				".astro/config.yaml": "project:\n  name: example-project\n" +
					"instances:\n" +
					"  - auth:\n      deployment_id: cexampledeployment0000001\n      kind: astro_pat\n" +
					"    name: example-dev\n    source: astro\n",
				"Dockerfile": runtime2,
			},
			airflow:     "2",
			projectName: "example-project",
			kept:        []string{"Dockerfile", ".astro/config.yaml"},
			notes:       []string{"`astro link add example-dev --deployment cexampledeployment0000001`"},
		},
		{
			// A stated name that cannot be a [project] name as written is
			// respelled, and the respelling is reported: the project said what
			// it was called and this is not quite that.
			name: "a 1.x name that is not a legal project name",
			files: map[string]string{
				".astro/config.yaml": "project:\n  name: Orders Pipeline\n",
				"Dockerfile":         runtime3,
			},
			airflow:     "3.1",
			projectName: "orders-pipeline",
			manifestHas: []string{"name = 'orders-pipeline'"},
			kept:        []string{".astro/config.yaml"},
			// An advisory, not a note: it is already named that.
			//
			// And no config note, which is the whole rule: leftovers fires it
			// on the file naming a deploy target, never on the file being
			// present, and this fixture's config names none.
			advisories: []string{"from Orders Pipeline in .astro/config.yaml"},
		},
		{
			// The property behind all of it: a file is retired only once
			// everything it said is in the manifest, so a line that could not
			// be carried is also a reason to keep the file — with that line
			// still in it.
			name: "one file fully carried beside one that was not",
			files: map[string]string{
				"Dockerfile":       runtime2,
				"requirements.txt": "pandas==2.1.0\n-e .\n",
				"packages.txt":     "libpq-dev\n",
			},
			airflow: "2",
			retired: []string{"packages.txt"},
			kept:    []string{"requirements.txt"},
			keptHas: map[string]string{"requirements.txt": "-e ."},
			notes:   []string{"-e . is an editable install"},
		},
	}
}

func TestInitConvertsA1xProject(t *testing.T) {
	tier(t, 0)

	for _, tc := range cases1x() {
		t.Run(tc.name, func(t *testing.T) {
			p := newProject(t)
			for path, content := range tc.files {
				full := filepath.Join(p.Dir, path)
				mkdir(t, filepath.Dir(full))
				write(t, full, content)
			}

			var res initResult
			args := append([]string{"init", "--output", "json"}, tc.args...)
			if tc.platform == "" {
				p.run(args...).requireSuccess().requireJSON(&res)
			} else {
				// runIn keeps every request off the network.
				writeContext(t, p, tc.platform)
				runIn(t, p, args...).requireSuccess().requireJSON(&res)
			}

			if tc.airflow != "" && res.Airflow != tc.airflow {
				t.Errorf("airflow = %q, want %q", res.Airflow, tc.airflow)
			}
			if tc.projectName != "" && res.Name != tc.projectName {
				t.Errorf("name = %q, want %q", res.Name, tc.projectName)
			}
			if res.Adopted != tc.adopted {
				t.Errorf("adopted = %v, want %v (created and adopted are different events)", res.Adopted, tc.adopted)
			}
			checkManifest(t, p, &tc)
			checkFiles(t, p, &res, &tc)
			checkLists(t, &res, &tc)
			// Universal, not per-case: no fixture here carries a value, so any
			// of them reaching the vault is drift that would block the whole
			// tier on a keyring prompt.
			checkVaultUntouched(t, p)
		})
	}
}

// checkManifest requires each expected line or fragment, and the absence of
// anything a conversion must not have written.
func checkManifest(t *testing.T, p *project, tc *case1x) {
	t.Helper()
	manifest := read(t, filepath.Join(p.Dir, "pyproject.toml"))
	for _, line := range tc.manifestLines {
		if !strings.Contains(manifest, line) {
			t.Errorf("the manifest is missing the line\n\t%s\ngot:\n%s", line, manifest)
		}
	}
	for _, want := range tc.manifestHas {
		if !strings.Contains(manifest, want) {
			t.Errorf("the manifest is missing %q\ngot:\n%s", want, manifest)
		}
	}
	for _, unwanted := range tc.manifestLacks {
		if strings.Contains(manifest, unwanted) {
			t.Errorf("the manifest carries %q, which it cannot express\ngot:\n%s", unwanted, manifest)
		}
	}
	// The requirement states the version, and a manifest with a second
	// [tool.astro] airflow line beside it does not load.
	if regexp.MustCompile(`(?m)^airflow\s*=`).MatchString(manifest) {
		t.Errorf("the conversion wrote a [tool.astro] airflow line\ngot:\n%s", manifest)
	}
}

// checkFiles holds the conversion to both halves of every claim it makes about
// a file: reported and actually done.
//
// Reporting without removing, or removing without reporting, are each their own
// bug in a command that edits somebody's repository — and a file kept but
// rewritten is a third, which is what keptHas is for.
func checkFiles(t *testing.T, p *project, res *initResult, tc *case1x) {
	t.Helper()
	for _, name := range tc.retired {
		if !reports(res.Deleted, name) {
			t.Errorf("%s reached the manifest, so the run should report removing it: %v", name, res.Deleted)
		}
		if _, err := os.Stat(filepath.Join(p.Dir, name)); !os.IsNotExist(err) {
			t.Errorf("%s was reported removed but is still there (stat: %v)", name, err)
		}
	}
	for _, name := range tc.kept {
		if _, err := os.Stat(filepath.Join(p.Dir, name)); err != nil {
			t.Errorf("%s should have survived the conversion: %v", name, err)
		}
		if reports(res.Deleted, name) {
			t.Errorf("%s survived, so it must not be reported as removed: %v", name, res.Deleted)
		}
	}
	for name, want := range tc.keptHas {
		if got := read(t, filepath.Join(p.Dir, name)); !strings.Contains(got, want) {
			t.Errorf("%s survived but no longer contains %q:\n%s", name, want, got)
		}
	}
}

// checkLists requires each fragment to land in the list that describes it.
//
// Notes are work left for a person; advisories are changes already made; and
// which of created, updated and skipped a file lands in is what a preview
// renders. A consumer cannot tell them apart from the text, which is why they
// are separate fields and why this checks them separately.
func checkLists(t *testing.T, res *initResult, tc *case1x) {
	t.Helper()
	for _, want := range tc.notes {
		if !reports(res.Notes, want) {
			t.Errorf("no note mentions %q\nnotes: %v", want, res.Notes)
		}
		if reports(res.Advisory, want) {
			t.Errorf("%q is work left to do, so it belongs in notes, not advisories", want)
		}
	}
	for _, want := range tc.advisories {
		if !reports(res.Advisory, want) {
			t.Errorf("no advisory mentions %q\nadvisories: %v", want, res.Advisory)
		}
		if reports(res.Notes, want) {
			t.Errorf("%q describes something already carried, so it belongs in advisories, not notes", want)
		}
	}
	if tc.noNotes && len(res.Notes) > 0 {
		t.Errorf("everything this project said was carried, so the run should have had nothing to add: %v", res.Notes)
	}
	for _, want := range tc.reportedUpdated {
		if !reports(res.Updated, want) {
			t.Errorf("%q was changed rather than created, so it belongs in updated: created=%v updated=%v",
				want, res.Created, res.Updated)
		}
	}
	for _, want := range tc.reportedSkipped {
		if !reports(res.Skipped, want) {
			t.Errorf("%q was already there and left alone, so it belongs in skipped: created=%v skipped=%v",
				want, res.Created, res.Skipped)
		}
	}
}

// checkVaultUntouched requires that no conversion here opened the vault.
//
// The boundary this whole file depends on. A value carried to the vault opens
// the OS keyring, which no environment variable relocates, and a keyring prompt
// does not fail a test — it hangs it, on every pull request. So the claim that
// these fixtures carry no values is checked rather than trusted.
func checkVaultUntouched(t *testing.T, p *project) {
	t.Helper()
	// pkg/secrets keeps the vault under $HOME/.astro, and the harness points
	// HOME at the project's own temp directory.
	if _, err := os.Stat(filepath.Join(p.home, ".astro", "secrets")); !os.IsNotExist(err) {
		t.Errorf("a fixture reached the vault (stat: %v); a case that carries a value needs a keyring gate, not tier 0", err)
	}
}

// A config this run cannot parse does not reach stdout.
//
// The 1.x config loader prints its read errors, and printed them to stdout —
// so `astro init --output json` in a project whose .astro/config.yaml will not
// parse put a line of prose in front of the object and no consumer could read
// the result. The command still succeeds: the file is 1.x CLI configuration
// that a conversion only takes a name out of.
//
// requireJSON unmarshals the whole of stdout, so it is the assertion: one
// stray line and it fails.
func TestInitKeepsStdoutParseableWhenThe1xConfigWillNot(t *testing.T) {
	tier(t, 0)

	p := newProject(t)
	mkdir(t, filepath.Join(p.Dir, ".astro"))
	write(t, filepath.Join(p.Dir, ".astro", "config.yaml"), "project:\n  name: [not a string\n")
	write(t, filepath.Join(p.Dir, "Dockerfile"), "FROM quay.io/astronomer/astro-runtime:3.1-12\n")

	var res initResult
	r := p.run("init", "--output", "json").requireSuccess()
	r.requireJSON(&res)

	// The name falls back to the directory, since the file that states one
	// could not be read.
	if res.Name == "" {
		t.Error("a project still needs a name when its 1.x config will not parse")
	}
	// And the warning is not lost, it is on the stream prose belongs on — and
	// names the file it is about, rather than the home directory, which is
	// where it used to send people looking.
	if !strings.Contains(r.Stderr, "project config") {
		t.Errorf("the parse failure should be reported on stderr, naming the project's config:\n%s", r.output())
	}
}

// reports is true when any entry contains want.
//
// Entries are "<file> (<why>)" or "<file>: <what>", so a substring is the
// honest check: the reason is prose that will be reworded, and the file and the
// fact are what the assertion is about.
func reports(entries []string, want string) bool {
	for _, e := range entries {
		if strings.Contains(e, want) {
			return true
		}
	}
	return false
}
