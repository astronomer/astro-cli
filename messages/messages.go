package messages

var (
	ERROR_INVALID_CLI_VERSION     = "Astronomer CLI version is not valid"
	ERROR_GITHUB_JSON_MARSHALLING = "Failed to JSON decode Github response from %s"

	CLI_CURR_VERSION        = "Astro CLI Version: v%s"
	CLI_CURR_COMMIT         = "Git Commit: %s"
	CLI_CURR_VERSION_DATE   = CLI_CURR_VERSION + " (%s)"
	CLI_LATEST_VERSION      = "Astro CLI Latest: %s "
	CLI_LATEST_VERSION_DATE = CLI_LATEST_VERSION + " (%s)"
	CLI_INSTALL_CMD         = "\t$ curl -sL https://install.astronomer.io | sudo bash"
	CLI_UPGRADE_PROMPT      = "There is a more recent version of the Astronomer CLI available.\nYou can install the latest tagged release with the following command"
	CLI_UNTAGGED_PROMPT     = "Your current Astronomer CLI is not tagged.\nThis is likely the result of building from source. You can install the latest tagged release with the following command"

	CONFIG_CREATE_DIR_ERROR        = "Error creating config directory"
	CONFIG_CREATE_HOME_ERROR       = "Error creating default config in home dir: %s"
	CONFIG_CREATE_FILE_ERROR       = "Error creating config file"
	CONFIG_DOMAIN_NOT_SET_ERROR    = "No domain specified (`cloud.domain` in config.yaml). Use -d to pass your cluster domain\n\nEx.\nastro auth login -d EXAMPLE_DOMAIN.com\n "
	CONFIG_PATH_KEY_MISSING_ERROR  = "Must specify config key"
	CONFIG_PATH_KEY_INVALID_ERROR  = "Config does not exist, check your config key"
	CONFIG_PROJECT_NAME_ERROR      = "Project name is invalid"
	CONFIG_PROJECT_DIR_ERROR       = "Error: Not in an astronomer project directory"
	CONFIG_INIT_PROJECT_CONFIG     = "Initialized empty astronomer project in %s"
	CONFIG_INVALID_SET_ARGS        = "Must specify exactly two arguments (key value) when setting a config"
	CONFIG_READ_ERROR              = "Error reading config in home dir: %s"
	CONFIG_REINIT_PROJECT_CONFIG   = "Reinitialized existing astronomer project in %s"
	CONFIG_SAVE_ERROR              = "Error saving config"
	CONFIG_SEARCH_ERROR            = "Error searching for project dir: %v"
	CONFIG_SET_SUCCESS             = "Setting %s to %s successfully"
	CONFIG_USE_OUTSIDE_PROJECT_DIR = "You are attempting to %s a project config outside of a project directory\n To %s a global config try\n%s"

	COMPOSE_CREATE_ERROR         = "Error creating docker-compose project"
	COMPOSE_IMAGE_BUILDING_PROMT = "Building image..."
	COMPOSE_STATUS_CHECK_ERROR   = "Error checking docker-compose status"
	COMPOSE_STOP_ERROR           = "Error stopping and removing containers"
	COMPOSE_PAUSE_ERROR          = "Error pausing project containers"
	COMPOSE_PROJECT_RUNNING      = "Project is already running, cannot start"
	COMPOSE_RECREATE_ERROR       = "Error building, (re)creating or starting project containers"
	COMPOSE_PUSHING_IMAGE_PROMPT = "Pushing image to Astronomer registry"
	COMPOSE_LINK_WEBSERVER       = "Airflow Webserver: http://localhost:8080/admin/"
	COMPOSE_LINK_POSTGRES        = "Postgres Database: localhost:5432/postgres"

	EE_LINK_AIRFLOW = "Airflow Dashboard: https://%s-airflow.%s"
	EE_LINK_FLOWER  = "Grafana Dashboard: https://%s-grafana.%s"
	EE_LINK_GRAFANA = "Flower Dashboard: https://%s-flower.%s"

	HOUSTON_DEPLOYING_PROMPT        = "Deploying: %s\n"
	HOUSTON_NO_DEPLOYMENTS_ERROR    = "No airflow deployments found"
	HOUSTON_SELECT_DEPLOYMENT_PROMT = "Select which airflow deployment you want to deploy to:"
	HOUSTON_INVALID_DEPLOYMENT_KEY  = "Invalid deployment selection"

	INPUT_PASSWORD = "Password: "
	INPUT_USERNAME = "Username: "

	REGISTRY_AUTH_SUCCESS        = "Successfully authenticated to %s"
	REGISTRY_AUTH_FAIL           = "Failed to authenticate to registry\nYou can re-authenticate to the registry with\n\t\tastro auth login"
	REGISTRY_DECODE_AUTH_ERROR   = "Error fetching credentials"
	REGISTRY_UNCOMMITTED_CHANGES = "Project directory has uncommmited changes, use `astro airflow deploy [releaseName] -f` to force deploy."
	REGISTRY_USE_DEFAULT         = "No registry set, using default: registry.%s"
	REGISTRY_TAGS_REQUEST_ERROR  = "Error requesting respostory tags"

	NA = "N/A"
)
