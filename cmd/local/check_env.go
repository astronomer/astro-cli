package local

import (
	"github.com/astronomer/astro-cli/internal/plan"
	"github.com/astronomer/astro-cli/pkg/checks"
	"github.com/astronomer/astro-cli/pkg/envschema"
)

// envFindings turns what the start gate would say about [tool.astro.env] into
// check findings: a required value with no source is an error, as it blocks
// `astro local start`, and a value that is not what its declaration says is a
// warning, as a start runs past it.
//
// A name declared source = "workspace" that nothing local holds is left out.
// The check runs offline and never asks the Environment Manager, so it cannot
// tell a value the workspace holds from one it does not, and a start that
// reads it may well find it.
func envFindings(rep plan.EnvReport) []checks.Finding {
	var out []checks.Finding
	for i := range rep.Missing {
		m := &rep.Missing[i]
		if m.Workspace {
			continue
		}
		out = append(out, checks.Finding{
			Kind:     checks.KindEnvMissing,
			Severity: checks.SeverityError,
			Section:  string(m.Section),
			Key:      m.Name,
			Message:  "required, and not set on this machine; provide it: " + plan.SetCommand(m),
		})
	}
	for _, v := range rep.Warnings {
		out = append(out, checks.Finding{
			Kind:     checks.KindEnvInvalid,
			Severity: checks.SeverityWarning,
			Section:  string(v.Section),
			Key:      v.Key,
			Message:  v.Reason,
		})
	}
	return out
}

// withEnvFindings leads res with the env findings and counts them into its
// totals, so the verdict and the exit code include them.
func withEnvFindings(res checks.Result, env []checks.Finding) checks.Result {
	if len(env) == 0 {
		return res
	}
	for i := range env {
		if env[i].Severity == checks.SeverityError {
			res.Errors++
		} else {
			res.Warnings++
		}
	}
	res.Findings = append(env, res.Findings...)
	return res
}

// envFindingLocation is an env finding's LOCATION cell: the declaration it is
// about, as a start's own messages name it.
func envFindingLocation(f checks.Finding) string {
	return sectionLabel(envschema.Section(f.Section)) + " " + f.Key
}
