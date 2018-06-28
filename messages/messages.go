package messages

var (
	ERROR_INVALID_CLI_VERSION     = "Astronomer CLI version is not valid"
	ERROR_GITHUB_JSON_MARSHALLING = "Failed to JSON decode Github response from %s"

	INFO_CURR_CLI_VERSION        = "Astro CLI Version: v%s"
	INFO_CURR_CLI_COMMIT         = "Git Commit: %s"
	INFO_CURR_CLI_VERSION_DATE   = INFO_CURR_CLI_VERSION + " (%s)"
	INFO_LATEST_CLI_VERSION      = "Astro CLI Latest: %s "
	INFO_LATEST_CLI_VERSION_DATE = INFO_LATEST_CLI_VERSION + " (%s)"
	INFO_CLI_INSTALL_CMD         = "\t$ curl -sL https://install.astronomer.io | sudo bash"
	INFO_UPGRADE_CLI             = "There is a more recent version of the Astronomer CLI available.\nYou can install the latest tagged release with the following command"
	INFO_UNTAGGED_VERSION        = "Your current Astronomer CLI is not tagged.\nThis is likely the result of building from source. You can install the latest tagged release with the following command"

	NA = "N/A"
)
