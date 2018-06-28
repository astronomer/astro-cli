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

	NA = "N/A"
)
