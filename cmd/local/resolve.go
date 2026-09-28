package local

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/internal/instancelocate"
	"github.com/astronomer/astro-cli/internal/project"
	"github.com/astronomer/astro-cli/internal/userstate"
	"github.com/astronomer/astro-cli/pkg/airflowapi"
	"github.com/astronomer/astro-cli/pkg/awsauth"
	"github.com/astronomer/astro-cli/pkg/googleauth"
	"github.com/astronomer/astro-cli/pkg/instances"
	"github.com/astronomer/astro-cli/pkg/localrt"
	"github.com/astronomer/astro-cli/pkg/manifest"
)

// This file is the composition root for every command that acts on an Airflow:
// which one it acts on, and the client it opens. The two answers are reached
// differently and deliberately cannot be confused — a deployment is resolved
// through the precedence rule, and the machine is simply looked up, because
// `astro local …` said so.

// deploymentSet builds what this project can resolve to: the manifest's links,
// and nothing else. The machine's own Airflow is deliberately absent — it is
// reached by spelling a command `astro local …` (see machineInstance).
func (c *cli) deploymentSet() (projectDir string, set instances.Set, err error) {
	dir, err := c.projectPath()
	if err != nil {
		return "", instances.Set{}, err
	}
	m, err := manifest.Load(filepath.Join(dir, project.Marker))
	if err != nil {
		return "", instances.Set{}, err
	}
	return dir, instances.Build(m), nil
}

// runningLocals is every local Airflow alive on this machine, for the inventory
// that promises to list them all. Liveness is why the runtime does the listing
// rather than the record store directly — a leftover record is not a running
// Airflow.
//
// Every path is canonicalized here, at the boundary, because identity is a
// filesystem question: a record written from /private/tmp/x and a command run
// from /tmp/x are the same project, and pkg/instances compares the strings
// it is given rather than touching the disk itself.
func (c *cli) runningLocals() ([]instances.Local, error) {
	statuses, err := c.d.Runtime.List()
	if err != nil {
		return nil, err
	}
	running := make([]instances.Local, 0, len(statuses))
	for i := range statuses {
		if statuses[i].State != localrt.StateRunning {
			continue
		}
		running = append(running, instances.Local{
			ProjectPath:  canonical(statuses[i].ProjectPath),
			Port:         statuses[i].Port,
			AirflowMajor: statuses[i].AirflowMajor,
			Mode:         string(statuses[i].Mode),
		})
	}
	return running, nil
}

// canonical resolves symlinks in a path so two spellings of one directory
// compare equal. A path that cannot be resolved — a project deleted while its
// Airflow runs — keeps its original spelling, which is still the best name for
// it.
func canonical(path string) string {
	resolved, err := localrt.CanonicalPath(path)
	if err != nil {
		return path
	}
	return resolved
}

// deploymentRequest fills the two layers a command reads rather than parses:
// the env var and the project's pin. A command that also has flags to add sets
// them on the result.
func deploymentRequest(projectDir string) (instances.Request, error) {
	req := instances.Request{Env: os.Getenv(instances.EnvVar)}
	state, err := userstate.Load(projectDir)
	if err != nil {
		// State that does not parse is not this command's to repair, but the
		// command that does repair it should be in the message rather than left
		// for the user to find.
		return req, pinStateError(err)
	}
	req.Pin = state.Instance
	return req, nil
}

// standing is the front half bare `astro use` needs: the project, its
// deployments, and the layers that stand between runs. It parses no flag into
// the rule — it reports what is true right now.
func (c *cli) standing() (projectDir string, set instances.Set, req instances.Request, err error) {
	dir, set, err := c.deploymentSet()
	if err != nil {
		return "", instances.Set{}, instances.Request{}, err
	}
	req, err = deploymentRequest(dir)
	if err != nil {
		return "", instances.Set{}, instances.Request{}, err
	}
	return dir, set, req, nil
}

// deploymentFlags carries the two flags every top-level Airflow-facing command
// takes. The `astro local` registration of those same commands takes neither:
// its target is the machine, and there is nothing to select.
type deploymentFlags struct {
	deployment string
	url        string
}

// addDeploymentFlags registers the selector flags on cmd. One registration
// keeps the spelling identical across the whole query surface: -d is settled
// for --deployment, and --url is the escape hatch for an Airflow no project
// declares.
func addDeploymentFlags(cmd *cobra.Command, f *deploymentFlags) {
	cmd.PersistentFlags().StringVarP(&f.deployment, "deployment", "d", "", "Deployment to act on, by the name the manifest links it under")
	cmd.PersistentFlags().StringVar(&f.url, "url", "", "Airflow base URL to act on directly, for an Airflow no project declares")
}

// deploymentClient resolves which deployment this run acts on and opens a
// client on it.
func (c *cli) deploymentClient(ctx context.Context, f deploymentFlags) (*airflowapi.Client, error) {
	sel, err := c.resolveDeployment(f)
	if err != nil {
		return nil, err
	}
	domain, err := c.projectDomain()
	if err != nil {
		return nil, err
	}
	return c.clientFor(ctx, sel.Instance, domain)
}

// projectDomain is the Astro host whose login this project's astro links use,
// or empty for the current context's: outside a project, and in one that
// names no host. A manifest that is there and does not load is an error, not
// "names no host": read that way, a project whose links live on another host
// would authenticate against the current login's.
func (c *cli) projectDomain() (string, error) {
	dir, err := c.projectPath()
	if err != nil {
		return "", nil //nolint:nilerr // outside a project there is no host to name, which is not a failure
	}
	m, err := manifest.Load(filepath.Join(dir, project.Marker))
	switch {
	case errors.Is(err, manifest.ErrNotFound), errors.Is(err, manifest.ErrNoAstroSection):
		return "", nil
	case err != nil:
		return "", err
	}
	return m.Astro.LoginDomain(), nil
}

// machineClient opens a client on the Airflow this project has running, with
// nothing to resolve.
func (c *cli) machineClient(ctx context.Context) (*airflowapi.Client, error) {
	i, err := c.machineInstance()
	if err != nil {
		return nil, err
	}
	return c.clientFor(ctx, i, "")
}

// clientFor is the one step that reaches the network, and the one place the
// target is announced — so whatever picked it, the Airflow a command is about
// to act on is never invisible, and stdout stays clean for json. domain is the
// Astro host whose login an astro link proves itself with.
func (c *cli) clientFor(ctx context.Context, i instances.Instance, domain string) (*airflowapi.Client, error) {
	c.announceInstance(i)
	transport, err := i.Transport(ctx, c.instanceDeps(domain))
	if err != nil {
		return nil, err
	}
	if i.Kind == instances.KindAstro {
		c.outage = &outageWatch{next: transport, instance: i, domain: domain}
		transport = c.outage
	}
	return airflowapi.New(transport), nil
}

// errNoLocalAirflow reports an `astro local` query with nothing running. The
// fix is to start Airflow, never to point somewhere else — that is what the
// top-level spelling is for.
var errNoLocalAirflow = fmt.Errorf(
	"%w — start one with `astro local start`", localrt.ErrNotRunning)

// machineInstance is the Airflow every `astro local` query command acts on: the
// one this project has running right now. It reads the project's own runtime
// record, the same one `astro local status` reports, so the two can never
// disagree about whether Airflow is up. No flag, env var, or pin can move it,
// which is exactly why the machine has its own spelling.
func (c *cli) machineInstance() (instances.Instance, error) {
	status, err := c.readStatus()
	if err != nil {
		return instances.Instance{}, err
	}
	if status.State != localrt.StateRunning {
		return instances.Instance{}, errNoLocalAirflow
	}
	return instances.LocalInstance(instances.Local{
		ProjectPath:  status.ProjectPath,
		Port:         status.Port,
		AirflowMajor: status.AirflowMajor,
		Mode:         string(status.Mode),
	}, instances.LocalName), nil
}

// instanceDeps hands resolution what it needs from the process: the login and
// the coordinate lookups, the two reads that touch config/ and the cloud
// clients this tree cannot import, both for domain. Everything else it asks
// for itself.
//
// The Google chain rides along when the lookup exposes one, so a Composer link
// resolves its URL and proves itself to the Airflow behind it through the same
// credentials. Two chains would mean a run that finds an environment it cannot
// then talk to.
func (c *cli) instanceDeps(domain string) instances.Deps {
	var session func(ctx context.Context) (string, error)
	if c.d.Session != nil {
		session = func(ctx context.Context) (string, error) { return c.d.Session(ctx, domain) }
	}
	var locator instances.Locator
	if c.d.Locator != nil {
		locator = c.d.Locator(domain)
	}
	var google googleauth.Options
	if chain, ok := locator.(instancelocate.GoogleChain); ok {
		google.Token, google.Account = chain.Google()
	}
	return instances.Deps{
		Session: session,
		Locator: locator,
		Providers: instances.Providers{
			manifest.AuthGoogle: googleauth.Provider(google),
			manifest.AuthAWS:    awsauth.Provider(awsauth.Options{}),
		},
	}
}

// resolveDeployment applies the rule, and when nothing selects it asks — once —
// and pins the answer, so nobody is asked twice. A run that cannot ask fails
// with the message naming every way to say which deployment; the caller adds
// the `astro local` spelling that needs none.
func (c *cli) resolveDeployment(f deploymentFlags) (instances.Selection, error) {
	if f.deployment != "" && f.url != "" {
		return instances.Selection{}, instances.ErrMutuallyExclusive
	}
	// --url is the no-project escape hatch, so it is answered before anything
	// looks for a project: `astro af dags list --url https://airflow.corp.dev` has
	// to work from any directory on the machine.
	if f.url != "" {
		return instances.Selection{Instance: instances.URLInstance(f.url), From: instances.LayerURL}, nil
	}
	dir, set, err := c.deploymentSet()
	if err != nil {
		return instances.Selection{}, err
	}
	req, err := deploymentRequest(dir)
	if err != nil {
		return instances.Selection{}, err
	}
	req.Flag = f.deployment
	sel, err := set.Select(req)
	if err != nil {
		var ambiguous *instances.AmbiguousError
		if !errors.As(err, &ambiguous) || !c.mayPrompt() {
			return instances.Selection{}, err
		}
		return c.pickAndPin(dir, set, ambiguous.Choices)
	}
	return sel, nil
}

// mayPrompt reports whether this run may ask a question: someone has to be
// there to answer, and json output has to stay a stream a program can parse —
// a prompt on stderr with the run blocked on stdin is not that.
func (c *cli) mayPrompt() bool {
	return c.interactive() && c.output != string(FormatJSON)
}

// pickAndPin asks which deployment to use and writes the answer to the pin, so
// the question is asked once and never again for this project.
func (c *cli) pickAndPin(projectDir string, set instances.Set, choices []string) (instances.Selection, error) {
	name, err := c.promptForDeployment(choices)
	if err != nil {
		return instances.Selection{}, err
	}
	if err := savePin(projectDir, name); err != nil {
		return instances.Selection{}, err
	}
	fmt.Fprintf(c.d.Stderr, "picked %s — pinned for this project (astro use --unset to clear)\n", name)
	return set.Select(instances.Request{Pin: name})
}

// announceInstance prints the one line every command puts on stderr before it
// acts. The parenthetical carries only what the name does not: a local Airflow
// and a --url target are already their own explanation.
func (c *cli) announceInstance(i instances.Instance) {
	switch {
	case i.Where == "" || i.Name == i.Where:
		fmt.Fprintf(c.d.Stderr, "→ %s\n", i.Name)
	case i.Kind == instances.KindLocal:
		fmt.Fprintf(c.d.Stderr, "→ %s (%s)\n", i.Name, i.Where)
	default:
		fmt.Fprintf(c.d.Stderr, "→ %s (%s %s)\n", i.Name, i.Kind, i.Where)
	}
}

// promptForDeployment asks which deployment to use and returns the answer. It
// takes a number or a name and nothing else: there is no default on Enter,
// because the safe answer to "which Airflow should I act on" is never one this
// picked for you. It is only ever reached on an interactive run.
func (c *cli) promptForDeployment(choices []string) (string, error) {
	fmt.Fprintln(c.d.Stderr, "Which deployment should this project use?")
	for i, name := range choices {
		fmt.Fprintf(c.d.Stderr, "  %d) %s\n", i+1, name)
	}
	in := bufio.NewReader(c.d.Stdin)
	for attempt := 0; attempt < promptAttempts; attempt++ {
		fmt.Fprintf(c.d.Stderr, "Choose 1-%d: ", len(choices))
		line, err := in.ReadString('\n')
		answer := strings.TrimSpace(line)
		if err != nil && answer == "" {
			if errors.Is(err, io.EOF) {
				return "", &instances.AmbiguousError{Choices: choices}
			}
			return "", err
		}
		if name, ok := matchChoice(choices, answer); ok {
			return name, nil
		}
		fmt.Fprintf(c.d.Stderr, "Not one of the choices. ")
	}
	return "", &instances.AmbiguousError{Choices: choices}
}

// promptAttempts bounds the re-asking, so a stdin that answers but never
// answers usefully ends with the message naming the flags rather than looping.
const promptAttempts = 3

// matchChoice reads an answer as a name or a number, names first. A deployment
// may legally be called "2", and reading the number first would pin whichever
// entry happened to sit in that position instead. What the user typed is what
// they meant; the numbers are only a shorthand for names nobody wants to retype.
func matchChoice(choices []string, answer string) (string, bool) {
	for _, name := range choices {
		if name == answer {
			return name, true
		}
	}
	if n, err := strconv.Atoi(answer); err == nil && n >= 1 && n <= len(choices) {
		return choices[n-1], true
	}
	return "", false
}

// interactive reports whether this run can ask a question: someone has to be at
// a terminal to answer it. A piped or redirected stdin is a script, and a
// script must decide with -d, ASTRO_DEPLOYMENT, or a pin rather than be asked.
func (c *cli) interactive() bool {
	if c.d.Interactive != nil {
		return c.d.Interactive()
	}
	return false
}
