package airflowrt

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"sync"
)

// UIThemeEnv is the Airflow setting that themes Airflow 3.2+'s UI.
const UIThemeEnv = "AIRFLOW__API__THEME"

// uiTheme is the Airflow UI theme Astro Runtime ships, a verbatim copy of
// astro-runtime's image/files/airflow-ui-theme.json. Runtime's entrypoint
// exports it as UIThemeEnv, which makes it how Airflow looks on Astro.
//
//go:embed airflow-ui-theme.json
var uiTheme string

// UITheme returns Astro Runtime's Airflow UI theme, the JSON document
// UIThemeEnv takes, compacted to one line: it goes into the environment of
// every standalone Airflow process, where a multi-line value is noise to
// anything that reads that environment line by line.
var UITheme = sync.OnceValue(func() string {
	var b bytes.Buffer
	if err := json.Compact(&b, []byte(uiTheme)); err != nil {
		// pkg/proxy's tests parse this file; one that is not JSON never ships.
		return uiTheme
	}
	return b.String()
})
