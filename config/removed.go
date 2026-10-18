package config

// removedSetting is a setting this build does not register: a 1.x setting
// nothing in v2 reads, or one an earlier v2 build had.
type removedSetting struct {
	// replacement says what to do instead, for `astro config set` and `get`
	// to refuse the setting with.
	replacement string
	// defaults are the values the setting shipped as its default, which an
	// older CLI wrote into the file without being asked; migrateHomeConfig
	// removes an entry still holding one. None for a setting whose default
	// was empty, which was never written.
	defaults []string
}

// removedSettings are the settings v2 refuses and leaves in the home config,
// where Astro CLI 1.x still reads what it wrote itself. One table, so a
// setting the cleanup knows to be stale is also one `astro config` refuses
// by name.
var removedSettings = map[string]removedSetting{
	"airflow.expose_port": {"astro local has no setting for the interfaces local Airflow listens on.", []string{"false"}},
	"api-server.port":     {portRemoved, []string{"8080"}},
	"beta.remote_cmd":     {"It was a beta flag, and nothing is gated on it any more.", []string{"false"}},
	"cloud.api.token":     {"Log in with astro login, or set ASTRO_API_TOKEN to use an API token.", nil},
	"dev.build_secrets": {"Pass --build-secret to astro local start, or declare build-secrets under [tool.astro] in pyproject.toml " +
		"for a project that builds its own Dockerfile.", nil},
	"dev.mode": {"Local Airflow runs in standalone mode by default; start it in Docker with astro local start --docker. " +
		"astro local restart keeps the mode it is running in.", []string{"docker"}},
	"disable_astro_run":   {"astro run was removed: test a Dag with astro local run airflow dags test <dag>.", []string{"false"}},
	"duplicate_volumes":   {"astro local start --docker manages its own volumes and has no setting.", []string{"true"}},
	"local.registry":      {"astro local start --docker builds its image locally and pushes it nowhere.", []string{"localhost:5555"}},
	"machine.cpu":         {machineRemoved, []string{"2"}},
	"machine.memory":      {machineRemoved, []string{"4096"}},
	"postgres.host":       {postgresRemoved, []string{"postgres"}},
	"postgres.password":   {postgresRemoved, []string{"postgres"}},
	"postgres.port":       {postgresRemoved, []string{"5432"}},
	"postgres.repository": {postgresRemoved, []string{"docker.io/postgres"}},
	// 15 is 1.x's default today, 12.6 the one it had until v1.46.0.
	"postgres.tag":  {postgresRemoved, []string{"15", "12.6"}},
	"postgres.user": {postgresRemoved, []string{"postgres"}},
	// astro deploy --save wrote it into a 1.x project's .astro/config.yaml.
	"project.deployment": {"Name the Deployment on each deploy, as the argument or with --deployment. " +
		"On Astro, link a project's Deployments with astro link add, and mark one default = true to preselect it.", nil},
	"project.name":      {"A project's name is [project] name in its pyproject.toml.", nil},
	"project.workspace": {"A project's default workspace is workspace under [tool.astro] in its pyproject.toml.", nil},
	"proxy.port":        {"The local proxy listens on 6563, or on another free port when 6563 is taken, and has no setting.", []string{"6563"}},
	"ruff.image":        {"Nothing in v2 runs ruff in a container: run it with uv run ruff.", []string{"ghcr.io/astral-sh/ruff:latest"}},
	"skip_parse":        {"astro deploy does not parse Dags first. Check them with astro local check.", []string{"false"}},
	"webserver.port":    {portRemoved, []string{"8080"}},
}

const (
	// portRemoved replaces 1.x's webserver.port and api-server.port.
	portRemoved = "Pick the local Airflow port with astro local start --port <port>; without it a free port is chosen."
	// postgresRemoved replaces every 1.x postgres.* setting: the metadata
	// database a Docker-mode start runs is fixed, and a standalone one has
	// none.
	postgresRemoved = "astro local start --docker runs its own metadata database, which has no settings."
	// machineRemoved replaces 1.x's machine.cpu and machine.memory, the size
	// of the Podman machine 1.x ran the project in.
	machineRemoved = "Size a Podman machine with podman machine set --cpus and --memory."
)

// RemovedSetting reports whether name is a setting v2 refuses, and what
// replaces it.
func RemovedSetting(name string) (replacement string, removed bool) {
	s, ok := removedSettings[name]
	return s.replacement, ok
}
