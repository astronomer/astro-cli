package include

import "strings"

// Dockerfile is the Dockerfile template

var Dockerfile = strings.TrimSpace(`
FROM python:%s-slim-bullseye

RUN apt-get update -y && apt-get install -y gcc g++ libpq-dev python-dev libssl-dev libffi-dev

RUN pip install astro-sql-cli==%s
`)
