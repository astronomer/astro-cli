package include

import "strings"

// Dockerignore is the .dockerignore template
var Dockerignore = strings.TrimSpace(`
astro
.git
.env
airflow_settings.yaml
logs/
`)
