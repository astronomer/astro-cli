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

RUN pip install "astro-sql-cli %s" %s

RUN id -u %s 2>/dev/null || useradd --uid %s --create-home %s
# This is necessary to run the docker image in GNU Linux since Astro CLI 1.8
# It is temporary, since some SQL commands still rely on the default airflow config
# https://github.com/astronomer/astro-sdk/issues/1219
RUN chmod -R 777 /usr/local/airflow

ENTRYPOINT ["flow"]
CMD ["--help"]
`)
