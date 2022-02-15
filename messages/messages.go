package messages

var (
	ErrInvalidCLIVersion     = "Astronomer CLI version is not valid"
	ErrGithubJSONMarshalling = "Failed to JSON decode Github response from %s"
	ErrInvalidAirflowVersion = "Unsupported Airflow Version specified. Please choose from: %s \n"
	ErrNewMajorVersion       = "There is an update for Astro CLI. You're using version %s, but %s is the server version.\nPlease upgrade to the matching version before continuing. See https://www.astronomer.io/docs/cli-quickstart for more information.\nTo skip this check use the --skip-version-check flag.\n"

	CLICMDDeprecate               = "Deprecated in favor of %s\n"
	CLICurrVersion                = "Astro CLI Version: %s"
	CLICurrCommit                 = "Git Commit: %s"
	CLICurrVersionDate            = CLICurrVersion + " (%s)"
	CLILatestVersion              = "Astro CLI Latest: %s"
	CLILatestVersionDate          = CLILatestVersion + " (%s)"
	CLIInstallCMD                 = "\t$ curl -fsSL https://install.astronomer.io | sudo bash \nOR for homebrew users:\n\t$ brew install astronomer/tap/astro"
	CLIRunningLatest              = "You are running the latest version."
	CLIChooseWorkspace            = "Please choose a workspace:"
	CLISetWorkspaceExample        = "\nNo default workspace detected, you can list workspaces with \n\tastro workspace list\nand set your default workspace with \n\tastro workspace switch [WORKSPACEID]\n\n"
	CLIUpgradePrompt              = "A newer version of the Astronomer CLI is available.\nTo upgrade to latest, run:"
	CLIUntaggedPrompt             = "Your current Astronomer CLI is not tagged.\nThis is likely the result of building from source. You can install the latest tagged release with the following command"
	CLIDeploymentHardDeletePrompt = "\nWarning: This action permanently deletes all data associated with this Deployment, including the database. You will not be able to recover it. Proceed with hard delete?"

	ErrConfigDirCreation        = "Error creating config directory\n"
	ErrConfigHomeCreation       = "Error creating default config in home dir: %s"
	ErrConfigFileCreation       = "Error creating config file\n"
	ErrMissingConfigPathKey     = "Must specify config key\n"
	ErrInvalidConfigPathKey     = "Config does not exist, check your config key\n"
	ErrInvalidConfigProjectName = "Project name is invalid\n"
	ErrReadingConfig            = "Error reading config in home dir: %s\n"
	ErrSavingConfig             = "Error saving config\n"

	ConfigInitProjectConfig    = "Initialized empty astronomer project in %s"
	ConfigInvalidSetArgs       = "Must specify exactly two arguments (key value) when setting a config\n"
	ConfigReinitProjectConfig  = "Reinitialized existing astronomer project in %s\n"
	ConfigSetDefaultWorkspace  = "Default \"%s\" (%s) workspace found, setting default workspace.\n"
	ConfigSetSuccess           = "Setting %s to %s successfully\n"
	ConfigUseOutsideProjectDir = "You are attempting to %s a project config outside of a project directory\n To %s a global config try\n%s\n"

	ErrContainerCreate      = "Error creating the containers"
	ErrContainerStatusCheck = "Error checking checking container status"
	ErrContainerStop        = "Error stopping and removing containers"
	ErrContainerPause       = "Error pausing the containers"
	ErrContainerRecreate    = "Error building, (re)creating or starting the containers"
	ErrContainerNotFound    = "container not found"

	ImageBuildingPrompt    = "Building image..."
	PushingImagePrompt     = "Pushing image to Astronomer registry"
	ContainerLinkWebserver = "Airflow Webserver: http://localhost:%s"
	ContainerLinkPostgres  = "Postgres Database: localhost:%s/postgres"
	ContainerUserPassword  = "The default credentials are admin:admin"

	EnvPath     = "Error looking for \"%s\""
	EnvFound    = "Env file \"%s\" found. Loading...\n"
	EnvNotFound = "Env file \"%s\" not found. Skipping...\n"

	HoustonBasicAuthDisabled      = "Basic authentication is disabled, conact administrator or defer back to oAuth"
	HoustonCurrentVersion         = "Astro Server Version: %s"
	HoustonDeploymentHeader       = "Authenticated to %s \n\n"
	HoustonDeploymentPrompt       = "Deploying: %s\n"
	ErrNoHoustonDeployment        = "No airflow deployments found"
	ErrHoustonDeploymentName      = "Please specify a valid deployment name"
	HoustonSelectDeploymentPrompt = "Select which airflow deployment you want to deploy to:"
	HoustonOAuthRedirect          = "Please visit the following URL, authenticate and paste token in next prompt\n"
	HoustonInvalidDeploymentKey   = "Invalid deployment selection\n"

	// TODO: @adam2k remove this message once the Houston API work is completed that will surface a similar error message

	HoustonInvalidDeploymentUsers = "No users were found for this deployment.  Check the deploymentId and try again.\n"

	InputPassword   = "Password: "
	InputUsername   = "Username (leave blank for oAuth): "
	InputOAuthToken = "oAuth Token: " // nolint:gosec // false positive

	RegistryAuthSuccess        = "Successfully authenticated to %s\n"
	RegistryAuthFail           = "\nFailed to authenticate to the registry. Do you have Docker running?\nYou will not be able to push new images to your Airflow Deployment unless Docker is running.\nIf Docker is running and you are seeing this message, the registry is down or cannot be reached.\n"
	RegistryUncommittedChanges = "Project directory has uncommmited changes, use `astro deploy [releaseName] -f` to force deploy."

	SettingsPath = "Error looking for settings.yaml"

	NA                                        = "N/A"
	ValidDockerfileBaseImage                  = "quay.io/astronomer/ap-airflow"
	WarningDowngradeVersion                   = "Your Astro CLI Version (%s) is ahead of the server version (%s).\nConsider downgrading your Astro CLI to match. See https://www.astronomer.io/docs/cli-quickstart for more information.\n"
	WarningInvalidImageName                   = "WARNING! The image in your Dockerfile is pulling from '%s', which is not supported. We strongly recommend that you use Astronomer Certified images that pull from 'astronomerinc/ap-airflow' or 'quay.io/astronomer/ap-airflow'. If you're running a custom image, you can override this. Are you sure you want to continue?\n"
	WarningInvalidNameTag                     = "WARNING! You are about to push an image using the '%s' tag. This is not recommended.\nPlease use one of the following tags: %s.\nAre you sure you want to continue?"
	WarningInvalidNameTagEmptyRecommendations = "WARNING! You are about to push an image using the '%s' tag. This is not recommended.\nAre you sure you want to continue?"
	WarningNewMinorVersion                    = "A new minor version of Astro CLI is available. Your version is %s and %s is the latest.\nSee https://www.astronomer.io/docs/cli-quickstart for more information.\n"
)
