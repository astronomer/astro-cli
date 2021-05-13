package include

import "strings"

// Settingsyml is the settings template
var Settingsyml = strings.TrimSpace(`

# Please report any bugs to support@astronomer.io

# NOTE: If putting a dict in conn_extra, please wrap in single quotes.

# For more information, refer to our docs: https://www.astronomer.io/docs/cloud/stable/develop/customize-image#configure-airflowsettingsyaml

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
