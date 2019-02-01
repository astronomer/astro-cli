package messages

var (
	ERROR_INVALID_CLI_VERSION     = "Astronomer CLI version is not valid"
	ERROR_GITHUB_JSON_MARSHALLING = "Failed to JSON decode Github response from %s"
	ERROR_INVALID_AIRFLOW_VERSION = "Unsupported Airflow Version specified. Please choose from: %s \n"

	CLI_CMD_DEPRECATE         = "Deprecated in favor of %s\n"
	CLI_CURR_VERSION          = "Astro CLI Version: %s "
	CLI_CURR_COMMIT           = "Git Commit: %s"
	CLI_CURR_VERSION_DATE     = CLI_CURR_VERSION + " (%s)"
	CLI_LATEST_VERSION        = "Astro CLI Latest: %s "
	CLI_LATEST_VERSION_DATE   = CLI_LATEST_VERSION + " (%s)"
	CLI_INSTALL_CMD           = "\t$ curl -sL https://install.astronomer.io | sudo bash"
	CLI_RUNNING_LATEST        = "You are running the latest version."
	CLI_SET_WORKSPACE_EXAMPLE = "\nNo default workspace detected, you can list workspaces with \n\tastro workspace list\nand set your default workspace with \n\tastro workspace switch [WORKSPACEID]\n\n"
	CLI_UPGRADE_PROMPT        = "There is a more recent version of the Astronomer CLI available.\nYou can install the latest tagged release with the following command"
	CLI_UNTAGGED_PROMPT       = "Your current Astronomer CLI is not tagged.\nThis is likely the result of building from source. You can install the latest tagged release with the following command"

	CONFIG_CREATE_DIR_ERROR        = "Error creating config directory\n"
	CONFIG_CREATE_HOME_ERROR       = "Error creating default config in home dir: %s"
	CONFIG_CREATE_FILE_ERROR       = "Error creating config file\n"
	CONFIG_PATH_KEY_MISSING_ERROR  = "Must specify config key\n"
	CONFIG_PATH_KEY_INVALID_ERROR  = "Config does not exist, check your config key\n"
	CONFIG_PROJECT_NAME_ERROR      = "Project name is invalid\n"
	CONFIG_INIT_PROJECT_CONFIG     = "Initialized empty astronomer project in %s"
	CONFIG_INVALID_SET_ARGS        = "Must specify exactly two arguments (key value) when setting a config\n"
	CONFIG_READ_ERROR              = "Error reading config in home dir: %s\n"
	CONFIG_REINIT_PROJECT_CONFIG   = "Reinitialized existing astronomer project in %s\n"
	CONFIG_SAVE_ERROR              = "Error saving config\n"
	CONFIG_SET_DEFAULT_WORKSPACE   = "Default \"%s\" (%s) workspace found, setting default workspace.\n"
	CONFIG_SET_SUCCESS             = "Setting %s to %s successfully\n"
	CONFIG_USE_OUTSIDE_PROJECT_DIR = "You are attempting to %s a project config outside of a project directory\n To %s a global config try\n%s\n"

	COMPOSE_CREATE_ERROR         = "Error creating docker-compose project"
	COMPOSE_IMAGE_BUILDING_PROMT = "Building image..."
	COMPOSE_STATUS_CHECK_ERROR   = "Error checking docker-compose status"
	COMPOSE_STOP_ERROR           = "Error stopping and removing containers"
	COMPOSE_PAUSE_ERROR          = "Error pausing project containers"
	COMPOSE_RECREATE_ERROR       = "Error building, (re)creating or starting project containers"
	COMPOSE_PUSHING_IMAGE_PROMPT = "Pushing image to Astronomer registry"
	COMPOSE_LINK_WEBSERVER       = "Airflow Webserver: http://localhost:%s/admin/"
	COMPOSE_LINK_POSTGRES        = "Postgres Database: localhost:%s/postgres"

	HOUSTON_BASIC_AUTH_DISABLED      = "Basic authentication is disabled, conact administrator or defer back to oAuth"
	HOUSTON_DEPLOYMENT_HEADER        = "Authenticated to %s \n\n"
	HOUSTON_DEPLOYING_PROMPT         = "Deploying: %s\n"
	HOUSTON_NO_DEPLOYMENTS_ERROR     = "No airflow deployments found"
	HOUSTON_SELECT_DEPLOYMENT_PROMPT = "Select which airflow deployment you want to deploy to:"
	HOUSTON_OAUTH_REDIRECT           = "Please visit the following URL, authenticate and paste token in next prompt\n"
	HOUSTON_INVALID_DEPLOYMENT_KEY   = "Invalid deployment selection\n"

	INPUT_PASSWORD    = "Password: "
	INPUT_USERNAME    = "Username (leave blank for oAuth): "
	INPUT_OAUTH_TOKEN = "oAuth Token: "

	REGISTRY_AUTH_SUCCESS        = "Successfully authenticated to %s\n"
	REGISTRY_AUTH_FAIL           = "Failed to authenticate to the registry, this can occur when registry is offline. Until authenticated you will not be able to push new images to your Airflow clusters\n"
	REGISTRY_UNCOMMITTED_CHANGES = "Project directory has uncommmited changes, use `astro airflow deploy [releaseName] -f` to force deploy."

	SETTINGS_PATH = "Error looking for settings.yaml"

	NA = "N/A"
)
