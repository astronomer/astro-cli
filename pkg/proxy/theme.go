package proxy

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// themeTagID marks the theme's <style> block. InjectAirflowTheme looks for it
// before injecting, so a page that already carries the theme is left alone.
const themeTagID = "astro-theme"

// runtimeThemeJSON is the Airflow UI theme Astro Runtime ships, a verbatim copy
// of astro-runtime's image/files/airflow-ui-theme.json. Runtime hands it to
// Airflow 3.2+ as AIRFLOW__API__THEME, which makes it how Airflow looks on
// Astro; the proxy paints the same tokens so a local Airflow looks the same.
//
//go:embed airflow-ui-theme.json
var runtimeThemeJSON []byte

// cssVar is one CSS custom property the theme sets.
type cssVar struct{ name, value string }

// airflowTheme is the part of an Airflow UI theme the proxy paints.
type airflowTheme struct {
	// colors are the theme's color tokens as Chakra's CSS variables, in a
	// stable order: palettes by name, a scale's steps from lightest to darkest.
	colors []cssVar
	// black and white are the page backgrounds Airflow paints from the theme:
	// Chakra's bg token is black in dark mode and white in light.
	black, white string
}

// parseAirflowTheme reads the color tokens of an AIRFLOW__API__THEME document.
// A token is a single value ({"value": ...}, as black and white are) or a scale
// of numbered steps, each one a single value (brand, gray).
func parseAirflowTheme(raw []byte) (airflowTheme, error) {
	type token struct {
		Value string `json:"value"`
	}
	var doc struct {
		Tokens struct {
			Colors map[string]json.RawMessage `json:"colors"`
		} `json:"tokens"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return airflowTheme{}, fmt.Errorf("parsing the Airflow theme: %w", err)
	}

	names := make([]string, 0, len(doc.Tokens.Colors))
	for name := range doc.Tokens.Colors {
		names = append(names, name)
	}
	sort.Strings(names)

	var theme airflowTheme
	for _, name := range names {
		var single token
		if err := json.Unmarshal(doc.Tokens.Colors[name], &single); err == nil && single.Value != "" {
			theme.colors = append(theme.colors, cssVar{"--chakra-colors-" + name, single.Value})
			switch name {
			case "black":
				theme.black = single.Value
			case "white":
				theme.white = single.Value
			}
			continue
		}
		var scale map[string]token
		if err := json.Unmarshal(doc.Tokens.Colors[name], &scale); err != nil {
			return airflowTheme{}, fmt.Errorf("color %q in the Airflow theme: %w", name, err)
		}
		steps := make([]int, 0, len(scale))
		for step, tok := range scale {
			n, err := strconv.Atoi(step)
			if err != nil || tok.Value == "" {
				return airflowTheme{}, fmt.Errorf("color %q in the Airflow theme: step %q is not a numbered color", name, step)
			}
			steps = append(steps, n)
		}
		sort.Ints(steps)
		for _, n := range steps {
			step := strconv.Itoa(n)
			theme.colors = append(theme.colors, cssVar{"--chakra-colors-" + name + "-" + step, scale[step].Value})
		}
	}
	if theme.black == "" || theme.white == "" {
		return airflowTheme{}, errors.New("the Airflow theme has no black and white tokens to paint the page with")
	}
	return theme, nil
}

// css sets the theme's color tokens on an Airflow 3 page.
//
// Only the palettes are set, the way AIRFLOW__API__THEME sets them: Airflow's
// semantic tokens (bg, border, brand.solid, ...) refer to palette steps and pick
// a different step per color mode, so light and dark both come out as they do
// on Astro. The rule is unlayered, which outranks the variables Chakra keeps in
// its tokens layer: Airflow's own palette before 3.2, and on 3.2+ the same
// values again when the Runtime image has set the theme. Airflow 3.0 has no
// brand palette, so it takes the theme's neutrals and keeps its blue accent.
//
// html.light and html.dark are the classes next-themes puts on Airflow 3's
// root. Airflow 2 never sets them, and Runtime does not theme it either, so it
// keeps its own colors.
func (t airflowTheme) css() string {
	var b strings.Builder
	b.WriteString("html.light, html.dark {\n")
	for _, v := range t.colors {
		fmt.Fprintf(&b, "  %s: %s;\n", v.name, v.value)
	}
	b.WriteString("}\n")
	return b.String()
}

// scrollbarCSS gives every Airflow page the thin scrollbars Astro's own
// surfaces use. Translucent gray rather than a Chakra color token: the thumb
// must read on both of Airflow's themes and outside Chakra-styled subtrees (log
// panes, Airflow 2).
const scrollbarCSS = `::-webkit-scrollbar {
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
func prePaintScript(dark, light string) string {
	return `<script>
(function(){
  var pref;
  try { pref = localStorage.getItem('theme'); } catch(e){}
  var dark = pref === 'dark' ||
    (pref !== 'light' && window.matchMedia &&
     window.matchMedia('(prefers-color-scheme: dark)').matches);
  var s = document.createElement('style');
  s.textContent = 'html:not(.light):not(.dark){background:' +
    (dark ? '` + dark + `' : '` + light + `') + '}';
  document.head.appendChild(s);
  addEventListener('DOMContentLoaded', function(){
    if (!document.getElementById('root')) s.remove();
  });
})();
</script>`
}

// airflowThemeSnippet is everything that has to be in place before an Airflow
// page's first paint, injected in one pass.
var airflowThemeSnippet = func() string {
	theme, err := parseAirflowTheme(runtimeThemeJSON)
	if err != nil {
		panic(err)
	}
	return `<style id="` + themeTagID + `">` + theme.css() + scrollbarCSS + `</style>` +
		prePaintScript(theme.black, theme.white)
}()

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

// InjectIntoHTML inserts snippet into an HTML response immediately before
// beforeTag, matched ignoring case: the first "</head>", or the last occurrence
// of any other tag such as "</body>". It reads through gzip and writes the same
// encoding back.
//
// Responses it should not rewrite pass through untouched: anything not HTML, a
// HEAD or bodyless response, and an encoding other than none, identity or gzip.
// A body that fails to read or decode is an error, since part of it is already
// consumed and cannot be passed on as the page.
func InjectIntoHTML(resp *http.Response, snippet, beforeTag string) error {
	return injectIntoHTML(resp, snippet, beforeTag, "")
}

// markerIndex is where a snippet anchored on marker goes in body, or -1.
//
// </head> anchors on its first occurrence: the real one closes the head near
// the top, and a later "</head>" is body content (an inline script's string, a
// rendered Dag doc). Any other marker anchors on its last, which for </body> is
// the real close at the end, past any earlier mention in a script.
//
// Matched ignoring ASCII case, byte by byte, not by searching a lowered copy:
// bytes.ToLower changes the length of invalid UTF-8 (one byte becomes a
// three-byte replacement character) and of runes whose lower case is a
// different size (the Kelvin sign), so an index found in the copy can point
// anywhere in the original. HTML tag names are ASCII, so folding A-Z is all the
// match needs.
func markerIndex(body []byte, marker string) int {
	first := equalFoldASCII([]byte(marker), "</head>")
	n := len(body) - len(marker)
	for k := 0; k <= n; k++ {
		i := n - k
		if first {
			i = k
		}
		if equalFoldASCII(body[i:i+len(marker)], marker) {
			return i
		}
	}
	return -1
}

func equalFoldASCII(b []byte, s string) bool {
	if len(b) != len(s) {
		return false
	}
	for i := range b {
		if lowerASCII(b[i]) != lowerASCII(s[i]) {
			return false
		}
	}
	return true
}

func lowerASCII(c byte) byte {
	if 'A' <= c && c <= 'Z' {
		return c + ('a' - 'A')
	}
	return c
}

// rewritable reports whether resp is an HTML page the injector can safely read
// and rewrite. Everything else passes through as it came:
//
//   - Anything not HTML.
//   - A HEAD response or a status that carries no body (1xx, 204, 304). Their
//     headers describe a page that isn't there to read, and rewriting
//     Content-Length from an empty read would tell the client the page is empty.
//   - An encoding other than none, identity or gzip. The injector can't read
//     brotli, deflate or zstd, and stripping their Content-Encoding would have
//     the browser render compressed bytes as text.
func rewritable(resp *http.Response) bool {
	if !strings.Contains(resp.Header.Get("Content-Type"), "text/html") {
		return false
	}
	if resp.Request != nil && resp.Request.Method == http.MethodHead {
		return false
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusNotModified {
		return false
	}
	switch resp.Header.Get("Content-Encoding") {
	case "", "identity", "gzip":
		return true
	default:
		return false
	}
}

// injectIntoHTML is InjectIntoHTML that also leaves the body alone when the
// part before the marker already contains unlessPresent (empty means always
// inject).
func injectIntoHTML(resp *http.Response, snippet, beforeTag, unlessPresent string) error {
	if !rewritable(resp) {
		return nil
	}

	var reader io.ReadCloser
	wasGzipped := resp.Header.Get("Content-Encoding") == "gzip"
	// A read that fails has already consumed part of the body, so there is no
	// passing the page on unchanged: the error goes back to the proxy, which
	// answers with its error handler rather than a truncated page under the
	// original Content-Length.
	if wasGzipped {
		gr, err := gzip.NewReader(resp.Body)
		if err != nil {
			return fmt.Errorf("reading a gzip page: %w", err)
		}
		reader = gr
	} else {
		reader = resp.Body
	}

	body, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil {
		return fmt.Errorf("reading the page: %w", err)
	}

	// unlessPresent is looked for only before the marker: for the theme that is
	// the head, and a body that mentions the theme's id (a Dag doc, a log line)
	// is still an unthemed page.
	if idx := markerIndex(body, beforeTag); idx >= 0 {
		if unlessPresent == "" || !bytes.Contains(body[:idx], []byte(unlessPresent)) {
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
