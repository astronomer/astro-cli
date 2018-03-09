package include

import "strings"

// Dockerfile is the Dockerfile template
var Dockerfile = strings.TrimSpace(`
FROM astronomerinc/ap-airflow:latest-onbuild
`)
