package proxy

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/astronomer/astro-cli/pkg/airflowrt"
)

// themeTagID marks the theme's <style> block. InjectAirflowTheme looks for it
// before injecting, so a page that already carries the theme is left alone.
const themeTagID = "astro-theme"

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

// themeSteps are the steps a palette may have, lightest first: the ones
// Airflow accepts.
var themeSteps = []string{"50", "100", "200", "300", "400", "500", "600", "700", "800", "900", "950"}

// oklchColor is Airflow's own pattern for a theme color (OklchColor in
// airflow/api_fastapi/common/types.py), the one syntax it accepts. Holding a
// value to it also keeps the value from ending the <style> block or the
// pre-paint script's string it is written into.
var oklchColor = regexp.MustCompile(`^oklch\((-?\d+(?:\.\d+)?) (-?\d+(?:\.\d+)?) (-?\d+(?:\.\d+)?)\)$`)

// validOklch reports whether Airflow would take v as a theme color: its
// pattern, and its ranges for lightness, chroma and hue.
func validOklch(v string) bool {
	m := oklchColor.FindStringSubmatch(v)
	if m == nil {
		return false
	}
	var lch [3]float64
	for i, s := range m[1:] {
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return false
		}
		lch[i] = f
	}
	return lch[0] >= 0 && lch[0] <= 1 && lch[1] >= 0 && lch[1] <= 0.5 && lch[2] >= 0 && lch[2] <= 360
}

// themeObject parses raw as a JSON object whose keys are all among allowed.
//
// Keys are matched exactly, as Airflow's model matches them. A struct would not
// do: encoding/json folds case when it fills one, even with
// DisallowUnknownFields, so "Brand" would be painted as brand where Airflow
// refuses it.
func themeObject(raw json.RawMessage, what string, allowed ...string) (map[string]json.RawMessage, error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, fmt.Errorf("parsing the Airflow theme's %s: %w", what, err)
	}
	for key := range obj {
		if !slices.Contains(allowed, key) {
			return nil, fmt.Errorf("the Airflow theme's %s has %q, which Airflow's theme does not", what, key)
		}
	}
	return obj, nil
}

// parseAirflowTheme reads an AIRFLOW__API__THEME document into what the proxy
// paints, holding it to Airflow's own Theme model
// (airflow/api_fastapi/common/types.py). It refuses what Airflow would, and
// also what Airflow would show but the proxy cannot paint (icons, globalCss
// rules), so a theme copied in from Runtime that the proxy would render
// differently fails the tests instead of quietly drifting from Astro.
func parseAirflowTheme(raw []byte) (airflowTheme, error) {
	doc, err := themeObject(raw, "document", "tokens", "globalCss", "icon", "icon_dark_mode")
	if err != nil {
		return airflowTheme{}, err
	}
	if err := refuseUnpaintable(doc); err != nil {
		return airflowTheme{}, err
	}
	colors, err := themeColors(doc)
	if err != nil {
		return airflowTheme{}, err
	}

	var theme airflowTheme
	if theme.black, err = theme.addShade("black", colors["black"]); err != nil {
		return airflowTheme{}, err
	}
	if err := errors.Join(theme.addScale("brand", colors), theme.addScale("gray", colors)); err != nil {
		return airflowTheme{}, err
	}
	if theme.white, err = theme.addShade("white", colors["white"]); err != nil {
		return airflowTheme{}, err
	}
	return theme, nil
}

// refuseUnpaintable refuses what Airflow would show from a theme but the proxy
// cannot paint: an icon, and a globalCss rule. Runtime carries an empty rule
// to work around Airflow 3.2.0, which is nothing to paint.
func refuseUnpaintable(doc map[string]json.RawMessage) error {
	for _, name := range []string{"icon", "icon_dark_mode"} {
		if icon, ok := doc[name]; ok && string(icon) != "null" {
			return fmt.Errorf("the Airflow theme sets %s, which the proxy cannot paint", name)
		}
	}
	raw, ok := doc["globalCss"]
	if !ok {
		return nil
	}
	var rules map[string]map[string]json.RawMessage
	if err := json.Unmarshal(raw, &rules); err != nil {
		return fmt.Errorf("parsing the Airflow theme's globalCss: %w", err)
	}
	for selector, r := range rules {
		if len(r) > 0 {
			return fmt.Errorf("the Airflow theme has globalCss rules for %q, which the proxy cannot paint", selector)
		}
	}
	return nil
}

// themeColors is the theme's tokens.colors, which must hold black and white:
// they are what the proxy paints the page with before Airflow boots.
func themeColors(doc map[string]json.RawMessage) (map[string]json.RawMessage, error) {
	errNoColors := errors.New("the Airflow theme has no colors")
	tokensRaw, ok := doc["tokens"]
	if !ok {
		return nil, errNoColors
	}
	tokens, err := themeObject(tokensRaw, "tokens", "colors")
	if err != nil {
		return nil, err
	}
	colorsRaw, ok := tokens["colors"]
	if !ok {
		return nil, errNoColors
	}
	colors, err := themeObject(colorsRaw, "colors", "brand", "gray", "black", "white")
	if err != nil {
		return nil, err
	}
	for _, name := range []string{"black", "white"} {
		if _, ok := colors[name]; !ok {
			return nil, errors.New("the Airflow theme has no black and white tokens to paint the page with")
		}
	}
	return colors, nil
}

// addShade adds the color {"value": "oklch(...)"} as the variable for name, and
// returns its value.
func (t *airflowTheme) addShade(name string, raw json.RawMessage) (string, error) {
	shade, err := themeObject(raw, "color "+name, "value")
	if err != nil {
		return "", err
	}
	var v string
	if raw, ok := shade["value"]; ok {
		if err := json.Unmarshal(raw, &v); err != nil {
			return "", fmt.Errorf("parsing the Airflow theme's color %s: %w", name, err)
		}
	}
	if !validOklch(v) {
		return "", fmt.Errorf("color %s in the Airflow theme is %q, not an oklch() color Airflow takes", name, v)
	}
	t.colors = append(t.colors, cssVar{"--chakra-colors-" + name, v})
	return v, nil
}

// addScale adds the palette name, if the theme has one, step by step from
// lightest to darkest.
func (t *airflowTheme) addScale(name string, colors map[string]json.RawMessage) error {
	raw, ok := colors[name]
	if !ok {
		return nil
	}
	scale, err := themeObject(raw, "color "+name, themeSteps...)
	if err != nil {
		return err
	}
	for _, step := range themeSteps {
		if s, ok := scale[step]; ok {
			if _, err := t.addShade(name+"-"+step, s); err != nil {
				return err
			}
		}
	}
	return nil
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
// It outranks a theme of the user's own just the same: an AIRFLOW__API__THEME
// they set lands in that tokens layer, so behind the proxy the page shows
// Runtime's colors anyway. The proxy cannot see the setting from the page it
// relays, and leaving the variables to Chakra instead would leave Airflow
// before 3.2 unthemed.
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
//
// Built on first use, and empty if Runtime's theme does not parse: Airflow
// pages then go out unthemed, rather than every program linking this package
// failing as it starts. The tests parse it, so that is a build that never
// ships.
var airflowThemeSnippet = sync.OnceValue(func() string {
	theme, err := parseAirflowTheme([]byte(airflowrt.UITheme()))
	if err != nil {
		slog.Error("Airflow pages will not be themed", "error", err)
		return ""
	}
	return `<style id="` + themeTagID + `">` + theme.css() + scrollbarCSS + `</style>` +
		prePaintScript(theme.black, theme.white)
})

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
	snippet := airflowThemeSnippet()
	if snippet == "" {
		return nil
	}
	return injectIntoHTML(resp, snippet, "</head>", `id="`+themeTagID+`"`)
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
