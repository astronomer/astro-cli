package include

import "strings"

// Settingsyml is the settings template
var Settingsyml = strings.TrimSpace(`
  # This feature is in Beta.
  # Please report any bugs to https://github.com/astronomer/astro-cli/issues
  airflow:
    connections:
      - conn_id:
        conn_type:
        conn_host:
        conn_login:
        conn_password:
        conn_port:
    pools:
      - pool_name:
        pool_slot:
        pool_description:
    variables:
      - variable_name:
        variable_value:`)
