package proxy

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The not-found heading is a contract, not copy. The CLI's takeover of an astro
// 1.x proxy (airflow/proxy's notFoundHeading) recognizes an astro proxy by this
// exact markup, so a restyle that recases or punctuates it would stop matching.
// Spelled out here rather than imported: pkg/proxy cannot import the root module.
func TestNotFoundKeepsTheHeadingTheTakeoverProbesFor(t *testing.T) {
	p := NewProxy("6563", testStore(t))
	rec := httptest.NewRecorder()
	p.notFoundPage(rec, "nothing.localhost")

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Contains(t, rec.Body.String(), "<h1>Project Not Found</h1>")
}

// A host changes the copy, not the page.
func TestPagesCarryTheHostsCopy(t *testing.T) {
	p := NewProxy("6564", testStore(t))
	p.Pages = Pages{Title: "Astro Desktop Proxy", StartHint: "Start a project in Astro Desktop."}

	rec := httptest.NewRecorder()
	p.landingPage(rec)
	landing := rec.Body.String()
	assert.Contains(t, landing, "<title>Astro Desktop Proxy</title>")
	assert.Contains(t, landing, "Start a project in Astro Desktop.")
	assert.NotContains(t, landing, "astro local start", "the hint replaces the CLI command, it does not sit beside it")
	assert.NotContains(t, landing, defaultTitle)

	rec = httptest.NewRecorder()
	p.notFoundPage(rec, "nothing.localhost")
	notFound := rec.Body.String()
	assert.Contains(t, notFound, "Start a project in Astro Desktop.")
	assert.NotContains(t, notFound, "astro local start")
	assert.Contains(t, notFound, `href="http://localhost:6564"`, "the way back is this proxy's own port")
}

// Everything a page prints that someone else chose is escaped: the Host header
// is the client's, and the hint and the route fields are the host's.
func TestPagesEscapeWhatTheyPrint(t *testing.T) {
	const evil = `<script>alert(1)</script>`
	p := NewProxy("6563", testStore(t))
	p.Pages = Pages{Title: evil, StartHint: evil}

	rec := httptest.NewRecorder()
	p.notFoundPage(rec, evil)
	assert.NotContains(t, rec.Body.String(), evil)

	rec = httptest.NewRecorder()
	p.landingPage(rec)
	assert.NotContains(t, rec.Body.String(), evil)

	var b strings.Builder
	require.NoError(t, pageTmpl.ExecuteTemplate(&b, landingPageName, landingData{
		Title:  "t",
		Routes: []LandingRoute{{Name: evil, URL: "http://x.localhost:6563", Port: "1", ProjectDir: evil}},
	}))
	assert.NotContains(t, b.String(), evil)
}

// The pages fetch nothing. The proxy serves them with no network, and a page
// that reached out for a font or a script would also tell a third party which
// local projects someone has open.
func TestPagesAreSelfContained(t *testing.T) {
	pages := map[string]any{
		landingPageName:  landingData{Title: "t", Routes: []LandingRoute{{Name: "p", URL: "http://p.localhost:1", Port: "2"}}},
		notFoundPageName: notFoundData{Hostname: "h.localhost", Port: "1"},
	}
	for name, data := range pages {
		var b strings.Builder
		require.NoError(t, pageTmpl.ExecuteTemplate(&b, name, data), name)
		for _, ref := range []string{"https://", "//fonts.", "<link", "src="} {
			assert.NotContains(t, b.String(), ref, "%s loads something from outside", name)
		}
	}
}
