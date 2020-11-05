package include

import "strings"

// Dockerfile is the Dockerfile template
var Dockerfile = strings.TrimSpace(`
FROM quay.io/astronomer/ap-airflow:%s
`)
