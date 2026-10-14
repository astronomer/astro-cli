package proxy

import (
	"embed"
	"html/template"
	"io"
)

//go:embed templates/*.html
var templateFS embed.FS

// pageTmpl holds every generated page, executed by file name, so the pages can
// share base.html's head, starfield and copy.
var pageTmpl = template.Must(template.ParseFS(templateFS, "templates/*.html"))

const (
	landingPageName     = "landing.html"
	notFoundPageName    = "notfound.html"
	unavailablePageName = "unavailable.html"

	defaultTitle = "Astro Local Proxy"
)

// Pages is the copy on the pages the proxy generates itself, for the parts that
// differ between the tools that run it. The zero value is the CLI's.
//
// Copy rather than templates: the pages are shared so that every tool serving
// these routes looks the same, and a host that swaps in its own markup to change
// one sentence starts the two copies drifting again.
type Pages struct {
	// Title names this proxy on its landing page. Empty is "Astro Local Proxy".
	Title string

	// StartHint is the sentence that tells someone how to start a project,
	// shown on the landing page with no projects and on the not-found page.
	// Empty points at `astro local start`, which suits a shell and not an app.
	StartHint string
}

func (pg Pages) title() string {
	if pg.Title == "" {
		return defaultTitle
	}
	return pg.Title
}

type landingData struct {
	Title  string
	Pages  Pages
	Routes []LandingRoute
}

type notFoundData struct {
	Pages    Pages
	Hostname string
	Port     string
}

// RenderUnavailable writes the page shown while a project's backend is not
// answering. It polls the page's own URL and reloads once the backend is up.
//
// Exported for a host that serves Airflow outside this proxy, and so has its own
// error path to put the page on: the desktop's iframe proxies.
func RenderUnavailable(w io.Writer) error {
	return pageTmpl.ExecuteTemplate(w, unavailablePageName, nil)
}
