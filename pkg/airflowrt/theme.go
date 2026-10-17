package airflowrt

import _ "embed"

// UIThemeEnv is the Airflow setting that themes Airflow 3.2+'s UI.
const UIThemeEnv = "AIRFLOW__API__THEME"

// uiTheme is the Airflow UI theme Astro Runtime ships, a verbatim copy of
// astro-runtime's image/files/airflow-ui-theme.json. Runtime's entrypoint
// exports it as UIThemeEnv, which makes it how Airflow looks on Astro.
//
//go:embed airflow-ui-theme.json
var uiTheme string

// UITheme returns Astro Runtime's Airflow UI theme, the JSON document
// UIThemeEnv takes.
func UITheme() string { return uiTheme }
