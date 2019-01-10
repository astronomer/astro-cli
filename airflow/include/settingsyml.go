package include

import "strings"

var Settingsyml = strings.TrimSpace(`
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
