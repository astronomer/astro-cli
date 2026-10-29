package local

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/deploy"
	"github.com/astronomer/astro-cli/internal/project"
	"github.com/astronomer/astro-cli/internal/userstate"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/picker"
	"github.com/astronomer/astro-cli/pkg/scaffold"
)

// `astro link` writes the project's links into its pyproject.toml: the
// deployment links under [tool.astro.deployments], and the workspace link,
// [tool.astro] workspace and domain. Every write goes through pkg/scaffold's
// link writer, which Astro Desktop's link editor uses too, so both tools
// follow the same rules about what a link inherits and keep the file loadable.
//
// It sits beside `astro use` rather than under `astro local`: `astro local` is
// the Airflow on this machine, and a link is never that. `astro use` reads the
// same links, pins one per user, and lists them all, so the pair covers the
// inventory: link writes what is committed, use picks from it.
//
// Nothing here reads a credential. A link names the env vars its credentials
// come from, and the output repeats those names only.

// The flag names `astro link add` registers.
const (
	flagLinkDeployment      = "deployment"
	flagLinkWorkspace       = "workspace"
	flagLinkTarget          = "target"
	flagLinkEnvironment     = "environment"
	flagLinkURL             = "url"
	flagLinkProject         = "project"
	flagLinkLocation        = "location"
	flagLinkRegion          = "region"
	flagLinkAuth            = "auth"
	flagLinkTokenEnv        = "token-env"
	flagLinkUsernameEnv     = "username-env"
	flagLinkPasswordEnv     = "password-env"
	flagLinkClientIDEnv     = "client-id-env"
	flagLinkClientSecretEnv = "client-secret-env"
	flagLinkReplace         = "replace"
	flagLinkUnset           = "unset"
)

// The status values of a linkResult.
const (
	linkStatusAdded     = "added"
	linkStatusReplaced  = "replaced"
	linkStatusRemoved   = "removed"
	linkStatusDefault   = "default"
	linkStatusCleared   = "cleared"
	linkStatusUnchanged = "unchanged"
)

// linkResult is what add, remove and default report, the same value in text
// and json. Name and Kind are empty for `default --unset`, which names no link.
type linkResult struct {
	Name     string `json:"name,omitempty"`
	Kind     string `json:"kind,omitempty"`
	Status   string `json:"status"`
	Manifest string `json:"manifest"`
	// KeptDefault names the link an add marked default = true, because it was
	// the project's only link and so its default; omitted when none was.
	KeptDefault string `json:"kept_default,omitempty"`
}

// NewLinkCmd builds `astro link` for the root, wired with its own deps.
func NewLinkCmd(d Deps) *cobra.Command {
	c := &cli{d: d}
	cmd := &cobra.Command{
		Use:   "link",
		Short: "Link Deployments to this project",
		Long: "Link the Deployments this project works with. Links are " +
			"saved in the project, so everyone who clones it gets them. astro use lists them.",
		Example: "  astro link add                              # pick a Deployment to link\n" +
			"  astro link add prod --deployment clx123abc\n" +
			"  astro link default prod",
		Args: cobra.NoArgs,
	}
	cmd.AddCommand(newLinkAddCmd(c), newLinkRemoveCmd(c), newLinkDefaultCmd(c))
	cliout.AddOutputFlag(cmd, &c.output)
	markSkipPreRun(cmd)
	return cmd
}

// linkAddInput holds add's flags.
type linkAddInput struct {
	deployment, workspace, target, environment, url string
	project, location, region                       string
	auth                                            string
	tokenEnv, usernameEnv, passwordEnv              string
	clientIDEnv, clientSecretEnv                    string
	replace                                         bool
}

func newLinkAddCmd(c *cli) *cobra.Command {
	in := &linkAddInput{}
	cmd := &cobra.Command{
		Use:   "add [NAME] [-- COMMAND...]",
		Short: "Link a Deployment to this project",
		Long: "Link a Deployment to this project as NAME. In a terminal, with no --deployment, it asks " +
			"which Astro Deployment to link and, with no NAME, names the link after it: lowercased, with " +
			"each run of spaces and punctuation turned into one dash.\n\n" +
			"Under an Astro Private Cloud context it links that platform's Deployments instead, on the " +
			"context's domain, in --workspace or the context's workspace.\n\n" +
			"MWAA, Composer and other Airflows are linked with flags. Credentials are never saved: the " +
			"*-env flags name the environment variables they are read from.",
		Example: "  astro link add\n" +
			"  astro link add prod --deployment clx123abc\n" +
			"  astro link add orders --target mwaa --environment orders-prod --region eu-west-1\n" +
			"  astro link add gcp --target composer --environment orders --project acme-data \\\n" +
			"    --location us-central1\n" +
			"  astro link add staging --url https://airflow.example.com --auth token --token-env AIRFLOW_TOKEN\n" +
			"  astro link add custom --url https://airflow.example.com \\\n" +
			"    --auth exec -- my-token-tool --profile prod",
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			name, command := "", []string(nil)
			switch dash := cmd.ArgsLenAtDash(); {
			case dash > 1:
				return errors.New("add takes one NAME before --: the rest is the exec command")
			case dash >= 0:
				command = args[dash:]
				if dash == 1 {
					name = args[0]
				}
			case len(args) > 1:
				return fmt.Errorf("add takes one NAME, not %d: put an exec command after --", len(args))
			case len(args) == 1:
				name = args[0]
			}
			return c.runLinkAdd(cmd, in, name, command)
		},
	}
	f := cmd.Flags()
	f.StringVar(&in.deployment, flagLinkDeployment, "", "Deployment ID (Astro's, or Astro Private Cloud's under an APC context)")
	f.StringVar(&in.workspace, flagLinkWorkspace, "", "Workspace the Deployment is in (default: this project's, or the APC context's)")
	f.StringVar(&in.target, flagLinkTarget, "", "Platform: astro, mwaa, composer or apc (default: this project's, else the context's platform)")
	f.StringVar(&in.environment, flagLinkEnvironment, "", "MWAA or Composer environment name")
	f.StringVar(&in.url, flagLinkURL, "", "Airflow URL, for an Airflow no platform can look up")
	f.StringVar(&in.project, flagLinkProject, "", "Google Cloud project of the Composer environment (default: the one already linked)")
	f.StringVar(&in.location, flagLinkLocation, "", "Region of the Composer environment (default: the one already linked)")
	f.StringVar(&in.region, flagLinkRegion, "", "AWS region of the MWAA environment; empty clears it")
	f.StringVar(&in.auth, flagLinkAuth, "", "How to authenticate: astro, aws, google, basic, token, airflow-token, exec or none")
	f.StringVar(&in.tokenEnv, flagLinkTokenEnv, "", "Env var holding the token (token)")
	f.StringVar(&in.usernameEnv, flagLinkUsernameEnv, "", "Env var holding the username (basic, airflow-token)")
	f.StringVar(&in.passwordEnv, flagLinkPasswordEnv, "", "Env var holding the password (basic, airflow-token)")
	f.StringVar(&in.clientIDEnv, flagLinkClientIDEnv, "", "Env var holding the client ID (airflow-token)")
	f.StringVar(&in.clientSecretEnv, flagLinkClientSecretEnv, "", "Env var holding the client secret (airflow-token)")
	f.BoolVar(&in.replace, flagLinkReplace, false, "Replace a link with the same name")
	return cmd
}

func newLinkRemoveCmd(c *cli) *cobra.Command {
	return &cobra.Command{
		Use:     "remove [NAME]",
		Short:   "Unlink a Deployment from this project",
		Long:    "Unlink a Deployment from this project. In a terminal, with no NAME, it asks which one.",
		Example: "  astro link remove prod",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if len(args) == 1 {
				return c.runLinkRemove(args[0])
			}
			name, err := c.pickLink("Select a link to remove", "", "", "", "name the link to remove: astro link remove NAME")
			if err != nil {
				return err
			}
			return c.runLinkRemove(name)
		},
	}
}

func newLinkDefaultCmd(c *cli) *cobra.Command {
	var unset bool
	cmd := &cobra.Command{
		Use:   "default [NAME]",
		Short: "Choose the Deployment commands use by default",
		Long: "Choose the linked Deployment commands use when you don't name one. astro deploy still " +
			"asks, with it preselected. In a terminal, with no NAME, it asks which one.",
		Example: "  astro link default prod\n" +
			"  astro link default --unset",
		Args: cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			switch {
			case unset && len(args) > 0:
				return errors.New("--unset takes no link name: it clears the default")
			case unset:
				return c.runLinkDefault("")
			case len(args) == 1:
				return c.runLinkDefault(args[0])
			}
			name, err := c.pickLink("Select the default link", "clear the default", "", "", "name the link to make the default, or pass --unset to clear it")
			if err != nil {
				return err
			}
			return c.runLinkDefault(name)
		},
	}
	cmd.Flags().BoolVar(&unset, flagLinkUnset, false, "Clear the default")
	return cmd
}

// pickLink asks which of the project's links a command acts on, with the table
// picker the Deployment picker uses, or fails with missing when this run cannot
// be asked. A non-empty none adds a last row, described by it, that clears the
// way --unset does, and picking it returns "". current, when it names a link,
// is drawn bold green, as the workspace and organization pickers draw theirs,
// so the reader sees what they are choosing away from. A non-empty mark goes
// in a last column on that row, for when the reader also needs telling why it
// is current. An answer that is not a row number is refused, as the Deployment
// picker refuses one.
func (c *cli) pickLink(title, none, current, mark, missing string) (string, error) {
	if !c.mayPrompt() {
		return "", input.Required(errors.New(missing))
	}
	_, _, m, err := c.linkProject()
	if err != nil {
		return "", err
	}
	names := make([]string, 0, len(m.Astro.Deployments))
	for name := range m.Astro.Deployments {
		names = append(names, name)
	}
	if len(names) == 0 {
		return "", errors.New("this project links no Deployments yet: link one with astro link add")
	}
	slices.Sort(names)
	list := picker.List{
		Title:   title,
		Header:  []string{"NAME", "KIND", "DEPLOYMENT, ENVIRONMENT OR URL"},
		Ask:     []input.Option{input.About("a link")},
		Invalid: errInvalidLinkSelection,
	}
	// The mark column exists only when some row carries one; otherwise every
	// row would end in blank padding.
	defaultName, _, _ := manifest.DefaultLink(m.Astro.Deployments)
	if !slices.Contains(names, current) {
		mark = ""
	}
	marks := linkMarks(defaultName, current, mark)
	marked := len(marks) > 0
	if marked {
		list.Header = append(list.Header, "")
	}
	row := func(cells ...string) []string {
		if marked {
			cells = append(cells, "")
		}
		return cells
	}
	for _, name := range names {
		l := m.Astro.Deployments[name]
		cells := row(name, string(l.Kind()), linkWhere(&l))
		if marked {
			cells[len(cells)-1] = marks[name]
		}
		list.AddRow(name == current, cells...)
	}
	if none != "" {
		list.AddRow(false, row("none", "", none)...)
	}
	i, err := list.Pick(c.d.Stdout, c.d.Stdin)
	switch {
	case err != nil:
		return "", err
	case i == len(names):
		return "", nil
	}
	return names[i], nil
}

// linkMarks is each link's mark in a list of the project's links, by name, in
// the words the deploy prompt uses: "default" (deploy.DefaultMark) on the
// project's default link, and the caller's currentMark on the current row.
// When both fall on one row they are joined — "default, from ASTRO_DEPLOYMENT"
// — so neither fact hides the other. The `astro link` pickers, the `astro use`
// picker and its listing all mark through here.
func linkMarks(defaultName, current, currentMark string) map[string]string {
	marks := map[string]string{}
	if defaultName != "" {
		marks[defaultName] = deploy.DefaultMark
	}
	if current != "" && currentMark != "" {
		if prev := marks[current]; prev != "" {
			currentMark = prev + ", " + currentMark
		}
		marks[current] = currentMark
	}
	return marks
}

// errInvalidLinkSelection is the link pickers' answer to a choice that is not a
// row number, worded as the Deployment picker words its own.
var errInvalidLinkSelection = errors.New("invalid link selected")

// linkWhere is what a link points at, for the picker's last column.
func linkWhere(l *manifest.Link) string {
	switch l.Kind() {
	case manifest.KindAstro:
		return l.Deployment
	case manifest.KindAPC:
		return l.Deployment + " on " + l.Domain
	case manifest.KindMWAA, manifest.KindComposer:
		return l.Environment
	case manifest.KindEndpoint:
	}
	return l.URL
}

// linkProject is the project a link edit writes to, its manifest path, and the
// manifest as it reads now, which the checks before a write consult. The
// writer reads the file again under its own rules; this read only shapes the
// request and the messages.
func (c *cli) linkProject() (dir, path string, m *manifest.Manifest, err error) {
	dir, err = c.projectPath()
	if err != nil {
		return "", "", nil, err
	}
	path = filepath.Join(dir, project.Marker)
	m, err = manifest.Load(path)
	if err != nil {
		return "", "", nil, err
	}
	return dir, path, m, nil
}

func (c *cli) runLinkAdd(cmd *cobra.Command, in *linkAddInput, name string, command []string) error {
	r := c.renderer()
	dir, path, m, err := c.linkProject()
	if err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	// bundle is astro deploy bundle, so a new link by that name could not be
	// deployed to by name: refused before anything is asked. One that exists
	// is the --replace check's.
	if _, exists := m.Astro.Deployments[name]; name == scaffold.BundleLinkName && !exists {
		return cliout.Usage(scaffold.ErrBundleLinkName)
	}
	// A NAME already linked is refused before anything asks a question or
	// calls the platform.
	if err := refuseExisting(m, name, in.replace); err != nil {
		return err
	}
	link, err := in.link(cmd, m, name, command, c.d.APC)
	if err != nil {
		return err
	}
	picking := !cmd.Flags().Changed(flagLinkDeployment) && c.mayPrompt()
	switch {
	case link.Kind == manifest.KindAstro && picking && c.d.PickDeployment != nil:
		if name, err = c.pickAstroDeployment(&link, m, name); err != nil {
			return err
		}
	case link.Kind == manifest.KindAPC && picking && c.d.APC.PickDeployment != nil:
		if name, err = c.pickAPCDeployment(&link, m, name); err != nil {
			return err
		}
	case link.Kind == manifest.KindAPC && link.Deployment != "":
		// With no Deployment the save refuses the link for that first, so the
		// workspace is not asked about.
		if err := c.apcWorkspace(&link); err != nil {
			return err
		}
	}
	if name == "" {
		return errors.New("add needs a NAME for the link. In a terminal, with no --deployment, it asks for a Deployment and names the link after it")
	}
	link.Name = name
	if err := refuseExisting(m, name, in.replace); err != nil {
		return err
	}
	_, exists := m.Astro.Deployments[name]
	if err := scaffold.SaveLink(dir, nil, link); err != nil {
		return err
	}
	res := linkResult{Name: name, Kind: string(link.Kind), Status: linkStatusAdded, Manifest: path}
	if exists {
		res.Status = linkStatusReplaced
	}
	// A save beside a project's only link marks that one default = true. Read
	// back rather than predicted, so what is reported is what was written.
	if after, err := manifest.Load(path); err == nil {
		res.KeptDefault = scaffold.NewlyDefault(m, after)
	}
	return r.Emit(res, func(w io.Writer) error {
		verb := "added link %s to %s\n"
		if exists {
			verb = "replaced link %s in %s\n"
		}
		if _, werr := fmt.Fprintf(w, verb, name, path); werr != nil {
			return werr
		}
		return keptDefaultNote(w, res.KeptDefault)
	})
}

// keptDefaultNote says a save marked the project's former only link default =
// true, so the change to a line the user did not touch is never silent.
func keptDefaultNote(w io.Writer, kept string) error {
	if kept == "" {
		return nil
	}
	_, err := fmt.Fprintf(w, "marked %s default = true: it was the project's only link, so its default, and stays so now there are two (change it with astro link default)\n", kept)
	return err
}

// refuseExisting refuses name when the project already links it and the run
// did not pass --replace. An empty name is not checked: it is not known yet.
func refuseExisting(m *manifest.Manifest, name string, replace bool) error {
	if _, exists := m.Astro.Deployments[name]; name != "" && exists && !replace {
		return fmt.Errorf("a link named %s already exists: pass --%s to overwrite it, or astro link remove %s first", name, flagLinkReplace, name)
	}
	return nil
}

// pickAstroDeployment asks which Astro Deployment the link points at, with the
// picker `astro deploy` uses, and fills link with it. The Deployments listed are
// the workspace's: --workspace, else the project's, else the current login's,
// else one picked from the login's list. It returns the link's name: the one
// given, or else the Deployment's name as scaffold.LinkNameFor spells it.
func (c *cli) pickAstroDeployment(link *scaffold.Link, m *manifest.Manifest, name string) (string, error) {
	ws := firstNonEmpty(link.Workspace, m.Astro.Workspace)
	if ws == "" && c.d.CurrentWorkspace != nil {
		ws = c.d.CurrentWorkspace()
	}
	if ws == "" {
		if c.d.PickWorkspace == nil {
			return "", fmt.Errorf("no workspace to list Deployments from: pass --%s", flagLinkWorkspace)
		}
		picked, err := c.d.PickWorkspace()
		if err != nil {
			return "", err
		}
		ws = picked
	}
	// A Deployment the project already links is not offered: picking it
	// would only be refused as a second link to the same place.
	linked := map[string]string{}
	for other := range m.Astro.Deployments {
		if l := m.Astro.Deployments[other]; l.Kind() == manifest.KindAstro {
			linked[l.Deployment] = other
		}
	}
	dep, err := c.d.PickDeployment(ws, linked)
	if errors.Is(err, ErrAllDeploymentsLinked) {
		return "", allLinked(m, ws)
	}
	if err != nil {
		return "", err
	}
	link.Deployment = dep.ID
	if link.Workspace == "" {
		link.Workspace = firstNonEmpty(dep.WorkspaceID, ws)
	}
	return pickedLinkName(name, dep)
}

// pickedLinkName is the name of the link a picked Deployment gets: the one
// given, or else the Deployment's name as scaffold.LinkNameFor spells it.
func pickedLinkName(name string, dep PickedDeployment) (string, error) {
	if name != "" {
		return name, nil
	}
	name = scaffold.LinkNameFor(dep.Name)
	if name == "" || name == manifest.ReservedLinkName || name == scaffold.BundleLinkName {
		return "", fmt.Errorf("the Deployment's name does not make a link name: pass one, astro link add NAME --%s %s", flagLinkDeployment, dep.ID)
	}
	return name, nil
}

// pickAPCDeployment is pickAstroDeployment for an Astro Private Cloud
// Deployment. The Deployments listed are those of --workspace, else the APC
// context's workspace; [tool.astro] workspace is never asked, being an Astro
// workspace. A Deployment the project already links on this domain is not
// offered.
func (c *cli) pickAPCDeployment(link *scaffold.Link, m *manifest.Manifest, name string) (string, error) {
	ws := link.Workspace
	if ws == "" {
		var err error
		if ws, err = c.apcContextWorkspace(); err != nil {
			return "", err
		}
	}
	linked := map[string]string{}
	for other := range m.Astro.Deployments {
		if l := m.Astro.Deployments[other]; l.Kind() == manifest.KindAPC && l.Domain == link.Domain {
			linked[l.Deployment] = other
		}
	}
	dep, err := c.d.APC.PickDeployment(ws, linked)
	if errors.Is(err, ErrAllDeploymentsLinked) {
		return "", allLinked(m, ws)
	}
	if err != nil {
		return "", err
	}
	link.Deployment = dep.ID
	link.Workspace = firstNonEmpty(dep.WorkspaceID, ws)
	return pickedLinkName(name, dep)
}

// apcWorkspace fills in the workspace of an apc link that was not picked:
// --workspace, else the APC context's. A replaced link's workspace is never
// carried over: the Deployment named may be in another one.
func (c *cli) apcWorkspace(link *scaffold.Link) error {
	if link.Workspace != "" {
		return nil
	}
	ws, err := c.apcContextWorkspace()
	link.Workspace = ws
	return err
}

// apcContextWorkspace is the APC context's current workspace. None set is
// errNoAPCWorkspace; a context that cannot be read says so in its own words.
func (c *cli) apcContextWorkspace() (string, error) {
	if c.d.APC == nil || c.d.APC.CurrentWorkspace == nil {
		return "", errNoAPCWorkspace
	}
	ws, err := c.d.APC.CurrentWorkspace()
	switch {
	case err != nil:
		// Never a bare context error, whose text is about Astro.
		return "", APCWorkspaceUnreadable(err)
	case ws == "":
		return "", errNoAPCWorkspace
	}
	return ws, nil
}

// errNoAPCWorkspace is an apc link with no workspace to name: none given and
// none current. [tool.astro] workspace does not count, being Astro's.
var errNoAPCWorkspace = fmt.Errorf("no Astro Private Cloud workspace to link from: pass --%s, or choose one with astro workspace switch", flagLinkWorkspace)

// allLinked says every Deployment in workspace ws is already linked, names the
// links that point into it, and says how to point one somewhere else.
func allLinked(m *manifest.Manifest, ws string) error {
	var names []string
	for name := range m.Astro.Deployments {
		l := m.Astro.Deployments[name]
		if kind := l.Kind(); (kind == manifest.KindAstro || kind == manifest.KindAPC) && l.Workspace == ws {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return fmt.Errorf("every Deployment in workspace %s is already linked, as %s. To relink one, run astro link add NAME --%s <id> --%s",
		ws, strings.Join(names, ", "), flagLinkDeployment, flagLinkReplace)
}

// link turns add's flags into the link the writer saves, refusing a flag that
// does not belong to the kind the others pick. The writer ignores a coordinate
// its kind does not use, so without this a mistyped command would save a link
// quietly missing what was asked for.
//
// apc is the Astro Private Cloud side, nil under an Astro context: it makes
// apc the kind a link takes by default, and names the domain one is on.
func (in *linkAddInput) link(cmd *cobra.Command, m *manifest.Manifest, name string, command []string, apc *APCLinking) (scaffold.Link, error) {
	given := cmd.Flags().Changed
	kind, err := in.kind(given, m, apc != nil)
	if err != nil {
		return scaffold.Link{}, err
	}
	allowed := map[manifest.LinkKind][]string{
		manifest.KindAstro:    {flagLinkDeployment, flagLinkWorkspace},
		manifest.KindAPC:      {flagLinkDeployment, flagLinkWorkspace},
		manifest.KindMWAA:     {flagLinkEnvironment, flagLinkRegion},
		manifest.KindComposer: {flagLinkEnvironment, flagLinkProject, flagLinkLocation},
		manifest.KindEndpoint: {flagLinkURL},
	}[kind]
	for _, f := range []string{flagLinkDeployment, flagLinkWorkspace, flagLinkEnvironment, flagLinkRegion, flagLinkProject, flagLinkLocation, flagLinkURL} {
		if given(f) && !slices.Contains(allowed, f) {
			return scaffold.Link{}, fmt.Errorf("--%s does not apply to %s link", f, manifest.KindWithArticle(kind))
		}
	}
	if len(command) > 0 && in.auth != string(manifest.AuthExec) {
		return scaffold.Link{}, fmt.Errorf("a command after -- is for --%s exec", flagLinkAuth)
	}
	// The writer keeps only the fields the method takes, so a credential flag
	// for another method, or for none, would be dropped from a link reported
	// as added.
	takes := credentialFlags[manifest.AuthMethod(in.auth)]
	for _, f := range []string{flagLinkTokenEnv, flagLinkUsernameEnv, flagLinkPasswordEnv, flagLinkClientIDEnv, flagLinkClientSecretEnv} {
		switch {
		case !given(f) || slices.Contains(takes, f):
		case in.auth == "":
			return scaffold.Link{}, fmt.Errorf("--%s needs --%s naming the method that reads it", f, flagLinkAuth)
		default:
			return scaffold.Link{}, fmt.Errorf("--%s does not apply to the %s auth method", f, in.auth)
		}
	}
	link := scaffold.Link{
		Name:        name,
		Kind:        kind,
		Workspace:   in.workspace,
		Deployment:  in.deployment,
		Environment: in.environment,
		URL:         in.url,
		Auth: manifest.Auth{
			Method:          manifest.AuthMethod(in.auth),
			TokenEnv:        in.tokenEnv,
			UsernameEnv:     in.usernameEnv,
			PasswordEnv:     in.passwordEnv,
			ClientIDEnv:     in.clientIDEnv,
			ClientSecretEnv: in.clientSecretEnv,
			Command:         command,
		},
		TargetProject:  in.project,
		TargetLocation: in.location,
		TargetRegion:   in.region,
		// --region '' asks for the shared region to go; the flag left out
		// leaves it alone.
		ClearTargetRegion: given(flagLinkRegion) && strings.TrimSpace(in.region) == "",
	}
	if kind == manifest.KindAPC {
		link.Domain = apc.Domain
	}
	// The Composer coordinates are shared, so a second Composer link names
	// the ones the project already has rather than having to repeat them.
	if kind == manifest.KindComposer {
		section := m.Astro.Targets[string(manifest.KindComposer)]
		if !given(flagLinkProject) {
			link.TargetProject, _ = section["project"].(string)
		}
		if !given(flagLinkLocation) {
			link.TargetLocation, _ = section["location"].(string)
		}
	}
	return link, nil
}

// credentialFlags is the *-env flags each auth method reads, mirroring the
// manifest's fields for it. A method absent from it reads none.
var credentialFlags = map[manifest.AuthMethod][]string{
	manifest.AuthBasic:        {flagLinkUsernameEnv, flagLinkPasswordEnv},
	manifest.AuthToken:        {flagLinkTokenEnv},
	manifest.AuthAirflowToken: {flagLinkClientIDEnv, flagLinkClientSecretEnv, flagLinkUsernameEnv, flagLinkPasswordEnv},
}

// kind is what add's flags point at: a url makes an endpoint; otherwise
// --target, or the target the link would inherit. Under an Astro Private
// Cloud context (onAPC), a link that names no target in a project whose target
// is unset or astro is an apc link: that context's Deployments are the ones
// it lists. A project whose target is mwaa or composer keeps that kind, unless
// --deployment is given: that names a Deployment of the context's platform,
// so the link is apc, as on v2 under Astro it is astro.
func (in *linkAddInput) kind(given func(string) bool, m *manifest.Manifest, onAPC bool) (manifest.LinkKind, error) {
	platform := manifest.KindAstro
	if onAPC {
		platform = manifest.KindAPC
	}
	if given(flagLinkURL) {
		if given(flagLinkTarget) {
			return "", fmt.Errorf("--%s and --%s cannot be combined: a url link points at that url", flagLinkURL, flagLinkTarget)
		}
		return manifest.KindEndpoint, nil
	}
	target := in.target
	if !given(flagLinkTarget) {
		target = m.Astro.Target
		if target == "" || (onAPC && target == string(manifest.KindAstro)) {
			target = string(platform)
		}
		// An environment names an MWAA or Composer environment, so in a
		// project whose target is astro or apc it needs the platform spelled
		// out.
		if given(flagLinkEnvironment) && (target == string(manifest.KindAstro) || target == string(manifest.KindAPC)) {
			return "", fmt.Errorf("--%s names an MWAA or Composer environment: pass --%s mwaa or --%s composer", flagLinkEnvironment, flagLinkTarget, flagLinkTarget)
		}
		if given(flagLinkDeployment) {
			target = string(platform)
		}
	}
	switch manifest.LinkKind(target) {
	case manifest.KindAstro, manifest.KindMWAA, manifest.KindComposer:
		return manifest.LinkKind(target), nil
	case manifest.KindAPC:
		// The domain is the context's, so an apc link is added from there.
		if !onAPC {
			return "", fmt.Errorf("an apc link is added under the Astro Private Cloud context it links: switch to it first (astro context switch DOMAIN), or pass --%s astro", flagLinkTarget)
		}
		return manifest.KindAPC, nil
	case manifest.KindEndpoint:
	}
	return "", fmt.Errorf("--%s must be astro, mwaa, composer or apc, not %q", flagLinkTarget, target)
}

func (c *cli) runLinkRemove(name string) error {
	r := c.renderer()
	dir, path, m, err := c.linkProject()
	if err != nil {
		return err
	}
	kind := ""
	if l, ok := m.Astro.Deployments[name]; ok {
		kind = string(l.Kind())
	}
	removed, err := scaffold.RemoveLink(dir, nil, name)
	if err != nil {
		return err
	}
	if !removed {
		return fmt.Errorf("no link named %s in %s", name, path)
	}
	// The pin is per user and outside the repo, so the writer cannot see it.
	// Left pointing at a link that is gone, it fails every command until
	// cleared, which is better said now than at the next one.
	if state, serr := userstate.Load(dir); serr == nil && state.Instance == name {
		cliout.Note(c.d.Stderr, fmt.Sprintf("astro use still names %s for this project. Clear it with astro use --unset", name))
	}
	res := linkResult{Name: name, Kind: kind, Status: linkStatusRemoved, Manifest: path}
	return r.Emit(res, func(w io.Writer) error {
		_, werr := fmt.Fprintf(w, "removed link %s from %s\n", name, path)
		return werr
	})
}

func (c *cli) runLinkDefault(name string) error {
	r := c.renderer()
	dir, path, m, err := c.linkProject()
	if err != nil {
		return err
	}
	if name != "" {
		if _, ok := m.Astro.Deployments[name]; !ok {
			return fmt.Errorf("no link named %s in %s", name, path)
		}
	}
	changed, err := watchManifest(dir, func(wrap func(run func() error) error) error {
		return scaffold.SetDefaultLink(dir, wrap, name)
	})
	if err != nil {
		return err
	}
	res := linkResult{Name: name, Status: linkStatusDefault, Manifest: path}
	if name != "" {
		res.Kind = string(m.Astro.Deployments[name].Kind())
	} else {
		res.Status = linkStatusCleared
	}
	if !changed {
		res.Status = linkStatusUnchanged
	}
	return r.Emit(res, func(w io.Writer) error {
		var werr error
		switch {
		case name == "" && changed:
			_, werr = fmt.Fprintf(w, "cleared the default link in %s\n", path)
		case name == "":
			_, werr = fmt.Fprintf(w, "no link is marked default in %s\n", path)
		case changed:
			_, werr = fmt.Fprintf(w, "set link %s as the default in %s\n", name, path)
		default:
			_, werr = fmt.Fprintf(w, "%s is already the default link in %s\n", name, path)
		}
		return werr
	})
}
