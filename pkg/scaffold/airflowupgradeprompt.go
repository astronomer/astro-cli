package scaffold

import (
	"strings"
)

// UpgradePromptOptions name the two things an Airflow upgrade prompt says that
// differ between the tools that send it.
type UpgradePromptOptions struct {
	// Editor is who made the edit, as the start of a sentence: "Astro Desktop"
	// or "The Astro CLI".
	Editor string
	// BringUp is how Otto brings Airflow up on the new pin once its work is in,
	// covering both a running and a stopped Airflow: for the desktop, "airflow_restart if it is
	// running, airflow_start if it is stopped".
	BringUp string
}

// AirflowUpgradePrompt is the prompt that hands Otto an Airflow upgrade after
// SetAirflowVersionWith wrote the pin. It says the mechanical edit is done, so
// Otto does not redo it, lists what changed with it, and leaves Otto the parts
// that need judgment: providers, Dag code, and a declared Dockerfile, which the
// write never touches. It tells Otto to bring Airflow up on the new pin only
// after those changes are in.
//
// English on purpose: it is an instruction to Otto, not UI copy.
func AirflowUpgradePrompt(change *AirflowPinChange, opts UpgradePromptOptions) string {
	parts := []string{
		upgradeOpening(change.Previous, change.Version),
	}

	if change.Changed && len(change.Requirements) > 0 {
		quoted := make([]string, len(change.Requirements))
		for i, r := range change.Requirements {
			quoted[i] = backticked(r)
		}
		edit := opts.Editor + " has already made the mechanical edit: the Airflow requirement in the " +
			"`[project] dependencies` array of pyproject.toml now reads " + strings.Join(quoted, ", ")
		if change.Previous != "" {
			edit += " (it pinned " + backticked(change.Previous) + ")"
		}
		parts = append(parts, edit+".")
	} else {
		parts = append(parts, "The Airflow requirement in the `[project] dependencies` array of pyproject.toml already pins "+
			backticked(change.Version)+".")
	}
	if change.CoreReplaced {
		parts = append(parts, "`apache-airflow-core` is published only for Airflow 3, so that entry became `apache-airflow` at the new version.")
	}
	if change.RemovedAirflowKey {
		parts = append(parts, "The old `airflow` line under `[tool.astro]`, which nothing reads any more, was deleted.")
	}
	if change.RequiresPython != "" {
		parts = append(parts, "`requires-python` under `[project]` was moved with it too and now reads "+backticked(change.RequiresPython)+".")
	}
	switch {
	case change.Runtime != "":
		parts = append(parts, "The `runtime` line under `[tool.astro]`, the Astro Runtime build the image is built from, named a build of the old "+
			"series, so it was moved to "+backticked(change.Runtime)+", the newest build of the new one.")
	case change.RuntimeRemoved:
		parts = append(parts, "The `runtime` line under `[tool.astro]`, the Astro Runtime build the image is built from, named a build of the old "+
			"series and no build of the new one could be named, so it was deleted: the image now builds from the newest build of the new series.")
	}
	parts = append(parts, "Do not change the Airflow requirement again.")

	if change.Dockerfile != "" {
		parts = append(parts, "This project declares `dockerfile = \""+change.Dockerfile+"\"` under `[tool.astro]`, and Docker mode "+
			"builds from that file, so its `FROM` line, not the requirement, decides the image. It was not changed. "+
			"Update its base image and any build steps for Airflow "+change.Version+".")
	} else {
		parts = append(parts, "This project has no Dockerfile.")
	}

	parts = append(parts, "What is left: check the providers in the `[project] dependencies` array for versions that need to "+
		"move with Airflow "+change.Version+" and propose updates, and migrate any Dag code the upgrade "+
		"breaks. Once those changes are in, and not before, bring Airflow up on "+change.Version+": "+
		opts.BringUp+".")
	return strings.Join(parts, " ")
}

// AirflowUpgradeFallbackPrompt is the prompt that hands Otto the whole upgrade
// from from to to when the pin could not be written, with errText, the reason
// the write failed. A manifest that does not parse is then fixed rather than
// tripped over, and a refused version is not written anyway. from may be
// empty when the manifest could not be read for it.
func AirflowUpgradeFallbackPrompt(from, to, errText string, opts UpgradePromptOptions) string {
	return upgradeOpening(from, to) + " " +
		"This project has no Dockerfile: its Airflow version is the `apache-airflow` requirement in " +
		"the `[project] dependencies` array of pyproject.toml, and its providers are in the same array. " +
		opts.Editor + " tried to move the requirement and could not: " + errText + " " +
		"Tell me why. If the cause is in the project, for example a pyproject.toml that does not " +
		"parse, fix it, then move the apache-airflow requirement to the new version. If the " +
		"version itself was refused, do not set it. Then check those providers for outdated Airflow " +
		"providers and propose updates, and migrate any Dag code the upgrade breaks."
}

// upgradeOpening is the skill invocation both prompts open with. With no
// previous version it names only the new one.
func upgradeOpening(from, to string) string {
	if from == "" {
		return "/skill:airflow-upgrade Upgrade my Airflow to " + to + "."
	}
	return "/skill:airflow-upgrade Upgrade my Airflow from " + from + " to " + to + "."
}

// backticked wraps s in backticks.
func backticked(s string) string { return "`" + s + "`" }
