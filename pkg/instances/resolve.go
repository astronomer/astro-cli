package instances

import (
	"errors"
	"fmt"
	"strings"

	"github.com/astronomer/astro-cli/pkg/manifest"
)

// Layer is one level of the resolution rule, in precedence order:
//
//	--deployment/-d  >  ASTRO_DEPLOYMENT  >  pin  >  default link
//
// --url sits alongside --deployment as the stateless escape hatch for an
// Airflow no project declares. The machine's own Airflow is on none of these
// layers: it is reached by spelling the command `astro local …`.
type Layer string

const (
	LayerFlag    Layer = "flag"
	LayerURL     Layer = "url"
	LayerEnv     Layer = "env"
	LayerPin     Layer = "pin"
	LayerDefault Layer = "default"
)

// Request is what the command layer knows about this invocation: the flags it
// parsed, the env var it read, and the pin it loaded.
type Request struct {
	// Flag is -d/--deployment.
	Flag string
	// URL is --url, the stateless target. It is mutually exclusive with Flag.
	URL string
	// Env is the value of ASTRO_DEPLOYMENT.
	Env string
	// Pin is the project's userstate pin, written by `astro use`.
	Pin string
}

// Selection is the resolved instance and the layer that chose it.
type Selection struct {
	Instance Instance
	From     Layer
}

// ErrMutuallyExclusive reports --deployment and --url together: one names
// something the project declares, the other deliberately declares nothing, and
// obeying both is impossible.
var ErrMutuallyExclusive = errors.New("--deployment and --url cannot be used together: --deployment names a deployment this project links, --url targets an Airflow with no name at all")

// localSpelling closes the failures a project with no links can reach. The
// whole point of the split is that the machine is spelled differently rather
// than waiting quietly at the bottom of the rule, so a run that finds no
// deployment must not leave the reader thinking there is nothing to talk to.
// The command layer adds the family's own `astro local` form on top.
const localSpelling = "The Airflow on this machine is not a deployment and never resolves here; it has its own commands, under astro local."

// ErrNone reports a project with no deployment to act on.
var ErrNone = errors.New("no deployment to act on: this project links none ([tool.astro.deployments] in pyproject.toml). " +
	"To reach an Airflow no project declares, pass --url. " + localSpelling)

// ErrLocalNotADeployment reports `local` used where a deployment name belongs —
// a flag, the env var, or a pin left by an older release. The name is reserved
// rather than unknown, so the message says where the machine went instead of
// listing deployments it is not one of.
var ErrLocalNotADeployment = errors.New(LocalName + " is not a deployment: it is this machine, and it has its own commands — " +
	"astro local start runs it, astro local af dags list and astro local af health read it. " +
	"astro use selects among deployments only")

// UnknownError reports a name no link declares.
type UnknownError struct {
	// Layer is where the name came from, so the message points at the thing to
	// fix — a flag, an exported variable, or a stale pin.
	Layer Layer
	Name  string
	Known []string
}

func (e *UnknownError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "no deployment named %q", e.Name)
	switch e.Layer {
	case LayerEnv:
		fmt.Fprintf(&b, " (from %s)", EnvVar)
	case LayerPin:
		b.WriteString(" (selected with astro use; clear it with astro use --unset)")
	case LayerFlag, LayerURL, LayerDefault:
	}
	if len(e.Known) == 0 {
		b.WriteString("; this project links none — declare one under [tool.astro.deployments] in pyproject.toml. " + localSpelling)
		return b.String()
	}
	b.WriteString("; known deployments: " + strings.Join(e.Known, ", "))
	return b.String()
}

// AmbiguousError reports resolution falling all the way through with several
// deployments available and none of them the default. An interactive command
// prompts on this and pins the answer; a non-interactive one returns it, and
// the message names every way to say which one.
type AmbiguousError struct {
	Choices []string
}

func (e *AmbiguousError) Error() string {
	return fmt.Sprintf("several deployments are linked and none is the default (%s): pick one with -d <name>, export %s=<name>, or select one with astro use <name>",
		strings.Join(e.Choices, ", "), EnvVar)
}

// Unknown is the error for a name this set does not hold, named at layer. It is
// one call rather than a constructor per caller so `astro use` and the
// resolution rule refuse the same names the same way — the reserved `local`
// above all, which is a name with a new home rather than a typo.
func (s Set) Unknown(layer Layer, name string) error {
	if name != LocalName {
		return &UnknownError{Layer: layer, Name: name, Known: s.Names()}
	}
	if layer == LayerPin {
		// A pin an older release wrote fails every command until it is cleared,
		// so the way out has to travel with the refusal.
		return fmt.Errorf("%w (clear it with astro use --unset)", ErrLocalNotADeployment)
	}
	return ErrLocalNotADeployment
}

// Select applies the resolution rule. It reads only the set and the request —
// no network, no files — so a command can render the answer, or the reason
// there is none, before it does anything.
func (s Set) Select(req Request) (Selection, error) {
	if req.Flag != "" && req.URL != "" {
		return Selection{}, ErrMutuallyExclusive
	}
	if req.URL != "" {
		return Selection{Instance: URLInstance(req.URL), From: LayerURL}, nil
	}
	for _, named := range []struct {
		layer Layer
		name  string
	}{
		{LayerFlag, req.Flag},
		{LayerEnv, req.Env},
		{LayerPin, req.Pin},
	} {
		if named.name == "" {
			continue
		}
		it, ok := s.Lookup(named.name)
		if !ok && named.layer == LayerFlag && named.name != LocalName {
			it, ok = s.DeploymentByID(named.name), true
		}
		if !ok {
			return Selection{}, s.Unknown(named.layer, named.name)
		}
		return Selection{Instance: it, From: named.layer}, nil
	}
	// Nothing was said, so the manifest answers.
	if it, ok := s.defaultInstance(); ok {
		return Selection{Instance: it, From: LayerDefault}, nil
	}
	switch names := s.Names(); len(names) {
	case 0:
		return Selection{}, ErrNone
	case 1:
		// One link and no default marked. Today the lone-link rule in
		// manifest.DefaultLink already catches this, so nothing reaches here —
		// but a set of one is never a choice, and it must not be announced as
		// "several".
		return Selection{Instance: s.items[0], From: LayerDefault}, nil
	default:
		return Selection{}, &AmbiguousError{Choices: names}
	}
}

// defaultInstance is the manifest's default link as an instance. The rule
// itself lives in manifest.DefaultLink, which `astro deploy` calls too; Build
// applies it, so all this does is look up the name it settled on.
func (s Set) defaultInstance() (Instance, bool) {
	if s.defaultLink == "" {
		return Instance{}, false
	}
	return s.Lookup(s.defaultLink)
}

// URLInstance is the instance behind --url: an endpoint no project declares,
// named by its own URL because there is no other name to call it. Its auth
// comes from the environment (see EnvToken, EnvUsername, EnvPassword), since a
// bare URL says nothing about how its Airflow checks callers.
func URLInstance(url string) Instance {
	return Instance{Name: url, Kind: KindEndpoint, Source: SourceURL, URL: url}
}

// DeploymentByID reads id as an Astro Deployment id, the way `astro deploy
// --deployment` reads a name no link carries. A link pointing at that
// Deployment answers for it, so its name and auth apply; otherwise the
// instance is the Deployment alone, reached with the Astro login.
func (s Set) DeploymentByID(id string) Instance {
	for i := range s.items {
		if s.items[i].Kind == KindAstro && s.items[i].Link.Deployment == id {
			return s.items[i]
		}
	}
	link := manifest.Link{Deployment: id, Auth: manifest.Auth{Method: manifest.AuthAstro}}
	return Instance{Name: id, Kind: KindAstro, Source: SourceDeploymentID, Where: linkWhere(link), Link: link}
}
