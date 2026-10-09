package proxy

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// themeTagID marks the theme's <style> block. InjectAirflowTheme looks for it
// before injecting, so a page that already carries the theme is left alone.
const themeTagID = "astro-theme"

// Airflow's own page backgrounds, not brand colors: what Airflow 3 paints once
// booted (its dark token resolves to oklch(0.23185 0.0323 266.44)). They are
// only on screen for the moment before Airflow's bundle runs; see
// prePaintScript.
const (
	airflowPageDark  = "#161d2d"
	airflowPageLight = "#ffffff"
)

// The Astronomer colors the theme maps onto Airflow's Chakra tokens.
const (
	newMoon80 = "#343247"
	purple60  = "#872DED"
	gold      = "#FFB32D"
	success   = "#19BA5A"
	danger    = "#F03A47"
	info      = "#2676FF"
)

// airflowCSS themes Airflow 3's Chakra UI with Astronomer brand colors, and
// gives every Airflow page the thin scrollbars Astro's own surfaces use.
const airflowCSS = `
:root {
  --chakra-colors-brand-muted: ` + newMoon80 + `;
  --chakra-colors-brand-emphasized: #434060;
  --chakra-colors-brand-500: ` + purple60 + `;
  --chakra-colors-brand-600: ` + purple60 + `;
  --chakra-colors-fg-muted: #b8b5c4;
  --chakra-colors-blue-500: ` + purple60 + `;
  --chakra-colors-blue-600: ` + purple60 + `;
  --chakra-colors-green-500: ` + success + `;
  --chakra-colors-red-500: ` + danger + `;
  --chakra-colors-orange-500: ` + gold + `;
  --chakra-colors-cyan-500: ` + info + `;
}
/* Thin scrollbars. Translucent gray rather than a Chakra color token: the
 * thumb must read on both of Airflow's themes and outside Chakra-styled
 * subtrees (log panes, Airflow 2). */
::-webkit-scrollbar {
  width: 6px;
  height: 6px;
}
::-webkit-scrollbar-track {
  background: transparent;
}
::-webkit-scrollbar-thumb {
  background: rgba(128, 128, 128, 0.4);
  border-radius: 2px;
}
::-webkit-scrollbar-thumb:hover {
  background: rgba(128, 128, 128, 0.65);
}
::-webkit-scrollbar-corner {
  background: transparent;
}
`

// prePaintScript paints the document background Airflow is about to use, before
// its bundle boots. Airflow 3's HTML carries no background of its own and all of
// its styling arrives with the JS, so between navigation and React's first paint
// the page is the browser's default canvas: a white flash in a dark browser or
// a dark app panel.
//
// The mode is resolved the way Airflow (next-themes) resolves it — the "theme"
// key, falling back to the OS preference — and written as a rule rather than an
// inline style on the root, which would outrank Airflow's own rules and sit on
// top of a theme switch later in the session. The rule deselects itself two
// ways, because a page can stop needing it either way:
//
//   - Airflow 3 marks the root `light` or `dark` as next-themes boots, so the
//     selector stops matching the moment the real theme lands.
//   - Airflow 2 never marks the root at all — its styling is already in the
//     head, so it never needed a bridge — and would otherwise keep this
//     background on the canvas under its own light chrome for the whole
//     session. It has no #root, which is what the removal below tests for.
const prePaintScript = `<script>
(function(){
  var pref;
  try { pref = localStorage.getItem('theme'); } catch(e){}
  var dark = pref === 'dark' ||
    (pref !== 'light' && window.matchMedia &&
     window.matchMedia('(prefers-color-scheme: dark)').matches);
  var s = document.createElement('style');
  s.textContent = 'html:not(.light):not(.dark){background:' +
    (dark ? '` + airflowPageDark + `' : '` + airflowPageLight + `') + '}';
  document.head.appendChild(s);
  addEventListener('DOMContentLoaded', function(){
    if (!document.getElementById('root')) s.remove();
  });
})();
</script>`

// airflowThemeSnippet is everything that has to be in place before an Airflow
// page's first paint, injected in one pass.
const airflowThemeSnippet = `<style id="` + themeTagID + `">` + airflowCSS + `</style>` + prePaintScript

// InjectAirflowTheme puts the Astronomer theme into an Airflow HTML page: the
// brand colors, thin scrollbars, and a background painted before Airflow's
// bundle boots. Non-HTML responses pass through unchanged, and so does a page
// that already carries the theme, so a host that injects it too cannot double
// it up.
//
// The proxy runs it on every response it relays, before a host's
// ModifyResponse hooks. Exported for a host that serves Airflow outside this
// proxy: the desktop's iframe proxies.
func InjectAirflowTheme(resp *http.Response) error {
	return injectIntoHTML(resp, airflowThemeSnippet, "</head>", `id="`+themeTagID+`"`)
}

// InjectIntoHTML inserts snippet into an HTML response immediately before the
// last occurrence of beforeTag (matched case-insensitively, e.g. "</head>" or
// "</body>"). It reads through gzip and writes the same encoding back. Non-HTML
// responses, and bodies that cannot be read or decoded, pass through without
// the snippet.
func InjectIntoHTML(resp *http.Response, snippet, beforeTag string) error {
	return injectIntoHTML(resp, snippet, beforeTag, "")
}

// injectIntoHTML is InjectIntoHTML that also leaves the body alone when it
// already contains unlessPresent (empty means always inject).
func injectIntoHTML(resp *http.Response, snippet, beforeTag, unlessPresent string) error {
	if !strings.Contains(resp.Header.Get("Content-Type"), "text/html") {
		return nil
	}

	var reader io.ReadCloser
	wasGzipped := resp.Header.Get("Content-Encoding") == "gzip"
	if wasGzipped {
		gr, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil
		}
		reader = gr
	} else {
		reader = resp.Body
	}

	body, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil {
		return nil
	}

	if unlessPresent == "" || !bytes.Contains(body, []byte(unlessPresent)) {
		marker := []byte(strings.ToLower(beforeTag))
		if idx := bytes.LastIndex(bytes.ToLower(body), marker); idx >= 0 {
			body = append(body[:idx], append([]byte(snippet), body[idx:]...)...)
		}
	}

	if wasGzipped {
		var buf bytes.Buffer
		gw := gzip.NewWriter(&buf)
		if _, err := gw.Write(body); err != nil {
			return fmt.Errorf("re-encoding page: %w", err)
		}
		if err := gw.Close(); err != nil {
			return fmt.Errorf("re-encoding page: %w", err)
		}
		body = buf.Bytes()
	} else {
		resp.Header.Del("Content-Encoding")
	}

	resp.Body = io.NopCloser(bytes.NewReader(body))
	resp.ContentLength = int64(len(body))
	resp.Header.Set("Content-Length", strconv.Itoa(len(body)))
	return nil
}
