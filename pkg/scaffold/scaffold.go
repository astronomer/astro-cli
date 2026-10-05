// Package scaffold makes a directory an `astro init` project: the
// pyproject.toml manifest — written fresh, or adopted where one is already
// there — the standard directories, .gitignore, and AGENTS.md (with CLAUDE.md
// as a symlink to it outside Windows). It follows the layer rules in
// docs/v2-architecture.md: it returns data and errors, never prints, never
// exits.
package scaffold

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"

	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/manifest/tomledit"
	"github.com/astronomer/astro-cli/pkg/runtimeversions"
)

// DefaultAirflow resolves the Airflow a project starts on when nothing it has
// states one: the series, the requires-python that series' runtime ships with
// (empty to use this package's built-in rule), and where the answer came from.
//
// It is how Plan stays offline. The answer lives in Astronomer's runtime
// catalog, and reading that is the caller's business: astro init and
// Astro Desktop pass runtimeversions.Default bound to their own cache and
// User-Agent. Plan calls it only when no pin applies, so adopting a project
// that states its Airflow asks nobody anything.
type DefaultAirflow func() (series, requiresPython string, src runtimeversions.Source)

// manifestKeyAirflow is the [tool.astro] key that carried the Airflow pin
// before the apache-airflow requirement became the only place a project
// states its version. Nothing writes it; SetAirflowVersionWith and
// MigrateAirflowKey delete a leftover one.
const manifestKeyAirflow = "airflow"

// manifestKeyDockerfile is the [tool.astro] key naming the project's own
// Dockerfile, written by both manifest arms whenever a run keeps one.
const manifestKeyDockerfile = "dockerfile"

// Options adjust what Run scaffolds.
type Options struct {
	// Name is the [project] name. Empty derives it from the directory name.
	Name string
	// AirflowVersion is the Airflow the project pins, written as the
	// apache-airflow requirement in [project] dependencies. Empty means the pin
	// the project already states, else Default's answer.
	AirflowVersion string
	// Default resolves the Airflow a project starts on when nothing states one.
	// nil means runtimeversions.FallbackAirflowSeries, with no lookup, reported
	// as runtimeversions.SourceBuiltIn.
	Default DefaultAirflow
	// GOOS overrides runtime.GOOS, so tests can check the Windows layout
	// (no CLAUDE.md symlink) from any host.
	GOOS string
	// SecretWriter stores the connection and Airflow variable values a v1
	// airflow_settings.yaml carries, at the project scope the writer itself
	// decides. Without one, Plan leaves the values in the file and says so.
	SecretWriter SecretWriter
}

// Result reports what Run did. It is the `astro init` output payload in
// both text and json mode.
type Result struct {
	Dir            string `json:"dir"`
	Name           string `json:"name"`
	AirflowVersion string `json:"airflow"`
	// AirflowDefaultSource says where AirflowVersion came from when nothing in
	// the project stated one: "catalog", "cache", "stale-cache", "fallback",
	// "catalog-empty" or "built-in" (see runtimeversions.Source). Empty when a
	// flag or a pin decided it. Consumers should treat an unknown value as a
	// built-in default: the set may grow.
	AirflowDefaultSource runtimeversions.Source `json:"airflowDefaultSource,omitempty"`
	Created              []string               `json:"created"`
	// Skipped lists entries that already existed and were left untouched.
	Skipped []string `json:"skipped,omitempty"`
	// Updated lists files this run changed rather than created.
	Updated []string `json:"updated,omitempty"`
	// Deleted lists the v1 files this run removed, once their contents were in
	// the manifest. Separate from Updated because a caller that renders the two
	// the same way tells the user a destroyed file was edited.
	Deleted []string `json:"deleted,omitempty"`
	// Adopted reports that the manifest was already there and gained a
	// [tool.astro] section, rather than being written by this run. A status
	// bool a json consumer reads, so it stays present when false.
	Adopted bool `json:"adopted"`
	// Notes lists the files init found but did not read, and where their
	// contents belong. It is the work left to do by hand.
	Notes []string `json:"notes,omitempty"`
	// Advisories describe something this run DID carry that now behaves
	// differently, which is the opposite of Notes: nothing is left to do, and
	// the project has changed anyway.
	//
	// Separate rather than mixed in because the two need opposite framing and a
	// caller cannot tell them apart from the text. A UI that renders Notes as
	// "your remaining work" and puts one of these in it is telling someone to
	// go and do something that has already happened; one that hides Notes
	// pending classification — as the desktop's preview does — hides this too,
	// and this is the half that describes a change the user did not ask for.
	Advisories []string `json:"advisories,omitempty"`
}

// ErrAlreadyAstroProject reports a directory whose pyproject.toml already
// carries [tool.astro]. It is the one shape Run refuses: every other
// directory is either scaffolded or adopted. Callers branch with errors.Is.
var ErrAlreadyAstroProject = errors.New("is already an Astro project")

// Project files are the user's own; world-readable is right (never the v1
// helpers' 0o777 — an earlier fix).
const (
	dirPerm  = 0o755
	filePerm = 0o644
)

// projectHasNoDags reports whether the project has no DAGs yet: either no dags
// directory at all, or one holding nothing that could be a DAG.
//
// It answers conservatively, because the two mistakes do not cost the same.
// Skipping the example leaves someone without a file they could write in a
// minute. Writing it can put a file somewhere that is not ours, or fail a run
// that had no reason to fail — so anything this cannot read as a plain,
// DAG-less directory counts as a project that already has DAGs.
//
// That is why `dags` present as anything other than a real directory — a
// regular file, a symlink, an entry that will not stat — means skip, and why
// nothing here returns an error. A regular file named `dags` used to scaffold
// fine, and turning it into a hard failure of the whole run would be a
// regression for a state nobody asked us to police. The symlink case matters on
// its own: os.ReadDir follows one, so the example would be written THROUGH it,
// landing outside the directory the preview showed and Change.resolve undertakes
// not to leave. A dangling symlink is worse still, reading as empty right up
// until the write fails half way through a run.
//
// Only the directory's own entries are counted, never a recursive walk: walking
// a large tree to decide whether to write one small file is the wrong trade.
// That makes an empty `dags/archive/` read as a project WITH DAGs. It is the
// wrong answer for that one shape, and it is the conservative one.
func projectHasNoDags(dir string) bool {
	path := filepath.Join(dir, dagsDir)
	info, err := os.Lstat(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return true
	case err != nil || !info.IsDir():
		return false
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !isDagsPlaceholder(e.Name()) {
			return false
		}
	}
	return true
}

// isDagsPlaceholder reports whether an entry in dags/ is bookkeeping rather than
// a DAG.
//
// Git cannot track an empty directory, so a repository that committed an empty
// dags/ carries a .gitkeep inside it — which makes "a clone with a placeholder
// in it" the single most common shape of a project with no DAGs, and counting
// that entry would deny the example to exactly the projects it is for.
// .DS_Store arrives from opening the folder in Finder; the .gitignore this
// package writes lists it for that reason.
func isDagsPlaceholder(name string) bool {
	return strings.HasPrefix(name, ".") || name == "__pycache__"
}

// starterDagSuits reports whether the starter DAG can run on the Airflow this
// project pins.
//
// The example imports airflow.sdk, which is the Airflow 3 Task SDK and does not
// exist before it: Airflow 2 spells the same two decorators airflow.decorators,
// which is why this repo's v1 templates were a per-major pair
// (pkg/airflowrt/include/airflow2 beside .../airflow3). The pin is not always 3.
// pickAirflowVersion reads it from --airflow-version, the manifest, a Dockerfile
// runtime tag or requirements.txt, and any of those can say 2 — adopting a v1
// project is the ordinary way it happens.
//
// So a project pinning Airflow 2 gets no example, deliberately. A DAG that
// cannot import is the first-run failure this whole file exists to avoid, and it
// is worse than an empty dags directory rather than better — the same trade the
// example's third-party imports were already decided on. An Airflow 2 variant
// would differ by one import line, but nothing has asked for one, and a second
// copy of the file earns its keep only once something does.
//
// A pin this cannot parse is treated as suitable. It has already been through
// pickAirflowVersion by the time it arrives, so an unreadable one means the
// manifest is unusual in ways this decision has no business adjudicating.
func starterDagSuits(airflowVersion string) bool {
	major, _, _ := strings.Cut(airflowVersion, ".")
	n, err := strconv.Atoi(major)
	if err != nil {
		return true
	}
	return n >= firstAirflowWithTaskSDK
}

// firstAirflowWithTaskSDK is the Airflow major that introduced airflow.sdk, the
// one import the starter DAG makes.
const firstAirflowWithTaskSDK = 3

// templateFile is one scaffolded file: where it goes, and what goes in it.
type templateFile struct{ name, content string }

// fileExampleDag is the starter DAG's path. It sits under the dags directory
// projectDirs creates, and planFiles writes files after directories for that
// reason.
//
// A new project had an empty dags/ until this landed: v1's scaffold wrote an
// example and pkg/scaffold did not carry it over, so `astro init` followed by
// `astro local start` gave you an Airflow with nothing in it. No decision was
// recorded against having one, so this reads as an omission rather than a
// choice.
const fileExampleDag = dagsDir + "/exampledag.py"

// Names written in more than one place, kept as constants so the spellings
// never drift. gitignore.go uses these too — it arrived with its own
// .gitignore and 0o644 constants, which is the drift this comment forbids.
const (
	fileGitignore    = ".gitignore"
	fileDockerignore = ".dockerignore"
	fileAgents       = "AGENTS.md"
	fileClaude       = "CLAUDE.md"
	// fileDockerfile is the one place a v1 layout can put a Dockerfile, so it
	// is both the file the retirement decision is about and the value the
	// manifest declaration carries. The literals in v1files.go are left alone
	// deliberately: several of them are note prefixes with the name inside
	// prose ("Dockerfile: its RUN instructions..."), which a constant cannot
	// cover, and half-converting them would read worse than neither.
	fileDockerfile = "Dockerfile"
)

// projectDirs are the standard project directories, in creation order.
var projectDirs = []string{dagsDir, "include", "plugins", "tests"}

// dagsDir is named because three things key off it: projectDirs creates it,
// projectHasNoDags decides the starter DAG from what is in it, and
// fileExampleDag is a path inside it.
const dagsDir = "dags"

// windowsOS is the GOOS whose layout skips the CLAUDE.md symlink.
const windowsOS = "windows"

// manifestFacts is what the hand-off list needs to know about the manifest,
// beyond the files sitting beside it.
type manifestFacts struct {
	// defaultedPin reports that nothing named an Airflow version, so the pin
	// is the CLI's default.
	defaultedPin bool
	// nameAdvisory is what to tell somebody when the project is not called
	// what it said it was called — respelled to fit a [project] name, or
	// discarded because nothing in it could. Empty when the name carried
	// unchanged, which is the ordinary case.
	//
	// Worded by chooseName, which is where the reason is known, and carried
	// here because both the scaffold and adopt arms have to hand it back to
	// Plan.
	nameAdvisory string
	// loosePython reports that the manifest states a requires-python of its
	// own that still admits an interpreter the pinned Airflow cannot run
	// under. Left as the author wrote it — which interpreters a project
	// supports is their call — but said out loud, because the run can see the
	// collision and the first `astro local start` is where it otherwise
	// surfaces, from inside a dependency that names neither.
	loosePython bool
	// migrationNotes is what building the manifest could not migrate, discovered
	// while building it rather than while reading the v1 files: extras on an
	// apache-airflow requirement that the generated pin does not reproduce.
	migrationNotes []string
	// migratedLabels describes what the manifest write absorbed, for the
	// Result's lists. The adopt arm returns its own labels directly; the
	// greenfield arm cannot, because its label is fixed by Plan, so it reports
	// them here instead.
	migratedLabels []string
}

// Run makes dir an Astro project, creating dir if needed. A directory with no
// pyproject.toml is scaffolded; one that has a pyproject.toml without
// [tool.astro] is adopted, so `astro init` runs in an Airflow repo as it
// stands. Either way, files already there are kept, and what Run could not
// carry over is reported in Notes.
//
// Run is Plan followed by Apply, which is what a command wants: nobody is going
// to review a change set at a terminal that has already asked for it. A caller
// that shows the change set to a person first calls the two halves itself.
func Run(dir string, opts Options) (*Result, error) {
	cs, err := Plan(dir, opts)
	if err != nil {
		return nil, err
	}
	return cs.Apply()
}

// Plan works out what making dir an Astro project would do, and returns it
// without touching the project.
//
// The split exists because this package's output lands in someone's repository.
// A command can reasonably scaffold on request, but Astro Desktop offers to
// convert a project the user already has, and O3 requires it to show the diff
// first — which is impossible if the only way to learn what a run does is to
// let it happen. Plan reads the project (it has to: an adopted manifest is
// computed from the one already there) and writes nothing.
func Plan(dir string, opts Options) (*Changeset, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolving %s: %w", dir, err)
	}

	goos := opts.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	marker := filepath.Join(abs, manifest.Marker)
	// Refused with its own error, before anything is read: otherwise it would
	// surface as a requirement the manifest cannot read.
	if v := opts.AirflowVersion; v != "" && !manifest.ValidAirflowVersion(v) {
		return nil, fmt.Errorf("%w: %q is not a version like 3, 3.1, or 3.1.2", ErrInvalidAirflowVersion, v)
	}

	// What the v1 files say, read before either arm, because both need it: a
	// greenfield manifest is BUILT from them and an adopted one is extended
	// with them. This is the difference between init-in-an-existing-project
	// converting it and init leaving a hand-off list beside files nobody read.
	v1, err := readV1Project(abs)
	if err != nil {
		return nil, err
	}

	// A manifest already there is adopted; its absence is the greenfield path.
	// Both arms settle the manifest and write nothing.
	cs := &Changeset{Result: Result{Dir: abs}}
	var out []byte
	var manifestLabels []string
	var pin manifestFacts
	data, readErr := os.ReadFile(marker)
	switch {
	case readErr == nil:
		out, manifestLabels, pin, err = adopt(abs, data, opts, v1, &cs.Result)
	case errors.Is(readErr, os.ErrNotExist):
		// No labels from this arm: a scaffolded manifest is created rather than
		// edited, so its one line is the filename, supplied below.
		out, pin, err = scaffoldManifest(abs, opts, v1, &cs.Result)
	default:
		err = fmt.Errorf("reading %s: %w", marker, readErr)
	}
	if err != nil {
		return nil, err
	}

	if err := planFiles(abs, goos != windowsOS, cs); err != nil {
		return nil, err
	}
	if err := planKeptDockerfileIgnore(abs, v1, cs); err != nil {
		return nil, err
	}

	// The manifest goes after everything that ADDS, and the ordering is the
	// reason Apply walks a slice rather than a map. A manifest carrying
	// [tool.astro] is the one thing that makes a rerun refuse, so a run that
	// dies part-way through is safe to repeat only while the manifest is still
	// absent. Deletions then go after the manifest — see below, where the
	// invariant is "manifest before any deletion, deletions last".
	manifestChange := Change{Kind: UpdateFile, Path: manifest.Marker, Content: out, Labels: manifestLabels}
	if !cs.Adopted {
		manifestChange.Kind = CreateFile
		manifestChange.Labels = append([]string{manifest.Marker}, pin.migratedLabels...)
	}
	cs.Changes = append(cs.Changes, manifestChange)

	// The notes are computed BEFORE the retirements, because they are what the
	// retirements are decided from. Deciding earlier is what made the first
	// version of this delete a requirements.txt that had been carried nowhere.
	//
	// The v1 notes lead: they are about the files this run just read, so they
	// describe what it could not carry. leftovers is about files it did not read
	// at all, which is a weaker statement and belongs after.
	lefts, leftsMayRetire := leftovers(abs, cs.AirflowVersion, &pin, v1)
	cs.Notes = slices.Concat(v1.notes, pin.migrationNotes, lefts)

	// The values airflow_settings.yaml supplied. They ride the changeset rather
	// than being written here because Plan writes nothing, and they are a
	// separate list rather than Changes because a vault write is neither a path
	// in the project nor bytes a preview may show.
	//
	// Settled before the retirements, because whether every value reaches the
	// vault decides whether the file may go.
	cs.Secrets = v1.settings.secrets
	cs.secrets = opts.SecretWriter
	switch {
	case len(cs.Secrets) == 0:
	case cs.secrets == nil:
		// No writer is a caller declining to move credentials, not a caller who
		// forgot. It carries the declarations and leaves the values in the file,
		// with a note saying so.
		//
		// This used to be an error, on the reasoning that a changeset which
		// cannot be applied must not first be approved. The reasoning assumed
		// every caller intends the carry. Failing would make a v1 directory
		// unopenable; carrying silently would move credentials nobody was shown.
		//
		// So the writer IS the consent, and its absence is a decision the note
		// reports rather than a fault.
		n, values := len(cs.Secrets), "values"
		if n == 1 {
			values = "value"
		}
		cs.Notes = append(cs.Notes, SettingsRelPath+": the "+values+" of its "+valueCount(cs.Secrets)+
			" stayed in the file. Convert this project in Astro Desktop, or run "+
			setCommands(cs.Secrets)+", to move "+pronoun(n)+" into the encrypted vault")
		cs.Secrets = nil
		v1.settings.unstored = true
	default:
		v1.settings.checkVault(cs.secrets)
	}

	// And the deletions go last of all. Apply walks this slice in order and
	// stops at the first failure, so removing requirements.txt before the
	// manifest that replaces it means a run dying in between has taken the
	// dependencies away and put nothing in their place. Failing the other way
	// round leaves the project carrying both, which a person can sort out.
	//
	// That state does make a rerun refuse, since a manifest carrying
	// [tool.astro] is what ErrAlreadyAstroProject tests. Refusing over a project
	// whose dependencies are intact is the better half of the trade.
	for _, name := range planRetirements(v1,
		slices.Concat(v1.notes, pin.migrationNotes, leftsMayRetire), cs.AirflowVersion) {
		label := name + " (migrated into " + manifest.Marker + ", removed)"
		if name == SettingsRelPath {
			switch {
			case len(cs.Secrets) > 0:
				label = name + " (migrated into the encrypted vault and " + manifest.Marker + ", removed)"
			case !v1.settings.declares() && len(v1.settings.pools.byName) == 0:
				label = name + " (nothing to carry, removed)"
			}
		}
		cs.Changes = append(cs.Changes, Change{
			Kind:   Delete,
			Path:   name,
			Labels: []string{label},
		})
	}

	// Not in Notes, and not merely because planRetirements matches filenames
	// against that slice — though it does, so an env var called "Dockerfile"
	// would keep the project's Dockerfile alive if these went there. They are
	// the opposite KIND of statement: Notes is work outstanding, an advisory is
	// a change already made.
	cs.Advisories = append(cs.Advisories, v1.settings.carriedAdvisories()...)
	cs.Advisories = append(cs.Advisories, v1.settings.pools.advisories...)
	if a := v1.deployLinkAdvisory(); a != "" {
		cs.Advisories = append(cs.Advisories, a)
	}

	// The project said what it was called and this run did not use that. An
	// advisory rather than a note for the usual reason: nothing is left to do,
	// it is already named that.
	if pin.nameAdvisory != "" {
		cs.Advisories = append(cs.Advisories, pin.nameAdvisory)
	}

	// Created, Updated and Deleted are derived from the changes rather than
	// appended beside them, so a change that is performed but unreported — or
	// reported but not performed — cannot be constructed.
	cs.report()

	// An ADVISORY, not an entry in Updated.
	//
	// Reporting it is not optional: a preview showing only file changes shows a
	// manifest full of required declarations and says nothing about where their
	// values went, and what this describes is a credential leaving the project
	// for a keychain. Advisories are the list for "something this run did that
	// you would not otherwise see", and they are the list every consumer
	// renders — `astro init` prints them, and the app's conversion preview
	// shows them beside the file changes.
	//
	// Updated is the wrong home twice over: it is derived from Changes by
	// report(), so an appended line has to dodge that rebuild, and a vault write
	// is not a Change and never appears there. Nothing in the app read it.
	if len(cs.Secrets) > 0 {
		stored := slices.DeleteFunc(slices.Clone(cs.Secrets), func(w SecretWrite) bool {
			return slices.Contains(v1.settings.held, w.Name)
		})
		if len(stored) > 0 {
			cs.Advisories = append(cs.Advisories,
				valueCount(stored)+" from "+SettingsRelPath+": values stored in this machine's encrypted vault, "+
					"scoped to this project. "+manifest.Marker+" declares them without their values")
		}
		// When the file stays, the first line on its own reads as though the
		// values MOVED. They were copied, so the run says the plaintext original
		// is still on disk and why the file was kept.
		//
		// It states a fact and stops. It does not tell the reader to delete
		// the entries: when the vault already held a name, the file's copy is
		// the only one, and a connection is carried whenever it has a host,
		// schema, login, port or extra, not only a password — a warning that
		// fires on a hostname is one people stop reading.
		//
		// The names are spelled out rather than pronouned: every consumer renders
		// this list its own way, so a line whose "it" resolves against a
		// neighboring advisory dangles wherever the two are shown apart.
		if kept := v1.settings.keptFor(); kept != "" {
			cs.Advisories = append(cs.Advisories,
				SettingsRelPath+" still contains "+carriedNames(cs.Secrets)+" in plaintext, and is kept "+kept)
		}
	}
	return cs, nil
}

// planRetirements names the v1 files this run may delete.
//
// The rule is "carried, so removable", and the whole difficulty is that only
// this function is in a position to know. readV1Project sees what each file
// SAID; what reached the manifest is settled later, by mergeDependencies,
// renderPyproject and pickAirflowVersion, any of which can drop what it read.
//
// So the test is the run's own notes. Every path that fails to carry something
// writes one, and every one of them names the file it is about — that is a
// convention this now depends on, and the tests pin the cases. A file any note
// mentions is still the only record of whatever the note describes, so it stays.
//
// It is not a clever test and it does not need to be. It errs toward keeping,
// which is the direction to err: a file wrongly kept is untidy, a file wrongly
// deleted is gone.
func planRetirements(v1 *v1Project, notes []string, pinned string) []string {
	var out []string
	for _, name := range v1.present {
		if namedInAny(notes, name) {
			continue
		}
		if name == "Dockerfile" && !dockerfileIsSpent(v1, pinned) {
			continue
		}
		// airflow_settings.yaml is retired only when nothing in it stays
		// behind: no value the vault did not take and no pool the manifest
		// could not carry.
		//
		// Explicit, and not left to the notes that mention those cases: this
		// deletes files, a note is prose, and "is that string still in the
		// list" is not what should stand between a project's values and rm.
		if name == SettingsRelPath && !v1.settings.retirable() {
			continue
		}
		out = append(out, name)
	}
	// A DECLARED Dockerfile makes requirements.txt and packages.txt load-bearing
	// again, whether or not it names them.
	//
	// This is the bug the pin-only fix left behind. Declaring the file puts the
	// build into imagebuild's Dockerfile mode, where Dependencies and Packages
	// are ignored by design because the file decides what goes in — and if that
	// file is FROM an Astro runtime, the base's own ONBUILD `COPY
	// requirements.txt .` fires and reads the file from the build context. So an
	// entirely ordinary conversion (a Dockerfile with an ENV, plus the two v1
	// files) migrated both lists into the manifest, deleted both files, and left
	// a build that either fails on the missing COPY or produces an image with
	// none of the project's packages.
	//
	// Kept rather than un-migrated: the manifest lists stay, because a project
	// that later drops its Dockerfile needs them, and they cost nothing while the
	// declaration stands. The note below tells the user both exist.
	if declaresDockerfile(v1) {
		out = slices.DeleteFunc(out, func(name string) bool {
			return name == "requirements.txt" || name == "packages.txt"
		})
	}
	// A Dockerfile that SURVIVES may name the other files inside it, and a build
	// that reads a file this run deleted is broken in a way neither outcome
	// alone would be. `RUN pip install -r requirements.txt` is the common one,
	// and packages.txt is consumed the same way by the runtime image's ONBUILD
	// step, from the build context.
	if !slices.Contains(out, "Dockerfile") && len(v1.dockerfileBody) > 0 {
		body := string(v1.dockerfileBody)
		out = slices.DeleteFunc(out, func(name string) bool {
			return strings.Contains(body, name)
		})
	}
	return out
}

// dockerfileIsSpent reports a Dockerfile with nothing left to say: it names only
// a base image, and the version that image named is the one the manifest pinned.
//
// Both halves are needed and the second is easy to miss. pickAirflowVersion
// ranks an explicit --airflow-version, and an existing manifest pin, ABOVE the
// Dockerfile's tag. A run that took either of those and then deleted the
// Dockerfile would destroy the only record of a version the project actually
// built on, while the manifest claims a different one.
func dockerfileIsSpent(v1 *v1Project, pinned string) bool {
	return v1.dockerfilePinOnly && v1.airflow != "" && v1.airflow == pinned
}

// declaresDockerfile reports that this project's Dockerfile IS the build: it is
// present, and it does more than name a base image.
//
// Named because three places ask it — the declaration, and a label in each
// manifest arm — and because the whole point of separating it from
// dockerfileIsSpent was that they are different questions. Spelled out three
// times, any refinement (a file with only comments and a FROM, an ARG-only one)
// lands in some of them and the label and the declaration disagree.
func declaresDockerfile(v1 *v1Project) bool {
	return len(v1.dockerfileBody) > 0 && !v1.dockerfilePinOnly
}

// buildPython is the Python this project's image runs when its Dockerfile is
// the build and the base's tag names one, or "". A Dockerfile that is only a
// pin gives nothing here: the image is then generated from the runtime series
// and runs that runtime's default Python.
func (v1 *v1Project) buildPython() string {
	if !declaresDockerfile(v1) {
		return ""
	}
	return v1.basePython
}

func setV1Declarations(ed tomledit.Editor, v1 *v1Project) error {
	if err := setDockerfileDeclaration(ed, v1); err != nil {
		return err
	}
	if err := setEnvDeclarations(ed, &v1.settings); err != nil {
		return err
	}
	if err := setDeployLink(ed, v1); err != nil {
		return err
	}
	return setPools(ed, v1.settings.pools.byName)
}

// setDockerfileDeclaration records the project's own Dockerfile in the manifest,
// for a Dockerfile that IS the build. A no-op otherwise.
//
// The condition is that the file does more than name a base image, which is what
// dockerfilePinOnly already answers. It is deliberately NOT "the file survives
// this run", which is a different question that an earlier version of this
// conflated with it, via dockerfileIsSpent:
//
//   - dockerfileIsSpent answers "may this be DELETED", and its extra clauses are
//     about not destroying the only record of a version. A pin-only Dockerfile
//     survives whenever the pin came from somewhere else — an explicit
//     --airflow-version, or an existing manifest pin. Declaring that file made
//     imagebuild treat it as the build, and imagebuild ignores Dependencies and
//     Packages in that mode, so a conversion that had just migrated
//     requirements.txt into the manifest AND deleted it produced an image with
//     none of those packages in it. `FROM python:3.12-slim` is worse: no Airflow.
//   - planRetirements keeps a file for a further reason still — being named in
//     any note — so "kept" is broader than either. A pin-only Dockerfile kept by
//     an Airflow 2 tag note is on disk and undeclared, and that is the right
//     answer for it: it contributes nothing a generated image does not, and
//     declaring it would cost the project its dependencies.
//
// So the two decisions overlap and are not the same, and only this one is about
// what builds the image.
//
// The value is the filename rather than a discovered path because the v1 layout
// this converts from has exactly one place a Dockerfile can be. A project that
// wants its build somewhere else can say so by hand; the manifest accepts any
// path inside the project.
func setDockerfileDeclaration(ed tomledit.Editor, v1 *v1Project) error {
	if !declaresDockerfile(v1) {
		return nil
	}
	return ed.Set([]string{"tool", "astro", manifestKeyDockerfile}, fileDockerfile)
}

// namedInAny reports whether any note is about this file.
func namedInAny(notes []string, name string) bool {
	for _, n := range notes {
		if strings.Contains(n, name) {
			return true
		}
	}
	return false
}

// scaffoldManifest renders the manifest for a directory that has none, and
// records on the Result what it chose. It returns the manifest rather than
// writing it, so write puts every file on disk in one place.
func scaffoldManifest(dir string, opts Options, v1 *v1Project, res *Result) ([]byte, manifestFacts, error) {
	name, nameAdvisory := chooseName(dir, opts, v1)
	pick := pickAirflowVersion(opts.AirflowVersion, nil, v1, opts.Default)
	if opts.AirflowVersion != "" {
		if err := refuseKeptDockerfileOfAnotherAirflow(dir, v1, airflowRequirement(pick.version),
			"--airflow-version "+opts.AirflowVersion); err != nil {
			return nil, manifestFacts{}, err
		}
	}
	pyproject, notes, err := renderPyproject(name, pick, v1)
	if err != nil {
		return nil, manifestFacts{}, err
	}
	res.Name, res.AirflowVersion, res.AirflowDefaultSource = name, pick.version, pick.source
	return pyproject, manifestFacts{
		defaultedPin:   pick.defaulted(),
		migrationNotes: notes,
		migratedLabels: migratedLabels(v1),
		nameAdvisory:   nameAdvisory,
	}, nil
}

// migratedLabels describes what a greenfield manifest absorbed from the v1 files.
//
// Without this the common case said nothing. A real v1 project has no
// pyproject.toml, so it takes the greenfield arm, where Plan hardcodes the
// manifest's label to the filename — so `astro init` printed "pyproject.toml"
// and never mentioned that thirty requirement lines and a list of apt packages
// had just been moved into it. The rarer adopt arm did say so.
func migratedLabels(v1 *v1Project) []string {
	var out []string
	if n := len(v1.dependencies); n > 0 {
		out = append(out, manifest.Marker+" (migrated "+strconv.Itoa(n)+" from requirements.txt into dependencies)")
	}
	if len(v1.packages) > 0 {
		out = append(out, manifest.Marker+" (migrated packages.txt into packages)")
	}
	if v1.settings.declares() {
		out = append(out, manifest.Marker+" (migrated "+SettingsRelPath+" into [tool.astro.env])")
	}
	out = appendLabel(out, poolsLabel(v1.settings.pools.byName))

	if v1.airflow != "" {
		out = append(out, manifest.Marker+" (read Airflow "+v1.airflow+" from the Dockerfile)")
	}
	// The declaration is the one key here that changes what gets BUILT — with it
	// the project's own Dockerfile is the image, and the dependencies and
	// packages above stop describing it. Reporting the two migrations and not
	// this would show a preview whose most consequential line is missing, which
	// is the rule Plan's own comment states: a change performed but unreported
	// cannot be reviewed.
	if declaresDockerfile(v1) {
		out = append(out, manifest.Marker+" (declared "+fileDockerfile+" as this project's build)")
	}
	return out
}

// airflowPick is the Airflow a run settled on.
type airflowPick struct {
	version string
	// source says where a defaulted version came from, and is empty when a
	// flag or a pin stated it.
	source runtimeversions.Source
	// requiresPython is the catalog's bound for a defaulted version. Empty for
	// a stated one, and when the catalog lists no Python for the series.
	requiresPython string
}

// defaulted reports that nothing named an Airflow version.
func (p airflowPick) defaulted() bool { return p.source != "" }

// pythonBound is the requires-python a manifest this run writes gets: the
// minor a declared Dockerfile's base runs, when its tag names one; else the
// catalog's for a defaulted series when it has one, else the built-in rule.
//
// The image's minor is pinned whole rather than given a floor, because uv
// locks for every Python requires-python allows and, with no other hint, runs
// the newest it finds: a floor resolves for interpreters the image never runs,
// can fail on one of them, and can start standalone on another.
func (p airflowPick) pythonBound(v1 *v1Project) string {
	if python := v1.buildPython(); python != "" {
		return "==" + python + ".*"
	}
	if p.requiresPython != "" {
		return p.requiresPython
	}
	return requiresPython(p.version)
}

// pickAirflowVersion resolves the pin from every source that can state one, in
// precedence order, and reports whether the answer is only the default.
//
//	Options.AirflowVersion
//	  → an apache-airflow pin in the MANIFEST's [project.dependencies]
//	    → the Dockerfile's runtime tag
//	      → an apache-airflow pin in requirements.txt
//	        → Options.Default (the runtime catalog, its cache, or
//	          runtimeversions.FallbackAirflowSeries)
//
// The default is resolved last and only when reached, so a project that states
// its Airflow never causes a catalog request.
//
// Two of those orderings were wrong before, and both produced a project pinned a
// whole Airflow generation from where it actually was.
//
// The manifest's own pin now outranks the Dockerfile. Folding the Dockerfile
// into resolveAirflowVersion's flag slot put it above everything, so adopting a
// manifest pinning apache-airflow==2.9.1 in a directory with a stale
// runtime:3.1-12 Dockerfile wrote airflow = "3.1" beside a dependency list still
// saying 2.9.1 — a manifest contradicting itself, with the image built for one
// and the venv installing the other. Nothing cross-validates the two. A pin its
// author wrote in the manifest is the strongest statement short of an explicit
// flag.
//
// And requirements.txt is consulted on BOTH paths. The adopt arm passed the
// manifest's dependencies and never looked at v1's, so the same project answered
// differently depending on whether an unrelated pyproject.toml happened to
// exist: greenfield read the requirements pin, adopt defaulted and then dropped
// the pin during the merge.
//
// The caller's option stays on top, which is what lets Plan stay offline: an
// Airflow 2 tag names no minor, so a caller that wants the exact one resolves it
// through the release index and passes it here.
//
// The Dockerfile sits above a requirements.txt pin because the image tag is what
// the project runs today, while a pin in requirements.txt is what pip was asked
// to install INTO that image.
func pickAirflowVersion(flag string, manifestDeps []string, v1 *v1Project, def DefaultAirflow) airflowPick {
	if flag != "" {
		return airflowPick{version: flag}
	}
	if v, ok := pinFromDeps(manifestDeps); ok {
		return airflowPick{version: v}
	}
	if v1.airflow != "" {
		return airflowPick{version: v1.airflow}
	}
	if v, ok := pinFromDeps(v1.dependencies); ok {
		return airflowPick{version: v}
	}
	return defaultAirflow(def)
}

// defaultAirflow asks the caller's resolver, and answers the built-in series
// when there is none or it names nothing. With no resolver no lookup was
// tried, so the source says built-in rather than claiming a failed one.
func defaultAirflow(def DefaultAirflow) airflowPick {
	if def != nil {
		if series, rp, src := def(); series != "" {
			if src == "" {
				src = runtimeversions.SourceBuiltIn
			}
			return airflowPick{version: series, source: src, requiresPython: rp}
		}
	}
	return airflowPick{version: runtimeversions.FallbackAirflowSeries, source: runtimeversions.SourceBuiltIn}
}

// renderPyproject builds the greenfield manifest. It fills the template
// through the surgical editor, so the name and the dependencies are quoted the
// way the manifest expects, then round-trips through the same load check
// EditManifest applies (manifest.Parse, then envschema.ParseSchema over the
// carried declarations), so every scaffolded project is guaranteed to load.
// An invalid --name surfaces here as the manifest's own validation error; Plan
// refuses an invalid --airflow-version before this runs. [project.dependencies]
// leads with the requirement that states the Airflow version, the only place
// the manifest states it, so init → start needs no hand-edit.
func renderPyproject(name string, pick airflowPick, v1 *v1Project) (pyproject []byte, notes []string, err error) {
	version := pick.version
	tmpl := "[project]\n" +
		"name = 'astro-project'\n" +
		"version = '" + defaultProjectVersion + "'\n" +
		"requires-python = ''\n" +
		"dependencies = []\n\n" +
		"[tool.astro]\n"
	ed, err := tomledit.NewSurgical([]byte(tmpl))
	if err != nil {
		return nil, nil, err
	}
	if err := ed.Set([]string{"project", "name"}, name); err != nil {
		return nil, nil, err
	}
	if err := ed.Set([]string{"project", "requires-python"}, pick.pythonBound(v1)); err != nil {
		return nil, nil, err
	}
	// The Airflow requirement leads, then whatever requirements.txt carried,
	// deduplicated by distribution name the way the adopt arm does.
	//
	// The dedup is not tidiness. A requirements.txt naming one distribution
	// twice — "pandas==1.5.0" early and "pandas==2.1.0" later, or "Flask" and
	// "flask", which are the same PEP 503 name — produced two entries for it.
	// manifest.Parse accepts that, then uv intersects the specifiers and the
	// environment is unsatisfiable at the first start. Only the adopt arm
	// guarded against it, and greenfield is the arm a real v1 project takes.
	deps := []any{airflowRequirement(version)}
	seen := map[string]bool{}
	for _, d := range v1.dependencies {
		// An Airflow entry, apache-airflow or apache-airflow-core, is where
		// `version` came from, so the generated requirement above already
		// says it. Carrying a core entry beside it would state the version
		// twice, which the manifest refuses.
		if manifest.NamesAirflow(d) {
			notes = append(notes, airflowExtrasNote(d)...)
			continue
		}
		name := manifest.DistName(d)
		if seen[name] {
			// Every other duplicate is a specifier that exists in exactly one
			// place and is about to exist in none. requirements.txt permits two
			// entries for one distribution and pip intersects them; PEP 621 has
			// no such rule, so one of them has to go — but which one went is the
			// user's to know, and until this said so the losing pin was dropped
			// in silence and the file holding it was then retired.
			notes = append(notes, "requirements.txt: "+d+
				" names a distribution the manifest already lists, so this specifier was not carried")
			continue
		}
		seen[name] = true
		deps = append(deps, d)
	}
	if err := ed.Set([]string{"project", "dependencies"}, deps); err != nil {
		return nil, nil, err
	}
	if len(v1.packages) > 0 {
		if err := ed.Set([]string{"tool", "astro", "packages"}, asAny(v1.packages)); err != nil {
			return nil, nil, err
		}
	}
	if err := setV1Declarations(ed, v1); err != nil {
		return nil, nil, err
	}
	data, err := ed.Bytes()
	if err != nil {
		return nil, nil, err
	}
	if _, err := loadable(nil, data); err != nil {
		return nil, nil, err
	}
	return data, notes, nil
}

// planFiles works out the scaffold half of a run: the standard directories, the
// template files, the .env rule, and the CLAUDE.md symlink. It decides
// everything by reading and appends the operations to cs, writing nothing.
//
// The decisions were previously made as each file was written, which is why the
// preview did not exist: "does .gitignore already cover .env" was answered
// inside the call that healed it.
func planFiles(dir string, withSymlink bool, cs *Changeset) error {
	for _, d := range projectDirs {
		if _, err := os.Lstat(filepath.Join(dir, d)); err == nil {
			cs.Skipped = append(cs.Skipped, d+"/")
			continue
		}
		cs.Changes = append(cs.Changes, Change{Kind: CreateDir, Path: d, Labels: []string{d + "/"}})
	}

	files := []templateFile{
		{fileGitignore, gitignoreTemplate},
		{fileAgents, agentsContent},
	}
	// The starter DAG is for a project that has none, which is not the same as
	// a project that lacks a file called exampledag.py.
	//
	// Skip-existing answers the second question, and would drop an example into
	// a repo full of real pipelines just because nothing there happened to carry
	// that name. Adoption is the common case for init in an existing repo, so
	// that would be clutter in someone's dags directory far more often than it
	// would be a helpful first DAG.
	if starterDagSuits(cs.AirflowVersion) && projectHasNoDags(dir) {
		files = append(files, templateFile{fileExampleDag, exampleDag})
	}
	for _, f := range files {
		if _, err := os.Lstat(filepath.Join(dir, f.name)); err == nil {
			cs.Skipped = append(cs.Skipped, f.name)
			continue
		}
		cs.Changes = append(cs.Changes, Change{
			Kind: CreateFile, Path: f.name, Content: []byte(f.content), Labels: []string{f.name},
		})
	}

	// Local env values and per-machine .astro/ files must never be committed, so
	// a .gitignore lacking those rules gets them added. Computed here rather
	// than performed, so the bytes can be shown before they land.
	//
	// Run unconditionally, deliberately. Guarding it on "we are not writing the
	// template" reads like an optimization and is a dependency: it makes the
	// heal rely on gitignoreTemplate containing a .env line, so an edit to that
	// template would ship every new project with .env tracked by git and nothing
	// would notice. Unguarded, planIgnoreRules reads whatever is on disk — which
	// during Plan is still the pre-scaffold state — and returns nil when there
	// is nothing to do.
	healed, err := planIgnoreRules(dir)
	if err != nil {
		return err
	}
	if healed != nil {
		cs.Changes = append(cs.Changes, *healed)
	}

	if !withSymlink {
		return nil
	}
	if _, err := os.Lstat(filepath.Join(dir, fileClaude)); err == nil {
		cs.Skipped = append(cs.Skipped, fileClaude)
		return nil
	}
	cs.Changes = append(cs.Changes, Change{
		Kind: CreateSymlink, Path: fileClaude, Target: fileAgents,
		Labels: []string{fileClaude + " -> " + fileAgents},
	})
	return nil
}

// chooseName resolves a project's [project] name, and words the advisory when
// that is not the name the project gave for itself.
//
// Three sources, in this order: --name, because it is the only one somebody
// typed on purpose; the name .astro/config.yaml states, because a v1 project
// that calls itself orders-pipeline is called orders-pipeline whatever the
// directory it sits in happens to be; and the directory, which is all a
// greenfield scaffold has to go on.
//
// The middle one is the fix for a conversion that silently renamed a project.
// [project] name is the project's identity in the manifest: it is what
// `astro package` names its artifact after, what the env checklist reports
// against, and what `Created Astro project X` says. It is NOT where the
// hostname comes from — proxy.DeriveHostname reads the project DIRECTORY, and
// no caller passes it a manifest name — so a rename here does not move the URL
// Airflow answers on, and an advisory saying it did would send somebody
// looking for a host that does not exist.
//
// The advisory is worded here rather than by the caller because this is where
// the reason is known: respelled, or discarded entirely, are different sizes
// of surprise.
//
// Only the PROJECT's config is read, never the home one, though v1 resolved
// this key with a fallback to it. A global project.name would otherwise rename
// every project converted on that machine to the same thing, which is a worse
// answer than the directory in every case where the two differ.
func chooseName(dir string, opts Options, v1 *v1Project) (name, advisory string) {
	if opts.Name != "" {
		return opts.Name, ""
	}
	if v1 == nil || v1.projectName == "" {
		return deriveName(dir), ""
	}

	stated := v1.projectName
	legal := sanitizeName(stated)
	switch {
	case legal == stated:
		// Carried as written, which needs no comment.
		return legal, ""

	case legal != "":
		// Respelled. The reason is whatever sanitizeName had to change, and
		// listing the rules would be a lie for most names — it lowercases, and
		// maps every rune a [project] name cannot hold, which is most
		// punctuation and everything non-ASCII. So the advisory shows the two
		// names and lets them speak.
		return legal, "named " + legal + ", from " + stated + " in " + v1ConfigRelPath +
			": a [project] name holds lower-case letters, digits, and - _ . only"

	default:
		// Nothing a [project] name can hold survived, so the directory is no
		// worse — and this is the loudest case, not the quietest: the stated
		// name was discarded rather than respelled.
		fallback := deriveName(dir)
		return fallback, "named " + fallback + " after the directory: " + stated +
			" in " + v1ConfigRelPath + " has nothing a [project] name can hold, " +
			"which is lower-case letters, digits, and - _ ."
	}
}

// deriveName turns a directory basename into a valid [project] name, falling
// back to a fixed one when nothing legal survives.
func deriveName(dir string) string {
	if name := sanitizeName(filepath.Base(dir)); name != "" {
		return name
	}
	return "astro-project"
}

// sanitizeName turns a string into a valid [project] name (PEP 508): letters
// lower, invalid runes become "-", separators neither lead, trail, nor repeat.
// Empty when nothing legal is left, which the callers read as "no name here".
func sanitizeName(s string) string {
	var b strings.Builder
	sep := true // true also strips leading separators
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			b.WriteRune(r)
			sep = false
		case r == '-' || r == '_' || r == '.':
			if !sep {
				b.WriteRune(r)
				sep = true
			}
		default:
			if !sep {
				b.WriteByte('-')
				sep = true
			}
		}
	}
	return strings.TrimRight(b.String(), "-_.")
}

// leftovers reports what init found and cannot carry over on its own, with
// where each one belongs. Reading a Dockerfile means guessing what its RUN
// lines were for, so init names it and stops there. The list is the hand-off:
// what a person, or the agent working with them, does next.
// facts is a pointer only because the struct crossed gocritic's hugeParam
// threshold when it gained a field; nothing here writes through it.
// The second return is the same list minus any note built from a value out of
// a user's file, which is what planRetirements is given. That function decides
// deletions by substring-matching filenames against notes, so a saved deploy
// target called "Dockerfile" would otherwise keep this project's Dockerfile
// alive after the manifest had already taken its pin. The comment further down
// this file flags that hazard for environment variable names; this is the same
// one, arriving through a different door.
func leftovers(dir, version string, facts *manifestFacts, v1 *v1Project) (notes, forRetirement []string) {
	// requirements.txt, packages.txt, the Dockerfile, airflow_settings.yaml and
	// .astro/config.yaml are READ now, so none of them is matched on presence
	// here: whatever they could not carry is a note from the reader that says
	// which line and why, which is strictly better than "move its pins" about a
	// file that was mostly carried. Leaving the settings entry here told a user
	// to hand-move connections the same run had just carried for them, and
	// leaving the config entry here told nearly everyone to move a Deployment
	// their file did not name.
	// Every name printed below is slash-form, and joined only to look a file
	// up. A name reaches a json contract a consumer parses, so it has to read
	// the same on every platform: filepath.Join once reported
	// `.astro\config.yaml` on Windows while the other names in the same list
	// were slash-form, one contract disagreeing with itself.
	// v1ConfigRelPath, the only nested name emitted now, is that same kind of
	// constant and is concatenated rather than joined.
	var out []string
	// Reported on the target being there, never on the file being there. The
	// file is in every v1 project, `project.deployment` is in very few, and the
	// note keyed on the wrong one: it told most conversions to move a
	// Deployment their config did not name.
	//
	// Called a saved deploy target rather than a Deployment because the key
	// holds either. cmd/astro/deploy.go saves an Astro Deployment id here and
	// cmd/apc/deploy.go saves a Software release name, and nothing in the file
	// tells them apart, so the note says what to do with it if it is the first
	// rather than asserting that it is.
	var deployNote string
	// A target the conversion linked (setDeployLink) is carried, so it is an
	// advisory rather than a note; only one it could not link is left to do.
	if _, linked := v1.deployLink(); v1.deployment != "" && !linked {
		deployNote = v1ConfigRelPath + ": " + deployTargetNote(v1.deployment, v1.workspace)
		out = append(out, deployNote)
	}
	// Kept out of forRetirement for the reason deployNote is: planRetirements
	// matches file names as substrings of notes, and link names are the user's.
	var linksNote string
	if len(v1.instances) > 0 {
		linksNote = v1ConfigRelPath + ": " + instancesNote(v1.instances)
		out = append(out, linksNote)
	}
	checks := []struct{ file, note string }{
		{"docker-compose.yml", "not read — `astro local start` replaces it"},
		{"docker-compose.yaml", "not read — `astro local start` replaces it"},
		{"docker-compose.override.yml", "read in Docker mode — `astro local start --docker` merges it over the services it generates; standalone mode does not run it"},
		{"docker-compose.override.yaml", "not read — Docker mode merges docker-compose.override.yml, so rename it to use it there"},
	}
	for _, c := range checks {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(c.file))); err != nil {
			continue
		}
		out = append(out, c.file+": "+c.note)
	}
	// A pin nobody chose leads the list. Most repos state the Airflow they run
	// in a Dockerfile image tag, and most of those are on 2.x, so the default
	// is the likeliest way this ends up a project that cannot start.
	//
	// v1.statedVersion is now what gates this rather than the presence of a
	// Dockerfile or a requirements.txt. Presence used to stand in for "we did
	// not read it", and that is no longer true: a Dockerfile whose tag we read
	// leaves defaultedPin false, and one whose tag we could not read has
	// already said so in its own note. What is left for this warning is the
	// case where a file named a version and we still ended up defaulting.
	if facts.defaultedPin && v1.statedVersion {
		out = append([]string{"Airflow " + version + " is the default, not this project's version: " +
			"set the " + airflowRequirement(version) + " requirement in " + manifest.Marker +
			" to the Airflow this project already names"}, out...)
	}
	if facts.loosePython {
		out = append([]string{"[project] requires-python admits a Python that Airflow " + version +
			" cannot run: it needs " + requiresPython(version) + ", and yours has no upper bound — " +
			"uv will build the environment on the newest interpreter it allows"}, out...)
	}
	if deployNote == "" && linksNote == "" {
		return out, out
	}
	return out, slices.DeleteFunc(slices.Clone(out), func(n string) bool { return n == deployNote || n == linksNote })
}

// instancesNote names the deployment links .astro/config.yaml's `instances:`
// list holds and the command that links each one. A name or id that is not
// plain is not printed, for the reason deployTargetNote gives.
func instancesNote(instances []v1Instance) string {
	cmds := make([]string, 0, len(instances))
	for _, inst := range instances {
		name, id := "<name>", "<id>"
		if plainDeployID(inst.name) {
			name = inst.name
		}
		if plainDeployID(inst.deploymentID) {
			id = inst.deploymentID
		}
		cmd := "astro link add " + name + " --deployment " + id
		switch {
		case inst.source == "" || inst.source == "astro":
		case plainDeployID(inst.source):
			cmd = "astro link add " + name + " --target " + inst.source
		default:
			cmd = "astro link add " + name + " --target <platform>"
		}
		if !slices.Contains(cmds, "`"+cmd+"`") {
			cmds = append(cmds, "`"+cmd+"`")
		}
	}
	return "its `instances` list names deployment links this run did not carry into " + manifest.Marker +
		". Link each one with " + strings.Join(cmds, ", ")
}

// deployTargetNote says what .astro/config.yaml saved and what a manifest entry
// for it looks like, including the table NAME that entry needs. An earlier
// wording said only "under [tool.astro.deployments]"; written out literally
// that is `deployment = '...'` as a bare key of the deployments table, which
// manifest.Parse rejects with CodeExpectedTable. A hand-off that produces an
// unparseable manifest when followed is worse than no hand-off.
//
// The value is printed only when it is a plain id. planRetirements aside, a
// value carrying a newline would put a second, unindented line under "Left to
// do:" and a multi-line string into the json notes array a consumer groups by
// the file name each note starts with.
func deployTargetNote(deployment, workspace string) string {
	const entry = ". If that is an Astro Deployment, give it a name under " +
		"[tool.astro.deployments], say [tool.astro.deployments.prod], with "
	if !plainDeployID(deployment) {
		return "project.deployment holds this project's saved deploy target" + entry +
			"deployment set to it and the workspace it lives in"
	}
	if plainDeployID(workspace) {
		return deployment + " in workspace " + workspace + " is this project's saved deploy target" +
			entry + "deployment = '" + deployment + "' and workspace = '" + workspace + "'"
	}
	return deployment + " is this project's saved deploy target" + entry +
		"deployment = '" + deployment + "' and the workspace it lives in"
}

// plainDeployID reports a value safe to read at the end of a note: one line, no
// spaces, and short. Both shapes this key holds qualify, an Astro Deployment
// cuid and a Software release name.
func plainDeployID(s string) bool {
	if s == "" || len(s) > 96 {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
		default:
			return false
		}
	}
	return true
}

// pronoun is "it" for one thing and "them" for several.
func pronoun(n int) string {
	if n == 1 {
		return "it"
	}
	return "them"
}

// carriedNames lists the names a run stored, as prose.
func carriedNames(writes []SecretWrite) string {
	names := make([]string, 0, len(writes))
	for _, w := range writes {
		names = append(names, w.Name)
	}
	return joinNames(names)
}

// joinNames lists names as prose: "warehouse", "orders and warehouse",
// "billing, orders and warehouse". Sorted, because the names come from a map
// walk and an advisory that reorders itself between runs looks like a change.
func joinNames(names []string) string {
	names = slices.Sorted(slices.Values(names))
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}
