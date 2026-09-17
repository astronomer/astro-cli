// The e2e suite is its own module so that a dependency it takes on — a test
// fixture library, an HTTP probe, an Airflow client — can never reach the
// binary we ship. The root module's go.sum stays the shipping surface.
//
// It deliberately requires nothing. The suite drives the CLI as a user does,
// through argv and stdout, so it needs no astro-cli package; importing the code
// under test is how an end-to-end test quietly becomes a unit test that agrees
// with itself. Where a case genuinely needs a fixture from pkg/*, add the
// require and a `replace ../` then, and not before.
module github.com/astronomer/astro-cli/e2e

go 1.26.1
