package include

import "strings"

// Dockerfile is the Dockerfile template

var Dockerfile = strings.TrimSpace(`
FROM %s

ENV ASTRO_CLI Yes
ENV AIRFLOW__ASTRONOMER__UPDATE_CHECK_INTERVAL 0

# build-essential is necessary to be able to build wheels for snowflake-connector-python
RUN apt-install-and-clean \
        build-essential

RUN pip install astro-sql-cli==%s
`)
