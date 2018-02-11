package airflow

// dockerfile is the Dockerfile template
const dockerfile = `FROM astronomerinc/ap-airflow:latest-onbuild`

// dockerignore is the .dockerignore template
const dockerignore = `.astro
.git
`
