module github.com/astronomer/astro-cli/pkg/platformversions

go 1.26.1

// No requires a consumer inherits, and none expected: this is a table of
// Airflow versions the managed platforms offer plus the rules for mapping a
// pin onto them, and the package itself imports only strings. It is a pure
// leaf on purpose — two consumers that are otherwise unrelated share it
// (pkg/checks picks the version a pre-flight check runs against, pack pins a
// constraints file), so it holds the data and neither owns it.
//
// testify below is test-only, and pinned to the main module rather than to
// whatever go mod tidy resolves: a fresh module tidies to latest, which is how
// a promotion once raised the AWS SDK four minors without anyone asking.

require github.com/stretchr/testify v1.12.0

require gopkg.in/yaml.v3 v3.0.1 // indirect
