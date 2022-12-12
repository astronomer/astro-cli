package include

import "strings"

// Gitignore is the .gitignore template
var Gitignore = strings.TrimSpace(`
.git
.env
.DS_Store # macOS specific ignore
airflow_settings.yaml
__pycache__/
astro
`)
