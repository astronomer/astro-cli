package agent

import (
	"encoding/json"
	"strings"
)

// astroConfig returns the opencode configuration JSON for the Astro distribution.
// This is passed via OPENCODE_CONFIG_CONTENT to customize opencode's behavior
// for Airflow development within the Astronomer ecosystem.
//
// Skills are NOT configured here — opencode auto-discovers them from
// ~/.agents/skills/ where ensureSkills() extracts them.
func astroConfig() string {
	config := map[string]any{
		// Disable opencode self-update — astro cli manages the embedded binary
		"autoupdate": false,

		// Disable session sharing by default for enterprise safety
		"share": "disabled",

		// Default agent tuned for Airflow
		"default_agent": "airflow",
		"agent": map[string]any{
			"airflow": map[string]any{
				"description": "Airflow DAG development and debugging",
				"mode":        "primary",
				"color":       "#4A90D9",
				"steps":       15,
				"prompt":      astroSystemPrompt,
			},
		},

		// Sensible default permissions
		"permission": map[string]any{
			"*":         "ask",
			"read":      "allow",
			"glob":      "allow",
			"grep":      "allow",
			"list":      "allow",
			"todoread":  "allow",
			"todowrite": "allow",
		},
	}

	b, _ := json.Marshal(config)
	return string(b)
}

// appendConfigEnv adds OPENCODE_CONFIG_CONTENT to the environment,
// preserving any existing env vars. If the user already set
// OPENCODE_CONFIG_CONTENT, we don't override it.
func appendConfigEnv(env []string) []string {
	for _, e := range env {
		if strings.HasPrefix(e, "OPENCODE_CONFIG_CONTENT=") {
			return env // User explicitly set their own config — respect it
		}
	}
	return append(env, "OPENCODE_CONFIG_CONTENT="+astroConfig())
}

const astroSystemPrompt = `You are an expert Apache Airflow developer working within an Astronomer environment.

You have deep knowledge of:
- DAG authoring patterns and best practices
- Task dependencies, sensors, and operators
- XComs, connections, and variables
- The Astro CLI (astro dev start, astro deploy, etc.)
- Airflow REST API
- Astronomer platform features (deployments, worker queues, etc.)
- Testing DAGs locally with pytest

When writing or modifying DAGs:
- Follow Airflow 2.x+ best practices (TaskFlow API where appropriate)
- Use meaningful task IDs and DAG IDs
- Set appropriate retries, timeouts, and SLAs
- Prefer dynamic task generation over repetitive code
- Include proper error handling and alerting configuration

When debugging:
- Check task logs for root cause before suggesting fixes
- Consider common issues: import errors, connection misconfigs, resource limits
- Use the Astronomer skills available to you — they contain detailed workflows
  for common tasks like authoring DAGs, testing, debugging, managing deployments,
  data lineage, dbt integration, and more. Invoke them with /skill-name.

When working with the af CLI:
- Run commands via: uvx --from astro-airflow-mcp af <command>
- Use 'af instance discover' to find available Airflow instances
- Use 'af dags list', 'af dags test', 'af tasks logs' for diagnostics
`
