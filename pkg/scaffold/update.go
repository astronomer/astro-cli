package scaffold

import (
	"bytes"
	"errors"
	"fmt"
	"os"
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
//   - the install helper a Dockerfile on -base runs, written when missing and
//     refreshed when this astro's differs (updateInstallHelper).
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

// updateSteps are run in order. The Dockerfile goes first, since whether the
// helper is needed depends on what it builds FROM once the run is done.
var updateSteps = []updateStep{updateDockerfile, updateInstallHelper}

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
	if u.body == nil {
		return nil
	}
	rw, blocker, _ := dockerfileRewriteOf(u.body)
	if blocker != "" {
		u.cs.Notes = append(u.cs.Notes, onBuildRefusalNote(blocker))
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
		u.cs.Notes = append(u.cs.Notes, onBuildRefusalNote(u.dockerfile+": kept FROM "+rw.from+" rather than moved onto "+rw.base+
			", because the lists above are not all in "+manifest.Marker+". Once they are, run astro init again"))
		return nil
	}
	// The helper the moved file runs has to be committable, as in a
	// conversion (planHelperCommittable).
	switch blocker, err := planHelperCommittable(u.dir, u.cs); {
	case err != nil:
		return err
	case blocker != "":
		u.cs.Notes = append(u.cs.Notes, onBuildRefusalNote(blocker))
		return nil
	}
	u.cs.Changes = append(u.cs.Changes, Change{
		Kind: UpdateFile, Path: u.dockerfile, Content: rw.body,
		Labels: []string{u.dockerfile + " (now builds FROM " + rw.base + " and installs this project's packages from " +
			manifest.Marker + " and uv.lock, so docker build . and astro deploy build the same image)"},
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

// onBuildRefusalNote is a note about a Dockerfile left on its ONBUILD tag,
// ending with what that means now: every build refuses it.
func onBuildRefusalNote(note string) string {
	return note + ". Until it builds FROM a -base tag, astro local start, deploy and package refuse it"
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
		for _, d := range deps {
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

// updateInstallHelper writes the install helper a Dockerfile on -base runs, or
// refreshes it when this astro's differs, and keeps what the install reads in
// the ignore file's context. A Dockerfile that does not run the helper gets
// neither.
func updateInstallHelper(u *update) error {
	if u.body == nil || !bytes.Contains(u.body, []byte(airflowrt.InstallHelperPath)) {
		return nil
	}
	helper := airflowrt.InstallHelper()
	switch got, err := readIfPresent(filepath.Join(u.dir, filepath.FromSlash(airflowrt.InstallHelperPath))); {
	case err != nil:
		return err
	case got == nil:
		u.cs.Changes = append(u.cs.Changes, Change{
			Kind: CreateFile, Path: airflowrt.InstallHelperPath, Content: helper,
			Labels: []string{airflowrt.InstallHelperPath + " (reads " + manifest.Marker + " and uv.lock inside the build)"},
		})
	case !bytes.Equal(got, helper):
		u.cs.Changes = append(u.cs.Changes, Change{
			Kind: UpdateFile, Path: airflowrt.InstallHelperPath, Content: helper,
			Labels: []string{airflowrt.InstallHelperPath + " (refreshed to this astro's version)"},
		})
	}
	ignore, err := planDockerignore(u.dir, u.dockerfile, installInputs)
	if err != nil {
		return err
	}
	if ignore != nil {
		u.cs.Changes = append(u.cs.Changes, *ignore)
	}
	return nil
}

// InstallHelperWarning is the warning for a build of the project's declared
// Dockerfile, dockerfile (the manifest's value), when that file runs the
// install helper and the project's copy of it is missing or is not the one
// this astro writes, or "" when there is nothing to say. dir is the project
// root.
//
// A warning and not a refusal: a copy someone edited on purpose is theirs to
// keep, and an older astro's helper still installs. But the copy is what the
// image installs with, whichever astro builds it, so a stale one is worth a
// line naming the command that refreshes it.
func InstallHelperWarning(dir, dockerfile string) string {
	if dockerfile == "" {
		return ""
	}
	body, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(dockerfile)))
	if err != nil || !bytes.Contains(body, []byte(airflowrt.InstallHelperPath)) {
		return ""
	}
	got, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(airflowrt.InstallHelperPath)))
	switch {
	case errors.Is(err, os.ErrNotExist):
		return dockerfile + " runs " + airflowrt.InstallHelperPath + ", which this project does not have, so the build will fail. Run astro init to write it"
	case err != nil:
		return ""
	case !bytes.Equal(got, airflowrt.InstallHelper()):
		return airflowrt.InstallHelperPath + ", which " + dockerfile + " runs to install this project's packages, is not the one this astro writes. Run astro init to refresh it"
	}
	return ""
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
