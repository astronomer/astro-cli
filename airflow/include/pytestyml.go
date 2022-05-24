package include

import "strings"

// Pytestyml is the pytest docker-compose template
var Pytestyml = strings.TrimSpace(`
version: '3.1'
services:
  test:
    image: {{ .AirflowImage }}
    command: >
      bash -c "pytest {{ .PytestFile }}"
    user: {{ .AirflowUser }}
    environment:
      AIRFLOW__CORE__LOAD_EXAMPLES: "False"
      ASTRONOMER_ENVIRONMENT: local
    volumes:
      - {{ .AirflowHome }}/dags:/usr/local/airflow/dags:ro
      - {{ .AirflowHome }}/plugins:/usr/local/airflow/plugins:{{ .MountLabel }}
      - {{ .AirflowHome }}/include:/usr/local/airflow/include:{{ .MountLabel }}
      - {{ .AirflowHome }}/tests:/usr/local/airflow/tests:{{ .MountLabel }}
      - {{ .AirflowHome }}/.astro:/usr/local/airflow/.astro:{{ .MountLabel }}
    {{ .AirflowEnvFile }}
`)
