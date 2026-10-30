package local

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/pkg/localrt"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/runtimeversions"
	"github.com/astronomer/astro-cli/pkg/scaffold"
)

const flagWithOtto = "with-otto"

// errUnsupportedAirflow reports a version naming an Airflow generation no
// local runtime runs.
var errUnsupportedAirflow = errors.New("unsupported Airflow version")

// cliUpgradePrompt is what the upgrade prompt says that is the CLI's own: who
// made the edit, and how Otto brings Airflow up afterwards, which from a
// terminal is the CLI's commands rather than the desktop's tools.
var cliUpgradePrompt = scaffold.UpgradePromptOptions{
	Editor:  "The Astro CLI",
	BringUp: "`" + replaceRestart + "` if it is running, `" + replaceStart + "` if it is stopped",
}

// airflowUpgrade is `astro local upgrade airflow --output json`: what the pin
// change did, where, and whether a running Airflow needs a restart onto it.
type airflowUpgrade struct {
	scaffold.AirflowPinChange
	// Manifest is the pyproject.toml that was edited.
	Manifest string `json:"manifest"`
	// Target is the version picked from the runtime catalog when none was
	// given. Empty when the caller named the version.
	Target string `json:"target,omitempty"`
	// Available is the next generation's Airflow, when no version was given and
	// the catalog offers one. A bare upgrade never moves to it.
	Available string `json:"available,omitempty"`
	// Refresh is what followed the pin into [tool.uv] and uv.lock.
	Refresh pinRefresh `json:"refresh"`
	// RestartNeeded reports that this project's Airflow is running on the old
	// pin. Nothing here restarts it.
	RestartNeeded bool `json:"restart_needed,omitempty"`
}

// runtimeUpgrade is `astro local upgrade runtime --output json`.
type runtimeUpgrade struct {
	scaffold.RuntimeBuildChange
	// Manifest is the pyproject.toml that was edited.
	Manifest string `json:"manifest"`
	// Target is the build picked from the runtime catalog when none was given.
	// Empty when the caller named the build.
	Target string `json:"target,omitempty"`
	// Findings are what the runtime catalog says about the build that the
	// manifest checks warn about too: yanked, unlisted, or carrying another
	// patch than an exact requirement pins.
	Findings []runtimeFinding `json:"findings,omitempty"`
	// Refresh is what followed the build into [tool.uv] and uv.lock.
	Refresh pinRefresh `json:"refresh"`
	// RestartNeeded reports that this project's Airflow is running on the old
	// build. Nothing here restarts it.
	RestartNeeded bool `json:"restart_needed,omitempty"`
}

// pinRefresh is what an upgrade brought up to date after it moved the pin, as
// `astro local init` does after it writes one: the [tool.uv] settings that hold
// the project to its runtime build (writeAstroBuild), then uv.lock. So a deploy
// or `astro package` straight after sees the new runtime without a local start.
type pinRefresh struct {
	// Constraints is the runtime build whose constraints [tool.uv] now holds,
	// and ConstraintsRemoved the one whose it no longer holds, when either moved.
	Constraints        string `json:"constraints,omitempty"`
	ConstraintsRemoved string `json:"constraints_removed,omitempty"`
	// AstroAirflow is the Astronomer build of Airflow [tool.uv] now installs,
	// when it moved; AstroAirflowRemoved reports that Airflow now comes from
	// PyPI.
	AstroAirflow        string `json:"astro_airflow,omitempty"`
	AstroAirflowRemoved bool   `json:"astro_airflow_removed,omitempty"`
	// Lock is what `uv lock` did. Absent when this build cannot lock.
	Lock *lockOutcome `json:"lock,omitempty"`
	// Warnings says what could not be refreshed, and why: offline, the index
	// or the runtime's constraints could not be read, or uv lock failed.
	Warnings []string `json:"warnings,omitempty"`
}

// refreshAfterUpgrade is pinRefresh's work for the project in dir. It never
// fails the upgrade, which has done its part: what it could not do is said.
func (c *cli) refreshAfterUpgrade(ctx context.Context, dir string) pinRefresh {
	w := c.writeAstroBuild(ctx, dir)
	out := pinRefresh{Constraints: w.runtime, ConstraintsRemoved: w.runtimeGone}
	if w.changed && w.airflowChanged {
		out.AstroAirflow, out.AstroAirflowRemoved = w.build.Airflow, w.build.Airflow == ""
	}
	switch {
	case errors.Is(w.lookupErr, runtimeversions.ErrNoAstroBuild):
		out.Warnings = append(out.Warnings, fmt.Sprintf(
			"Airflow %s comes from PyPI: Astronomer's index has no build of it, so the environment may differ from a deployment's", w.pin))
	case w.lookupErr != nil && w.changed:
		out.Warnings = append(out.Warnings, fmt.Sprintf(
			"Airflow %s comes from PyPI for now: Astronomer's index could not be read for the new build: %v", w.pin, w.lookupErr))
	case w.lookupErr != nil:
		out.Warnings = append(out.Warnings, fmt.Sprintf(
			"[tool.uv] was not refreshed, so its Airflow build and runtime constraints may still be the previous runtime's, "+
				"and uv.lock was locked against them: Astronomer's index could not be read: %v. Run this command again once it can be", w.lookupErr))
	}
	if w.constraintsErr != nil {
		out.Warnings = append(out.Warnings, fmt.Sprintf(
			"the runtime's constraints in [tool.uv] were not refreshed: %v. Run this command again once they can be", w.constraintsErr))
	}
	if w.writeErr != nil {
		out.Warnings = append(out.Warnings, fmt.Sprintf("could not update %s for Astronomer's build of Airflow: %v", manifest.Marker, w.writeErr))
	}
	if lock := c.relock(ctx, dir); lock.Status != "" {
		out.Lock = &lock
		if lock.Status == lockFailed || (lock.Status == lockNoUV && lock.hadLock) {
			out.Warnings = append(out.Warnings, lock.text())
		}
	}
	return out
}

// changed reports that the refresh rewrote [tool.uv] or uv.lock, which an
// upgrade that left the pin alone can still do: a new build shipped, or an
// older CLI moved the pin without refreshing.
func (p pinRefresh) changed() bool {
	return p.Constraints != "" || p.ConstraintsRemoved != "" || p.AstroAirflow != "" || p.AstroAirflowRemoved ||
		(p.Lock != nil && (p.Lock.Status == lockLocked || p.Lock.Status == lockUpdated))
}

// nothingChanged ends the line an upgrade that kept the pin says: nothing
// changed, unless the refresh changed something, whose lines follow.
func nothingChanged(p pinRefresh) string {
	if p.changed() {
		return " Refreshed what follows from it:"
	}
	return " Nothing changed."
}

// lines is what a refresh says in text: what moved, never what stayed.
// Warnings are said apart, on stderr.
func (p pinRefresh) lines() []string {
	var lines []string
	if p.Constraints != "" {
		lines = append(lines, "  [tool.uv]: now holds what Astro Runtime "+p.Constraints+" constrains")
	}
	if p.ConstraintsRemoved != "" {
		lines = append(lines, "  [tool.uv]: no longer holds what Astro Runtime "+p.ConstraintsRemoved+" constrains")
	}
	switch {
	case p.AstroAirflow != "":
		lines = append(lines, "  [tool.uv]: now installs Astronomer's Airflow "+p.AstroAirflow)
	case p.AstroAirflowRemoved:
		lines = append(lines, "  [tool.uv]: no longer points Airflow at Astronomer's index")
	}
	if p.Lock != nil && p.Lock.Status != lockFailed && p.Lock.Status != lockNoUV {
		if t := p.Lock.text(); t != "" {
			lines = append(lines, "  "+t)
		}
	}
	return lines
}

// warnRefresh says on stderr what a refresh could not do. In json mode the
// result carries it instead.
func (c *cli) warnRefresh(r cliout.Renderer, p pinRefresh) {
	if r.Format == cliout.FormatJSON {
		return
	}
	for _, w := range p.Warnings {
		cliout.Warn(c.d.Stderr, w)
	}
}

func newUpgradeCmd(c *cli) *cobra.Command {
	cmd := &cobra.Command{
		Use:                        "upgrade",
		Short:                      "Move this project to a new version of what it runs",
		Args:                       cobra.ArbitraryArgs,
		RunE:                       helpOrUnknownSubcommand,
		SuggestionsMinimumDistance: 2,
	}
	cmd.AddCommand(newUpgradeAirflowCmd(c), newUpgradeRuntimeCmd(c))
	return cmd
}

const flagPython = "python"

func newUpgradeRuntimeCmd(c *cli) *cobra.Command {
	var python string
	cmd := &cobra.Command{
		Use:   "runtime [BUILD]",
		Short: "Move this project to an Astro Runtime build",
		Long: "Set this project's Astro Runtime build in pyproject.toml ([tool.astro] runtime), and move the " +
			"apache-airflow requirement too when the build carries an Airflow it does not cover. Older builds " +
			"are allowed, to match what a Deployment runs. With no build, use the newest build of the project's " +
			"Airflow series in the runtime catalog.\n\n" +
			"--python picks the Python the image and the standalone environment run, through requires-python. " +
			"Without it, the build's default Python is used.\n\n" +
			"Afterwards the runtime's constraints in [tool.uv] are refreshed and uv.lock is relocked, so a deploy " +
			"or astro package sees the new build without a local start. If Airflow is running, restart it.",
		Example: "  # Move to the newest build of the project's Airflow series\n" +
			"  astro local upgrade runtime\n\n" +
			"  # Move to a given build, older ones included\n" +
			"  astro local upgrade runtime 3.3-5\n\n" +
			"  # Move to a build and run it on Python 3.12\n" +
			"  astro local upgrade runtime 3.3-10 --python 3.12",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			build := ""
			if len(args) == 1 {
				build = args[0]
			}
			return c.runUpgradeRuntime(cmd.Context(), build, python)
		},
	}
	cmd.Flags().StringVar(&python, flagPython, "", "Python version to run, like 3.12. Default: the build's default Python")
	return cmd
}

func (c *cli) runUpgradeRuntime(ctx context.Context, build, python string) error {
	r := c.renderer()
	dir, err := c.projectPath()
	if err != nil {
		return err
	}
	opts := scaffold.AirflowPinOptions{}
	if c.d.RuntimeCatalog != nil {
		opts.Catalog = c.d.RuntimeCatalog(ctx)
	}
	target := ""
	if build == "" {
		if build, err = pickRuntimeTarget(opts.Catalog, currentPin(dir), currentRuntime(dir)); err != nil {
			return err
		}
		target = build
	}
	var change scaffold.RuntimeBuildChange
	if _, err := watchManifest(dir, func(wrap func(run func() error) error) error {
		change, err = scaffold.SetRuntimeBuildWith(dir, wrap, build, python, opts)
		return err
	}); err != nil {
		return fmt.Errorf("setting the runtime build: %w", err)
	}
	res := runtimeUpgrade{
		RuntimeBuildChange: change,
		Manifest:           filepath.Join(dir, manifest.Marker),
		Target:             target,
		Findings:           runtimeFindings(opts.Catalog, change.Runtime, currentPin(dir)),
		Refresh:            c.refreshAfterUpgrade(ctx, dir),
	}
	res.RestartNeeded = (change.Changed || res.Refresh.changed()) && c.airflowRunning(dir)
	if r.Format != cliout.FormatJSON {
		for _, f := range res.Findings {
			cliout.Warn(c.d.Stderr, f.Message)
		}
	}
	c.warnRefresh(r, res.Refresh)
	return r.Emit(res, func(w io.Writer) error { return renderRuntimeUpgrade(w, res) })
}

// pickRuntimeTarget is the build an upgrade with none given moves to: the
// newest published build of the series pin names (of its generation, for a
// pin naming only that). Published, so the build has an image to pull. A
// current build of the series newer than that (one the catalog dates ahead,
// or a cached copy predates) stays: an upgrade never moves a project back.
func pickRuntimeTarget(catalog *runtimeversions.Catalog, pin, current string) (string, error) {
	if catalog == nil {
		return "", errors.New("could not read the runtime catalog to pick the newest build. " +
			"Pass a build instead, like astro local upgrade runtime 3.3-10")
	}
	if pin == "" {
		return "", fmt.Errorf("could not read the Airflow version %s pins, so there is no series to pick a build of. "+
			"Pass a build instead, like astro local upgrade runtime 3.3-10", manifest.Marker)
	}
	series := manifest.Airflow{Pin: pin}.Series()
	build, ok := catalog.NewestPublishedRuntimeFor(series)
	if !ok {
		return "", fmt.Errorf("the runtime catalog lists no build of Airflow %s", series)
	}
	if t, ok := manifest.ParseRuntimeTag(current); ok && t.Build && t.Agrees(series) &&
		runtimeversions.CompareRuntimes(current, build) > 0 {
		return current, nil
	}
	return build, nil
}

// currentRuntime is the [tool.astro] runtime the manifest in dir names, read
// as for a repair. Empty when it names none or cannot be read.
func currentRuntime(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, manifest.Marker))
	if err != nil {
		return ""
	}
	m, err := manifest.ParseForRepair(data)
	if err != nil {
		return ""
	}
	return m.Astro.Runtime
}

// runtimeFindings is what the catalog says about build, as an image build's
// runtime check says it (runtimeversions.Catalog.CheckRuntime, nil catalog
// included: offline an Airflow 3 build's tag already said its series).
func runtimeFindings(catalog *runtimeversions.Catalog, build, pin string) []runtimeFinding {
	var out []runtimeFinding
	for _, f := range catalog.CheckRuntime(build, pin) {
		out = append(out, runtimeFinding{
			Kind: string(f.Kind), Message: f.Message, AirflowVersion: f.AirflowVersion,
			YankedReason: f.YankedReason, Suggested: f.Suggested,
		})
	}
	return out
}

// runtimeFinding is a runtimeversions.Finding as the CLI's json says it, in
// snake_case. Runtime and AirflowPin are the result's own runtime and pin, and
// Blocking never reaches here: the pin moves with the build, so the two agree.
type runtimeFinding struct {
	Kind           string `json:"kind"`
	Message        string `json:"message"`
	AirflowVersion string `json:"airflow_version,omitempty"`
	YankedReason   string `json:"yanked_reason,omitempty"`
	Suggested      string `json:"suggested,omitempty"`
}

func renderRuntimeUpgrade(w io.Writer, res runtimeUpgrade) error {
	var lines []string
	if res.Target != "" && res.Changed {
		lines = append(lines, fmt.Sprintf("Upgrading to %s (the newest build of the project's Airflow series in the runtime catalog)", res.Target))
	}
	if !res.Changed {
		lines = append(lines, fmt.Sprintf("The runtime is already %s.%s", res.Runtime, nothingChanged(res.Refresh)))
	} else {
		from := res.Previous
		if from == "" {
			from = "no build"
		}
		lines = append(lines, fmt.Sprintf("Runtime: %s -> %s in %s", from, res.Runtime, res.Manifest))
		if res.AirflowVersion != "" {
			lines = append(lines, "  carries Airflow "+res.AirflowVersion)
		}
		if a := res.Airflow; a != nil {
			prev := a.Previous
			if prev == "" {
				prev = "no version"
			}
			lines = append(lines, fmt.Sprintf("  Airflow: %s -> %s", prev, a.Version))
			for _, req := range a.Requirements {
				lines = append(lines, "  requirement: "+req)
			}
		}
		if res.RequiresPython != "" {
			lines = append(lines, "  requires-python: "+res.RequiresPython)
		}
	}
	if res.Python != "" {
		lines = append(lines, "  Python: "+res.Python)
	}
	lines = append(lines, res.Refresh.lines()...)
	if res.Airflow != nil {
		lines = append(lines, "Not changed: providers and Dag code. Check them against the new Airflow.")
	}
	if res.RestartNeeded {
		lines = append(lines, fmt.Sprintf("Airflow is still running the old build. Run %s when the project is ready.", replaceRestart))
	}
	_, err := io.WriteString(w, strings.Join(lines, "\n")+"\n")
	return err
}

func newUpgradeAirflowCmd(c *cli) *cobra.Command {
	var withOtto bool
	cmd := &cobra.Command{
		Use:   "airflow [VERSION]",
		Short: "Move this project to a new Airflow version",
		Long: "Set this project's Airflow version in pyproject.toml: the apache-airflow requirement, " +
			"plus requires-python and the runtime pin when they have to move. With no version, use the " +
			"newest Airflow the runtime catalog offers in the project's own Airflow generation. It never " +
			"moves an Airflow 2 project to Airflow 3. It says when Airflow 3 is available.\n\n" +
			"Dag code, providers and a declared Dockerfile are not changed. If Airflow is running, " +
			"restart it afterwards.",
		Example: "  # Move to the newest Airflow in the project's generation\n" +
			"  astro local upgrade airflow\n\n" +
			"  # Move to a given version\n" +
			"  astro local upgrade airflow 3.1\n\n" +
			"  # Upgrade, then start Otto to update the Dags and providers for it\n" +
			"  astro local upgrade airflow --with-otto",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			version := ""
			if len(args) == 1 {
				version = args[0]
			}
			return c.runUpgradeAirflow(cmd.Context(), version, withOtto)
		},
	}
	cmd.Flags().BoolVar(&withOtto, flagWithOtto, false, "Start Otto afterwards to update Dags and providers for the new version")
	return cmd
}

func (c *cli) runUpgradeAirflow(ctx context.Context, version string, withOtto bool) error {
	r := c.renderer()
	if withOtto && r.Format == cliout.FormatJSON {
		return fmt.Errorf("--%s starts an interactive session, so it cannot be combined with --output json", flagWithOtto)
	}
	if withOtto && c.d.LaunchOtto == nil {
		return fmt.Errorf("--%s is not available in this build", flagWithOtto)
	}
	if version != "" {
		if err := checkSupportedAirflow(version); err != nil {
			return err
		}
	}
	dir, err := c.projectPath()
	if err != nil {
		return err
	}
	opts := scaffold.AirflowPinOptions{}
	if c.d.RuntimeCatalog != nil {
		opts.Catalog = c.d.RuntimeCatalog(ctx)
	}
	target, available := "", ""
	if version == "" {
		if version, available, err = pickAirflowTarget(opts.Catalog, currentPin(dir)); err != nil {
			return err
		}
		target = version
	}
	var change scaffold.AirflowPinChange
	if _, err := watchManifest(dir, func(wrap func(run func() error) error) error {
		change, err = scaffold.SetAirflowVersionWith(dir, wrap, version, opts)
		return err
	}); err != nil {
		err = fmt.Errorf("setting the Airflow version: %w", err)
		if !withOtto {
			return err
		}
		// The version passed the checks above, so the cause is the project:
		// a manifest that does not parse, or one the writer refuses. Otto
		// gets the whole job and the reason, as the desktop does.
		fmt.Fprintf(c.d.Stderr, "%s %v\nStarting Otto to make the upgrade instead.\n", cliout.ErrorLabel(c.d.Stderr), err)
		return c.d.LaunchOtto(scaffold.AirflowUpgradeFallbackPrompt(currentPin(dir), version, err.Error(), cliUpgradePrompt))
	}
	res := airflowUpgrade{
		AirflowPinChange: change,
		Manifest:         filepath.Join(dir, manifest.Marker),
		Target:           target,
		Available:        available,
		Refresh:          c.refreshAfterUpgrade(ctx, dir),
	}
	res.RestartNeeded = (change.Changed || res.Refresh.changed()) && c.airflowRunning(dir)
	c.warnRefresh(r, res.Refresh)
	if err := r.Emit(res, func(w io.Writer) error { return renderAirflowUpgrade(w, res) }); err != nil {
		return err
	}
	// A bare upgrade that stayed put because the only newer Airflow is the
	// next generation has nothing for Otto to do: that move is the user's to
	// ask for by name.
	if !withOtto || (!change.Changed && available != "") {
		return nil
	}
	return c.d.LaunchOtto(scaffold.AirflowUpgradePrompt(&change, cliUpgradePrompt))
}

// checkSupportedAirflow refuses a version no local runtime runs, before the
// manifest is read: the manifest's own rule accepts any major, while Docker
// mode and standalone run Airflow 2 and Airflow 3 only. The same rule as
// Astro Desktop's SetProjectAirflowPin.
func checkSupportedAirflow(version string) error {
	if !manifest.ValidAirflowVersion(version) {
		return fmt.Errorf("%w: %q is not a version like 3, 3.1, or 3.1.2", scaffold.ErrInvalidAirflowVersion, version)
	}
	switch (manifest.Airflow{Pin: version}).Major() {
	case "2", "3":
		return nil
	}
	return fmt.Errorf("%w: %q. Local Airflow runs Airflow 2 and Airflow 3", errUnsupportedAirflow, version)
}

// pickAirflowTarget is the version an upgrade with none given moves pin to:
// the same-generation offer of the runtime catalog's AirflowUpgradeTargets.
// A bare upgrade never crosses a generation, so the Airflow 3 offer an Airflow
// 2 pin also gets comes back as available, to name. A pin already on the
// newest of its generation is its own target, so the write reports no change.
func pickAirflowTarget(catalog *runtimeversions.Catalog, pin string) (target, available string, err error) {
	if catalog == nil {
		return "", "", errors.New("could not read the runtime catalog to pick the newest Airflow. " +
			"Pass a version instead, like astro local upgrade airflow 3.1")
	}
	if pin == "" {
		return "", "", fmt.Errorf("could not read the Airflow version %s pins, so there is nothing to upgrade from. "+
			"Pass a version instead, like astro local upgrade airflow 3.1", manifest.Marker)
	}
	sameGen, crossGen := catalog.AirflowUpgradeTargets(pin)
	if sameGen == "" {
		sameGen = pin
	}
	return sameGen, crossGen, nil
}

// availableHint names the Airflow 3 an Airflow 2 project is offered, the only
// cross-generation offer there is, and how to move to it.
func availableHint(version string) string {
	return "Airflow 3 is available: astro local upgrade airflow " + version
}

// currentPin is the Airflow version the manifest in dir pins, read as for a
// repair so a manifest the writer refused can still name it. Empty when it
// cannot be read.
func currentPin(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, manifest.Marker))
	if err != nil {
		return ""
	}
	m, err := manifest.ParseForRepair(data)
	if err != nil {
		return ""
	}
	return m.Airflow().Pin
}

// airflowRunning reports whether the project's Airflow is up, or coming up.
// A stopped project, recorded or not, arrives as StateStopped with a nil
// error, so any error means "unknown": the pin has already moved, so it warns
// and keeps the restart hint rather than fail the upgrade or drop it.
func (c *cli) airflowRunning(dir string) bool {
	st, err := c.d.Runtime.ReadStatus(dir)
	if err != nil {
		cliout.Warn(c.d.Stderr, fmt.Sprintf("could not tell whether Airflow is running (%s); restart it to apply", err))
		return true
	}
	return st.State == localrt.StateRunning || st.State == localrt.StateStarting
}

func renderAirflowUpgrade(w io.Writer, res airflowUpgrade) error {
	var lines []string
	if res.Target != "" && res.Changed {
		major, _, _ := strings.Cut(res.Target, ".")
		lines = append(lines, fmt.Sprintf("Upgrading to %s (the latest Airflow %s in the runtime catalog)", res.Target, major))
	}
	switch {
	case !res.Changed && res.Available != "":
		// Only an Airflow 2 pin is offered another generation.
		lines = append(lines, fmt.Sprintf("Airflow is already on the newest Airflow 2 (%s). %s", res.Version, availableHint(res.Available)))
	case !res.Changed:
		lines = append(lines, fmt.Sprintf("Airflow is already pinned to %s.%s", res.Version, nothingChanged(res.Refresh)))
	default:
		from := res.Previous
		if from == "" {
			from = "no version"
		}
		lines = append(lines, fmt.Sprintf("Airflow: %s -> %s in %s", from, res.Version, res.Manifest))
		for _, req := range res.Requirements {
			lines = append(lines, "  requirement: "+req)
		}
		if res.CoreReplaced {
			lines = append(lines, "  apache-airflow-core became apache-airflow, since core is published only for Airflow 3")
		}
		if res.RequiresPython != "" {
			lines = append(lines, "  requires-python: "+res.RequiresPython)
		}
		switch {
		case res.Runtime != "":
			lines = append(lines, "  tool.astro runtime: moved to "+res.Runtime)
		case res.RuntimeRemoved:
			lines = append(lines, "  tool.astro runtime: removed, so the image builds from the newest runtime of the new series")
		}
		if res.RemovedAirflowKey {
			lines = append(lines, "  tool.astro airflow: removed. Nothing reads it any more")
		}
	}
	lines = append(lines, res.Refresh.lines()...)
	if res.Dockerfile != "" {
		lines = append(lines, fmt.Sprintf("Not changed: %s. This project declares it, so its FROM line decides the image in Docker mode. "+
			"Update its base image for Airflow %s yourself.", res.Dockerfile, res.Version))
	}
	if res.Changed {
		lines = append(lines, "Not changed: providers and Dag code. Check them against the new version.")
	}
	if res.Changed && res.Available != "" {
		lines = append(lines, availableHint(res.Available))
	}
	if res.RestartNeeded {
		lines = append(lines, fmt.Sprintf("Airflow is still running the old version. Run %s when the project is ready.", replaceRestart))
	}
	_, err := io.WriteString(w, strings.Join(lines, "\n")+"\n")
	return err
}
