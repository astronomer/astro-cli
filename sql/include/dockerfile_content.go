package include

import "strings"

// Dockerfile is the Dockerfile template

var Dockerfile = strings.TrimSpace(`
FROM %s

ENV ASTRO_CLI Yes

RUN pip install astro-sql-cli==%s
`)
