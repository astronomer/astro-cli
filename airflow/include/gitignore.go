package include

import "strings"

// Gitignore is the .gitignore template
var Gitignore = strings.TrimSpace(`
.git
.env
airflow_setttings.yaml
`)
