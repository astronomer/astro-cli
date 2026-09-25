package runtimeversions

import "context"

// FallbackAirflowSeries is the Airflow series a new project starts on when the
// CLI has no current copy of the catalog: none could be read, or the only copy
// is a stale cache naming an older series than this, or the catalog names no
// qualifying Airflow 3 series at all.
//
// It is never a floor under a current answer. A fresh cache or a successful
// fetch is authoritative, including when it answers below this constant: that
// is how Astronomer holds a series back after the constant has reached it, by
// yanking or deprecating its builds.
//
// It is kept current by .github/workflows/bump-default-airflow.yml, which
// applies LatestAirflowSeries's rule to the live catalog weekly and opens a PR
// when the answer moves up. Edit it through that workflow's script, not by
// hand.
const FallbackAirflowSeries = "3.3"

// Default is the Airflow series and requires-python a new project starts on.
// It never fails.
//
// A current catalog (SourceCatalog or SourceCache) is authoritative: the series
// is its LatestAirflowSeries("3"), whatever FallbackAirflowSeries says, and src
// is where it came from.
//
// A stale cache is used only when it is not behind the binary: its answer is
// floored at FallbackAirflowSeries, so a months-old copy cannot start a project
// on an older series than the binary knows. A floored answer reports
// SourceFallback, since the version came from the binary, and the fetch that
// would have given a current answer failed.
//
// With no catalog at all the answer is FallbackAirflowSeries with
// SourceFallback. A current catalog naming no qualifying Airflow 3 series gives
// FallbackAirflowSeries with SourceCatalogEmpty, so a caller can say the
// catalog was read and had nothing usable; a stale one gives SourceFallback.
//
// requiresPython is the catalog's RequiresPython for the series, and empty for
// either fallback answer or when the catalog lists no Python for the series.
// The caller then applies its own built-in rule; pkg/scaffold does.
func Default(ctx context.Context, o Options) (series, requiresPython string, src Source) {
	c, src, err := Load(ctx, o)
	if err != nil {
		return FallbackAirflowSeries, "", SourceFallback
	}
	series, ok := c.LatestAirflowSeries("3")
	switch {
	case !ok && src != SourceStaleCache:
		return FallbackAirflowSeries, "", SourceCatalogEmpty
	case !ok, src == SourceStaleCache && compareVersions(series, FallbackAirflowSeries) < 0:
		return FallbackAirflowSeries, "", SourceFallback
	}
	requiresPython, _ = c.RequiresPython(series)
	return series, requiresPython, src
}
