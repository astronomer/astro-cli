package config

import (
	"context"
	"errors"
	"fmt"
	iofs "io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofrs/flock"
	"github.com/spf13/afero"
	"github.com/spf13/viper"

	"github.com/astronomer/astro-cli/pkg/fileutil"
)

const (
	CloudPlatform    = "cloud"
	SoftwarePlatform = "software"
	PrPreview        = "prprievew"

	localhostDomain = "localhost"
	cloudDomain     = "cloud"
	houstonDomain   = "houston"

	// These go to STDERR, not stdout. A command that can emit `--output json`
	// has one parseable thing on stdout, and a config warning printed there
	// lands in front of it: `astro init --output json` in a project whose
	// .astro/config.yaml will not parse emitted a line of prose and then the
	// object, which no consumer can read.
	//
	// There is a message per file rather than one shared. Both project sites
	// used to report the home-dir wording, so a malformed project config was
	// announced as a problem with a file in the user's home directory — which
	// is somewhere they would look, and not where the file is.
	configCreateProjectErrorMsg = "Error creating project config %s: %s\n"
	configReadHomeErrorMsg      = "Error reading config %s: %s\n"
	configReadProjectErrorMsg   = "Error reading project config %s: %s\n"
	// configUnwritableMsg is the refusal a write gets against a file this
	// process could not read. viper's parse errors carry a line number and no
	// filename, so both of these say which file.
	configUnwritableMsg = "refusing to write %s: it could not be read at startup, and writing now would replace it with defaults — fix or remove the file"
)

var (
	// ConfigFileName is the name of the config files (home / project)
	ConfigFileName = "config"
	// ConfigFileType is the config file extension
	ConfigFileType = "yaml"
	// ConfigFileNameWithExt is the config filename with extension
	ConfigFileNameWithExt = fmt.Sprintf("%s.%s", ConfigFileName, ConfigFileType)
	// ConfigDir is the directory for astro files
	ConfigDir = ".astro"

	// HomePath is the path to a users home directory
	HomePath, _ = fileutil.GetHomeDir() //nolint:errcheck // error deliberately ignored in this v1 path
	// HomeConfigPath is the path to the users global config directory
	HomeConfigPath = filepath.Join(HomePath, ConfigDir)
	// HomeConfigFile is the global config file
	HomeConfigFile = filepath.Join(HomeConfigPath, ConfigFileNameWithExt)

	// WorkingPath is the path to the working directory
	WorkingPath, _ = fileutil.GetWorkingDir() //nolint:errcheck // error deliberately ignored in this v1 path

	// CFGStrMap maintains string to cfg mapping
	CFGStrMap = make(map[string]cfg)

	// CFG Houses configuration meta
	CFG = cfgs{
		CloudAPIProtocol:        newCfg("cloud.api.protocol", "https"),
		CloudAPIPort:            newCfg("cloud.api.port", "443"),
		CloudWSProtocol:         newCfg("cloud.api.ws_protocol", "wss"),
		CloudAPIToken:           newCfg("cloud.api.token", ""),
		Context:                 newCfg("context", ""),
		Contexts:                newCfg("contexts", ""),
		DockerCommand:           newCfg("container.binary", ""),
		LocalCore:               newCfg("local.core", "http://localhost:8888"),
		LocalRegistry:           newCfg("local.registry", "localhost:5555"),
		LocalHouston:            newCfg("local.houston", ""),
		LocalPlatform:           newCfg("local.platform", CloudPlatform),
		PostgresUser:            newCfg("postgres.user", "postgres"),
		PostgresPassword:        newCfg("postgres.password", "postgres"),
		PostgresHost:            newCfg("postgres.host", "postgres"),
		PostgresPort:            newCfg("postgres.port", "5432"),
		PostgresRepository:      newCfg("postgres.repository", "docker.io/postgres"),
		PostgresTag:             newCfg("postgres.tag", "12.6"),
		ProjectDeployment:       newCfg("project.deployment", ""),
		ProjectName:             newCfg("project.name", ""),
		ProjectWorkspace:        newCfg("project.workspace", ""),
		WebserverPort:           newCfg("webserver.port", "8080"),
		APIServerPort:           newCfg("api-server.port", "8080"),
		AirflowExposePort:       newCfg("airflow.expose_port", "false"),
		ShowWarnings:            newCfg("show_warnings", "true"),
		Verbosity:               newCfg("verbosity", "warning"),
		HoustonDialTimeout:      newCfg("houston.dial_timeout", "10"),
		HoustonSkipVerifyTLS:    newCfg("houston.skip_verify_tls", "false"),
		DuplicateImageVolumes:   newCfg("duplicate_volumes", "true"),
		SkipParse:               newCfg("skip_parse", "false"),
		Interactive:             newCfg("interactive", "false"),
		PageSize:                newCfg("page_size", "20"),
		UpgradeMessage:          newCfg("upgrade_message", "true"),
		DisableAstroRun:         newCfg("disable_astro_run", "false"),
		AutoSelect:              newCfg("auto_select", "false"),
		ShaAsTag:                newCfg("sha_as_tag", "false"),
		RuffImage:               newCfg("ruff.image", "ghcr.io/astral-sh/ruff:latest"),
		RemoteClientRegistry:    newCfg("remote.client_registry", ""),
		RemoteBaseImageRegistry: newCfg("remote.base_image_registry", "images.astronomer.cloud"),
		DeployGitMetadata:       newCfg("deploy.git_metadata", "true"),
		DevMode:                 newCfg("dev.mode", "docker"),
		DevBuildSecrets:         newCfg("dev.build_secrets", ""),
		TelemetryEnabled:        newCfg("telemetry.enabled", "true"),
		TelemetryAnonymousID:    newCfg("telemetry.anonymous_id", ""),
		TelemetryNoticeShown:    newCfg("telemetry.notice_shown", ""),
		ProxyPort:               newCfg("proxy.port", "6563"),
		OttoAutoUpdate:          newCfg("otto.auto_update", "true"),
		CosmosBoostPreDeploy:    newCfg("cosmos_boost.pre_deploy", "false"),
	}

	// unreadableConfigs names config files that exist and could not be parsed.
	//
	// A write to one of them would destroy it. viper takes its write target
	// from SetConfigFile, which runs BEFORE the read, so a failed read leaves
	// the object holding nothing but registered defaults while still pointed
	// at the user's file — and configExists, which is the only guard the
	// setters have, tests ConfigFileUsed() and is therefore still true. The
	// next `astro config set`, `astro login` or context switch then serialized
	// AllSettings() over the top: contexts, tokens and workspaces replaced by
	// defaults, reported as success.
	//
	// Keyed by path rather than by viper object so the check lives in
	// saveConfig, which every write goes through, instead of at the eight
	// call sites that would each have to remember it.
	unreadableConfigs = map[string]bool{}

	// viperHome is the viper object in the users home directory
	viperHome *viper.Viper
	// viperProject is the viper object in a project directory
	viperProject *viper.Viper
	// configFs is the filesystem the viper objects above are bound to.
	//
	// viper has no exported accessor for its own fs, and saveConfig needs one
	// to create a file at the right mode before viper writes it. Reaching
	// around to the os package instead would put a real file in the
	// developer's home every time a test wrote config through a MemMapFs.
	configFs afero.Fs = afero.NewOsFs()
	// createConfigPath dir path, file path
	dirPerm  os.FileMode = 0o775
	filePerm os.FileMode = 0o600
)

// InitConfig initializes the config files
func InitConfig(fs afero.Fs) {
	initHome(fs)
	initProject(fs)
	registerValidators()
}

// Init viper for config file in home directory
func initHome(fs afero.Fs) {
	viperHome = viper.New()
	viperHome.SetFs(fs)
	configFs = fs
	viperHome.SetConfigName(ConfigFileName)
	viperHome.SetConfigType(ConfigFileType)

	configPath := os.Getenv("ASTRO_HOME")
	if configPath != "" {
		HomeConfigPath = filepath.Join(configPath, ConfigDir)
		HomeConfigFile = filepath.Join(HomeConfigPath, ConfigFileNameWithExt)
	} else {
		HomeConfigPath = filepath.Join(HomePath, ConfigDir)
		HomeConfigFile = filepath.Join(HomeConfigPath, ConfigFileNameWithExt)
	}
	viperHome.SetConfigFile(HomeConfigFile)

	for _, cfg := range CFGStrMap {
		if cfg.Default != "" {
			viperHome.SetDefault(cfg.Path, cfg.Default)
		}
	}

	// Reading the home config is not a reason to create one.
	//
	// CreateConfig wrote out AllSettings() of an object holding nothing but
	// the defaults registered just above, so the file it left behind carried
	// no value this process did not already have — but InitConfig runs
	// unconditionally from main, before cobra has looked at argv. Every
	// command that never touches v1 config paid for it: `astro init` in a
	// home with no .astro left a 54-line config.yaml and a config.yaml.lock
	// behind it.
	//
	// Nothing needs the file to exist. Its absence reads as those same
	// defaults, the setters guard on ConfigFileUsed() which SetConfigFile
	// above has already set, and saveConfig creates the parent directory
	// before viper's WriteConfigAs creates the file — so the first `astro
	// login` or `astro config set -g` still writes it.
	switch err := viperHome.ReadInConfig(); {
	case err == nil, errors.Is(err, iofs.ErrNotExist):
		// A file that is not there cannot be destroyed by writing one, so a
		// missing config is not an unreadable config.
		delete(unreadableConfigs, HomeConfigFile)
	default:
		unreadableConfigs[HomeConfigFile] = true
		fmt.Fprintf(os.Stderr, configReadHomeErrorMsg, HomeConfigFile, err)
	}
}

// Init viper for config file in project directory
// If project config does not exist, just exit
func initProject(fs afero.Fs) {
	// Set up viper object for project config
	viperProject = viper.New()
	viperProject.SetFs(fs)
	configFs = fs
	viperProject.SetConfigName(ConfigFileName)
	viperProject.SetConfigType(ConfigFileType)

	// Construct the path to the config file
	workingConfigPath := filepath.Join(WorkingPath, ConfigDir)

	workingConfigFile := filepath.Join(workingConfigPath, ConfigFileNameWithExt)

	// If path is empty or config file does not exist, just return
	workingConfigExists, _ := fileutil.Exists(workingConfigFile, fs) //nolint:errcheck // treated as absent on error
	if workingConfigPath == "" || workingConfigPath == HomeConfigPath || !workingConfigExists {
		// A file that is gone is no longer a file a write could destroy, so
		// it does not keep its place on the unreadable list. Without this a
		// project config that failed to parse and was then deleted — by hand,
		// or by a conversion — stayed unwritable for the life of the process.
		delete(unreadableConfigs, workingConfigFile)
		return
	}

	// Add the path we discovered
	viperProject.SetConfigFile(workingConfigFile)

	// Read in project config. As with the home config, a file that vanished
	// between the check above and this read is absent rather than corrupt.
	switch readErr := viperProject.ReadInConfig(); {
	case readErr == nil, errors.Is(readErr, iofs.ErrNotExist):
		delete(unreadableConfigs, workingConfigFile)
	default:
		unreadableConfigs[workingConfigFile] = true
		fmt.Fprintf(os.Stderr, configReadProjectErrorMsg, workingConfigFile, readErr)
	}
}

// configExists returns a boolean indicating if the config is backed by a file
func configExists(v *viper.Viper) bool {
	return v.ConfigFileUsed() != ""
}

// IsProjectDir returns a boolean depending on if path is a valid project dir
func IsProjectDir(path string) (bool, error) {
	configPath := filepath.Join(path, ConfigDir)
	configFile := filepath.Join(configPath, ConfigFileNameWithExt)

	// Home directory is not a project directory
	if HomePath == path {
		return false, nil
	}

	return fileutil.Exists(configFile, nil)
}

// IsWithinProjectDir returns true if the path is at or within an Astro project directory
func IsWithinProjectDir(path string) (bool, error) {
	pathAbs, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return false, err
	}
	pathComponents := strings.Split(pathAbs, string(os.PathSeparator))
	for i := range pathComponents {
		componentAbs := strings.Join(pathComponents[:i+1], string(os.PathSeparator))
		isProjectDir, err := IsProjectDir(componentAbs)
		if err != nil {
			return false, err
		}
		if isProjectDir {
			return true, nil
		}
	}
	return false, nil
}

// saveConfig serializes viper writes under an exclusive OS-level file lock.
// viper.WriteConfigAs has no locking, so concurrent astro invocations can
// interleave writes and corrupt ~/.astro/config.yaml.
//
// The `<file>.lock` sidecar is only ever a handle for flock — the OS releases
// the lock when the holding process exits regardless of whether the file is
// deleted, so a stale `.lock` file on disk is never itself a problem.
func saveConfig(v *viper.Viper, file string) error {
	// A file this process could not read is a file it must not write. See
	// unreadableConfigs: v holds defaults after a failed read, and writing
	// them here replaces whatever the user had — which for the home config is
	// their contexts and tokens.
	//
	// An error rather than a silent skip, because the alternative failure is
	// somebody setting a value, being told nothing, and finding later that it
	// never persisted.
	if unreadableConfigs[file] {
		return fmt.Errorf(configUnwritableMsg, file)
	}

	// flock.Lock opens the sidecar file, which fails with ENOENT if the parent
	// dir hasn't been created yet. viper.WriteConfigAs creates the parent on
	// its own, but we need the lock held before we write — so do it upfront.
	if err := os.MkdirAll(filepath.Dir(file), dirPerm); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}

	lockFile := file + ".lock"
	lock := flock.New(lockFile)

	ctx, cancel := context.WithTimeout(context.Background(), lockTimeout)
	defer cancel()
	locked, err := lock.TryLockContext(ctx, lockRetryInterval)
	if err != nil {
		return fmt.Errorf("acquiring config lock %s: %w", lockFile, err)
	}
	if !locked {
		return fmt.Errorf("timed out after %s waiting for config lock %s — another astro process is holding it; wait for it to finish or kill it", lockTimeout, lockFile)
	}
	defer func() { _ = lock.Unlock() }() //nolint:errcheck // error deliberately ignored in this v1 path

	// viper's WriteConfigAs creates a new file 0644. Both configs are made
	// 0600 instead: the home one holds the API token, and the project one has
	// been 0600 since CreateConfig chmod'd it, which is the mode this moves
	// rather than invents. CreateConfig used to set it at startup while the
	// file was still empty; now that the file appears on first write, the
	// mode has to be established here.
	//
	// Only when the file is absent, so an existing file keeps whatever mode
	// its owner gave it and the common path costs no extra syscall. Before
	// the write rather than after, so a token is never momentarily
	// world-readable.
	created := false
	if _, statErr := configFs.Stat(file); errors.Is(statErr, iofs.ErrNotExist) {
		handle, oerr := configFs.OpenFile(file, os.O_CREATE|os.O_EXCL|os.O_WRONLY, filePerm)
		if oerr != nil {
			return fmt.Errorf("creating config file %s: %w", file, oerr)
		}
		if cerr := handle.Close(); cerr != nil {
			return fmt.Errorf("creating config file %s: %w", file, cerr)
		}
		created = true
	}

	if err := v.WriteConfigAs(file); err != nil {
		if created {
			// Take the empty file back out. An empty config parses cleanly,
			// so leaving it would make the next run read a healthy file full
			// of nothing and say so to no one.
			_ = configFs.Remove(file) //nolint:errcheck // the write error below is the one worth reporting
		}
		return fmt.Errorf("error saving config: %w", err)
	}
	return nil
}

const (
	lockTimeout       = 10 * time.Second
	lockRetryInterval = 100 * time.Millisecond
)
