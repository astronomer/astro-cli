// Package apirequest is the half of a raw API call that does not depend on
// which Airflow it goes to: the -F/-f fields, the -H headers, the query string
// fields become on a GET, the -i header block, and the `ls` listing and `spec`
// document read out of an OpenAPI spec.
//
// Two command trees share it. `astro api airflow` (and `astro api cloud`)
// builds its own HTTP requests, so it can generate curl and paginate;
// `astro local api` sends through the local Airflow's transport, whose
// credentials the engine minted. Their targets and credentials differ and stay
// in each command. What someone types after the endpoint means the same thing
// in both, and lives here so it is written once.
package apirequest
