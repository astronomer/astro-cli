package include

import "strings"

// Gitignore is the .gitignore template
var Gitignore = strings.TrimSpace(`
.git
.env
airflow_settings.yaml
pod-config.yml
`)
