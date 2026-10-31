package scaffold

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/astronomer/astro-cli/pkg/airflowrt"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/manifest/tomledit"
)

// `astro init` in a directory that is already an Astro project brings the
// parts of it astro writes up to date, and touches nothing else: no scaffold,
// no manifest edit, no 1.x conversion. Those parts are the ones a newer astro
// can change under a project that an older one converted:
//
//   - a declared Dockerfile still on an Astro Runtime ONBUILD tag, which every
//     build now refuses, moved onto the install base as a conversion would
//     (updateDockerfile);
//   - a declared Dockerfile an Astro CLI preview moved onto -base, which runs
//     the install helper those previews wrote, moved onto the install base,
//     and the helper removed (updateStaleHelper);
//   - a declared Dockerfile whose install steps an older astro wrote out, on
//     a -base tag reading the astro-install build context, moved onto the
//     install base (updateInstallBlock);
//   - the ignore file a moved Dockerfile's context needs (updateDockerignore).
//
// Each is a step that adds Changes to the changeset, so a later one is a new
// function in updateSteps. A Dockerfile moved onto the install base no longer
// names its runtime, so the move records it in the manifest as a conversion
// does ([tool.astro] runtime, registry, requires-python; recordRuntime). The
// [tool.uv] lines astro keeps there are refreshed by the command after Run, as
// on a first init.
//
// It runs only when the caller asks (Options.UpdateExisting): a caller that
// asks nothing gets ErrAlreadyAstroProject, as before, so Astro Desktop, which
// reads that error as "already converted", is not handed a changeset it never
// asked for.

// updateStep is one part of bringing an existing project up to date.
type updateStep func(u *update) error

// updateSteps are run in order. The Dockerfile goes first, since what the
// others do depends on what it builds FROM once the run is done.
var updateSteps = []updateStep{updateDockerfile, updateStaleHelper, updateInstallBlock, updateDockerignore}

// update is the state the steps share.
type update struct {
	dir string
	m   *manifest.Manifest
	cs  *Changeset
	// dockerfile is the declared Dockerfile's path relative to dir, slash
	// form, and body its bytes as the run leaves them: rewritten, when
	// updateDockerfile moved it.
	dockerfile string
	body       []byte
	// manifest is the manifest's bytes, and manifestLabels what the run
	// changed in them (recordRuntime); none, and no change, when it changed
	// nothing.
	manifest       []byte
	manifestLabels []string
}

// recordRuntime records in the manifest the runtime a Dockerfile moved onto
// the install base named in its FROM, since the file names none any more: the
// one build as [tool.astro] runtime, a mirror as registry, and a Python
// flavor as requires-python, each only when the manifest does not say it
// already.
func (u *update) recordRuntime(rw *dockerfileRewrite) error {
	ed, err := tomledit.NewSurgical(u.manifest)
	if err != nil {
		return fmt.Errorf("reading %s: %w", manifest.Marker, err)
	}
	set := func(key []string, value, label string) error {
		if current, ok := ed.Get(key); ok && current == value {
			return nil
		}
		if err := ed.Set(key, value); err != nil {
			return err
		}
		u.manifestLabels = append(u.manifestLabels, label)
		return nil
	}
	if rw.runtime != "" && u.m.Astro.Runtime == "" {
		if err := set(runtimePath, rw.runtime, "[tool.astro] runtime = '"+rw.runtime+"', the build "+u.dockerfile+" named"); err != nil {
			return err
		}
	}
	if rw.registry != "" {
		if err := set(registryPath, rw.registry, "[tool.astro] registry = '"+rw.registry+"', the mirror "+u.dockerfile+" named"); err != nil {
			return err
		}
	}
	if rw.python != "" {
		if err := set([]string{"project", "requires-python"}, "=="+rw.python+".*", "requires-python = '=="+rw.python+".*', the Python "+u.dockerfile+" named"); err != nil {
			return err
		}
	}
	u.manifest, err = ed.Bytes()
	return err
}

// planExisting is plan for a directory that is already an Astro project, when
// the caller asked for it to be brought up to date: ok reports that it was one
// and the answer is cs, err. Otherwise ok is false and plan goes on, to refuse
// it in the adopt arm as it always has.
func planExisting(abs, marker string, opts *Options) (cs *Changeset, ok bool, err error) {
	if !opts.UpdateExisting {
		return nil, false, nil
	}
	data, err := os.ReadFile(marker)
	if err != nil || !carriesAstroSection(data) {
		return nil, false, nil //nolint:nilerr // no readable manifest is the other arms' case, and theirs to report
	}
	if opts.Name != "" || opts.AirflowVersion != "" {
		return nil, true, fmt.Errorf("%s %w, so init only brings what astro writes in it up to date: "+
			"change the name or the Airflow version in its %s instead", abs, ErrAlreadyAstroProject, manifest.Marker)
	}
	cs, err = planUpdate(abs, data)
	return cs, true, err
}

// planUpdate is plan for a directory whose manifest already carries
// [tool.astro], data its bytes.
func planUpdate(abs string, data []byte) (*Changeset, error) {
	m, err := manifest.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", filepath.Join(abs, manifest.Marker), err)
	}
	cs := &Changeset{Result: Result{Dir: abs, Name: m.Project.Name, AirflowVersion: m.Airflow().Pin, Existing: true}}
	u := &update{dir: abs, m: m, cs: cs, dockerfile: m.Astro.Dockerfile, manifest: data}
	if u.dockerfile != "" {
		u.body, err = os.ReadFile(filepath.Join(abs, filepath.FromSlash(u.dockerfile))) //nolint:gosec // G703: manifest.Parse refuses a dockerfile path that leaves the project
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("reading %s: %w", u.dockerfile, err)
		}
	}
	for _, step := range updateSteps {
		if err := step(u); err != nil {
			return nil, err
		}
	}
	if len(u.manifestLabels) > 0 {
		cs.Changes = append(cs.Changes, Change{
			Kind: UpdateFile, Path: manifest.Marker, Content: u.manifest,
			Labels: []string{manifest.Marker + " (" + strings.Join(u.manifestLabels, "; ") + ")"},
		})
	}
	// u.body is the file as the run leaves it, rewritten when it moved.
	cs.OnBuildKept = u.body != nil && slices.ContainsFunc(airflowrt.DockerfileStages(u.body),
		func(st airflowrt.DockerfileStage) bool { return st.OnBuildRuntime() })
	// Deletions last, as in a conversion: a run that dies part-way has moved
	// the Dockerfile and kept the lists, which a person can sort out.
	slices.SortStableFunc(cs.Changes, func(a, b Change) int {
		return boolInt(a.Kind == Delete) - boolInt(b.Kind == Delete)
	})
	cs.report()
	return cs, nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// updateDockerfile moves a declared Dockerfile still on an ONBUILD tag onto the
// install base, under the gates a conversion applies (dockerfileRewriteOf), plus
// one a conversion has no need of: the 1.x lists the ONBUILD steps read have
// had all this time to drift from pyproject.toml, so each line of them has to
// be in it already, or the move would drop what the image installs today.
func updateDockerfile(u *update) error {
	// Only a file with a stage on an ONBUILD tag, which the build refuses
	// (CheckDockerfileOnBuild): one whose earlier stage merely might be,
	// built from a build argument, builds as it is.
	if u.body == nil || !slices.ContainsFunc(airflowrt.DockerfileStages(u.body),
		func(st airflowrt.DockerfileStage) bool { return st.OnBuildRuntime() }) {
		return nil
	}
	rw, blocker, _ := dockerfileRewriteOf(u.dockerfile, u.body)
	if blocker != "" {
		u.cs.Notes = append(u.cs.Notes, onBuildKeptNote(blocker))
		return nil
	}
	if rw == nil {
		return nil
	}
	lists, notes, err := listsCarried(u.dir, u.m)
	if err != nil {
		return err
	}
	if len(notes) > 0 {
		u.cs.Notes = append(u.cs.Notes, notes...)
		u.cs.Notes = append(u.cs.Notes, onBuildKeptNote(u.dockerfile+": kept FROM "+rw.from+" rather than moved onto the install base the Astro CLI builds"+
			", because the lists above are not all in "+manifest.Marker+". Once they are, run astro init again"))
		return nil
	}
	if err := u.recordRuntime(rw); err != nil {
		return err
	}
	u.cs.Changes = append(u.cs.Changes, Change{
		Kind: UpdateFile, Path: u.dockerfile, Content: rw.body,
		Labels: []string{u.dockerfile + " (now builds on the install base the Astro CLI makes from " + manifest.Marker +
			" and uv.lock; build it with astro package or astro deploy, or astro package --context-dir for docker build)"},
	})
	for _, name := range lists {
		u.cs.Changes = append(u.cs.Changes, Change{
			Kind: Delete, Path: name,
			Labels: []string{name + " (every line is in " + manifest.Marker + ", and nothing reads it now; removed)"},
		})
	}
	u.adviseMirror(rw)
	u.body = rw.body
	return nil
}

// adviseMirror says, for a Dockerfile moved onto the install base from a
// mirror, that the mirror has to carry the -base tag the base is built on.
func (u *update) adviseMirror(rw *dockerfileRewrite) {
	if rw.registry != "" {
		u.cs.Advisories = append(u.cs.Advisories, manifest.Marker+" records your mirror as [tool.astro] registry = '"+rw.registry+
			"': the install base builds FROM the runtime's -base tag there, so if your registry mirrors "+rw.from+" tag by tag, mirror its -base tag too")
	}
}

// onBuildKeptNote is a note about a Dockerfile left on its ONBUILD tag,
// ending with what that means for every build until it moves.
func onBuildKeptNote(note string) string {
	return note + ". Until it builds on the install base, each build of it warns, and installs requirements.txt and packages.txt " +
		"through that tag's ONBUILD steps rather than what " + manifest.Marker + " lists"
}

// onBuildFixable reports whether astro init, run in the project, would move
// its declared Dockerfile off an ONBUILD tag: the update step, run against a
// changeset nothing applies. When it would not, notes are what it would say
// instead.
func onBuildFixable(dir string, m *manifest.Manifest) (notes []string, fixable bool, err error) {
	u := &update{dir: dir, m: m, cs: &Changeset{}, dockerfile: m.Astro.Dockerfile}
	if u.body, err = os.ReadFile(filepath.Join(dir, filepath.FromSlash(u.dockerfile))); err != nil {
		return nil, false, nil //nolint:nilerr // a file that cannot be read is the build's to report
	}
	if err := updateDockerfile(u); err != nil {
		return nil, false, err
	}
	fixable = slices.ContainsFunc(u.cs.Changes, func(c Change) bool { return c.Path == u.dockerfile && c.Kind == UpdateFile })
	return u.cs.Notes, fixable, nil
}

// listsCarried reports the 1.x lists beside the project, requirements.txt and
// packages.txt, that are present, and notes for each line of them the
// manifest does not already hold. A line is held when [project] dependencies
// has it as written, whitespace aside, or [tool.astro] packages has the
// package.
func listsCarried(dir string, m *manifest.Manifest) (present, notes []string, err error) {
	have := map[string]bool{}
	for _, d := range m.Project.Dependencies {
		have[strings.Join(strings.Fields(d), "")] = true
	}
	if data, err := readIfPresent(filepath.Join(dir, "requirements.txt")); err != nil {
		return nil, nil, err
	} else if data != nil {
		present = append(present, "requirements.txt")
		deps, uncarried := parseRequirements(data)
		notes = append(notes, uncarried...)
		// Held to what a conversion carries (mergeDependencies): not a line
		// naming Airflow, which becomes the pin, and not a later line for a
		// distribution the file already listed, which a conversion drops.
		// Neither is in [project] dependencies as written, and neither ever
		// could be, so counting them would keep the file where it is forever.
		seen := map[string]bool{}
		for _, d := range deps {
			name := manifest.DistName(d)
			if manifest.NamesAirflow(d) || seen[name] {
				continue
			}
			seen[name] = true
			if !have[strings.Join(strings.Fields(d), "")] {
				notes = append(notes, "requirements.txt: "+d+" is not in [project] dependencies in "+manifest.Marker+
					", which is what the image installs once the Dockerfile moves. Add it there, or take it out of requirements.txt")
			}
		}
	}
	if data, err := readIfPresent(filepath.Join(dir, "packages.txt")); err != nil {
		return nil, nil, err
	} else if data != nil {
		present = append(present, "packages.txt")
		for _, p := range parsePackages(data) {
			if !slices.Contains(m.Astro.Packages, p) {
				notes = append(notes, "packages.txt: "+p+" is not in packages under [tool.astro] in "+manifest.Marker+
					". Add it there, or take it out of packages.txt")
			}
		}
	}
	return present, notes, nil
}

// updateStaleHelper brings a Dockerfile an Astro CLI preview moved onto -base
// up to date: those previews ran the install helper they wrote into the
// project (staleHelperPath) in a stage of the file's own, and the CLI builds
// the install itself now, into the install base. The file's install block is
// replaced with the FROM the install base takes, its own steps kept, its
// runtime recorded in the manifest, and the helper removed
// (planStaleHelperRemoval) once nothing runs it. A file whose block this
// cannot find is left as it is, with a note: its build still runs the helper,
// so the helper stays too.
func updateStaleHelper(u *update) error {
	if u.body != nil && runsStaleHelper(u.body) {
		rw, ok := previewBlockReplaced(u.body, true)
		if !ok {
			u.cs.Notes = append(u.cs.Notes, u.dockerfile+" runs "+staleHelperPath+", which an earlier Astro CLI preview wrote, "+
				"and astro init cannot find the install steps it wrote around it. Replace them with the lines in "+
				"docs/manifest-reference.md (Your own Dockerfile) and run astro init again, which then removes "+staleHelperPath)
			return nil
		}
		if err := u.moveOntoInstallBase(rw, " (moved onto the install base the Astro CLI makes from "+manifest.Marker+
			" and uv.lock: nothing runs "+staleHelperPath+" now)"); err != nil {
			return err
		}
	}
	return planStaleHelperRemoval(u.dir, u.cs)
}

// moveOntoInstallBase applies rw, a Dockerfile moved onto the install base by
// an update step: the file, with label, and its runtime in the manifest.
func (u *update) moveOntoInstallBase(rw *dockerfileRewrite, label string) error {
	if err := u.recordRuntime(rw); err != nil {
		return err
	}
	u.cs.Changes = append(u.cs.Changes, Change{Kind: UpdateFile, Path: u.dockerfile, Content: rw.body, Labels: []string{u.dockerfile + label}})
	u.adviseMirror(rw)
	u.body = rw.body
	return nil
}

// runsStaleHelper reports a Dockerfile, data, with an instruction naming the
// install helper an Astro CLI preview wrote. Comments are not read.
func runsStaleHelper(data []byte) bool {
	name := path.Base(staleHelperPath)
	for _, line := range strings.Split(string(data), "\n") {
		if t := strings.TrimSpace(line); t != "" && !strings.HasPrefix(t, "#") && strings.Contains(t, name) {
			return true
		}
	}
	return false
}

// previewBlockReplaced is a Dockerfile an older astro moved onto a runtime
// -base tag, data, with the install block it wrote there replaced by the FROM
// the install base takes (rewrittenBody), and what the manifest has to record
// of the tag it named. The block runs from the comment every such astro began
// it with (previewBlockPrefix) to the end of its steps: through the last line
// every version of the steps ends with (installBlockLastLine), or, for a
// preview that ran the helper (helper), to the comment ahead of the file's own
// steps (ownStepsComment), or to the end. What is before and after stays, and
// the final stage keeps its flags and name. ok is false when the block is not
// there to find, or the final stage is not on a runtime -base tag pyproject.toml
// can record.
func previewBlockReplaced(data []byte, helper bool) (rw *dockerfileRewrite, ok bool) {
	stages := airflowrt.DockerfileStages(data)
	if len(stages) == 0 {
		return nil, false
	}
	final := stages[len(stages)-1]
	if !strings.HasSuffix(final.Tag, "-base") || !airflowrt.IsAstroRuntimeRef(final.Image, final.Tag) {
		return nil, false
	}
	recorded, blocker := installBaseRuntimeOf(&final, "")
	if blocker != "" {
		return nil, false
	}
	lines := strings.Split(string(data), "\n")
	start := slices.IndexFunc(lines, func(l string) bool { return strings.HasPrefix(l, previewBlockPrefix) })
	if start < 0 || start > final.Line {
		return nil, false
	}
	end := len(lines)
	switch last := slices.IndexFunc(lines[final.Line:], func(l string) bool { return strings.TrimSpace(l) == installBlockLastLine }); {
	case last >= 0:
		end = final.Line + last + 1
	case !helper:
		return nil, false
	default:
		if own := slices.IndexFunc(lines[final.Line:], func(l string) bool { return l+"\n" == ownStepsComment }); own >= 0 {
			end = final.Line + own
		}
	}
	recorded.body = rewrittenBody(lines, &final, start, end)
	return &recorded, true
}

// updateInstallBlock moves a Dockerfile whose install steps an older astro
// wrote out, over a runtime -base tag and reading the lists from the
// astro-install build context, onto the install base, which builds those
// steps itself: only the block is replaced (previewBlockReplaced), the file's
// own steps kept, and its runtime recorded in the manifest. A file whose
// block this cannot find is left alone, with a note.
func updateInstallBlock(u *update) error {
	if u.body == nil || runsStaleHelper(u.body) || !readsAstroInstall(u.body) {
		return nil
	}
	rw, ok := previewBlockReplaced(u.body, false)
	if !ok {
		u.cs.Notes = append(u.cs.Notes, u.dockerfile+" reads its package lists from the astro-install build context, which the Astro CLI "+
			"no longer passes, and astro init cannot find the install steps an earlier astro wrote around it. Replace them with the lines "+
			"in docs/manifest-reference.md (Your own Dockerfile)")
		return nil
	}
	return u.moveOntoInstallBase(rw, " (moved onto the install base the Astro CLI makes from "+manifest.Marker+
		" and uv.lock, which installs what its steps did)")
}

// readsAstroInstall reports a Dockerfile, data, with an instruction copying
// from the astro-install build context an earlier astro passed. Comments are
// not read.
func readsAstroInstall(data []byte) bool {
	for _, line := range strings.Split(string(data), "\n") {
		if t := strings.TrimSpace(line); t != "" && !strings.HasPrefix(t, "#") && strings.Contains(t, "--from=astro-install") {
			return true
		}
	}
	return false
}

// installBlockLastLine is the line every version of the install steps an
// older astro wrote out ends with.
const installBlockLastLine = "COPY --chown=astro:0 . ."

// previewBlockPrefix is how every older astro began the comment of the install
// block it wrote out.
const previewBlockPrefix = "# Installs this project's packages from pyproject.toml and uv.lock"

// updateDockerignore keeps local secrets and per-machine files out of the
// context of a declared Dockerfile built on the install base, as a
// conversion does for the file it moves (planDockerignore).
func updateDockerignore(u *update) error {
	if u.body == nil || !airflowrt.UsesInstallBase(u.body) {
		return nil
	}
	ignore, err := planDockerignore(u.dir, u.dockerfile)
	if err != nil || ignore == nil {
		return err
	}
	u.cs.Changes = append(u.cs.Changes, *ignore)
	return nil
}

// StaleHelperWarning is the warning for a build of the project's declared
// Dockerfile, dockerfile (the manifest's value), when that file still runs the
// install helper an Astro CLI preview wrote, or "" when it does not. dir is
// the project root. The build goes ahead: the helper, while the project has
// it, still installs. But astro init replaces it with the install this astro
// resolves, so the warning names that command.
func StaleHelperWarning(dir, dockerfile string) string {
	if dockerfile == "" {
		return ""
	}
	body, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(dockerfile)))
	if err != nil || !runsStaleHelper(body) {
		return ""
	}
	return dockerfile + " runs " + staleHelperPath + ", which an earlier Astro CLI preview wrote. Run astro init to replace it with the install this astro resolves"
}

// carriesAstroSection reports a manifest with a [tool.astro] table, the one
// shape a first init refuses.
func carriesAstroSection(data []byte) bool {
	ed, err := tomledit.NewSurgical(data)
	if err != nil {
		return false
	}
	_, ok := ed.Get([]string{"tool", "astro"})
	return ok
}
