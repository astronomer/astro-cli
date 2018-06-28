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

	CONFIG_PROJECT_NAME_ERROR = "Project name is invalid"

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

	NA = "N/A"
)
