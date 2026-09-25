module github.com/astronomer/astro-cli/pkg/runtimeversions

go 1.26.1

// No requires, test-only or otherwise, and none expected: this reads one public
// JSON document over net/http and caches it with os. Everything that links it
// (astro init, the Airflow 2 image lookup, Astro Desktop's runtime poller) pays
// for nothing beyond the standard library, which is the point of it being a
// leaf of its own rather than a corner of pkg/imagebuild.
