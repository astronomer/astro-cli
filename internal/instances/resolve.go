package instances

import (
	"errors"
	"fmt"
	"strings"
)

// Layer is one level of the resolution rule, in precedence order:
//
//	--instance/-i  >  ASTRO_INSTANCE  >  pin  >  running local  >  default link
//
// --url sits alongside --instance as the stateless escape hatch for an Airflow
// no project declares.
type Layer string

const (
	LayerFlag    Layer = "flag"
	LayerURL     Layer = "url"
	LayerEnv     Layer = "env"
	LayerPin     Layer = "pin"
	LayerRunning Layer = "running"
	LayerDefault Layer = "default"
)

// layerLabel is how each layer names itself to a user: the thing they would
// type or change.
var layerLabel = map[Layer]string{
	LayerFlag:    "--instance",
	LayerURL:     "--url",
	LayerEnv:     EnvVar,
	LayerPin:     "astro use",
	LayerRunning: "running local Airflow",
	LayerDefault: "manifest default link",
}

// Label is the user-facing name of a layer, for the resolution table.
func (l Layer) Label() string {
	if s, ok := layerLabel[l]; ok {
		return s
	}
	return string(l)
}

// Request is what the command layer knows about this invocation: the flags it
// parsed, the env var it read, and the pin it loaded.
type Request struct {
	// Flag is -i/--instance.
	Flag string
	// URL is --url, the stateless target. It is mutually exclusive with Flag.
	URL string
	// Env is the value of ASTRO_INSTANCE.
	Env string
	// Pin is the project's userstate pin, written by `astro use`.
	Pin string
}

// Selection is the resolved instance and the layer that chose it.
type Selection struct {
	Instance Instance
	From     Layer
}

// ErrMutuallyExclusive reports --instance and --url together: one names
// something the project declares, the other deliberately declares nothing, and
// obeying both is impossible.
var ErrMutuallyExclusive = errors.New("--instance and --url cannot be used together: --instance names an instance this project knows, --url targets an Airflow with no name at all")

// ErrNone reports a project with nothing of its own to act on.
var ErrNone = errors.New("no Airflow to act on: this project declares no deployment links ([tool.astro.deployments] in pyproject.toml) and no local Airflow is running (`astro local start`). To reach one this project does not know, pass --url")

// NotRunningError reports a name that only this project's own local Airflow
// answers to, when nothing is running. It is its own error because the fix is
// the opposite of the one a stale pin needs: start Airflow, not clear the pin.
type NotRunningError struct {
	// Layer is where the name came from, so the message can say `astro use
	// --unset` only when a pin is what asked.
	Layer Layer
}

func (e *NotRunningError) Error() string {
	msg := "no local Airflow is running for this project — start one with `astro local start`"
	if e.Layer == LayerPin {
		msg += ", or point this project elsewhere with `astro use <name>` (`astro use --unset` clears the pin)"
	}
	return msg
}

// UnknownError reports a name that no link declares and no local Airflow
// answers to.
type UnknownError struct {
	// Layer is where the name came from, so the message points at the thing to
	// fix — a flag, an exported variable, or a stale pin.
	Layer Layer
	Name  string
	Known []string
}

func (e *UnknownError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "no instance named %q", e.Name)
	switch e.Layer {
	case LayerEnv:
		fmt.Fprintf(&b, " (from %s)", EnvVar)
	case LayerPin:
		b.WriteString(" (pinned for this project; clear it with `astro use --unset`)")
	case LayerFlag, LayerURL, LayerRunning, LayerDefault:
	}
	if len(e.Known) == 0 {
		b.WriteString("; this project knows none — declare one under [tool.astro.deployments] in pyproject.toml, or start a local Airflow with `astro local start`")
		return b.String()
	}
	b.WriteString("; known instances: " + strings.Join(e.Known, ", "))
	return b.String()
}

// AmbiguousError reports resolution falling all the way through with several of
// this project's own instances available and none of them the default. An
// interactive command prompts on this and pins the answer; a non-interactive
// one returns it, and the message names all three ways to say which one.
//
// Choices holds this project's own instances only. Another project's running
// Airflow is addressable by name and listed, but offering it here would invite
// someone to make a neighbor's Airflow this project's default.
type AmbiguousError struct {
	Choices []string
}

func (e *AmbiguousError) Error() string {
	return fmt.Sprintf("several instances are available and none is the default (%s): pick one with -i <name>, export %s=<name>, or pin one with `astro use <name>`",
		strings.Join(e.Choices, ", "), EnvVar)
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
		if !ok {
			// The reserved name always means something; it just may not be
			// running yet, which is a different problem with a different fix.
			if named.name == LocalName {
				return Selection{}, &NotRunningError{Layer: named.layer}
			}
			return Selection{}, &UnknownError{Layer: named.layer, Name: named.name, Known: s.Names()}
		}
		return Selection{Instance: it, From: named.layer}, nil
	}
	// Nothing was said, so the machine answers — from this project's own
	// instances only. What is running here beats what the manifest defaults to.
	if it, ok := s.Lookup(LocalName); ok {
		return Selection{Instance: it, From: LayerRunning}, nil
	}
	if it, ok := s.defaultInstance(); ok {
		return Selection{Instance: it, From: LayerDefault}, nil
	}
	own := s.ownNames()
	switch len(own) {
	case 0:
		if names := s.Names(); len(names) > 0 {
			// Other projects' Airflows are running and addressable, but none of
			// them is this project's default.
			return Selection{}, fmt.Errorf("%w. Other projects' Airflows are running (%s); name one with -i if that is what you meant", ErrNone, strings.Join(names, ", "))
		}
		return Selection{}, ErrNone
	case 1:
		// One instance of its own and no default marked. Today the lone-link
		// rule in manifest.DefaultLink already catches this, so nothing reaches
		// here — but a set of one is never a choice, and the day another kind of
		// own instance exists it must not be announced as "several".
		it, _ := s.Lookup(own[0])
		return Selection{Instance: it, From: LayerDefault}, nil
	default:
		return Selection{}, &AmbiguousError{Choices: own}
	}
}

// defaultInstance is the manifest's default link as an instance. The rule
// itself lives in manifest.DefaultLink, which `astro deploy` calls too; Build
// applies it, so all this does is look up the name it settled on. Nothing can
// have taken that name in between: `local` is the only name a discovered
// instance claims outright, and the manifest refuses to declare it.
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

// Row is one layer of the resolution rule and what it currently holds — the
// backing for a bare `astro use`, which shows the whole rule rather than only
// its answer.
type Row struct {
	Layer Layer  `json:"layer"`
	Value string `json:"value,omitempty"`
	// Where is the coordinate the layer's instance points at — detail about
	// something that works.
	Where string `json:"where,omitempty"`
	// Problem is why the layer's value cannot be used, such as a pin naming a
	// link the manifest no longer declares. It is kept apart from Where because
	// one is reassurance and the other is a fault, and a reader scanning a
	// column should not have to tell them apart by wording.
	Problem string `json:"problem,omitempty"`
	// Wins marks the layer that decides the instance right now.
	Wins bool `json:"wins"`
}

// Explain lays out the whole rule: every sticky layer with what it holds, and
// the selection those layers currently produce (or the error saying why they
// produce none). The flag and --url layers are left out — they exist only for
// the command being typed, and this describes the standing state — so the
// caller gets one answer rather than running Select itself and risking a
// second, drifting version of the rule.
func (s Set) Explain(req Request) (rows []Row, sel Selection, err error) {
	sel, err = s.Select(Request{Env: req.Env, Pin: req.Pin})
	winner := Layer("")
	if err == nil {
		winner = sel.From
	}
	rows = []Row{
		{Layer: LayerEnv, Value: req.Env},
		{Layer: LayerPin, Value: req.Pin},
	}
	if it, ok := s.Lookup(LocalName); ok {
		rows = append(rows, Row{Layer: LayerRunning, Value: it.Name, Where: it.Where})
	} else {
		rows = append(rows, Row{Layer: LayerRunning})
	}
	if it, ok := s.defaultInstance(); ok {
		rows = append(rows, Row{Layer: LayerDefault, Value: it.Name, Where: it.Where})
	} else {
		rows = append(rows, Row{Layer: LayerDefault})
	}
	// The two layers a user types into are the two that can name something that
	// is not there; the machine-filled rows below them are read from the set.
	for i := range rows {
		rows[i].Wins = rows[i].Layer == winner
		if rows[i].Value == "" || (rows[i].Layer != LayerEnv && rows[i].Layer != LayerPin) {
			continue
		}
		switch it, known := s.Lookup(rows[i].Value); {
		case known:
			rows[i].Where = it.Where
		case rows[i].Value == LocalName:
			rows[i].Problem = "no local Airflow is running for this project"
		default:
			rows[i].Problem = "names no instance this project knows"
		}
	}
	return rows, sel, err
}
