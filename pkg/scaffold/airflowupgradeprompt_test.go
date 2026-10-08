package scaffold

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// promptBase is the change the cases below vary, the same one Astro Desktop's
// own prompt tests start from.
func promptBase() AirflowPinChange {
	return AirflowPinChange{
		Previous:     "2.9",
		Version:      "3.1",
		Changed:      true,
		Requirements: []string{"apache-airflow==3.1.*"},
	}
}

// desktopPromptOptions is what the desktop says, so these cases assert the
// same text its own tests do.
var desktopPromptOptions = UpgradePromptOptions{
	Editor:  "Astro Desktop",
	BringUp: "airflow_restart if it is running, airflow_start if it is stopped",
}

func TestAirflowUpgradePromptSaysThePinIsWritten(t *testing.T) {
	p := AirflowUpgradePrompt(ptr(promptBase()), desktopPromptOptions)
	assert.True(t, strings.HasPrefix(p, "/skill:airflow-upgrade Upgrade my Airflow from 2.9 to 3.1."), p)
	assert.Contains(t, p, "Astro Desktop has already made the mechanical edit")
	assert.Contains(t, p, "now reads `apache-airflow==3.1.*` (it pinned `2.9`)")
	assert.Contains(t, p, "Do not change the Airflow requirement again.")
	assert.NotContains(t, p, "`airflow` pin")
	assert.NotContains(t, p, "Bump the pin")
	assert.Contains(t, p, "This project has no Dockerfile.")
	assert.Contains(t, p, "migrate any Dag code")
	assert.Contains(t, p,
		"and not before, bring Airflow up on 3.1: airflow_restart if it is running, airflow_start if it is stopped.")
	assert.NotContains(t, p, "has not been restarted")
}

func TestAirflowUpgradePromptNamesWhatMovedWithThePin(t *testing.T) {
	c := promptBase()
	c.Requirements = []string{"apache-airflow[celery]==3.1.*"}
	c.RequiresPython = ">=3.10,<3.14"
	p := AirflowUpgradePrompt(&c, desktopPromptOptions)
	assert.Contains(t, p, "now reads `apache-airflow[celery]==3.1.*`")
	assert.Contains(t, p, "`requires-python` under `[project]` was moved with it too and now reads `>=3.10,<3.14`")
	assert.NotContains(t, p, "apache-airflow-core")
	assert.NotContains(t, p, "was deleted")
}

func TestAirflowUpgradePromptListsEveryRequirement(t *testing.T) {
	c := promptBase()
	c.Requirements = []string{"apache-airflow==3.1.*", "apache-airflow[celery]==3.1.*; python_version >= '3.10'"}
	p := AirflowUpgradePrompt(&c, desktopPromptOptions)
	assert.Contains(t, p, "now reads `apache-airflow==3.1.*`, `apache-airflow[celery]==3.1.*; python_version >= '3.10'` (it pinned")
}

func TestAirflowUpgradePromptSaysWhatTheWriteRepaired(t *testing.T) {
	c := promptBase()
	c.Version, c.CoreReplaced, c.RemovedAirflowKey = "2.10", true, true
	p := AirflowUpgradePrompt(&c, desktopPromptOptions)
	assert.Contains(t, p, "so that entry became `apache-airflow` at the new version")
	assert.Contains(t, p, "The old `airflow` line under `[tool.astro]`, which nothing reads any more, was deleted.")
}

func TestAirflowUpgradePromptLeavesADockerfileToOtto(t *testing.T) {
	c := promptBase()
	c.Dockerfile = "docker/Dockerfile"
	p := AirflowUpgradePrompt(&c, desktopPromptOptions)
	assert.Contains(t, p, "`dockerfile = \"docker/Dockerfile\"`")
	assert.Contains(t, p, "its `FROM` line, not the requirement, decides the image")
	assert.Contains(t, p, "Update its base image and any build steps for Airflow 3.1.")
	assert.NotContains(t, p, "This project has no Dockerfile.")
}

func TestAirflowUpgradePromptSaysWhereTheRuntimeWent(t *testing.T) {
	moved := promptBase()
	moved.Version, moved.Runtime = "3.4", "3.4-2"
	assert.Contains(t, AirflowUpgradePrompt(&moved, desktopPromptOptions),
		"so it was moved to `3.4-2`, the newest build of the new one.")

	removed := promptBase()
	removed.Version, removed.RuntimeRemoved = "3.4", true
	assert.Contains(t, AirflowUpgradePrompt(&removed, desktopPromptOptions),
		"so it was deleted: the image now builds from the newest build of the new series.")

	assert.NotContains(t, AirflowUpgradePrompt(ptr(promptBase()), desktopPromptOptions), "`runtime` line")
}

func TestAirflowUpgradePromptClaimsNoEditItDidNotMake(t *testing.T) {
	c := promptBase()
	c.Previous, c.Changed = "3.1", false
	p := AirflowUpgradePrompt(&c, desktopPromptOptions)
	assert.Contains(t, p, "already pins `3.1`")
	assert.NotContains(t, p, "has already made the mechanical edit")
}

// The TS case "tolerates null lists from the bridge": a nil Requirements is
// what Go hands it.
func TestAirflowUpgradePromptToleratesNoRequirements(t *testing.T) {
	c := promptBase()
	c.Requirements = nil
	assert.Contains(t, AirflowUpgradePrompt(&c, desktopPromptOptions), "already pins `3.1`")
}

// What Astro Desktop's own prompt builder returns for the same changes,
// captured from it verbatim, so the two builders cannot drift apart a word at
// a time: every field set, then the other branch of each choice.
const (
	desktopPromptEveryField    = "/skill:airflow-upgrade Upgrade my Airflow from 2.9 to 3.4. Astro Desktop has already made the mechanical edit: the Airflow requirement in the `[project] dependencies` array of pyproject.toml now reads `apache-airflow==3.4.*`, `apache-airflow[celery]==3.4.*` (it pinned `2.9`). `apache-airflow-core` is published only for Airflow 3, so that entry became `apache-airflow` at the new version. The old `airflow` line under `[tool.astro]`, which nothing reads any more, was deleted. `requires-python` under `[project]` was moved with it too and now reads `>=3.10,<3.14`. The `runtime` line under `[tool.astro]`, the Astro Runtime build the image is built from, named a build of the old series, so it was moved to `3.4-2`, the newest build of the new one. Do not change the Airflow requirement again. This project declares `dockerfile = \"Dockerfile\"` under `[tool.astro]`, and Docker mode builds from that file, so its `FROM` line, not the requirement, decides the image. It was not changed. Update its base image and any build steps for Airflow 3.4. What is left: check the providers in the `[project] dependencies` array for versions that need to move with Airflow 3.4 and propose updates, and migrate any Dag code the upgrade breaks. Once those changes are in, and not before, bring Airflow up on 3.4: airflow_restart if it is running, airflow_start if it is stopped."
	desktopPromptOtherBranches = "/skill:airflow-upgrade Upgrade my Airflow from 2.9 to 3.4. The Airflow requirement in the `[project] dependencies` array of pyproject.toml already pins `3.4`. `apache-airflow-core` is published only for Airflow 3, so that entry became `apache-airflow` at the new version. The old `airflow` line under `[tool.astro]`, which nothing reads any more, was deleted. `requires-python` under `[project]` was moved with it too and now reads `>=3.10,<3.14`. The `runtime` line under `[tool.astro]`, the Astro Runtime build the image is built from, named a build of the old series and no build of the new one could be named, so it was deleted: the image now builds from the newest build of the new series. Do not change the Airflow requirement again. This project has no Dockerfile. What is left: check the providers in the `[project] dependencies` array for versions that need to move with Airflow 3.4 and propose updates, and migrate any Dag code the upgrade breaks. Once those changes are in, and not before, bring Airflow up on 3.4: airflow_restart if it is running, airflow_start if it is stopped."
)

func TestAirflowUpgradePromptMatchesTheDesktopsWordForWord(t *testing.T) {
	every := AirflowPinChange{
		Previous: "2.9", Version: "3.4", Changed: true,
		Requirements: []string{"apache-airflow==3.4.*", "apache-airflow[celery]==3.4.*"},
		CoreReplaced: true, RemovedAirflowKey: true, RequiresPython: ">=3.10,<3.14",
		Dockerfile: "Dockerfile", Runtime: "3.4-2",
	}
	assert.Equal(t, desktopPromptEveryField, AirflowUpgradePrompt(&every, desktopPromptOptions))

	other := every
	other.Changed, other.Runtime, other.RuntimeRemoved, other.Dockerfile = false, "", true, ""
	assert.Equal(t, desktopPromptOtherBranches, AirflowUpgradePrompt(&other, desktopPromptOptions))
}

func TestAirflowUpgradePromptUsesTheCallersWords(t *testing.T) {
	p := AirflowUpgradePrompt(ptr(promptBase()), UpgradePromptOptions{
		Editor:  "The Astro CLI",
		BringUp: "`astro local restart` if it is running, `astro local start` if it is stopped",
	})
	assert.Contains(t, p, "The Astro CLI has already made the mechanical edit")
	assert.Contains(t, p, "bring Airflow up on 3.1: `astro local restart` if it is running, `astro local start` if it is stopped.")
	assert.NotContains(t, p, "Astro Desktop")
	assert.NotContains(t, p, "airflow_restart")
}

func ptr[T any](v T) *T { return &v }

// What Astro Desktop's own fallback prompt builder returns for the same inputs,
// captured from it verbatim.
const desktopFallbackPrompt = "/skill:airflow-upgrade Upgrade my Airflow from 2.9 to 3.1. This project has no Dockerfile: its Airflow version is the `apache-airflow` requirement in the `[project] dependencies` array of pyproject.toml, and its providers are in the same array. Astro Desktop tried to move the requirement and could not: pyproject.toml: invalid TOML. Tell me why. If the cause is in the project, for example a pyproject.toml that does not parse, fix it, then move the apache-airflow requirement to the new version. If the version itself was refused, do not set it. Then check those providers for outdated Airflow providers and propose updates, and migrate any Dag code the upgrade breaks."

func TestAirflowUpgradeFallbackPromptMatchesTheDesktopsWordForWord(t *testing.T) {
	assert.Equal(t, desktopFallbackPrompt,
		AirflowUpgradeFallbackPrompt("2.9", "3.1", "pyproject.toml: invalid TOML.", desktopPromptOptions))
}

func TestAirflowUpgradeFallbackPromptGivesOttoTheWholeJob(t *testing.T) {
	p := AirflowUpgradeFallbackPrompt("2.9", "3.1", "pyproject.toml: invalid TOML.", desktopPromptOptions)
	assert.True(t, strings.HasPrefix(p, "/skill:airflow-upgrade Upgrade my Airflow from 2.9 to 3.1."), p)
	assert.Contains(t, p, "could not: pyproject.toml: invalid TOML.")
	assert.Contains(t, p, "then move the apache-airflow requirement to the new version")
	assert.NotContains(t, p, "[tool.astro]")
	assert.Contains(t, p, "If the version itself was refused, do not set it.")

	cli := AirflowUpgradeFallbackPrompt("2.9", "3.1", "boom.", UpgradePromptOptions{Editor: "The Astro CLI"})
	assert.Contains(t, cli, "The Astro CLI tried to move the requirement and could not: boom.")
}

// With no previous version neither prompt names an empty one.
func TestAirflowUpgradePromptsWithNoPreviousVersion(t *testing.T) {
	c := promptBase()
	c.Previous = ""
	p := AirflowUpgradePrompt(&c, desktopPromptOptions)
	assert.True(t, strings.HasPrefix(p, "/skill:airflow-upgrade Upgrade my Airflow to 3.1. "), p)
	assert.Contains(t, p, "now reads `apache-airflow==3.1.*`. ")
	assert.NotContains(t, p, "from  to")
	assert.NotContains(t, p, "(it pinned")

	fb := AirflowUpgradeFallbackPrompt("", "3.1", "boom.", desktopPromptOptions)
	assert.True(t, strings.HasPrefix(fb, "/skill:airflow-upgrade Upgrade my Airflow to 3.1. "), fb)
}
