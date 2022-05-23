package include

import "strings"

// Settingsyml is the settings template
var Settingsyml = strings.TrimSpace(`

# This file allows you to configure Airflow Connections, Pools, and Variables in a single place for local development only.
# NOTE: If putting a dict in conn_extra, please wrap in single quotes.

# For more information, refer to our docs: https://docs.astronomer.io/develop-project#configure-airflow_settingsyaml-local-development-only
# For issues or questions, reach out to: https://support.astronomer.io

airflow:
  connections:
    - conn_id:
      conn_type:
      conn_host:
      conn_schema:
      conn_login:
      conn_password:
      conn_port:
      conn_extra:
  pools:
    - pool_name:
      pool_slot:
      pool_description:
  variables:
    - variable_name:
      variable_value:`)
