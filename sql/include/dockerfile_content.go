package include

import "strings"

// Dockerfile is the Dockerfile template

var Dockerfile = strings.TrimSpace(`
FROM %s

# build-essential is necessary to be able to build wheels for snowflake-connector-python
RUN apt-get update -y && apt-get install -y --no-install-recommends \
        build-essential

ENV ASTRO_CLI Yes

RUN pip install astro-sql-cli==%s
`)
