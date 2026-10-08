//go:build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

// Every command, run by the shipped binary with --output json, writes JSON to
// stdout and nothing else: one value for a result, a line per record for a
// stream, and for a failure one {error, code, kind} object whose code is the
// exit status.
//
// Each family's unit tests pin the shapes it publishes, but they drive a root
// they built, with writers they bound, so they cannot see what the binary does
// with the real stdout. Two bugs lived exactly there: JSON written to stderr
// in production, and login notes printed to stdout ahead of the object. This
// checks the property itself, for every command at once.
//
// The commands are not a list kept here. They are read from the binary: the
// tree is walked with cobra's own completion command (`astro __complete`), so
// it is every command a user can see, and each command's --help says whether
// it runs, whether it offers --output json, and what its flags take. A command
// added tomorrow is run tomorrow. What this file keeps is the exceptions, each
// with its reason: a command with no --output by design, a command a state
// must not run, and the arguments a command is given. A command with no
// --output json that nothing excuses fails, and so does an exception for a
// command the binary no longer has.
//
// A group is run too: with nothing to run, under json it is a usage error
// (cliout.GroupHelp), where cobra on its own prints help to stdout and exits 0.
//
// A command with arguments it needs is run with none, and given placeholders
// one usage error at a time (runForJSON), so it gets as far as a placeholder
// takes it; every attempt is held to the contract on the way.
//
// Every command runs in four states, because which failure it reaches depends
// on what the machine holds:
//
//	no-login  a home with no config: the Astro tree, and the failure a command
//	          meets before any login
//	cloud     a current Astro context with an unexpired login, whose API
//	          refuses every connection: past the pre-run, to the request
//	apc       the same for an APC (Houston) context, which builds the other
//	          tree
//	project   no-login, in a copy of a directory `astro init` made, so the
//	          core tree's commands get past "no project"
//
// Tier 0, and nothing here reaches the network: see offline.
//
// Run it with -v to see every command run and every one skipped, with why.
func TestEveryCommandWritesOnlyJSONToStdout(t *testing.T) {
	tier(t, 0)

	var (
		mu     sync.Mutex
		report []string
		seen   = map[string]bool{}
	)
	t.Run("state", func(t *testing.T) {
		for _, st := range jsonStates {
			t.Run(st.name, func(t *testing.T) {
				t.Parallel()
				// One project per state, for the walk and as the template
				// every run copies: `astro init` once, not once a command.
				base := newStateProject(t, st, "")
				if st.initProject {
					runIn(t, base, "init", "--name", "jsonstdout").requireSuccess()
				}
				tree := discoverTree(t, base)
				if t.Failed() {
					return
				}
				template := ""
				if st.initProject {
					template = base.Dir
				}
				lines := runTree(t, st, tree, template)
				mu.Lock()
				defer mu.Unlock()
				report = append(report, lines...)
				for _, c := range tree {
					seen[c.name()] = true
				}
			})
		}
	})

	// An exception for a command no tree has any more excuses nothing.
	// Checked against all four trees at once, since APC and Astro differ.
	if !t.Failed() {
		checkExceptionsExist(t, seen)
	}

	sort.Strings(report)
	t.Logf("--output json, every command in every state:\n%s", strings.Join(report, "\n"))
}

// checkExceptionsExist fails on an entry in any of this file's tables that
// names a command none of the trees has.
func checkExceptionsExist(t *testing.T, seen map[string]bool) {
	t.Helper()
	var names []string
	for name := range noJSONOutput {
		names = append(names, strings.TrimSuffix(name, " *"))
	}
	for name := range presetArgs {
		names = append(names, name)
	}
	for name := range streams {
		names = append(names, name)
	}
	for name := range ownResultOnFailure {
		names = append(names, name)
	}
	for _, name := range names {
		if !seen[name] {
			t.Errorf("%q is excused in this file, but no tree the binary builds has it; delete the entry", name)
		}
	}
}

// jsonState is one machine the commands run against.
type jsonState struct {
	name string
	// platform is the local.platform of a login written to the home config,
	// "cloud" or "software"; empty for no login.
	platform string
	// initProject runs the commands in a project `astro init` made.
	initProject bool
}

var jsonStates = []jsonState{
	{name: "no-login"},
	{name: "cloud", platform: "cloud"},
	{name: "apc", platform: "software"},
	{name: "project", initProject: true},
}

// notRun are the commands a state does not run, by "state: path", each with
// its reason. Everything else the binary has, every state runs.
var notRun = map[string]string{
	"project: astro local start":   "provisions Airflow with uv or Docker: tiers 1 to 3",
	"project: astro local restart": "starts Airflow, as astro local start does",
	"project: astro local run":     "provisions the project's environment to run the command in: tier 1",
	"project: astro local shell":   "provisions the project's environment to open a shell in: tier 1",
	"project: astro local check":   "provisions an environment with uv to check the DAGs in: tier 1",
	"project: astro local open":    "opens a browser",
}

// noJSONOutput are the commands with no --output json by design: the ones
// cmd/output_flag_test.go's outputExempt names, and the two cobra adds to every
// binary. A path ending in " *" excuses everything under it. None of these is
// asked even for its help: `astro otto --help` is Otto's, and downloads Otto
// to answer it.
var noJSONOutput = map[string]string{
	"astro login":        "an interactive browser flow; exempt from --output by design",
	"astro logout":       "removes the stored login; exempt from --output by design",
	"astro auth login":   "the same flow as astro login",
	"astro auth logout":  "the same as astro logout",
	"astro otto":         "launches an interactive agent session",
	"astro api *":        "the api family shapes its output with --jq and --template",
	"astro help":         "cobra's help command prints help",
	"astro completion *": "cobra's completion scripts are shell code, not data",
}

// preset is the arguments a command runs with, where what runForJSON would
// work out for itself is not what this should run, and why.
type preset struct {
	args []string
	why  string
}

// keyringWhy is why a vault write is made plain: an encrypted value goes to
// the vault, whose key is in the OS keyring, which no isolation lever moves
// (doc.go).
const keyringWhy = "an encrypted value goes to the vault, whose key is in the OS keyring"

var presetArgs = map[string]preset{
	// The MWAA artifact is the project's own files, as package_test.go runs
	// it at tier 0.
	"astro package": {[]string{"mwaa"}, "bare, it builds the Astro image with Docker, which is tier 3"},
	// The value from a flag rather than a prompt, and stored plain.
	"astro local env variable set":         {[]string{placeholder, "--plain", "--value", placeholder}, keyringWhy},
	"astro local env airflow-variable set": {[]string{placeholder, "--plain", "--value", placeholder}, keyringWhy},
	"astro local env connection set":       {[]string{placeholder, "--plain", "--value", "postgres://e2e@127.0.0.1:1/e2e"}, keyringWhy},
}

// streams are the commands whose output may be NDJSON, one record per line.
// A stream that fails ends with the error object, on its own line.
var streams = map[string]string{
	"astro package": "the image build's log, then the result",
}

// ownResultOnFailure are the commands that carry their own exit code and
// publish their own result when they fail, instead of an error object
// (cliout.ExitError; docs/architecture.md, "Failures"). What they write still
// has to be JSON and nothing else.
var ownResultOnFailure = map[string]string{
	"astro local check":                "publishes its findings and verdict; exit 1 or 2",
	"astro af runs trigger-wait":       "publishes the run it waited for; exit 1 or 2",
	"astro local af runs trigger-wait": "publishes the run it waited for; exit 1 or 2",
	"astro local run":                  "exits with the child's status",
}

// newStateProject is a fresh project on st's machine, with a copy of template
// in its directory when template is not empty.
//
// Its vault holds the stamp a keyring that timed out leaves, so no command
// reaches the OS keyring for a login: a login the config holds is otherwise
// moved into the vault, whose key is in the keyring, and the keyring is the one
// store no isolation lever relocates (doc.go). With the stamp the login stays
// in the config, as on a machine with no keyring, which is what CI's are.
func newStateProject(t *testing.T, st jsonState, template string) *project {
	t.Helper()
	p := newProject(t)
	dir := filepath.Join(p.home, ".astro", "secrets")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("making the vault directory: %v", err)
	}
	// pkg/secrets' loginStamp, and its "timeout" content.
	if err := os.WriteFile(filepath.Join(dir, "login-keyring-unavailable"), []byte("timeout"), 0o600); err != nil {
		t.Fatalf("stamping the keyring unavailable: %v", err)
	}
	if st.platform != "" {
		writeLogin(t, p, st.platform)
	}
	if template != "" {
		copyTree(t, template, p.Dir)
	}
	return p
}

// copyTree copies the files under src into dst, which exists.
func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode().Perm())
	})
	if err != nil {
		t.Fatalf("copying the project %s: %v", src, err)
	}
}

// writeLogin gives p's home a current context, logged in to an API that
// refuses every connection. platform is the config's local.platform: "cloud"
// for Astro, "software" for APC.
//
// The domain is "localhost" because it is the one domain whose API address the
// config gives rather than the domain: local.core for Astro's, local.houston
// for APC's. Any other would have the CLI look up api.<domain>, and a DNS
// query is the network too.
//
// The login does not expire, so the pre-run takes it without renewing it, and
// the command goes on to its request, which fails. A note printed along the
// way would be on stdout ahead of the error object, as it once was.
func writeLogin(t *testing.T, p *project, platform string) {
	t.Helper()
	cfg := fmt.Sprintf(`context: localhost
local:
  core: %[1]s
  houston: %[1]s/v1
  platform: %[2]s
contexts:
  localhost:
    domain: localhost
    token: Bearer e2e-not-a-token
    expiresin: 2100-01-01T00:00:00Z
    organization: e2e-organization
    organization_product: HOSTED
    workspace: e2e-workspace
    last_used_workspace: e2e-workspace
    user_email: e2e@example.invalid
`, refusingHost, platform)
	dir := mkdir(t, p.home, ".astro")
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(cfg), 0o600); err != nil {
		t.Fatalf("writing the home config: %v", err)
	}
}

// refusingHost refuses connections at once: nothing listens on port 1.
const refusingHost = "http://127.0.0.1:1"

// offline is the environment every command here runs with, on top of the
// project's isolation.
//
// The hosts this file points commands at all refuse, but a command can reach
// one nobody pointed anywhere: `astro otto --help` downloads Otto, and was
// found that way. So every request goes through a proxy that is not there. Go,
// uv, curl and docker all read these, and none proxies a request to 127.0.0.1,
// so the deliberately refusing hosts still refuse directly.
var offline = map[string]string{
	"HTTP_PROXY":  refusingHost,
	"HTTPS_PROXY": refusingHost,
	"http_proxy":  refusingHost,
	"https_proxy": refusingHost,
	"NO_PROXY":    "",
	"no_proxy":    "",
	// And Docker, which is tier 3: a command that would build or run a
	// container fails at once rather than use the developer's engine.
	"DOCKER_HOST": "tcp://127.0.0.1:1",
}

// runBound is how long one command may take. Each works on local files or
// fails a connection that is refused, so one still running after this is
// waiting on something it should not be.
const runBound = 30 * time.Second

func runIn(t *testing.T, p *project, args ...string) *result {
	t.Helper()
	return p.forT(t).runBounded(runBound, offline, args...)
}

// cliCommand is one command the binary has.
type cliCommand struct {
	// path is the words after "astro".
	path []string
	// runnable is false for a group that only holds other commands.
	runnable bool
	// group is true for a command with subcommands.
	group bool
	// json says whether its --help offers --output json.
	json bool
	// flagTypes is each flag's value type as its --help shows it: "string",
	// "int", ...; "" for a flag that takes no value.
	flagTypes map[string]string
}

func (c cliCommand) name() string { return strings.Join(append([]string{"astro"}, c.path...), " ") }

// usageSection is the "Usage:" block of a command's help: one line per way to
// invoke it, "astro x [command]" for a group and "astro x ... [flags]" for a
// command that runs.
var usageSection = regexp.MustCompile(`(?m)^Usage:\n((?: {2}.*\n)+)`)

// outputFlag is an --output flag whose help offers json, which is how
// cmd/output_flag_test.go tells a format from a destination file. The help may
// wrap, so this reads on to the next flag rather than to the end of the line.
var outputFlag = regexp.MustCompile(`--output\s+\w+\s+Output format\b[^-]*\bjson\b`)

// flagSection is a help page's "Flags:" or "Global Flags:" list, up to the
// blank line that ends it. Read on its own, because the other sections name
// flags too: an example's `--workspace-id <WORKSPACE_ID>` would read as a
// flag whose type is its placeholder.
var flagSection = regexp.MustCompile(`(?m)^(?:Global )?Flags:\n((?:.+\n)+)`)

// flagLine is one flag in a flag list: its name, and the type of value it
// takes when it takes one.
var flagLine = regexp.MustCompile(`(?m)^\s+(?:-\w, )?--([\w-]+)(?: (\w+))?(?:\s|$)`)

// flagTypes reads each flag's value type from a help page's flag lists.
func flagTypes(help string) map[string]string {
	types := map[string]string{}
	for _, section := range flagSection.FindAllStringSubmatch(help, -1) {
		for _, f := range flagLine.FindAllStringSubmatch(section[1], -1) {
			types[f[1]] = f[2]
		}
	}
	return types
}

// discoverTree walks the binary's command tree as p's machine has it.
func discoverTree(t *testing.T, p *project) []cliCommand {
	t.Helper()
	var (
		mu    sync.Mutex
		found []cliCommand
	)
	add := func(c cliCommand) {
		mu.Lock()
		found = append(found, c)
		mu.Unlock()
	}
	var visit func(t *testing.T, path []string)
	visit = func(t *testing.T, path []string) {
		if _, ok := excused(noJSONOutput, cliCommand{path: path}.name()); ok {
			add(cliCommand{path: path, runnable: true})
			return
		}
		help := runIn(t, p, append(append([]string{}, path...), "--help")...)
		if help.ExitCode != 0 {
			t.Fatalf("`astro %s --help` failed\n%s", strings.Join(path, " "), help.output())
		}
		m := usageSection.FindStringSubmatch(help.Stdout)
		if m == nil {
			t.Fatalf("`astro %s --help` has no Usage section to read\n%s", strings.Join(path, " "), help.output())
		}
		c := cliCommand{path: path, json: outputFlag.MatchString(help.Stdout), flagTypes: flagTypes(help.Stdout)}
		for _, line := range strings.Split(strings.TrimRight(m[1], "\n"), "\n") {
			if strings.HasSuffix(line, "[command]") {
				c.group = true
			} else {
				c.runnable = true
			}
		}
		add(c)
		if !c.group {
			return
		}
		for _, sub := range completions(t, p, path) {
			child := append(append([]string{}, path...), sub)
			t.Run(sub, func(t *testing.T) {
				t.Parallel()
				visit(t, child)
			})
		}
	}
	t.Run("discover", func(t *testing.T) { visit(t, nil) })
	sort.Slice(found, func(i, j int) bool { return found[i].name() < found[j].name() })
	return found
}

// completions are the subcommands cobra offers after path: what a user's shell
// would complete, which is every visible command and no hidden one.
func completions(t *testing.T, p *project, path []string) []string {
	t.Helper()
	r := runIn(t, p, append(append([]string{"__complete"}, path...), "")...)
	if r.ExitCode != 0 {
		t.Fatalf("`astro __complete %s` failed\n%s", strings.Join(path, " "), r.output())
	}
	var names []string
	for _, line := range strings.Split(r.Stdout, "\n") {
		name, _, _ := strings.Cut(line, "\t")
		name = strings.TrimSpace(name)
		// The last line is cobra's directive, ":4".
		if name == "" || strings.HasPrefix(name, ":") || strings.HasPrefix(name, "-") {
			continue
		}
		names = append(names, name)
	}
	if len(names) == 0 {
		t.Fatalf("`astro %s` reads as a group, but cobra completes no subcommand of it\n%s",
			strings.Join(path, " "), r.output())
	}
	return names
}

// excused is the reason table gives for name, matching a " *" entry by prefix.
func excused(table map[string]string, name string) (string, bool) {
	if why, ok := table[name]; ok {
		return why, true
	}
	for k, why := range table {
		if base, ok := strings.CutSuffix(k, " *"); ok && (name == base || strings.HasPrefix(name, base+" ")) {
			return why, true
		}
	}
	return "", false
}

// runTree runs every command in tree, each in a fresh project of st's with a
// copy of template, and returns a line per command saying what was run, or
// why not.
func runTree(t *testing.T, st jsonState, tree []cliCommand, template string) []string {
	t.Helper()
	var (
		mu    sync.Mutex
		lines []string
	)
	note := func(format string, args ...any) {
		mu.Lock()
		lines = append(lines, fmt.Sprintf("%-8s ", st.name)+fmt.Sprintf(format, args...))
		mu.Unlock()
	}

	have := map[string]bool{}
	for _, c := range tree {
		have[c.name()] = true
	}
	for k := range notRun {
		if state, name, _ := strings.Cut(k, ": "); state == st.name && !have[name] {
			t.Errorf("notRun excuses %q in %s, which that tree does not have; delete the entry", name, state)
		}
	}

	t.Run("run", func(t *testing.T) {
		for _, c := range tree {
			name := c.name()
			why, exempt := excused(noJSONOutput, name)
			switch {
			case len(c.path) == 0:
				// The root has no --output; `astro --help` is help.
				continue
			case exempt:
				note("skip %s: %s", name, why)
				continue
			case !c.runnable:
				t.Run(strings.Join(c.path, " "), func(t *testing.T) {
					t.Parallel()
					r := runBareGroup(t, newStateProject(t, st, template), c)
					how := "a group"
					if !c.json {
						how = "a group with no --output, which refuses the flag"
					}
					note("ran  astro %s -> exit %d (%s)", strings.Join(r.Args, " "), r.ExitCode, how)
				})
				continue
			case !c.json:
				t.Errorf("%s offers no --output json, and nothing here says why. Every command offers one "+
					"(cliout.AddOutputFlag); one exempt by design goes in noJSONOutput", name)
				continue
			}
			if why, ok := notRun[st.name+": "+name]; ok {
				note("skip %s: %s", name, why)
				continue
			}
			t.Run(strings.Join(c.path, " "), func(t *testing.T) {
				t.Parallel()
				r := runForJSON(t, newStateProject(t, st, template), c)
				outcome := "exit 0"
				if r.ExitCode != 0 {
					outcome = fmt.Sprintf("exit %d", r.ExitCode)
					if e := lastErrorObject(r.Stdout); e != nil && e.Kind != "" {
						outcome += " " + e.Kind
					}
				}
				note("ran  astro %s -> %s", strings.Join(r.Args, " "), outcome)
			})
		}
	})
	return lines
}

// runBareGroup runs a group that has nothing of its own to run, with
// --output json. It is a usage error: one error object with kind usage, exit
// 2. A group that offers no --output of its own refuses the flag, which is a
// usage error too, reported as text on stderr; either way nothing is on
// stdout but the object, and above all not the group's help.
func runBareGroup(t *testing.T, p *project, c cliCommand) *result {
	t.Helper()
	args := append(append([]string{}, c.path...), "--output", "json")
	r := runIn(t, p, args...)
	cmdline := "astro " + strings.Join(args, " ")
	if r.ExitCode != 2 {
		t.Errorf("`%s` exited %d; a group run with nothing to run is a usage error, exit 2\n%s",
			cmdline, r.ExitCode, r.output())
		return r
	}
	if !c.json {
		if r.Stdout != "" {
			t.Errorf("`%s` refused --output, and still wrote to stdout\n%s", cmdline, r.output())
		}
		return r
	}
	if e := checkJSONStdout(t, c, r); e != nil && e.Kind != "usage" {
		t.Errorf("`%s` failed with kind %q; a bare group is a usage error\n%s", cmdline, e.Kind, r.output())
	}
	return r
}

// What cobra and pflag say when a command was invoked without something it
// needs. runForJSON answers each with a placeholder.
var (
	requiredFlags = regexp.MustCompile(`required flag\(s\) (.+) not set`)
	quotedName    = regexp.MustCompile(`"([^"]+)"`)
	oneRequired   = regexp.MustCompile(`at least one of the flags in the group \[(\S+)`)
	// argCount is an error about how many arguments a command got, in cobra's
	// words ("accepts 1 arg(s), received 0") or a command's own ("must
	// specify exactly two arguments").
	argCount = regexp.MustCompile(`(?i)\b(accepts|requires|expects|takes|must specify|received|exactly|at least|at most)\b[^.\n]*\b(arg\(s\)|args?\b|arguments?\b)`)
	// flagValue is pflag refusing a flag's value: about a flag, not about
	// arguments, so no positional answers it.
	flagValue = regexp.MustCompile(`^invalid argument "`)
)

// whichArgument are the refusals argCount reads as being about how many
// arguments a command got, when they are about which ones, each by a part of
// its message (contained rather than leading, so a refusal wrapped in context
// still matches) and with the reason it is not a usage error. Everything else
// argCount matches must exit 2, so a command that adds an arity check of its
// own as a plain error is caught; an entry here is a decision, not a filter.
var whichArgument = []string{
	// internal/platform/astro/deployment's errBundleSelector: a choice of one
	// identifier among an argument and two flags, not a count, checked below
	// cmd/, where cliout.Usage is not reachable, once the command is running.
	"specify exactly one bundle identifier",
}

// isArityRefusal reports whether msg refuses the number of arguments a
// command got, which is a usage error, exit 2. pflag's refusal of a flag's
// value ("invalid argument ... for --flag") is not one, and argCount does
// not read it as one (TestRunForJSONReadsTheRefusalsItAnswers).
func isArityRefusal(msg string) bool {
	if !argCount.MatchString(msg) {
		return false
	}
	for _, part := range whichArgument {
		if strings.Contains(msg, part) {
			return false
		}
	}
	return true
}

// placeholder is what a needed argument or flag is given. It names nothing
// that exists, so a command that looks it up fails the lookup.
const placeholder = "e2e"

// flagPlaceholder is a value for a flag of the given type that pflag parses,
// so a required flag gets past the parse to what the command does with it.
// Nil for a flag that takes no value.
func flagPlaceholder(typ string) []string {
	switch typ {
	case "":
		return nil
	case "int", "int32", "int64", "uint", "uint32", "uint64", "float32", "float64":
		return []string{"1"}
	case "duration":
		return []string{"1s"}
	case "stringToString":
		return []string{placeholder + "=" + placeholder}
	default:
		return []string{placeholder}
	}
}

// maxAttempts bounds how many usage errors runForJSON answers before it takes
// the last attempt as the command's result.
const maxAttempts = 5

// runForJSON runs c with --output json and holds what it wrote to the
// contract. A usage error that says what was missing is answered and the
// command run again, so the result comes from as deep in the command as
// placeholders reach; each attempt on the way is held to the contract too.
func runForJSON(t *testing.T, p *project, c cliCommand) *result {
	t.Helper()
	var extra []string
	addFlag := func(name string) {
		extra = append(append(extra, "--"+name), flagPlaceholder(c.flagTypes[name])...)
	}
	for attempt := 1; ; attempt++ {
		// --output straight after the command: `astro local run` stops
		// parsing flags at its first argument, the command it runs, so after
		// that --output would be the child's.
		args := append(append([]string{}, c.path...), "--output", "json")
		args = append(append(args, presetArgs[c.name()].args...), extra...)
		r := runIn(t, p, args...)
		e := checkJSONStdout(t, c, r)
		if e == nil || e.Kind != "usage" || attempt == maxAttempts {
			return r
		}
		switch {
		case flagValue.MatchString(e.Error):
			// A flag whose value a placeholder of its type does not satisfy
			// (an enum, say). The refusal is the result.
			return r
		case requiredFlags.MatchString(e.Error):
			for _, f := range quotedName.FindAllStringSubmatch(requiredFlags.FindStringSubmatch(e.Error)[1], -1) {
				addFlag(f[1])
			}
		case oneRequired.MatchString(e.Error):
			addFlag(oneRequired.FindStringSubmatch(e.Error)[1])
		case argCount.MatchString(e.Error):
			// Positionals before the flags placeholders answered: a command
			// that stops at its first argument would otherwise pass the
			// flags on.
			extra = append([]string{placeholder}, extra...)
		default:
			return r
		}
	}
}

// errorObject is the object a failure publishes.
type errorObject struct {
	Error string   `json:"error"`
	Code  *float64 `json:"code"`
	Kind  string   `json:"kind"`
}

// cobraOnStderr is what cobra prints for a failure in text mode: "Error: "
// and the usage block. Under json the failure is the object on stdout, so
// either on stderr means the run reported it as text as well, or instead.
// Notes on stderr are allowed; that is where they go.
var cobraOnStderr = regexp.MustCompile(`(?m)^(?:Error: |(?:Usage:)$)`)

// checkJSONStdout holds r to the output contract, and returns the error object
// when the command failed with one.
func checkJSONStdout(t *testing.T, c cliCommand, r *result) *errorObject {
	t.Helper()
	cmdline := "astro " + strings.Join(r.Args, " ")
	name := c.name()

	values, err := jsonValues(r.Stdout)
	if err != nil {
		t.Errorf("`%s` wrote something other than JSON to stdout: %v\n%s", cmdline, err, r.output())
		return nil
	}
	_, stream := streams[name]
	if len(values) > 1 && !stream {
		t.Errorf("`%s` wrote %d JSON values to stdout. A result is one value, a failure one error object, "+
			"and only a stream (EmitEvent) writes a line per record\n%s", cmdline, len(values), r.output())
		return nil
	}
	if stream && !oneLinePerValue(r.Stdout) {
		t.Errorf("`%s` streams, and its stdout is not one JSON value per line\n%s", cmdline, r.output())
		return nil
	}
	if len(values) == 0 {
		t.Errorf("`%s` exited %d and wrote nothing to stdout; under --output json its result or its error "+
			"object goes there\n%s", cmdline, r.ExitCode, r.output())
		return nil
	}
	last := asErrorObject(values[len(values)-1])
	if r.ExitCode == 0 {
		if last != nil {
			t.Errorf("`%s` exited 0, and what it wrote is an error object\n%s", cmdline, r.output())
		}
		return nil
	}
	if cobraOnStderr.MatchString(r.Stderr) {
		t.Errorf("`%s` failed, and reported it on stderr as text too (\"Error: \" or the usage block)\n%s",
			cmdline, r.output())
	}
	if _, ok := ownResultOnFailure[name]; ok && last == nil {
		// Its own result. When it fails before it has one (a usage error,
		// no project), it publishes the error object like any command, and
		// is held to it below.
		return nil
	}
	if last == nil {
		t.Errorf("`%s` failed, and its stdout does not end in an {error, code, kind} object\n%s", cmdline, r.output())
		return nil
	}
	if int(*last.Code) != r.ExitCode {
		t.Errorf("`%s` exited %d, but its error object says code %v\n%s", cmdline, r.ExitCode, *last.Code, r.output())
	}
	if isArityRefusal(last.Error) && r.ExitCode != 2 {
		t.Errorf("`%s` refused its arguments and exited %d; the wrong number of arguments is a usage "+
			"error, exit 2 (cliout.Usage)\n%s", cmdline, r.ExitCode, r.output())
	}
	return last
}

// asErrorObject is v as an error object, or nil when it is not one.
func asErrorObject(v json.RawMessage) *errorObject {
	var e errorObject
	if json.Unmarshal(v, &e) != nil || e.Error == "" || e.Code == nil {
		return nil
	}
	return &e
}

// jsonValues splits stdout into the JSON values it holds, and fails on
// anything that is not one: a line of prose before the object, a banner after.
func jsonValues(stdout string) ([]json.RawMessage, error) {
	dec := json.NewDecoder(strings.NewReader(stdout))
	var values []json.RawMessage
	for {
		var v json.RawMessage
		err := dec.Decode(&v)
		if err == io.EOF {
			return values, nil
		}
		if err != nil {
			return nil, err
		}
		values = append(values, v)
	}
}

// oneLinePerValue reports whether every non-empty line of stdout is one whole
// JSON value, which is what makes it NDJSON rather than values that merely
// follow each other.
func oneLinePerValue(stdout string) bool {
	for _, line := range bytes.Split([]byte(stdout), []byte("\n")) {
		if len(bytes.TrimSpace(line)) > 0 && !json.Valid(line) {
			return false
		}
	}
	return true
}

// runForJSON tells cobra's and pflag's refusals apart by their words, so the
// words it reads are pinned here: a flag value pflag refused is not an
// argument count, and no positional answers it.
func TestRunForJSONReadsTheRefusalsItAnswers(t *testing.T) {
	tier(t, 0)

	for msg, want := range map[string]string{
		"accepts 1 arg(s), received 0":                                "args",
		"requires at least 1 arg(s), only received 0":                 "args",
		"accepts between 1 and 2 arg(s), received 3":                  "args",
		"must specify exactly two arguments (key value) when setting": "args",
		`invalid argument "e2e" for "--port" flag: strconv.ParseInt`:  "flag value",
		`invalid argument "e2e" for "-n, --count" flag: parse error`:  "flag value",
		`required flag(s) "name", "type" not set`:                     "required",
		"at least one of the flags in the group [id key] is required": "one of",
		`unknown command "e2e" for "astro deployment"`:                "",
		`"astro af" needs a subcommand: dags, runs`:                   "",
	} {
		got := ""
		switch {
		case flagValue.MatchString(msg):
			got = "flag value"
		case requiredFlags.MatchString(msg):
			got = "required"
		case oneRequired.MatchString(msg):
			got = "one of"
		case argCount.MatchString(msg):
			got = "args"
		}
		if got != want {
			t.Errorf("%q reads as %q, want %q", msg, got, want)
		}
	}
	for typ, want := range map[string]string{"int": "1", "duration": "1s", "string": placeholder, "stringToString": "e2e=e2e"} {
		if got := flagPlaceholder(typ); len(got) != 1 || got[0] != want {
			t.Errorf("a %s flag is given %v, want %q", typ, got, want)
		}
	}
	if got := flagPlaceholder(""); got != nil {
		t.Errorf("a flag that takes no value is given %v", got)
	}

	// Which refusals must exit 2: the count, not which arguments.
	for msg, want := range map[string]bool{
		"accepts 1 arg(s), received 0":                                                                        true,
		"requires at least 1 arg(s), only received 0":                                                         true,
		"must specify exactly two arguments (key value) when setting a":                                       true,
		"expects exactly one argument, the deployment ID":                                                     true,
		"specify exactly one bundle identifier: the BUNDLE-ID argument, --name (DAG bundle), or --mount-path": false,
		"deleting the bundle: specify exactly one bundle identifier: the BUNDLE-ID argument, or --name":       false,
		`invalid argument "e2e" for "--port" flag: strconv.ParseInt`:                                          false,
	} {
		if got := isArityRefusal(msg); got != want {
			t.Errorf("%q reads as an arity refusal: %v, want %v", msg, got, want)
		}
	}
}

// Flag types come from the flag lists alone: an example that names a flag
// with a placeholder after it does not give the flag a type.
func TestFlagTypesReadOnlyTheFlagLists(t *testing.T) {
	tier(t, 0)

	help := "Usage:\n  astro workspace user add [flags]\n\n" +
		"Flags:\n" +
		"  -e, --email string        The user's email\n" +
		"      --role string         The role\n" +
		"      --count int           How many\n" +
		"  -h, --help                Show help for this command\n\n" +
		"Global Flags:\n" +
		"  -o, --output string   Output format: text or json (default \"text\")\n\n" +
		"Examples:\n" +
		"  astro workspace user add --email e@x --workspace-id <WORKSPACE_ID>\n" +
		"  --count <N>\n"

	got := flagTypes(help)
	want := map[string]string{"email": "string", "role": "string", "count": "int", "help": "", "output": "string"}
	if len(got) != len(want) {
		t.Errorf("flag types %v, want %v", got, want)
	}
	for name, typ := range want {
		if got[name] != typ {
			t.Errorf("--%s reads as type %q, want %q (all: %v)", name, got[name], typ, got)
		}
	}
}

// lastErrorObject is the error object stdout ends with, if it ends with one.
func lastErrorObject(stdout string) *errorObject {
	values, err := jsonValues(stdout)
	if err != nil || len(values) == 0 {
		return nil
	}
	return asErrorObject(values[len(values)-1])
}
