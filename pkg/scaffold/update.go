package scaffold

import (
	"bytes"
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
//     build now refuses, moved onto its -base tag as a conversion would
//     (updateDockerfile);
//   - a declared Dockerfile an Astro CLI preview moved onto -base, which runs
//     the install helper those previews wrote, given the install steps this
//     astro writes, and the helper removed (updateStaleHelper);
//   - a declared Dockerfile whose install steps an older astro wrote, given
//     this astro's (updateInstallBlock);
//   - the ignore file a moved Dockerfile's context needs (updateDockerignore).
//
// Each is a step that adds Changes to the changeset, so a later one is a new
// function in updateSteps. What it does not do is decide anything about the
// manifest: the [tool.uv] lines astro keeps there are refreshed by the command
// after Run, as on a first init.
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
	u := &update{dir: abs, m: m, cs: cs, dockerfile: m.Astro.Dockerfile}
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

// updateDockerfile moves a declared Dockerfile still on an ONBUILD tag onto its
// -base tag, under the gates a conversion applies (dockerfileRewriteOf), plus
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
		u.cs.Notes = append(u.cs.Notes, onBuildKeptNote(u.dockerfile+": kept FROM "+rw.from+" rather than moved onto "+rw.base+
			", because the lists above are not all in "+manifest.Marker+". Once they are, run astro init again"))
		return nil
	}
	u.cs.Changes = append(u.cs.Changes, Change{
		Kind: UpdateFile, Path: u.dockerfile, Content: rw.body,
		Labels: []string{u.dockerfile + " (now builds FROM " + rw.base + " and installs the packages the Astro CLI resolves from " +
			manifest.Marker + " and uv.lock; build it with astro package or astro deploy, or astro package --context-dir for docker build)"},
	})
	for _, name := range lists {
		u.cs.Changes = append(u.cs.Changes, Change{
			Kind: Delete, Path: name,
			Labels: []string{name + " (every line is in " + manifest.Marker + ", and nothing reads it now; removed)"},
		})
	}
	if rw.mirror {
		u.cs.Advisories = append(u.cs.Advisories, u.dockerfile+" now builds FROM "+rw.base+
			": if your registry mirrors "+rw.from+" tag by tag, mirror "+rw.base+" too")
	}
	u.body = rw.body
	return nil
}

// onBuildKeptNote is a note about a Dockerfile left on its ONBUILD tag,
// ending with what that means for every build until it moves.
func onBuildKeptNote(note string) string {
	return note + ". Until it builds FROM a -base tag, each build of it warns, and installs requirements.txt and packages.txt " +
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
// project (staleHelperPath) in a stage of the file's own, and the CLI resolves
// the install before the build now (airflowrt.InstallDockerfile). The file's
// install block is replaced with this astro's, its own steps kept, and the
// helper removed (planStaleHelperRemoval) once nothing runs it. A file whose
// block this cannot find is left as it is, with a note: its build still runs
// the helper, so the helper stays too.
func updateStaleHelper(u *update) error {
	if u.body != nil && runsStaleHelper(u.body) {
		body, ok := previewInstallReplaced(u.body)
		if !ok {
			u.cs.Notes = append(u.cs.Notes, u.dockerfile+" runs "+staleHelperPath+", which an earlier Astro CLI preview wrote, "+
				"and astro init cannot find the install steps it wrote around it. Replace them with the ones in "+
				"docs/manifest-reference.md (Your own Dockerfile) and run astro init again, which then removes "+staleHelperPath)
			return nil
		}
		u.cs.Changes = append(u.cs.Changes, Change{
			Kind: UpdateFile, Path: u.dockerfile, Content: body,
			Labels: []string{u.dockerfile + " (install steps replaced with the ones this astro writes: the Astro CLI resolves the packages " +
				"before the build now, and nothing runs " + staleHelperPath + ")"},
		})
		u.body = body
	}
	return planStaleHelperRemoval(u.dir, u.cs)
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

// previewInstallReplaced is a Dockerfile an Astro CLI preview moved onto -base,
// data, with the install block that preview wrote replaced by this astro's.
// The block runs from the comment every preview began it with
// (previewBlockPrefix) to the comment ahead of the file's own steps
// (ownStepsComment), or to the end; what is before and after stays. The final
// stage keeps its reference, flags and name. ok is false when the block is
// not there to find, or the final stage is not on a runtime -base tag.
func previewInstallReplaced(data []byte) (body []byte, ok bool) {
	stages := airflowrt.DockerfileStages(data)
	if len(stages) == 0 {
		return nil, false
	}
	final := stages[len(stages)-1]
	if !strings.HasSuffix(final.Tag, "-base") || !airflowrt.IsAstroRuntimeRef(final.Image, final.Tag) {
		return nil, false
	}
	lines := strings.Split(string(data), "\n")
	start := slices.IndexFunc(lines, func(l string) bool { return strings.HasPrefix(l, previewBlockPrefix) })
	if start < 0 || start > final.Line {
		return nil, false
	}
	end := slices.IndexFunc(lines[final.Line:], func(l string) bool { return l+"\n" == ownStepsComment })
	if end < 0 {
		// No steps of the file's own: the block runs to the end.
		end = len(lines)
	} else {
		end += final.Line
	}
	from := strings.Join(append(append([]string{}, final.Flags...), final.Ref), " ")
	var b strings.Builder
	if start > 0 {
		b.WriteString(strings.Join(lines[:start], "\n") + "\n")
	}
	b.WriteString(installBlockComment)
	b.WriteString(airflowrt.InstallDockerfileWith(from, airflowrt.InstallOptions{Alias: final.Alias, ListsFrom: airflowrt.InstallContext}))
	if end < len(lines) {
		b.WriteString("\n" + strings.Join(lines[end:], "\n"))
	}
	out := b.String()
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return []byte(out), true
}

// updateInstallBlock brings the install steps of a Dockerfile that reads the
// lists from the astro-install context up to the ones this astro writes, when
// an older astro wrote them: the lists it reads have grown (the routed
// install's two files), and a file that copies fewer than this astro writes
// installs fewer than the CLI resolved. Only the block is replaced
// (installBlockRefreshed); a file whose block this cannot find is left alone.
func updateInstallBlock(u *update) error {
	if u.body == nil || runsStaleHelper(u.body) || !airflowrt.ReadsInstallContext(u.body) {
		return nil
	}
	body, ok := installBlockRefreshed(u.body)
	if !ok || bytes.Equal(body, u.body) {
		return nil
	}
	u.cs.Changes = append(u.cs.Changes, Change{
		Kind: UpdateFile, Path: u.dockerfile, Content: body,
		Labels: []string{u.dockerfile + " (install steps updated to the ones this astro writes)"},
	})
	u.body = body
	return nil
}

// installBlockRefreshed is a Dockerfile an astro rewrite wrote, data, with its
// install block written again. The block runs from its comment
// (previewBlockPrefix) through the last line every version of the steps ends
// with (installBlockLastLine), so whatever follows, the file's own steps
// whether or not their comment is still there, stays as it is. The final
// stage keeps its reference, flags and name. ok is false when the block is
// not there to find, or the final stage is not on a runtime -base tag.
func installBlockRefreshed(data []byte) (body []byte, ok bool) {
	stages := airflowrt.DockerfileStages(data)
	if len(stages) == 0 {
		return nil, false
	}
	final := stages[len(stages)-1]
	if !strings.HasSuffix(final.Tag, "-base") || !airflowrt.IsAstroRuntimeRef(final.Image, final.Tag) {
		return nil, false
	}
	lines := strings.Split(string(data), "\n")
	start := slices.IndexFunc(lines, func(l string) bool { return strings.HasPrefix(l, previewBlockPrefix) })
	if start < 0 || start > final.Line {
		return nil, false
	}
	last := slices.IndexFunc(lines[final.Line:], func(l string) bool { return strings.TrimSpace(l) == installBlockLastLine })
	if last < 0 {
		return nil, false
	}
	end := final.Line + last + 1
	from := strings.Join(append(append([]string{}, final.Flags...), final.Ref), " ")
	var b strings.Builder
	if start > 0 {
		b.WriteString(strings.Join(lines[:start], "\n") + "\n")
	}
	b.WriteString(installBlockComment)
	b.WriteString(airflowrt.InstallDockerfileWith(from, airflowrt.InstallOptions{Alias: final.Alias, ListsFrom: airflowrt.InstallContext}))
	b.WriteString(strings.Join(lines[end:], "\n"))
	return []byte(b.String()), true
}

// installBlockLastLine is the line every version of the install steps ends
// with (airflowrt.InstallDockerfileWith).
const installBlockLastLine = "COPY --chown=astro:0 . ."

// previewBlockPrefix is how every rewrite has begun its install block's
// comment, an Astro CLI preview's and this astro's (installBlockComment).
const previewBlockPrefix = "# Installs this project's packages from pyproject.toml and uv.lock"

// updateDockerignore keeps local secrets and per-machine files out of the
// context of a declared Dockerfile that installs from the lists the CLI
// resolves, as a conversion does for the file it moves (planDockerignore).
func updateDockerignore(u *update) error {
	if u.body == nil || !airflowrt.ReadsInstallContext(u.body) {
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
