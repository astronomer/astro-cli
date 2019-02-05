package include

import "strings"

// Gitignore is the .gitignore template
var Gitignore = strings.TrimSpace(`
.astro
.git
.env
airflow_setttings.yaml
`)
