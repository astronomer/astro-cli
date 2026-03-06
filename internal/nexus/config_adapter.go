package nexus

import (
	"regexp"
	"strings"

	astroconfig "github.com/astronomer/astro-cli/config"
	"github.com/astronomer/nexus/shared"
)

// domainEnvRegex extracts the environment suffix (dev, stage, perf) from an
// Astronomer cloud domain. Matches domains like "astronomer-dev.io",
// "cloud.astronomer-stage.io", "pr1234.astronomer-dev.io", etc.
var domainEnvRegex = regexp.MustCompile(`astronomer(?:-(dev|stage|perf))?\.io`)

// ConfigAdapter implements nexus config.ConfigProvider using astro-cli's
// config system. The API is derived from the current astro-cli context domain,
// and org/workspace/deployment defaults are read from the context.
type ConfigAdapter struct{}

func (a *ConfigAdapter) GetDefaultAPI() string {
	domain, err := astroconfig.GetCurrentDomain()
	if err != nil {
		return ""
	}
	return domainToAPI(domain)
}

func (a *ConfigAdapter) SetDefaultAPI(_ string) error {
	return nil
}

func (a *ConfigAdapter) GetDefaultOrganizationForAPI(_ string) string {
	ctx, err := astroconfig.GetCurrentContext()
	if err != nil {
		return ""
	}
	return ctx.Organization
}

func (a *ConfigAdapter) SetDefaultOrganizationForAPI(_, org string) error {
	ctx, err := astroconfig.GetCurrentContext()
	if err != nil {
		return err
	}
	return ctx.SetContextKey("organization", org)
}

func (a *ConfigAdapter) GetDefaultWorkspaceForAPI(_ string) string {
	ctx, err := astroconfig.GetCurrentContext()
	if err != nil {
		return ""
	}
	return ctx.Workspace
}

func (a *ConfigAdapter) SetDefaultWorkspaceForAPI(_, ws string) error {
	ctx, err := astroconfig.GetCurrentContext()
	if err != nil {
		return err
	}
	return ctx.SetContextKey("workspace", ws)
}

func (a *ConfigAdapter) GetDefaultDeploymentForAPI(_ string) string {
	ctx, err := astroconfig.GetCurrentContext()
	if err != nil {
		return ""
	}
	return ctx.Deployment
}

func (a *ConfigAdapter) SetDefaultDeploymentForAPI(_, dep string) error {
	ctx, err := astroconfig.GetCurrentContext()
	if err != nil {
		return err
	}
	return ctx.SetContextKey("deployment", dep)
}

func (a *ConfigAdapter) GetVariable(_ string) string {
	return ""
}

func (a *ConfigAdapter) GetVariables() map[string]string {
	return nil
}

func (a *ConfigAdapter) SetVariables(_ map[string]string) error {
	return nil
}

func (a *ConfigAdapter) GetArgDefault(argName, _ string) string {
	ctx, err := astroconfig.GetCurrentContext()
	if err != nil {
		return ""
	}

	argLower := strings.ToLower(argName)

	if strings.Contains(argLower, shared.ResourceOrganization) {
		return ctx.Organization
	}
	if strings.Contains(argLower, shared.ResourceWorkspace) {
		return ctx.Workspace
	}
	if strings.Contains(argLower, shared.ResourceDeployment) {
		return ctx.Deployment
	}

	return ""
}

// domainToAPI maps an astro-cli context domain to a nexus API profile name.
func domainToAPI(domain string) string {
	matches := domainEnvRegex.FindStringSubmatch(domain)
	if matches == nil {
		return "astro-prod"
	}
	env := matches[1] // captured group: "dev", "stage", "perf", or ""
	if env == "" {
		return "astro-prod"
	}
	return "astro-" + env
}
