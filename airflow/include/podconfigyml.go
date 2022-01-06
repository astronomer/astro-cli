package include

import "strings"

var PodmanConfigYml = strings.TrimSpace(`
apiVersion: v1
kind: Pod
metadata:
  labels:
    app: {{ .ProjectName }}
  name: {{ .ProjectName }}
spec:
  containers:
  - args:
    - postgres
    command:
    - docker-entrypoint.sh
    env:
    - name: PG_MAJOR
      value: "12"
    - name: PG_VERSION
      value: 12.2-2.pgdg100+1
    - name: POSTGRES_PASSWORD
      value: {{ .PostgresPassword }}
    - name: PGDATA
      value: /var/lib/postgresql/data
    - name: POSTGRES_USER
      value: {{ .PostgresUser }}
    image: docker.io/library/postgres:12.2
    name: postgres
    resources: {}
    securityContext:
      allowPrivilegeEscalation: true
      privileged: false
      readOnlyRootFilesystem: false
    ports:
    - containerPort: 5432
      hostPort: {{ .PostgresPort }}
      protocol: TCP
    workingDir: /
  - args:
    - bash
    - -c
    - (airflow upgradedb || airflow db upgrade) && airflow scheduler
    command:
    - /entrypoint
    env:
    - name: AIRFLOW__CORE__FERNET_KEY
      value: d6Vefz3G9U_ynXB3cr7y_Ak35tAHkEGAVxuz_B-jzWw=
    - name: ASTRONOMER_UID
      value: "50000"
    - name: AIRFLOW_SNOWFLAKE_PARTNER
      value: ASTRONOMER
    - name: AIRFLOW__CORE__EXECUTOR
      value: LocalExecutor
    - name: AIRFLOW_HOME
      value: /usr/local/airflow
    - name: AIRFLOW__CORE__SQL_ALCHEMY_CONN
      value: postgresql://{{ .PostgresUser }}:{{ .PostgresPassword }}@localhost:5432
    - name: AIRFLOW__CORE__LOAD_EXAMPLES
      value: "False"
    - name: ASTRONOMER_USER
      value: astro
{{ .AirflowEnvFile }}
    image: {{ .AirflowImage }}
    name: {{ .SchedulerContainerName }}
    ports:
    - containerPort: 8080
      hostPort: {{ .AirflowWebserverPort }}
      protocol: TCP
    resources: {}
    securityContext:
      allowPrivilegeEscalation: true
      privileged: false
      readOnlyRootFilesystem: false
      runAsGroup: 50000
      runAsUser: 50000
      seLinuxOptions:
        type: spc_t
    volumeMounts:
    - mountPath: /usr/local/airflow/dags
      name: airflow-dags-dir
      readOnly: true
    - mountPath: /usr/local/airflow/plugins
      name: airflow-plugins-dir
    - mountPath: /usr/local/airflow/include
      name: airflow-include-dir
    workingDir: /usr/local/airflow
  - args:
    - bash
    - -c
    - |-
      if [[ -z "$AIRFLOW__API__AUTH_BACKEND" ]] && [[ $(pip show -f apache-airflow | grep basic_auth.py) ]];
        then export AIRFLOW__API__AUTH_BACKEND=airflow.api.auth.backend.basic_auth ;
        else export AIRFLOW__API__AUTH_BACKEND=airflow.api.auth.backend.default ; fi &&
        { airflow create_user "$@" || airflow users create "$@" ; } &&
        { airflow sync_perm || airflow sync-perm ;} &&
        airflow webserver
    - --
    - -r
    - Admin
    - -u
    - admin
    - -e
    - admin@example.com
    - -f
    - admin
    - -l
    - user
    - -p
    - admin
    command:
    - /entrypoint
    env:
    - name: ASTRONOMER_USER
      value: astro
    - name: AIRFLOW_HOME
      value: /usr/local/airflow
    - name: AIRFLOW__WEBSERVER__RBAC
      value: "True"
    - name: AIRFLOW__CORE__EXECUTOR
      value: LocalExecutor
    - name: ASTRONOMER_UID
      value: "50000"
    - name: AIRFLOW__CORE__FERNET_KEY
      value: d6Vefz3G9U_ynXB3cr7y_Ak35tAHkEGAVxuz_B-jzWw=
    - name: AIRFLOW__CORE__SQL_ALCHEMY_CONN
      value: postgresql://{{ .PostgresUser }}:{{ .PostgresPassword }}@localhost:5432
    - name: AIRFLOW_SNOWFLAKE_PARTNER
      value: ASTRONOMER
    - name: AIRFLOW__CORE__LOAD_EXAMPLES
      value: "False"
{{ .AirflowEnvFile }}
    image: {{ .AirflowImage }}
    name: {{ .WebserverContainerName }}
    resources: {}
    securityContext:
      allowPrivilegeEscalation: true
      privileged: false
      readOnlyRootFilesystem: false
      runAsGroup: 50000
      runAsUser: 50000
      seLinuxOptions:
        type: spc_t
    volumeMounts:
    - mountPath: /usr/local/airflow/dags
      name: airflow-dags-dir
    - mountPath: /usr/local/airflow/plugins
      name: airflow-plugins-dir
    - mountPath: /usr/local/airflow/include
      name: airflow-include-dir
    workingDir: /usr/local/airflow
{{if .TriggererEnabled}}
  - args:
    - bash
    - -c
    - (airflow upgradedb || airflow db upgrade) && airflow triggerer
    command:
    - /entrypoint
    env:
    - name: ASTRONOMER_USER
      value: astro
    - name: AIRFLOW_HOME
      value: /usr/local/airflow
    - name: AIRFLOW__WEBSERVER__RBAC
      value: "True"
    - name: AIRFLOW__CORE__EXECUTOR
      value: LocalExecutor
    - name: ASTRONOMER_UID
      value: "50000"
    - name: AIRFLOW__CORE__FERNET_KEY
      value: d6Vefz3G9U_ynXB3cr7y_Ak35tAHkEGAVxuz_B-jzWw=
    - name: AIRFLOW__CORE__SQL_ALCHEMY_CONN
      value: postgresql://{{ .PostgresUser }}:{{ .PostgresPassword }}@localhost:5432
    - name: AIRFLOW_SNOWFLAKE_PARTNER
      value: ASTRONOMER
    - name: AIRFLOW__CORE__LOAD_EXAMPLES
      value: "False"
{{ .AirflowEnvFile }}
    image: {{ .AirflowImage }}
    name: {{ .TriggererContainerName }}
    resources: {}
    securityContext:
      allowPrivilegeEscalation: true
      privileged: false
      readOnlyRootFilesystem: false
      runAsGroup: 50000
      runAsUser: 50000
      seLinuxOptions:
        type: spc_t
    volumeMounts:
    - mountPath: /usr/local/airflow/dags
      name: airflow-dags-dir
    - mountPath: /usr/local/airflow/plugins
      name: airflow-plugins-dir
    - mountPath: /usr/local/airflow/include
      name: airflow-include-dir
    workingDir: /usr/local/airflow
{{end}}
  dnsConfig: {}
  restartPolicy: OnFailure
  volumes:
  - hostPath:
      path: {{ .AirflowHome }}/dags
      type: Directory
    name: airflow-dags-dir
  - hostPath:
      path: {{ .AirflowHome }}/plugins
      type: Directory
    name: airflow-plugins-dir
  - hostPath:
      path: {{ .AirflowHome }}/include
      type: Directory
    name: airflow-include-dir
status: {}
`)
