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
	// RestartNeeded reports that this project's Airflow is running on the old
	// pin. Nothing here restarts it.
	RestartNeeded bool `json:"restartNeeded,omitempty"`
}

func newUpgradeCmd(c *cli) *cobra.Command {
	cmd := &cobra.Command{
		Use:                        "upgrade",
		Short:                      "Move this project to a new version of what it runs",
		Args:                       cobra.ArbitraryArgs,
		RunE:                       helpOrUnknownSubcommand,
		SuggestionsMinimumDistance: 2,
	}
	cmd.AddCommand(newUpgradeAirflowCmd(c))
	return cmd
}

func newUpgradeAirflowCmd(c *cli) *cobra.Command {
	var withOtto bool
	cmd := &cobra.Command{
		Use:   "airflow [version]",
		Short: "Move this project to a new Airflow version",
		Long: "Set this project's Airflow version in pyproject.toml: the apache-airflow requirement,\n" +
			"plus requires-python and the runtime pin when they have to move. With no version, use the\n" +
			"newest Airflow the runtime catalog offers in the project's own Airflow generation. It never\n" +
			"moves an Airflow 2 project to Airflow 3. It says when Airflow 3 is available.\n\n" +
			"Dag code, providers and a declared Dockerfile are not changed. If Airflow is running,\n" +
			"restart it afterwards.",
		Example: "  astro local upgrade airflow\n  astro local upgrade airflow 3.1\n  astro local upgrade airflow --with-otto",
		Args:    cobra.MaximumNArgs(1),
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
	r, err := c.renderer()
	if err != nil {
		return err
	}
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
		fmt.Fprintf(c.d.Stderr, "Error: %v\nStarting Otto to make the upgrade instead.\n", err)
		return c.d.LaunchOtto(scaffold.AirflowUpgradeFallbackPrompt(currentPin(dir), version, err.Error(), cliUpgradePrompt))
	}
	res := airflowUpgrade{
		AirflowPinChange: change,
		Manifest:         filepath.Join(dir, manifest.Marker),
		Target:           target,
		Available:        available,
		RestartNeeded:    change.Changed && c.airflowRunning(dir),
	}
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
			"Pass a version instead, like `astro local upgrade airflow 3.1`")
	}
	if pin == "" {
		return "", "", fmt.Errorf("could not read the Airflow version %s pins, so there is nothing to upgrade from. "+
			"Pass a version instead, like `astro local upgrade airflow 3.1`", manifest.Marker)
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

// airflowRunning reports whether the project's Airflow is up, or coming up. A
// status that cannot be read says nothing either way, and reads as not.
func (c *cli) airflowRunning(dir string) bool {
	st, err := c.d.Runtime.ReadStatus(dir)
	if err != nil {
		return false
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
		lines = append(lines, fmt.Sprintf("Airflow is already pinned to %s. Nothing changed.", res.Version))
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
		lines = append(lines, fmt.Sprintf("Airflow is still running the old version. Run `%s` when the project is ready.", replaceRestart))
	}
	_, err := io.WriteString(w, strings.Join(lines, "\n")+"\n")
	return err
}
